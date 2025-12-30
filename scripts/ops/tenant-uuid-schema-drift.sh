#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PROJECT_ROOT=$(cd -- "$SCRIPT_DIR/../.." && pwd)
CONFIG_FILE=${CONFIG_FILE:-"$PROJECT_ROOT/backend/etc/config.yaml"}
PSQL_BIN=${PSQL:-psql}
TEXTFILE_OUTPUT=${TEXTFILE_OUTPUT:-"$PROJECT_ROOT/backend/reports/tenant-uuid-schema-drift.prom"}

usage() {
  cat <<'USAGE'
Usage: scripts/ops/tenant-uuid-schema-drift.sh [--textfile path]

Options:
  --textfile <path>   Write Prometheus textfile metrics to the given path.

The script connects to the database, counts tables that still contain tenant_id
columns or are missing tenant_uuid, then emits Prometheus gauges so Grafana can
track tenant_uuid_schema_drift.
USAGE
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
    local key value
    key=$(trim "$raw_key")
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
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --textfile)
        TEXTFILE_OUTPUT="$2"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "unknown option: $1" >&2
        usage
        exit 1
        ;;
    esac
  done
}

query_schema_drift() {
  "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} \
    -v ON_ERROR_STOP=1 \
    -t -A <<'SQL'
WITH tables AS (
  SELECT table_schema,
         table_name,
         bool_or(column_name = 'tenant_id') AS has_tenant_id,
         bool_or(column_name = 'tenant_uuid') AS has_tenant_uuid
  FROM information_schema.columns
  WHERE table_schema NOT IN ('pg_catalog','information_schema')
  GROUP BY table_schema, table_name
)
SELECT COALESCE(count(*) FILTER (WHERE has_tenant_id), 0) AS tables_with_tenant_id,
       COALESCE(count(*) FILTER (WHERE NOT has_tenant_uuid), 0) AS tables_without_tenant_uuid
FROM tables;
SQL
}

write_metrics() {
  local with_tid=$1
  local without_uuid=$2
  cat <<EOF >"$TEXTFILE_OUTPUT"
# HELP tenant_uuid_schema_drift Number of tables still containing tenant_id column
# TYPE tenant_uuid_schema_drift gauge
tenant_uuid_schema_drift $with_tid
# HELP tenant_uuid_tables_without_uuid Number of tables missing tenant_uuid column
# TYPE tenant_uuid_tables_without_uuid gauge
tenant_uuid_tables_without_uuid $without_uuid
EOF
  echo "[metrics] tenant_uuid_schema_drift=$with_tid tenant_uuid_tables_without_uuid=$without_uuid"
  echo "[metrics] written to $TEXTFILE_OUTPUT"
}

main() {
  parse_args "$@"
  load_db_env_from_config
  local raw
  raw=$(query_schema_drift)
  if [[ -z "$raw" ]]; then
    echo "[error] failed to read schema drift counters" >&2
    exit 1
  fi
  local tables_with_tid tables_without_uuid
  IFS='|' read -r tables_with_tid tables_without_uuid <<<"$raw"
  tables_with_tid=${tables_with_tid:-0}
  tables_without_uuid=${tables_without_uuid:-0}
  write_metrics "$tables_with_tid" "$tables_without_uuid"
}

main "$@"
