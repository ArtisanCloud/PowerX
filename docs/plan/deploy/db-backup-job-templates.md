# 数据库备份任务模板（脚本 + 定时任务）

> 目标：给出一套可直接改造落地的最小模板，覆盖 Docker 与非 Docker。

## 1. 环境变量约定

```bash
export PGHOST=127.0.0.1
export PGPORT=5432
export PGUSER=powerx
export PGPASSWORD=change_me
export PGDATABASE=powerx

export BACKUP_ROOT=/opt/powerx/backups
export BACKUP_RETENTION_DAYS=30

# 对象存储（示例，按你的工具替换）
export BACKUP_S3_URI=s3://powerx-backups
```

## 2. 备份脚本模板（backup-db.sh）

```bash
#!/usr/bin/env bash
set -euo pipefail

TS=$(date +%Y%m%d_%H%M%S)
JOB_ID="backup_${TS}"
OUT_DIR="${BACKUP_ROOT}/logical/$(date +%Y/%m/%d)"
OUT_FILE="${OUT_DIR}/${PGDATABASE}_${TS}.dump"
META_FILE="${OUT_FILE}.meta.json"

mkdir -p "${OUT_DIR}"

STARTED_AT=$(date -Iseconds)

pg_dump -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -d "${PGDATABASE}" -Fc -f "${OUT_FILE}"

SIZE_BYTES=$(wc -c < "${OUT_FILE}" | tr -d ' ')
CHECKSUM=$(sha256sum "${OUT_FILE}" | awk '{print $1}')
ENDED_AT=$(date -Iseconds)

cat > "${META_FILE}" <<JSON
{
  "job_id": "${JOB_ID}",
  "backup_type": "logical",
  "started_at": "${STARTED_AT}",
  "ended_at": "${ENDED_AT}",
  "size_bytes": ${SIZE_BYTES},
  "checksum": "${CHECKSUM}",
  "status": "success",
  "storage_uri": "${BACKUP_S3_URI}/logical/$(date +%Y/%m/%d)/$(basename "${OUT_FILE}")"
}
JSON

# 上传对象存储（按实际 CLI 替换）
# aws s3 cp "${OUT_FILE}" "${BACKUP_S3_URI}/logical/$(date +%Y/%m/%d)/"
# aws s3 cp "${META_FILE}" "${BACKUP_S3_URI}/logical/$(date +%Y/%m/%d)/"

echo "[backup] success job_id=${JOB_ID} file=${OUT_FILE} size=${SIZE_BYTES}"
```

## 3. 清理脚本模板（cleanup-backups.sh）

```bash
#!/usr/bin/env bash
set -euo pipefail

find "${BACKUP_ROOT}/logical" -type f -name "*.dump" -mtime +"${BACKUP_RETENTION_DAYS}" -print -delete
find "${BACKUP_ROOT}/logical" -type f -name "*.meta.json" -mtime +"${BACKUP_RETENTION_DAYS}" -print -delete

echo "[cleanup] done retention_days=${BACKUP_RETENTION_DAYS}"
```

## 4. 恢复演练脚本模板（restore-drill.sh）

```bash
#!/usr/bin/env bash
set -euo pipefail

DUMP_FILE="$1"
TMP_DB="powerx_restore_drill"

psql -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -d postgres -c "DROP DATABASE IF EXISTS ${TMP_DB};"
psql -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -d postgres -c "CREATE DATABASE ${TMP_DB};"
pg_restore -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -d "${TMP_DB}" "${DUMP_FILE}"

psql -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -d "${TMP_DB}" -c "SELECT now();"

echo "[restore-drill] success db=${TMP_DB} dump=${DUMP_FILE}"
```

## 5. systemd 定时任务模板

`/etc/systemd/system/powerx-db-backup.service`:

```ini
[Unit]
Description=PowerX DB Backup Job

[Service]
Type=oneshot
User=powerx
Group=powerx
EnvironmentFile=/opt/powerx/shared/config/backup.env
ExecStart=/opt/powerx/scripts/backup-db.sh
```

`/etc/systemd/system/powerx-db-backup.timer`:

```ini
[Unit]
Description=Run PowerX DB Backup Daily

[Timer]
OnCalendar=*-*-* 02:30:00
Persistent=true
Unit=powerx-db-backup.service

[Install]
WantedBy=timers.target
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now powerx-db-backup.timer
systemctl list-timers | grep powerx-db-backup
```

## 6. Docker 定时任务建议

- 方式 A：单独 `backup` 容器内跑 `supercronic`
- 方式 B：宿主 cron 调 `docker exec powerx-backend /opt/powerx/scripts/backup-db.sh`

建议优先 A，职责更清晰。

