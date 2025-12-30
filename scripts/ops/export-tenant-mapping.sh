#!/usr/bin/env bash
set -euo pipefail

# Export tenant UUID ↔ ID mapping for downstream systems (Billing/CRM/etc).
# Usage:
#   scripts/ops/export-tenant-mapping.sh --output /tmp/tenant-mapping.csv
# Environment variables:
#   CONFIG_FILE (default backend/etc/config.yaml)
#   DATABASE_URL or standard PG envs will override config file values.

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PROJECT_ROOT=$(cd -- "$SCRIPT_DIR/../.." && pwd)
CONFIG_FILE=${CONFIG_FILE:-"$PROJECT_ROOT/backend/etc/config.yaml"}
OUTPUT=""
PSQL_BIN=${PSQL:-psql}

usage() {
  cat <<'USAGE'
Usage: scripts/ops/export-tenant-mapping.sh --output path/to/file.csv [--include-deleted]

Options:
  --output <path>        Output CSV file (required)
  --include-deleted      Include tenants with deleted_at != NULL
USAGE
}

INCLUDE_DELETED="false"

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --output)
        OUTPUT="$2"; shift 2 ;;
      --include-deleted)
        INCLUDE_DELETED="true"; shift ;;
      -h|--help)
        usage; exit 0 ;;
      *)
        echo "unknown option: $1" >&2
        usage; exit 1 ;;
    esac
  done

  if [[ -z "$OUTPUT" ]]; then
    echo "--output is required" >&2
    usage
    exit 1
  fi
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
    echo "[warn] config file not found: $CONFIG_FILE" >&2
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

export_mapping() {
  local deleted_clause="WHERE deleted_at IS NULL"
  if [[ "$INCLUDE_DELETED" == "true" ]]; then
    deleted_clause=""
  fi
  "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} \
    -v ON_ERROR_STOP=1 \
    -At -F',' \
    -c "COPY (
          SELECT tenant_uuid, id, key, name, created_at, deleted_at
          FROM iam_tenant
          ${deleted_clause}
          ORDER BY id
        ) TO STDOUT WITH CSV HEADER" >"$OUTPUT"
  echo "[info] exported mapping to $OUTPUT"
}

parse_args "$@"
load_db_env_from_config
mkdir -p "$(dirname "$OUTPUT")"
export_mapping
