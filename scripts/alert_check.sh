#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────
# 最小告警检查（零第三方依赖，三条规则，配 crontab 每分钟运行）：
#   R1 healthz    ：本地探活 GET /healthz，非 2xx / 超时即告警
#   R2 5xx        ：访问日志新增 [ACCESS-5XX] 行即告警（附请求 ID 便于排查）
#   R3 备份新鲜度 ：backups/ 最新备份文件年龄超阈值即告警
#                   （backup.sh 失败时不落新文件，本规则可兜底捕获）
#
# 定位：不替代 Prometheus/Alertmanager，而是「没有监控基建也能跑起来」
# 的最小闭环；规则是纯增量，随时可平滑迁到标准监控栈。
#
# 用法：
#   scripts/alert_check.sh
#   ALERT_WEBHOOK=https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx \
#     HEALTH_URL=http://127.0.0.1:8080/healthz \
#     LOG_FILE=/var/log/langhui/access.log \
#     BACKUP_DIR=./backups scripts/alert_check.sh
#
# 配置（环境变量，均有默认值）：
#   HEALTH_URL       默认 http://127.0.0.1:8080/healthz
#   LOG_FILE         后端访问日志文件（AccessLog 的 stderr 收集处），
#                    默认 /var/log/langhui/access.log
#   BACKUP_DIR       备份目录（与 backup.sh 的 BACKUP_DIR 一致），默认 ./backups
#   BACKUP_MAX_HOURS 最新备份允许的最大年龄（小时），默认 26（日备 + 2h 余量）
#   STATE_DIR        状态目录（日志偏移量 / 告警冷却时间戳），默认 /var/tmp/langhui-alert
#   ALERT_WEBHOOK    机器人 webhook（企业微信 / 钉钉，msgtype=text 同格式；
#                    飞书格式不同需自行调整 -d 负载）；未配置时告警打到 stderr
#   ALERT_COOLDOWN   同一规则两次告警的最小间隔（秒），默认 1800（防轰炸）
#
# 平台：Linux（GNU coreutils）；BSD/macOS 的 stat/date 差异已在 file_mtime 内兼容。
# 依赖：bash、curl（探活与 webhook 推送）、grep/tail/wc。
# ─────────────────────────────────────────────────────────────
set -uo pipefail

HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/healthz}"
LOG_FILE="${LOG_FILE:-/var/log/langhui/access.log}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
BACKUP_MAX_HOURS="${BACKUP_MAX_HOURS:-26}"
STATE_DIR="${STATE_DIR:-/var/tmp/langhui-alert}"
ALERT_WEBHOOK="${ALERT_WEBHOOK:-}"
ALERT_COOLDOWN="${ALERT_COOLDOWN:-1800}"

command -v curl >/dev/null 2>&1 || { echo "[alert_check] 需要 curl（探活与 webhook 推送）" >&2; exit 1; }
mkdir -p "$STATE_DIR"

# ── 通知：per-rule 冷却防轰炸；webhook 不可达时降级到 stderr ──
# 用法：send_alert <规则名> <消息> [冷却秒数]
send_alert() {
  local rule="$1" msg="$2" cooldown="${3:-$ALERT_COOLDOWN}"
  local now last stamp text safe
  now="$(date +%s)"
  stamp="$STATE_DIR/cooldown_$rule"
  last=0
  [ -f "$stamp" ] && last="$(cat "$stamp" 2>/dev/null || echo 0)"
  case "$last" in ''|*[!0-9]*) last=0 ;; esac
  if [ $((now - last)) -lt "$cooldown" ]; then
    return 0
  fi
  echo "$now" > "$stamp"

  # 日志行可能含引号 / 反斜杠 / 换行，转义后截断再进 JSON
  safe="$(printf '%s' "$msg" | tr -d '\r\n' | sed 's/\\/\\\\/g; s/"/\\"/g' | cut -c1-300)"
  text="[langhui-alert] $rule: $safe ($(date '+%F %T'))"
  if [ -n "$ALERT_WEBHOOK" ]; then
    if ! curl -fsS -m 10 -X POST "$ALERT_WEBHOOK" \
        -H 'Content-Type: application/json' \
        -d "{\"msgtype\":\"text\",\"text\":{\"content\":\"$text\"}}" >/dev/null 2>&1; then
      echo "$text（webhook 发送失败，降级为本行输出）" >&2
    fi
  else
    echo "$text" >&2
  fi
}

# ── R1: healthz 探活 ──
check_healthz() {
  if curl -fsS -m 10 -o /dev/null "$HEALTH_URL" 2>/dev/null; then
    return 0
  fi
  send_alert healthz "GET $HEALTH_URL 探活失败（进程退出 / 端口不通 / 反代故障）"
}

# ── R2: 访问日志新增 [ACCESS-5XX] 行 ──
# 字节偏移量只扫增量，避免每次全量重扫造成重复告警；
# 日志轮转（文件变小）时偏移归零重扫，由冷却时间兜底防轰炸。
check_5xx() {
  if [ ! -f "$LOG_FILE" ]; then
    echo "[alert_check] 日志文件不存在，跳过 R2：$LOG_FILE" >&2
    return 0
  fi
  local size last new chunk sample
  size="$(wc -c < "$LOG_FILE" | tr -d ' ')"
  last=0
  [ -f "$STATE_DIR/log_offset" ] && last="$(cat "$STATE_DIR/log_offset" 2>/dev/null || echo 0)"
  case "$last" in ''|*[!0-9]*) last=0 ;; esac
  if [ "$size" -lt "$last" ]; then
    last=0
  fi
  chunk="$(tail -c "+$((last + 1))" "$LOG_FILE" 2>/dev/null)"
  echo "$size" > "$STATE_DIR/log_offset"
  new="$(printf '%s\n' "$chunk" | grep -c '\[ACCESS-5XX\]')"
  if [ "${new:-0}" -gt 0 ]; then
    sample="$(printf '%s\n' "$chunk" | grep '\[ACCESS-5XX\]' | tail -1 | cut -c1-160)"
    send_alert 5xx "访问日志新增 $new 条 [ACCESS-5XX]；最新一条：$sample"
  fi
}

# ── R3: 备份新鲜度 ──
file_mtime() {
  stat -c %Y "$1" 2>/dev/null && return 0
  stat -f %m "$1" 2>/dev/null && return 0
  perl -e 'print((stat($ARGV[0]))[9])' "$1" 2>/dev/null
}

check_backup() {
  if [ ! -d "$BACKUP_DIR" ]; then
    echo "[alert_check] 备份目录不存在，跳过 R3：$BACKUP_DIR" >&2
    return 0
  fi
  local latest m age_h
  latest="$(ls -t "$BACKUP_DIR"/langhui-*.sql.gz 2>/dev/null | head -1)"
  if [ -z "$latest" ]; then
    # 从未备份，或备份停跑且旧文件已轮转——两种情况都意味着"当前没有可用备份"
    send_alert backup "备份目录 $BACKUP_DIR 中没有备份文件（从未备份，或备份停跑且旧文件已轮转）" 21600
    return 0
  fi
  m="$(file_mtime "$latest")"
  [ -n "$m" ] || return 0
  age_h=$(( ( $(date +%s) - m ) / 3600 ))
  if [ "$age_h" -ge "$BACKUP_MAX_HOURS" ]; then
    send_alert backup "最新备份 $(basename "$latest") 已 ${age_h}h 未更新（阈值 ${BACKUP_MAX_HOURS}h），备份任务可能未运行"
  fi
}

check_healthz
check_5xx
check_backup
exit 0
