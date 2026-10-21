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
#   ./verify_compose_dual_mode.sh --static            # 仅静态检查 compose 文件（无需 Docker）
#   ./verify_compose_dual_mode.sh                     # 静态 + 真机双形态端到端
#   ./verify_compose_dual_mode.sh --no-install        # 不真正安装（只验证容器/端口/健康）
#   ./verify_compose_dual_mode.sh --keep              # 结束后保留容器（不 teardown）
#
# 依赖：docker (compose v2)、curl、python3（仅静态检查用）
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail

# 切到仓库根目录（脚本位于 scripts/，compose 文件在根）
cd "$(cd "$(dirname "$0")" && pwd)/.." || exit 1

# ---------- 配置 ----------
COMPOSE_BIN="docker compose"
BPORT=8090          # 后端直连调试端口（绕过 nginx）
FPORT=8080          # 前端入口（nginx 反代 /api → backend:8080）
MYSQL_SVC=mysql
SQLITE_DSN="sqlite:///data/weichuan.db"
MYSQL_DSN="root:\${MYSQL_ROOT_PASSWORD}@tcp(mysql:3306)/weichuan"

# ---------- 标志 ----------
DO_STATIC=1
DO_LIVE=1
DO_INSTALL=1
KEEP=0
for a in "$@"; do
  case "$a" in
    --static)     DO_LIVE=0 ;;
    --no-install) DO_INSTALL=0 ;;
    --keep)       KEEP=1 ;;
    -h|--help)    sed -n '2,18p' "$0"; exit 0 ;;
    *) echo "未知参数: $a" >&2; exit 2 ;;
  esac
done

