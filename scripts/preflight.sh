#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────
# 上线前配置自检（零第三方依赖，纯静态检查，服务不必先启动）
#
# 定位：回答「这套配置能不能上线」。检查分四档：
#   FAIL  上线就出事（默认凭据 / 真机不可达的基址），必须修
#   WARN  能用但不妥（http 基址 / 弱口令 / Cookie 未加 Secure），建议修
#   PASS  已正确配置
#   SKIP  本机条件不足无法判定（如容器内的上传目录、服务未启动）
#
# 与后端 cmd/server 的启动门槛（config.InsecureGates）是同一套判据的两道闸：
# 门槛负责「跑不起来」，本脚本负责「跑起来之前先告诉你差哪几项」。
# 默认值不写死在本脚本里，而是从 backend/internal/config/config.go 现读，
# 避免代码改了默认口令、脚本还在检查旧值这种漂移。
#
# 用法：
#   scripts/preflight.sh                    # 读仓库根 .env（若存在）+ 当前环境变量
#   JWT_KEY=xxx ADMIN_PASS=yyy scripts/preflight.sh
#   PREFLIGHT_STRICT=1 scripts/preflight.sh # WARN 也算失败（接 CI / 发布流水线用）
#
# 配置（环境变量）：
#   PREFLIGHT_STRICT       =1 时存在 WARN 也返回退出码 1（默认 0）
#   PREFLIGHT_HEALTH_URL   服务探活地址，默认 http://127.0.0.1:8080/healthz
#
# 退出码：
#   0  无 FAIL（非严格模式下允许有 WARN）
#   1  存在 FAIL，或严格模式下存在 WARN
#
# 平台：Linux（GNU coreutils）/ macOS 均可。依赖：bash、grep、sed；curl 仅探活用（缺失则 SKIP）。
# ─────────────────────────────────────────────────────────────
set -uo pipefail

# 定位仓库根（允许从任意目录调用）
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

FAILS=0
WARNS=0

pass() { printf '  \033[32m[PASS]\033[0m %s\n' "$1"; }
warn() { WARNS=$((WARNS + 1)); printf '  \033[33m[WARN]\033[0m %s\n' "$1"; }
fail() { FAILS=$((FAILS + 1)); printf '  \033[31m[FAIL]\033[0m %s\n' "$1"; }
skip() { printf '  [SKIP] %s\n' "$1"; }
sec()  { printf '\n%s\n' "$1"; }

# ── 读取 .env：只补全「环境变量里没有」的项，环境变量优先 ──
# 不用 eval 整行（.env 可能含反引号等字符），只认 KEY=VALUE 并剥掉成对引号。
load_env() {
  local f="$1" line key val
  [ -f "$f" ] || return 0
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%$'\r'}"
    case "$line" in ''|'#'*) continue ;; esac
    line="${line#export }"
    key="${line%%=*}"
    [ "$key" = "$line" ] && continue
    case "$key" in *[!A-Za-z0-9_]*) continue ;; esac
    val="${line#*=}"
    val="$(printf '%s' "$val" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
    case "$val" in
      \"*\") val="${val#\"}"; val="${val%\"}" ;;
      \'*\') val="${val#\'}"; val="${val%\'}" ;;
    esac
    # 间接引用读取：环境变量已有值则不覆盖
    if [ -z "${!key:-}" ]; then
      export "$key=$val"
    fi
  done < "$f"
}
load_env "$ROOT/.env"

# ── 从代码现读默认值，避免脚本与实现漂移 ──
CFG="$ROOT/backend/internal/config/config.go"
const_str() { sed -n "s/.*${1} *= *\"\([^\"]*\)\".*/\1/p" "$CFG" | head -1; }
const_num() { sed -n "s/.*${1} *= *\([0-9]\{1,\}\).*/\1/p" "$CFG" | head -1; }
DEF_JWT="$(const_str DefaultJWTKey)";     [ -n "$DEF_JWT" ] || DEF_JWT="change-me"
DEF_PASS="$(const_str DefaultAdminPass)"; [ -n "$DEF_PASS" ] || DEF_PASS=""
MIN_JWT="$(const_num MinJWTKeyLen)";      [ -n "$MIN_JWT" ] || MIN_JWT=16
MIN_PASS="$(const_num MinAdminPassLen)";  [ -n "$MIN_PASS" ] || MIN_PASS=8

echo "微传名片 · 上线前配置自检"
echo "仓库根：$ROOT"

# ── 1. JWT 签名密钥 ──
sec "1. JWT 签名密钥"
jwt="${JWT_KEY:-}"
if [ -z "$jwt" ]; then
  fail "JWT_KEY 未设置：后端会回退内置缺省值，任何人都能伪造任意身份的 token。生成：openssl rand -base64 32"
elif [ "$jwt" = "$DEF_JWT" ]; then
  fail "JWT_KEY 仍是内置缺省值：任何人都能伪造 token。生成：openssl rand -base64 32"
