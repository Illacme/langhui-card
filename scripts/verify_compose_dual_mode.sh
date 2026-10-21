#!/usr/bin/env bash
#
# verify_compose_dual_mode.sh
# ─────────────────────────────────────────────────────────────────────────────
# 微传名片 (weichuan-card) compose 双形态真机验证脚本
#
# 两种部署形态（见 docker-compose.yml 顶部注释）：
#   形态 1  零依赖起步（默认）：docker compose up -d --build
#           → 只起 backend + frontend，不起 mysql，选 SQLite，无需任何数据库口令
#   形态 2  生产 MySQL：        docker compose --profile mysql up -d --build
#           → 额外起 mysql；MYSQL_ROOT_PASSWORD 必须给出，否则 mysql 容器
#             启动即报错退出（日志有明确提示），不影响 backend/frontend
#
# 用法：
#   ./verify_compose_dual_mode.sh --diag         # 只做环境诊断（docker/守护进程/compose 版本），不改状态
#   ./verify_compose_dual_mode.sh --static       # 仅静态检查 compose 文件（无需 Docker）
#   ./verify_compose_dual_mode.sh                # 静态 + 真机双形态端到端
#   ./verify_compose_dual_mode.sh --no-install   # 不真正安装（只验证容器/端口/健康）
#   ./verify_compose_dual_mode.sh --keep         # 结束后保留容器（不 teardown）
#
# 依赖：docker + compose v2（profiles 需要 v2）、curl、python3（仅静态检查用）
#
# 已规避的坑（首版在部分环境误报）：
#   - 不用 `compose ps --services`（部分 compose 版本不支持 --services）；改用 docker ps 按容器名判定
#   - `up` 失败时打印 compose 日志 + 容器日志，不把失败原因藏进重定向文件
#   - 先自检守护进程/compose 版本；Compose v1 不支持 profiles，直接明确中止而非给出假失败
#   - `--profile` 是顶层标志：必须 `compose --profile mysql up -d`，写在 up 之后报 unknown flag
#   - compose 对「未被 profile 激活的服务」也做变量插值，故 mysql 口令校验放在容器内
#     （entrypoint），不用 ${VAR:?}——否则默认（SQLite）形态的 up 会被未激活的 mysql 服务炸掉
#   - 中文（非 ASCII）目录名下 compose 会推出空项目名、报 "project name must not be empty"，
#     compose 文件已写死顶层 name: weichuan 规避
#   - 国内网络直连 Docker Hub 不通会让 build 死在拉基础镜像（i/o timeout，错误埋在日志里）：
#     registry_reachable 提前探测 + up 失败时按日志特征自动给出镜像加速配法
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail

# 切到仓库根目录（脚本位于 scripts/，compose 文件在根）
cd "$(cd "$(dirname "$0")" && pwd)/.." || exit 1

BPORT=8090          # 后端直连调试端口（绕过 nginx）
FPORT=8080          # 前端入口（nginx 反代 /api → backend:8080）
SQLITE_DSN="sqlite:///data/weichuan.db"
MYSQL_DSN="root:\${MYSQL_ROOT_PASSWORD}@tcp(mysql:3306)/weichuan"

# ---------- 标志 ----------
DO_STATIC=1
DO_LIVE=1
DO_INSTALL=1
DO_DIAG=0
KEEP=0
for a in "$@"; do
  case "$a" in
    --static)     DO_LIVE=0 ;;
    --no-install) DO_INSTALL=0 ;;
    --diag)       DO_DIAG=1; DO_LIVE=0 ;;
    --keep)       KEEP=1 ;;
    -h|--help)    sed -n '2,22p' "$0"; exit 0 ;;
    *) echo "未知参数: $a" >&2; exit 2 ;;
  esac
done