PASS=0; FAIL=0
ok()   { echo "  ✅ $1"; PASS=$((PASS+1)); }
bad()  { echo "  ❌ $1"; FAIL=$((FAIL+1)); }
info() { echo "🔹 $1"; }
http_code() { curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$@"; }
status_configured() {
  curl -s --max-time 5 "http://localhost:$BPORT/api/setup/status" \
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

# ---------- 主流程 ----------
[ "$DO_STATIC" = "1" ] && static_check

if [ "$DO_LIVE" = "1" ]; then
  if ! command -v docker >/dev/null 2>&1; then
    bad "未找到 docker，无法做真机验证（本地仅做静态检查即可）"; DO_LIVE=0
  fi
fi

if [ "$DO_LIVE" = "1" ]; then
  info "清理任何残留容器/卷"
  $COMPOSE_BIN down -v >/dev/null 2>&1 || true

  # ===== 形态 1：默认（SQLite 零依赖）=====
  info "形态1：默认（SQLite 零依赖）— docker compose up -d --build"
  $COMPOSE_BIN up -d --build >/tmp/compose_default.log 2>&1
  wait_cmd 90 "docker ps --filter name=weichuan-backend --format '{{.Names}}' | grep -q weichuan-backend"
  running=$($COMPOSE_BIN ps --services --filter status=running)
  cnt=$(printf '%s\n' "$running" | grep -c .)
  [ "$cnt" = "2" ] && ok "默认形态起 2 个服务 (backend+frontend)" || bad "默认形态运行服务数=$cnt (期望 2)"
  printf '%s\n' "$running" | grep -qx "$MYSQL_SVC" && bad "mysql 不应在默认形态启动" || ok "mysql 未启动（符合零依赖）"

  wait_cmd 60 "curl -s --max-time 5 http://localhost:$BPORT/api/setup/status >/dev/null"
  cfg=$(status_configured)
  [ "$cfg" = "false" ] && ok "未配置模式 status.configured=false" || bad "status.configured=$cfg (期望 false)"
  hc=$(http_code "http://localhost:$BPORT/healthz")
  [ "$hc" = "503" ] && ok "/healthz 未配置模式返回 503" || bad "/healthz=$hc (期望 503)"
  fp=$(http_code "http://localhost:$FPORT/")
  [ "$fp" = "200" ] && ok "前端 8080 返回 200（安装页可用）" || bad "前端首页=$fp"

  if [ "$DO_INSTALL" = "1" ]; then
    info "  安装（SQLite）"
    body="{\"DSN\":\"$SQLITE_DSN\",\"AdminUser\":\"admin\",\"AdminPass\":\"VerifyPass-123\",\"AdminPass2\":\"VerifyPass-123\"}"
    ic=$(curl -s -o /tmp/install.res -w '%{http_code}' --max-time 30 -X POST \
          -H 'Content-Type: application/json' -d "$body" "http://localhost:$BPORT/api/setup/install")
    [ "$ic" = "200" ] && ok "POST /api/setup/install 返回 200" || bad "install=$ic $(cat /tmp/install.res)"
    ready=0
    for i in $(seq 1 45); do [ "$(status_configured)" = "true" ] && { ready=1; break; }; sleep 2; done
    [ "$ready" = "1" ] && ok "安装完成，status.configured=true（backend 已自重启）" || bad "安装后未见 configured=true"
    hc2=$(http_code "http://localhost:$BPORT/healthz")
    [ "$hc2" = "200" ] && ok "安装后 /healthz=200" || bad "/healthz=$hc2"
  fi

  # ===== 切到形态 2：MySQL opt-in =====
  info "清理默认形态容器/卷"
  $COMPOSE_BIN down -v >/dev/null 2>&1 || true

  info "形态2a：MySQL opt-in 缺口令必须报错"
  unset MYSQL_ROOT_PASSWORD
  $COMPOSE_BIN up -d --build --profile mysql >/tmp/compose_mysql_nopw.log 2>&1
  if [ $? -ne 0 ] && grep -q "MYSQL_ROOT_PASSWORD" /tmp/compose_mysql_nopw.log; then
    ok "缺 MYSQL_ROOT_PASSWORD 时 compose 报错并提示 --profile mysql"
  else
    bad "缺口令时 compose 未如预期报错（见 /tmp/compose_mysql_nopw.log）"
  fi
  $COMPOSE_BIN down -v >/dev/null 2>&1 || true

  info "形态2b：MySQL opt-in 带口令启动"
  export MYSQL_ROOT_PASSWORD="Vp$(openssl rand -hex 8 2>/dev/null || date +%s%N)"
  $COMPOSE_BIN up -d --build --profile mysql >/tmp/compose_mysql.log 2>&1
  wait_cmd 90 'c=$($COMPOSE_BIN ps --services --filter status=running | grep -c .); [ "$c" = "3" ]'
  cnt=$($COMPOSE_BIN ps --services --filter status=running | grep -c .)
  [ "$cnt" = "3" ] && ok "三容器运行 (backend+frontend+mysql)" || bad "运行服务数=$cnt (期望 3)"
  wait_cmd 90 "docker exec weichuan-mysql mysqladmin ping -h 127.0.0.1 -uroot -p$MYSQL_ROOT_PASSWORD --silent >/dev/null 2>&1"
  [ $? -eq 0 ] && ok "mysql 健康检查通过" || bad "mysql 未就绪"
  wait_cmd 60 "curl -s --max-time 5 http://localhost:$BPORT/healthz >/dev/null"

  if [ "$DO_INSTALL" = "1" ]; then
    info "  安装（MySQL）"
    body="{\"DSN\":\"$MYSQL_DSN\",\"AdminUser\":\"admin\",\"AdminPass\":\"VerifyPass-123\",\"AdminPass2\":\"VerifyPass-123\"}"
    ic=$(curl -s -o /tmp/install2.res -w '%{http_code}' --max-time 30 -X POST \
          -H 'Content-Type: application/json' -d "$body" "http://localhost:$BPORT/api/setup/install")
    [ "$ic" = "200" ] && ok "MySQL 形态安装返回 200" || bad "install=$ic $(cat /tmp/install2.res)"
    ready=0
    for i in $(seq 1 45); do [ "$(status_configured)" = "true" ] && { ready=1; break; }; sleep 2; done
    [ "$ready" = "1" ] && ok "MySQL 形态安装完成，status.configured=true" || bad "MySQL 形态未见 configured=true"
    tbls=$(docker exec weichuan-mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "use weichuan; show tables;" 2>/dev/null | wc -l)
    [ "${tbls:-0}" -gt 1 ] && ok "weichuan 库已建表（表数=$tbls）" || bad "MySQL 建表异常"
  fi

  if [ "$KEEP" = "0" ]; then
    info "清理：teardown（含卷）"
    $COMPOSE_BIN down -v >/dev/null 2>&1 && ok "已 teardown" || bad "teardown 失败"
  else
    info "保留容器（--keep）；手动清理： docker compose down -v"
  fi
fi

echo
echo "══════════════════════════════════════════════════════"
echo "  通过 $PASS   失败 $FAIL"
echo "══════════════════════════════════════════════════════"
[ "$FAIL" = "0" ] && exit 0 || exit 1