elif [ "${#jwt}" -lt "$MIN_JWT" ]; then
  fail "JWT_KEY 长度 ${#jwt} < ${MIN_JWT}（不打印内容）：可被暴力破解。生成：openssl rand -base64 32"
else
  pass "JWT_KEY 已设置且长度 ${#jwt} ≥ ${MIN_JWT}"
fi

# ── 2. 初始超管口令 ──
sec "2. 初始超管口令（首启动 seed 用）"
ap="${ADMIN_PASS:-}"
if [ -z "$ap" ]; then
  fail "ADMIN_PASS 未设置：后端会回退内置缺省口令，而该口令已随演示账号写进公开文档"
elif [ -n "$DEF_PASS" ] && [ "$ap" = "$DEF_PASS" ]; then
  fail "ADMIN_PASS 仍是内置缺省口令（已写进公开文档）：任何人都能登录后台。生成：openssl rand -base64 12"
elif [ "${#ap}" -lt "$MIN_PASS" ]; then
  warn "ADMIN_PASS 长度 ${#ap} < ${MIN_PASS}：建议更强口令（首登后还应在后台改密）"
else
  pass "ADMIN_PASS 已设置且长度 ${#ap} ≥ ${MIN_PASS}"
fi

# ── 3. 小程序引导种子基址（真机首连的唯一入口）──
sec "3. 小程序引导种子基址"
seed_file="$ROOT/miniprogram/utils/config.ts"
if [ -f "$seed_file" ]; then
  seed="$(sed -n "s/.*DEFAULT_BASE_URL *= *['\"]\([^'\"]*\)['\"].*/\1/p" "$seed_file" | head -1)"
  if [ -z "$seed" ]; then
    skip "$seed_file 中未找到 DEFAULT_BASE_URL"
  else
    case "$seed" in
      *127.0.0.1*|*localhost*)
        fail "种子基址是 ${seed}：真机上 127.0.0.1 是手机自身、不可达，首次请求失败后就永远拿不到后台下发的正确基址。真机联调换本机局域网 IP，生产换 https 域名"
        ;;
      http://*)
        warn "种子基址 ${seed} 是 http：真机需在开发者工具勾选「不校验合法域名」，正式版必须用 https"
        ;;
      https://*)
        pass "种子基址 ${seed}"
        ;;
      *)
        warn "种子基址 ${seed} 既非 http 也非 https，请确认可解析"
        ;;
    esac
  fi
else
  skip "未找到 $seed_file"
fi

# ── 4. 存储驱动与其必需凭据 ──
sec "4. 文件存储"
drv="${STORAGE_DRIVER:-local}"
need() { # need <变量名> <驱动名>
  if [ -z "${!1:-}" ]; then
    fail "STORAGE_DRIVER=${2} 但 ${1} 为空：客户端构造失败、上传全挂（后端会降级本地磁盘，管理端能进但文件落错地方）"
    return 1
  fi
  return 0
}
case "$drv" in
  local)
    pass "存储驱动 local（无需额外凭据）"
    dir="${UPLOAD_DIR:-}"
    if [ -n "$dir" ]; then
      if [ -d "$dir" ]; then
        if [ -w "$dir" ]; then pass "UPLOAD_DIR=$dir 存在且可写"; else fail "UPLOAD_DIR=$dir 存在但不可写"; fi
      else
        skip "UPLOAD_DIR=$dir 在本机不存在（容器内部路径如 /app/uploads 属正常）"
      fi
    else
      skip "UPLOAD_DIR 未设置（由 .env / 环境变量提供）"
    fi
    ;;
  minio)
    ok=1
    for v in MINIO_ENDPOINT MINIO_ACCESS_KEY MINIO_SECRET_KEY MINIO_BUCKET; do need "$v" minio || ok=0; done
    [ "$ok" = 1 ] && pass "存储驱动 minio 的必需凭据齐全"
    ;;
  oss)
    ok=1
    for v in OSS_ENDPOINT OSS_ACCESS_KEY_ID OSS_ACCESS_KEY_SECRET OSS_BUCKET; do need "$v" oss || ok=0; done
    [ "$ok" = 1 ] && pass "存储驱动 oss 的必需凭据齐全"
    ;;
  *)
    fail "STORAGE_DRIVER=$drv 非法：只能是 local / minio / oss"
    ;;
esac

# ── 5. 上传文件对外基址（小程序拼图片 URL 用）──
sec "5. 上传文件对外基址"
ub="${UPLOAD_BASE_URL:-}"
if [ -z "$ub" ]; then
  warn "UPLOAD_BASE_URL 未设置：后端回退 http://127.0.0.1:8080/uploads，真机上图片全部加载不出来（若已在后台「系统设置」里覆盖并重启，可忽略本条）"
