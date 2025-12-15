#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PROJECT_ROOT=$(cd -- "$SCRIPT_DIR/../.." && pwd)
MIGRATION_DIR="$PROJECT_ROOT/scripts/migrations/tenant-uuid"
CHECK_SCRIPT="$PROJECT_ROOT/scripts/ops/checks/tenant_uuid_consistency.sql"
TMP_DIR="$PROJECT_ROOT/tmp"
DEFAULT_BACKUP="$TMP_DIR/tenant-uuid-schema-$(date +%Y%m%d%H%M%S).sql"
PSQL_BIN=${PSQL:-psql}
PG_DUMP_BIN=${PG_DUMP:-pg_dump}
CONFIG_FILE=${CONFIG_FILE:-"$PROJECT_ROOT/backend/etc/config.yaml"}
TENANT_TABLE=${TENANT_TABLE:-public.iam_tenant}

usage() {
  cat <<'USAGE'
Usage: scripts/ops/tenant-id-cleanup.sh <command> [options]
Commands:
  plan                     # 列出仍含 tenant_id 的表，并执行一致性检查
  run [--skip-backup] [--drop]  # 执行 001+002 脚本（可选顺便执行 999）
  drop                     # 仅执行 999_drop_tenant_id_columns.sql
  rollback <backup.sql>    # 通过 schema-only 备份回滚
  status                   # 打印最近一次 backfill 报告

需要设置 DATABASE_URL 或标准 PG 环境变量（PGHOST/PGUSER/PGDATABASE 等）。
USAGE
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "[error] missing command: $1" >&2; exit 1; }
}

trim() {
  local val="$1"
  val="${val%%#*}"
  val="${val%"${val##*[![:space:]]}"}"
  val="${val#"${val%%[![:space:]]*}"}"
  val="${val%\"}"
  val="${val#\"}"
  echo "$val"
}

load_db_env_from_config() {
  if [[ -n "${DATABASE_URL:-}" || -n "${PGHOST:-}" ]]; then
    return
  fi
  if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "[warn] database config file not found: $CONFIG_FILE" >&2
    return
  fi
  local section
  section=$(
    awk '
      /^database:/ {flag=1; next}
      flag && /^[^[:space:]]/ {flag=0}
      flag {print}
    ' "$CONFIG_FILE"
  )
  if [[ -z "$section" ]]; then
    echo "[warn] database section not found in $CONFIG_FILE" >&2
    return
  fi
  while IFS=: read -r raw_key raw_value; do
    [[ -n "$raw_key" ]] || continue
    local key
    key=$(trim "$raw_key")
    [[ -n "$key" ]] || continue
    local value
    value=$(trim "$raw_value")
    case "$key" in
      host) [[ -z "${PGHOST:-}" && -n "$value" ]] && export PGHOST="$value" ;;
      port) [[ -z "${PGPORT:-}" && -n "$value" ]] && export PGPORT="$value" ;;
      username) [[ -z "${PGUSER:-}" && -n "$value" ]] && export PGUSER="$value" ;;
      password) [[ -z "${PGPASSWORD:-}" && -n "$value" ]] && export PGPASSWORD="$value" ;;
      database) [[ -z "${PGDATABASE:-}" && -n "$value" ]] && export PGDATABASE="$value" ;;
      ssl_mode) [[ -z "${PGSSLMODE:-}" && -n "$value" ]] && export PGSSLMODE="$value" ;;
    esac
  done <<<"$section"
  if [[ -n "${PGHOST:-}" && -n "${PGUSER:-}" && -n "${PGDATABASE:-}" ]]; then
    echo "[info] loaded database config from $CONFIG_FILE"
  else
    echo "[warn] incomplete database config in $CONFIG_FILE; please set PG* env or DATABASE_URL manually" >&2
  fi
}

psql_exec() {
  local sql_file="$1"
  echo "[info] running $sql_file"
  "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} \
    -v ON_ERROR_STOP=1 \
    -v tenant_table="$TENANT_TABLE" \
    -f "$sql_file"
}