PASS=0; FAIL=0
ok()   { echo "  ✅ $1"; PASS=$((PASS+1)); }
bad()  { echo "  ❌ $1"; FAIL=$((FAIL+1)); }
info() { echo "🔹 $1"; }
http_code() { curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$@"; }
status_configured() {
  curl -s --max-time 5 "http://localhost:${BPORT}/api/setup/status" \
    | python3 -c 'import sys,json
try:
    print(json.load(sys.stdin).get("configured",""))
except Exception:
    pass' 2>/dev/null
}
wait_cmd() {            # wait_cmd <timeout_sec> "<shell command>"
  local t=$1; shift; local w=0
  while ! eval "$@" >/dev/null 2>&1; do
    sleep 2; w=$((w+2)); [ "$w" -ge "$t" ] && return 1
  done
  return 0
}

# ---------- 容器计数：只用 docker ps（所有 compose 版本都支持），不依赖 ps --services ----------
running_names() { docker ps --format '{{.Names}}' 2>/dev/null; }
has_ct() { running_names | grep -qx "$1"; }
port_owner() {   # 打印占用某 TCP 监听端口的进程一行（取不到则为空）
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$1" -sTCP:LISTEN 2>/dev/null | tail -n +2 | head -1
  elif command -v ss >/dev/null 2>&1; then
    ss -ltnp "sport = :$1" 2>/dev/null | tail -n +2 | head -1
  fi
}

# ---------- compose 探测：不按名字猜版本，直接测能力（是否支持 --profile） ----------
# 坑：独立二进制 docker-compose 也可能是新版（支持 profiles），
# 不能因为名字带连字符就当成 v1。用 `help up` 是否含 --profile 来判定。
COMPOSE_BIN=""
COMPOSE_HAS_PROFILE=0
compose_supports_profile() {   # 带 15s 超时保护，防止 help 卡住
  [ -n "${COMPOSE_BIN}" ] || return 1
  local tmp pid w
  tmp=$(mktemp)
  ${COMPOSE_BIN} help up >"$tmp" 2>&1 &
  pid=$!; w=0
  while kill -0 "$pid" 2>/dev/null; do
    sleep 1; w=$((w+1)); [ "$w" -ge 15 ] && { kill "$pid" 2>/dev/null; break; }
  done
  wait "$pid" 2>/dev/null
  if grep -q -- '--profile' "$tmp"; then rm -f "$tmp"; return 0; fi
  rm -f "$tmp"; return 1
}
detect_compose() {
  if docker compose version >/dev/null 2>&1; then
    COMPOSE_BIN="docker compose"
  elif command -v docker-compose >/dev/null 2>&1 && docker-compose version >/dev/null 2>&1; then
    COMPOSE_BIN="docker-compose"
  else
    return 1
  fi
  if compose_supports_profile; then COMPOSE_HAS_PROFILE=1; else COMPOSE_HAS_PROFILE=0; fi
  return 0
}

# ---------- 诊断 ----------
# ---------- registry 可达性 ----------
# 国内网络直连 Docker Hub（registry-1.docker.io）大概率不通，而失败发生在 build 里层、
# 报错很难读。这里提前探一次（--noproxy 模拟 dockerd 的直连——守护进程默认不走 shell 代理），
# 不通时给出镜像加速配法；注意：若镜像已全部本地缓存，本检查只提示、不拦截。
registry_reachable() {
  command -v curl >/dev/null 2>&1 || return 0   # 没 curl 就不探测，避免误判
  curl -s -o /dev/null -m 5 --noproxy '*' "https://registry-1.docker.io/v2/" 2>/dev/null
}
registry_hint() {
  echo "  → 直连 Docker Hub 不通，构建/拉镜像会失败（i/o timeout）。国内网络解法："
  echo "    A. colima：编辑 ~/.colima/default/colima.yaml，把 'docker: {}' 改成"
  echo "         docker:"
  echo "           registry-mirrors:"
  echo "             - https://docker.m.daocloud.io"
  echo "             - https://docker.1panel.live"
  echo "       保存后执行 colima restart。"
  echo "    B. Docker Desktop：Settings → Docker Engine，JSON 里加"
  echo "         \"registry-mirrors\": [\"https://docker.m.daocloud.io\"]，Apply & Restart。"
  echo "    配完先 docker pull busybox 验证，再重跑本脚本。"
  echo "    （若镜像已全部本地缓存可忽略本提示。）"
}
run_diag() {
  info "环境诊断"
  echo "── docker ──"
  if command -v docker >/dev/null 2>&1; then
    echo "  ✅ docker 可执行文件: $(command -v docker)"
    docker version --format '{{.Server.Version}}' 2>/dev/null | sed 's/^/  Server 版本: /' \
      || echo "  Server 版本: (取不到，可能守护进程未运行)"
  else
    echo "  ❌ PATH 中找不到 docker CLI（本脚本的容器计数/exec 断言依赖它）"
    for p in /opt/homebrew/bin/docker /usr/local/bin/docker; do
      [ -x "$p" ] && echo "     提示: $p 存在但不在 PATH，可 export PATH=\"\$PATH:$(dirname "$p")\""
    done
  fi
  echo "── 守护进程 ──"
  if docker info >/dev/null 2>&1; then
    docker info --format '{{.ServerVersion}}' 2>/dev/null | sed 's/^/  ✅ 守护进程可达，Server 版本: /'
  else
    echo "  ❌ 守护进程不可达（docker info 失败）：无 Docker daemon 或未启动 Docker Desktop"
  fi
  echo "── compose ──"
  docker compose version >/dev/null 2>&1 && echo "  ✅ docker compose (v2 插件)：$(docker compose version 2>&1 | head -1)"
  if command -v docker-compose >/dev/null 2>&1; then
    echo "  ℹ️  docker-compose：$(command -v docker-compose)｜$(docker-compose version 2>&1 | head -1)"
  fi
  if detect_compose; then
    if [ "${COMPOSE_HAS_PROFILE}" = "1" ]; then
      echo "  ✅ 选用 ${COMPOSE_BIN}，且支持 --profile（双形态可验证）"
    else
      echo "  ❌ ${COMPOSE_BIN} 不支持 --profile：需 Compose v2（升级 compose 或安装 'docker compose' 插件）"
    fi
  else
    echo "  ❌ 未检测到可用 compose（既无 'docker compose' 也无 'docker-compose'）"
  fi
  echo "── registry 可达性（拉基础镜像的前提）──"
  if registry_reachable; then
    echo "  ✅ Docker Hub 直连可达"
  elif [ "$(docker info --format '{{len .RegistryConfig.Mirrors}}' 2>/dev/null || echo 0)" -gt 0 ] 2>/dev/null; then
    echo "  ✅ Docker Hub 直连不通，但 daemon 已配 registry 镜像加速（可经加速拉取）"
  else
    echo "  ❌ Docker Hub 直连不通且未配镜像加速——build 会失败，除非镜像已本地缓存"
    registry_hint
  fi
  echo "── Docker 引擎 ──"
  if docker info >/dev/null 2>&1; then
    if command -v colima >/dev/null 2>&1; then
      echo "  ✅ 守护进程可达（colima 提供）"
    elif [ -d "/Applications/Docker.app" ] || [ -d "/Applications/Docker Desktop.app" ]; then
      echo "  ✅ 守护进程可达（Docker Desktop 提供）"
    else
      echo "  ✅ 守护进程可达"
    fi
  else
    if command -v colima >/dev/null 2>&1; then
      echo "  ❌ 守护进程不可达；检测到 colima，启动它： colima start"
    elif [ -d "/Applications/Docker.app" ] || [ -d "/Applications/Docker Desktop.app" ]; then
      echo "  ⚠️  Docker Desktop 已安装但未启动：打开它，等菜单栏鲸鱼图标稳定"
    else
      echo "  ❌ 无 Docker 引擎。macOS：brew install colima && colima start（或 brew install --cask docker）；Linux：启动 dockerd"
    fi
  fi
  echo "── 容器 ──"
  if docker info >/dev/null 2>&1; then
    docker ps -a --format '{{.Names}}\t{{.Status}}' 2>&1 | sed 's/^/  /'
    [ "$(docker ps -aq 2>/dev/null | wc -l)" = "0" ] && echo "  (无容器)"
  fi
  echo "── 端口占用（8080/8090/3306 是否被别的服务占用）──"
  if command -v lsof >/dev/null 2>&1; then
    for p in "${FPORT}" "${BPORT}" 3306; do
      r=$(lsof -nP -iTCP:"$p" -sTCP:LISTEN 2>/dev/null | tail -n +2)
      [ -n "$r" ] && echo "$r" | sed "s/^/  端口 $p: /" || echo "  端口 $p: 空闲"
    done
  else
    echo "  (无 lsof，跳过)"
  fi
}

# ---------- 静态检查（无需 Docker） ----------
static_check() {
  info "静态检查 docker-compose.yml"
  if [ ! -f docker-compose.yml ]; then bad "docker-compose.yml 不存在"; return; fi
  python3 - docker-compose.yml <<'PY'
import sys
s = open(sys.argv[1], encoding='utf-8').read()
ok = True
def chk(cond, msg):
    global ok
    print(("  ✅ " if cond else "  ❌ ")+msg); ok = ok and cond
chk("profiles: [mysql]" in s, "mysql 服务带 profiles: [mysql]（默认不起）")
chk("MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD:-}" in s, "MYSQL_ROOT_PASSWORD 用 :- （避免 :? 全局插值炸掉默认形态）")
chk("MYSQL_ROOT_PASSWORD 未设置" in s, "mysql 入口有缺口令提示（容器内 entrypoint 校验）")
chk('MYSQL_ROOT_HOST: "%"' in s, "MYSQL_ROOT_HOST 显式为 %（backend 容器以 root 跨主机连接的前提）")
chk("depends_on:" not in s or "depends_on:\n      - mysql" not in s, "backend 不 depends_on mysql")
chk("container_name: weichuan-" in s, "容器名前缀为 weichuan-（标识符已统一）")
chk("DB_DSN: ${DB_DSN:-}" in s, "DB_DSN 用 :- （留空进入安装向导）")
chk("name: weichuan" in s, "顶层 name: weichuan（中文目录名下 compose 也能用）")
sys.exit(0 if ok else 1)
PY
  [ $? -eq 0 ] && PASS=$((PASS+1)) || FAIL=$((FAIL+1))
}

# ---------- 失败时倾倒现场 ----------
dump_diag() {
  echo "  —— 现场诊断 ——"
  echo "  docker ps -a:"
  docker ps -a --format '{{.Names}}\t{{.Status}}' 2>&1 | sed 's/^/    /'
  for c in weichuan-backend weichuan-frontend weichuan-mysql; do
    if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx "$c"; then
      echo "  docker logs $c (tail 25):"
      docker logs --tail 25 "$c" 2>&1 | sed 's/^/    /'
    fi
  done
}

# ─────────────────────────── 主流程 ───────────────────────────
[ "$DO_DIAG" = "1" ] && { run_diag; exit 0; }
[ "${DO_STATIC}" = "1" ] && static_check

if [ "${DO_LIVE}" = "1" ]; then
  info "环境自检"
  LIVE_OK=1
  command -v docker >/dev/null 2>&1 || { bad "未找到 docker CLI（compose 需要它，本脚本的容器断言也依赖它）"; LIVE_OK=0; }
  if [ "${LIVE_OK}" = "1" ] && ! detect_compose; then
    bad "未检测到可用 compose（缺 'docker compose' 与 'docker-compose'）"; LIVE_OK=0
  fi
  if [ "${LIVE_OK}" = "1" ] && [ "${COMPOSE_HAS_PROFILE}" != "1" ]; then
    bad "${COMPOSE_BIN} 不支持 --profile：双形态（--profile mysql）无法验证，需 Compose v2"; LIVE_OK=0
  fi
  # 端口占用预检：compose 要绑 8080(frontend) 与 8090(backend)，被占则容器必然起不来
  if [ "${LIVE_OK}" = "1" ]; then
    CLASH=0
    for p in "${FPORT}" "${BPORT}"; do
      owner=$(port_owner "$p")
      if [ -n "$owner" ]; then
        bad "端口 $p 被占用：$owner"
        CLASH=1
      fi
    done
    if [ "${CLASH}" = "1" ]; then
      echo "  → compose 需绑定 ${FPORT}(frontend) 与 ${BPORT}(backend)；被占则 up 必然失败"
      echo "  → 腾出命令： lsof -nP -iTCP:${FPORT} -sTCP:LISTEN -t | xargs kill"
      LIVE_OK=0
    fi
  fi
  # 守护进程不可达时的提示要按实际安装方式给（colima / Docker Desktop / Linux）
  if [ "${LIVE_OK}" = "1" ] && ! docker info >/dev/null 2>&1; then
    if command -v colima >/dev/null 2>&1; then
      bad "Docker 守护进程不可达 — 检测到 colima，请先执行： colima start"
    elif [ -d "/Applications/Docker.app" ] || [ -d "/Applications/Docker Desktop.app" ]; then
      bad "Docker 守护进程不可达 — 请打开 Docker Desktop，等鲸鱼图标稳定"
    else
      bad "Docker 守护进程不可达 — macOS 装 colima 或 Docker Desktop，Linux 启动 dockerd"
    fi
    LIVE_OK=0
  fi
  if [ "${LIVE_OK}" != "1" ]; then
    echo "  （真机验证已跳过；环境就绪后重跑本脚本，或先跑 --diag 看环境）"
    DO_LIVE=0
  else
    ok "环境自检通过（${COMPOSE_BIN}）"
  fi
fi

if [ "${DO_LIVE}" = "1" ]; then
  info "清理任何残留容器/卷"
  ${COMPOSE_BIN} --profile mysql down -v >/dev/null 2>&1 || true

  info "形态1：默认（SQLite 零依赖）— ${COMPOSE_BIN} up -d --build"
  if ! ${COMPOSE_BIN} up -d --build >/tmp/compose_default.log 2>&1; then
    bad "${COMPOSE_BIN} up -d --build 失败"
    # 国内网络最常见死法：拉基础镜像 i/o timeout——错误埋在 build 日志里，这里识别并直接给解法
    if grep -qE "i/o timeout|failed to resolve reference|failed to do request" /tmp/compose_default.log; then
      registry_hint
    fi
    sed 's/^/    /' /tmp/compose_default.log
    dump_diag
    DO_LIVE=0
  fi
fi

if [ "${DO_LIVE}" = "1" ]; then
  wait_cmd 90 "docker ps --format '{{.Names}}' | grep -qx weichuan-backend"
  has_ct weichuan-backend && ok "backend 容器运行中" || { bad "backend 未运行"; dump_diag; }
  has_ct weichuan-frontend && ok "frontend 容器运行中" || bad "frontend 未运行"
  has_ct weichuan-mysql && bad "mysql 不应在默认形态启动" || ok "mysql 未启动（符合零依赖）"

  wait_cmd 60 "curl -s --max-time 5 http://localhost:${BPORT}/api/setup/status >/dev/null"
  cfg=$(status_configured)
  [ "$cfg" = "false" ] && ok "未配置模式 status.configured=false" || bad "status.configured=$cfg (期望 false)"
  hc=$(http_code "http://localhost:${BPORT}/healthz")
  [ "$hc" = "503" ] && ok "/healthz 未配置模式返回 503" || bad "/healthz=$hc (期望 503)"
  fp=$(http_code "http://localhost:${FPORT}/")
  if [ "$fp" = "200" ]; then ok "前端 8080 返回 200（安装页可用）"; else
    bad "前端首页=$fp（非 200 通常是 8080 被别的服务占用或前端没起来）"; dump_diag
  fi

  if [ "${DO_INSTALL}" = "1" ]; then
    info "  安装（SQLite）"
    body="{\"DSN\":\"${SQLITE_DSN}\",\"AdminUser\":\"admin\",\"AdminPass\":\"VerifyPass-123\",\"AdminPass2\":\"VerifyPass-123\"}"
    ic=$(curl -s -o /tmp/install.res -w '%{http_code}' --max-time 30 -X POST \
          -H 'Content-Type: application/json' -d "$body" "http://localhost:${BPORT}/api/setup/install")
    [ "$ic" = "200" ] && ok "POST /api/setup/install 返回 200" || bad "install=$ic $(cat /tmp/install.res 2>/dev/null)"
    ready=0
    for i in $(seq 1 45); do [ "$(status_configured)" = "true" ] && { ready=1; break; }; sleep 2; done
    [ "$ready" = "1" ] && ok "安装完成，status.configured=true（backend 已自重启）" || bad "安装后未见 configured=true"
    hc2=$(http_code "http://localhost:${BPORT}/healthz")
    [ "$hc2" = "200" ] && ok "安装后 /healthz=200" || bad "/healthz=$hc2"
  fi

  # ===== 切到形态 2：MySQL opt-in =====
  info "清理默认形态容器/卷"
  ${COMPOSE_BIN} --profile mysql down -v >/dev/null 2>&1 || true

  info "形态2a：MySQL opt-in 缺口令 → mysql 容器启动即报错退出"
  unset MYSQL_ROOT_PASSWORD
  # --profile 是顶层标志，必须放在 up 之前（写在 up 之后会报 unknown flag: --profile）
  if ! ${COMPOSE_BIN} --profile mysql up -d --build >/tmp/compose_mysql_nopw.log 2>&1; then
    bad "缺口令时 compose up 失败（设计上应成功起 backend/frontend，只让 mysql 报错退出）"
    sed 's/^/    /' /tmp/compose_mysql_nopw.log
    dump_diag
  else
    # mysql 带 restart: unless-stopped，entrypoint 校验失败会反复重启 → 状态 restarting/exited
    bad_state=""
    for i in $(seq 1 15); do
      st=$(docker inspect -f '{{.State.Status}}' weichuan-mysql 2>/dev/null || echo missing)
      if [ "$st" = "restarting" ] || [ "$st" = "exited" ]; then bad_state="$st"; break; fi
      sleep 2
    done
    if [ -n "$bad_state" ]; then
      if docker logs weichuan-mysql 2>&1 | grep -q "MYSQL_ROOT_PASSWORD 未设置"; then
        ok "缺口令时 mysql 容器 $bad_state，日志含明确提示"
      else
        bad "mysql $bad_state 但日志未见缺口令提示："
        docker logs weichuan-mysql 2>&1 | sed 's/^/    /'
      fi
    else
      bad "缺口令时 mysql 未报错退出（状态: $(docker inspect -f '{{.State.Status}}' weichuan-mysql 2>/dev/null || echo missing)）"
      dump_diag
    fi
    # backend/frontend 属默认 profile，此时仍在运行；一次 down -v 全清（含 mysql）
    ${COMPOSE_BIN} --profile mysql down -v >/dev/null 2>&1 || true
  fi

  info "形态2b：MySQL opt-in 带口令启动"
  owner3306=$(port_owner 3306)
  [ -n "$owner3306" ] && bad "端口 3306 被占用：$owner3306（mysql 容器无法绑定，MySQL 形态会失败）"
  export MYSQL_ROOT_PASSWORD="Vp$(openssl rand -hex 8 2>/dev/null || date +%s%N)"
  if ! ${COMPOSE_BIN} --profile mysql up -d --build >/tmp/compose_mysql.log 2>&1; then
    bad "带口令启动 mysql 形态失败"; sed 's/^/    /' /tmp/compose_mysql.log; dump_diag
  else
    wait_cmd 90 "docker ps --format '{{.Names}}' | grep -qx weichuan-mysql"
    n=0
    for c in weichuan-backend weichuan-frontend weichuan-mysql; do has_ct "$c" && n=$((n+1)); done
    if [ "$n" = "3" ]; then ok "三容器运行 (backend+frontend+mysql)"; else bad "运行容器数=$n (期望 3)"; dump_diag; fi
    wait_cmd 90 "docker exec weichuan-mysql mysqladmin ping -h 127.0.0.1 -uroot -p${MYSQL_ROOT_PASSWORD} --silent >/dev/null 2>&1"
    if [ $? -eq 0 ]; then ok "mysql 健康检查通过"; else bad "mysql 未就绪"; dump_diag; fi
    wait_cmd 60 "curl -s --max-time 5 http://localhost:${BPORT}/healthz >/dev/null"

    if [ "${DO_INSTALL}" = "1" ]; then
      info "  安装（MySQL）"
      body="{\"DSN\":\"${MYSQL_DSN}\",\"AdminUser\":\"admin\",\"AdminPass\":\"VerifyPass-123\",\"AdminPass2\":\"VerifyPass-123\"}"
      ic=$(curl -s -o /tmp/install2.res -w '%{http_code}' --max-time 30 -X POST \
            -H 'Content-Type: application/json' -d "$body" "http://localhost:${BPORT}/api/setup/install")
      [ "$ic" = "200" ] && ok "MySQL 形态安装返回 200" || bad "install=$ic $(cat /tmp/install2.res 2>/dev/null)"
      ready=0
      for i in $(seq 1 45); do [ "$(status_configured)" = "true" ] && { ready=1; break; }; sleep 2; done
      [ "$ready" = "1" ] && ok "MySQL 形态安装完成，status.configured=true" || bad "MySQL 形态未见 configured=true"
      tbls=$(docker exec weichuan-mysql mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "use weichuan; show tables;" 2>/dev/null | wc -l)
      [ "${tbls:-0}" -gt 1 ] && ok "weichuan 库已建表（表数=$tbls）" || bad "MySQL 建表异常"
    fi
  fi

  if [ "${KEEP}" = "0" ]; then
    info "清理：teardown（含卷）"
    ${COMPOSE_BIN} --profile mysql down -v >/dev/null 2>&1 && ok "已 teardown" || bad "teardown 失败"
  else
    info "保留容器（--keep）；手动清理： ${COMPOSE_BIN} --profile mysql down -v"
  fi
fi

echo
echo "══════════════════════════════════════════════════════"
echo "  通过 ${PASS}   失败 ${FAIL}"
echo "══════════════════════════════════════════════════════"
[ "${FAIL}" = "0" ] && exit 0 || exit 1