else
  case "$ub" in
    *127.0.0.1*|*localhost*)
      warn "UPLOAD_BASE_URL=$ub 是本机地址：真机上不可达，小程序图片全挂。生产应为 https 域名"
      ;;
    https://*)
      pass "UPLOAD_BASE_URL=$ub"
      ;;
    http://*)
      warn "UPLOAD_BASE_URL=$ub 未使用 https：正式版小程序会被拦截或告警，建议换 https"
      ;;
    *)
      warn "UPLOAD_BASE_URL=$ub 既非 http 也非 https，请确认小程序能拼出可访问的地址"
      ;;
  esac
fi

# ── 6. 鉴权 Cookie（HTTPS 生产）──
sec "6. 鉴权 Cookie"
cs="${COOKIE_SECURE:-false}"
if [ "$cs" = "true" ] || [ "$cs" = "1" ]; then
  pass "COOKIE_SECURE=true"
else
  warn "COOKIE_SECURE=false：HTTPS 生产建议设 true，避免登录 Cookie 被明文传输（http 联调保持 false；注意该值只能改环境变量并重启）"
fi

# ── 7. 数据库口令 ──
sec "7. 数据库"
# 驱动按 DSN 形态自动选择（internal/dbopen）：SQLite 部署没有 MySQL 口令可查，
# 硬查只会产出一条误导性 WARN，这里改为 SKIP 并提示 SQLite 自己的注意点。
dsn="${DB_DSN:-}"
dsn_path="${dsn%%\?*}"
is_sqlite=0
case "$dsn" in
  sqlite:* | file:*) is_sqlite=1 ;;
esac
case "$dsn_path" in
  *.db | *.sqlite | *.sqlite3) is_sqlite=1 ;;
esac
if [ "$is_sqlite" = "1" ]; then
  skip "DB_DSN 指向 SQLite（${dsn_path:-未设置}）：跳过 MySQL 口令检查。请自行确认——库文件在本地磁盘（勿放 NFS）、单机单进程、备份用 VACUUM INTO"
else
  mrp="${MYSQL_ROOT_PASSWORD:-}"
  # 示例值也从编排文件现读，避免改了 compose、脚本还在检查旧值
  # （编排里是 :- 留空 + 容器内 entrypoint 校验，不存在示例值可回退；
  #   未设置时该形态下 mysql 容器启动即报错退出，不会带空口令悄悄启动）
  def_mysql="$(sed -n 's/.*MYSQL_ROOT_PASSWORD: \${MYSQL_ROOT_PASSWORD:-\([^}]*\)}.*/\1/p' "$ROOT/docker-compose.yml" | head -1)"
  if [ -z "$mrp" ]; then
    warn "MYSQL_ROOT_PASSWORD 未设置：仅 --profile mysql 形态需要（未设则该形态下 mysql 容器启动即报错退出，日志有提示）；默认 SQLite 形态可不管"
  elif [ -n "$def_mysql" ] && [ "$mrp" = "$def_mysql" ]; then
    warn "MYSQL_ROOT_PASSWORD 仍是编排文件里的示例值：生产请改强随机值"
  else
    pass "MYSQL_ROOT_PASSWORD 已自定义"
  fi
fi

# ── 8. 服务探活（可选：服务已运行时才判）──
sec "8. 服务探活"
url="${PREFLIGHT_HEALTH_URL:-http://127.0.0.1:8080/healthz}"
if command -v curl >/dev/null 2>&1; then
  body="$(curl -fsS -m 5 "$url" 2>/dev/null)" || body=""
  if [ -z "$body" ]; then
    skip "$url 不可达（部署前跑本脚本属正常；部署后应连通）"
  else
    case "$body" in
      *'"db":"up"'*) pass "healthz 正常（db=up）" ;;
      *'"db":"down"'*) fail "healthz 报 db=down：数据库连不上，请查 DB_DSN 与实例状态" ;;
      *) warn "healthz 返回非预期内容：$(printf '%s' "$body" | cut -c1-120)" ;;
    esac
  fi
else
  skip "无 curl，跳过探活"
fi

# ── 汇总 ──
sec "汇总"
if [ "$FAILS" -gt 0 ]; then
  printf '  \033[31mFAIL %d 项\033[0m，WARN %d 项：请修掉 FAIL 再上线（详见 docs/deploy-runbook.md 的上线前清单）\n' "$FAILS" "$WARNS"
  exit 1
fi
if [ "$WARNS" -gt 0 ]; then
  printf '  无 FAIL，\033[33mWARN %d 项\033[0m：可上线，建议按提示加固\n' "$WARNS"
  if [ "${PREFLIGHT_STRICT:-0}" = "1" ]; then
    echo "  PREFLIGHT_STRICT=1：WARN 视为失败"
    exit 1
  fi
  exit 0
fi
echo "  全部检查通过，可以上线"
exit 0