plan() {
  require_cmd "$PSQL_BIN"
  mkdir -p "$TMP_DIR"
  echo "[plan] collecting tables with tenant_id..."
  "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} -v ON_ERROR_STOP=1 <<'SQL'
WITH target AS (
  SELECT table_schema, table_name
  FROM information_schema.columns
  WHERE column_name = 'tenant_id'
    AND table_schema NOT IN ('pg_catalog','information_schema')
)
SELECT format('%s.%s', table_schema, table_name) AS table,
       (SELECT count(*) FROM information_schema.columns c
         WHERE c.table_schema = target.table_schema AND c.table_name = target.table_name AND c.column_name = 'tenant_uuid') AS has_tenant_uuid
FROM target
ORDER BY 1;
SQL
  echo "[plan] running consistency check script"
  "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} -v ON_ERROR_STOP=1 -f "$CHECK_SCRIPT"
  echo "[plan] done. review above output for tables/results."
}

run() {
  local skip_backup=false
  local run_drop=false
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --skip-backup) skip_backup=true ; shift ;;
      --drop) run_drop=true ; shift ;;
      *) echo "unknown option: $1" >&2; usage; exit 1 ;;
    esac
  done
  require_cmd "$PSQL_BIN"
  require_cmd "$PG_DUMP_BIN"
  mkdir -p "$TMP_DIR"
  if [[ "$skip_backup" = false ]]; then
    local backup_path="$DEFAULT_BACKUP"
    echo "[run] taking schema backup to $backup_path"
    "$PG_DUMP_BIN" ${DATABASE_URL:+"$DATABASE_URL"} --schema-only --file "$backup_path"
  else
    echo "[run] skip backup enabled"
  fi
  psql_exec "$MIGRATION_DIR/001_add_tenant_uuid_columns.sql"
  psql_exec "$MIGRATION_DIR/002_backfill_tenant_uuid.sql"
  "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} -v ON_ERROR_STOP=1 -f "$CHECK_SCRIPT"
  if [[ "$run_drop" = true ]]; then
    psql_exec "$MIGRATION_DIR/999_drop_tenant_id_columns.sql"
  else
    echo "[run] skipping DROP. Use --drop when ready."
  fi
  echo "[run] 完成，日志保存在 $DEFAULT_BACKUP（如未跳过备份）。若需回滚，可执行:"
  echo "       scripts/ops/tenant-id-cleanup.sh rollback $DEFAULT_BACKUP"
}

run_drop_only() {
  require_cmd "$PSQL_BIN"
  psql_exec "$MIGRATION_DIR/999_drop_tenant_id_columns.sql"
}

rollback() {
  local backup_path=${1:-}
  [[ -f "$backup_path" ]] || { echo "[error] backup file not found: $backup_path" >&2; exit 1; }
  require_cmd "$PSQL_BIN"
  echo "[rollback] replaying $backup_path"
  "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} -v ON_ERROR_STOP=1 -f "$backup_path"
}

status() {
  require_cmd "$PSQL_BIN"
  "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} -v ON_ERROR_STOP=1 <<'SQL'
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'tenant_uuid_backfill_report'
  ) THEN
    RAISE NOTICE 'tenant_uuid_backfill_report table not found. Run the backfill first.';
    RETURN;
  END IF;
END $$;

SELECT * FROM public.tenant_uuid_backfill_report ORDER BY executed_at DESC LIMIT 50;
SQL
}

main() {
  load_db_env_from_config
  local cmd=${1:-plan}
  case "$cmd" in
    plan) shift; plan "$@" ;;
    run) shift; run "$@" ;;
    drop) shift; run_drop_only "$@" ;;
    rollback) shift; rollback "$@" ;;
    status) shift; status "$@" ;;
    *) usage; exit 1 ;;
  esac
}

main "$@"
