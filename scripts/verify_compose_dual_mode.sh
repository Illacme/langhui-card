#!/usr/bin/env bash
#
# verify_compose_dual_mode.sh
# ─────────────────────────────────────────────────────────────────────────────
# 微传名片 (weichuan-card) compose 双形态真机验证脚本
#
# 两种部署形态（见 docker-compose.yml 顶部注释）：
#   形态 1  零依赖起步（默认）：docker compose up -d --build
#           → 只起 backend + frontend，不起 mysql，选 SQLite，无需任何数据库口令
#   形态 2  生产 MySQL：        docker compose up -d --build --profile mysql
#           → 额外起 mysql，MYSQL_ROOT_PASSWORD 必须给出（否则 compose 报错）
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
chk("MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD:?" in s, "MYSQL_ROOT_PASSWORD 用 :?（缺省即报错）")
chk("depends_on:" not in s or "depends_on:\n      - mysql" not in s, "backend 不 depends_on mysql")
chk("container_name: weichuan-" in s, "容器名前缀为 weichuan-（标识符已统一）")
chk("DB_DSN: ${DB_DSN:-}" in s, "DB_DSN 用 :- （留空进入安装向导）")
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
  ${COMPOSE_BIN} down -v >/dev/null 2>&1 || true

  info "形态1：默认（SQLite 零依赖）— ${COMPOSE_BIN} up -d --build"
  if ! ${COMPOSE_BIN} up -d --build >/tmp/compose_default.log 2>&1; then
    bad "${COMPOSE_BIN} up -d --build 失败"
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
  ${COMPOSE_BIN} down -v >/dev/null 2>&1 || true

  info "形态2a：MySQL opt-in 缺口令必须报错"
  unset MYSQL_ROOT_PASSWORD
  if ${COMPOSE_BIN} up -d --build --profile mysql >/tmp/compose_mysql_nopw.log 2>&1; then
    bad "缺 MYSQL_ROOT_PASSWORD 时 compose 竟然成功（应报错）"
  else
    if grep -q "MYSQL_ROOT_PASSWORD" /tmp/compose_mysql_nopw.log; then
      ok "缺 MYSQL_ROOT_PASSWORD 时 compose 报错并提示 --profile mysql"
    else
      bad "compose 报错但未提及 MYSQL_ROOT_PASSWORD（见 /tmp/compose_mysql_nopw.log）"
      sed 's/^/    /' /tmp/compose_mysql_nopw.log
    fi
  fi
  ${COMPOSE_BIN} down -v >/dev/null 2>&1 || true

  info "形态2b：MySQL opt-in 带口令启动"
  owner3306=$(port_owner 3306)
  [ -n "$owner3306" ] && bad "端口 3306 被占用：$owner3306（mysql 容器无法绑定，MySQL 形态会失败）"
  export MYSQL_ROOT_PASSWORD="Vp$(openssl rand -hex 8 2>/dev/null || date +%s%N)"
  if ! ${COMPOSE_BIN} up -d --build --profile mysql >/tmp/compose_mysql.log 2>&1; then
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
    ${COMPOSE_BIN} down -v >/dev/null 2>&1 && ok "已 teardown" || bad "teardown 失败"
  else
    info "保留容器（--keep）；手动清理： ${COMPOSE_BIN} down -v"
  fi
fi

echo
echo "══════════════════════════════════════════════════════"
echo "  通过 ${PASS}   失败 ${FAIL}"
echo "══════════════════════════════════════════════════════"
[ "${FAIL}" = "0" ] && exit 0 || exit 1
