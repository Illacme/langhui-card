#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────
# MySQL 逻辑备份脚本（mysqldump → gzip → 按保留份数轮转）。
#
# 用法：
#   scripts/backup.sh                      # 用默认配置（容器名 weichuan-mysql）
#   DB_CONTAINER=my-mysql DB_USER=root DB_PASSWORD=secret DB_NAME=weichuan scripts/backup.sh
#   BACKUP_DIR=/data/backups KEEP_DAYS=14 scripts/backup.sh
#
# 依赖：mysqldump 在容器内（默认通过 docker exec 执行，不依赖宿主机装 MySQL 客户端）。
# 建议：加入 crontab，如每天凌晨 3 点全量备份——
#   0 3 * * * cd /path/to/repo && scripts/backup.sh >> /var/log/weichuan-backup.log 2>&1
#
# 注意：逻辑备份不含二进制日志，恢复点 = 备份时刻；需要 PITR（时间点恢复）
# 须另行开启 binlog 并定期备份 binlog 文件。
# ─────────────────────────────────────────────────────────────
set -euo pipefail

DB_CONTAINER="${DB_CONTAINER:-weichuan-mysql}"
DB_USER="${DB_USER:-root}"
DB_PASSWORD="${DB_PASSWORD:-root}"
DB_NAME="${DB_NAME:-weichuan}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
KEEP_DAYS="${KEEP_DAYS:-7}"

mkdir -p "$BACKUP_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT="$BACKUP_DIR/${DB_NAME}-${STAMP}.sql.gz"

echo "[backup] $STAMP dumping ${DB_NAME} from container ${DB_CONTAINER} ..."

# --single-transaction：InnoDB 一致性快照，不锁表（业务无感）
# --quick：逐行写出，避免大表撑爆内存
# --routines --events：含存储过程 / 事件（本项目未用，留着无害）
# --default-character-set=utf8mb4：与建表字符集一致，避免乱码
if docker exec -e MYSQL_PWD="$DB_PASSWORD" "$DB_CONTAINER" \
  mysqldump -u"$DB_USER" --single-transaction --quick --routines --events \
  --default-character-set=utf8mb4 "$DB_NAME" | gzip > "$OUT"; then
  SIZE="$(du -h "$OUT" | cut -f1)"
  echo "[backup] OK -> $OUT ($SIZE)"
else
  echo "[backup] FAILED (docker exec mysqldump 非零退出)" >&2
  rm -f "$OUT"
  exit 1
fi

# 轮转：删除超过保留天数的旧备份
if [ -d "$BACKUP_DIR" ]; then
  find "$BACKUP_DIR" -name "${DB_NAME}-*.sql.gz" -type f -mtime +"$KEEP_DAYS" -print -delete
fi

echo "[backup] done (keep ${KEEP_DAYS} days)"
