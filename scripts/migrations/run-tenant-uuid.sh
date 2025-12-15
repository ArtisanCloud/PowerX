#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PROJECT_ROOT=$(cd -- "$SCRIPT_DIR/../.." && pwd)
MIGRATION_DIR="$PROJECT_ROOT/scripts/migrations/tenant-uuid"
CHECK_SCRIPT="$PROJECT_ROOT/scripts/ops/checks/tenant_uuid_consistency.sql"
PSQL_BIN=${PSQL:-psql}
CONFIG_FILE=${CONFIG_FILE:-"$PROJECT_ROOT/backend/etc/config.yaml"}
LOG_DIR="$PROJECT_ROOT/tmp/reports"
DEFAULT_LOG="$LOG_DIR/tenant-run-$(date +%Y%m%d-%H%M%S).log"

TENANT_FILTER=""
DRY_RUN=false
RUN_DROP=false
RUN_CHECK=true
LOG_FILE=""

usage() {
  cat <<'USAGE'
Usage: scripts/migrations/run-tenant-uuid.sh [options]

Options:
  --tenants all|uuid1,uuid2   仅回填指定租户（默认 all）
  --dry-run                   在事务中执行并回滚，只用于演练
  --with-drop                 在回填后执行 999_drop_tenant_id_columns.sql
  --skip-check                跳过 tenant_uuid_consistency 检查
  --log <path>                自定义日志输出路径（默认 tmp/reports/tenant-run-<ts>.log）
  -h, --help                  显示帮助

环境依赖：DATABASE_URL 或 PGHOST/PGUSER/PGDATABASE，或在 backend/etc/config.yaml 中配置。
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
      --tenants)
        TENANT_FILTER=${2:-}
        shift 2
        ;;
      --tenants=*)
        TENANT_FILTER=${1#*=}
        shift 1
        ;;
      --dry-run)
        DRY_RUN=true
        shift
        ;;
      --with-drop)
        RUN_DROP=true
        shift
        ;;
      --skip-check)
        RUN_CHECK=false
        shift
        ;;
      --log)
        LOG_FILE=${2:-}
        shift 2
        ;;
      --log=*)
        LOG_FILE=${1#*=}
        shift 1
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

setup_logging() {
  mkdir -p "$LOG_DIR"
  local target=${LOG_FILE:-$DEFAULT_LOG}
  mkdir -p "$(dirname "$target")"
  LOG_FILE="$target"
  exec > >(tee "$LOG_FILE")
  exec 2>&1
  echo "[log] streaming output to $LOG_FILE"
}

normalize_tenant_filter() {
  if [[ -z "${TENANT_FILTER:-}" || "$TENANT_FILTER" == "all" ]]; then
    TENANT_FILTER=""
    echo "[args] tenants: all"
    return
  fi
  TENANT_FILTER=${TENANT_FILTER// /}
  TENANT_FILTER=${TENANT_FILTER%,}
  if [[ -z "$TENANT_FILTER" ]]; then
    echo "[args] tenants: all"
    return
  fi
  local invalid=()
  IFS=',' read -r -a items <<<"$TENANT_FILTER"
  for uuid in "${items[@]}"; do
    if [[ ! "$uuid" =~ ^[0-9a-fA-F-]{32,36}$ ]]; then
      invalid+=("$uuid")
    fi
  done
  if [[ ${#invalid[@]} -gt 0 ]]; then
    echo "[error] invalid tenant uuid(s): ${invalid[*]}" >&2
    exit 1
  fi
  echo "[args] tenants: ${TENANT_FILTER}"
}

run_sql_script() {
  local script="$1"
  shift
  local extra_args=("$@")
  echo "[sql] executing $script"
  if [[ "$DRY_RUN" = true ]]; then
    "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} -v ON_ERROR_STOP=1 ${extra_args[@]+"${extra_args[@]}"} <<SQL
BEGIN;
\\i $script
ROLLBACK;
SQL
  else
    "$PSQL_BIN" ${DATABASE_URL:+"$DATABASE_URL"} -v ON_ERROR_STOP=1 ${extra_args[@]+"${extra_args[@]}"} -f "$script"
  fi
}

run_check() {
  if [[ "$RUN_CHECK" = false ]]; then
    echo "[check] skipped"
    return
  fi
  if [[ ! -f "$CHECK_SCRIPT" ]]; then
    echo "[warn] check script not found: $CHECK_SCRIPT" >&2
    return
  fi
  echo "[check] running tenant_uuid_consistency"
  run_sql_script "$CHECK_SCRIPT"
}

main() {
  parse_args "$@"
  require_cmd "$PSQL_BIN"
  setup_logging
  normalize_tenant_filter
  echo "[args] dry-run: $DRY_RUN"
  echo "[args] with-drop: $RUN_DROP"
  load_db_env_from_config

  local tenant_args=()
  if [[ -n "$TENANT_FILTER" ]]; then
    tenant_args+=(-v tenant_uuids="$TENANT_FILTER")
  fi

  run_sql_script "$MIGRATION_DIR/001_add_tenant_uuid_columns.sql"
  run_sql_script "$MIGRATION_DIR/002_backfill_tenant_uuid.sql" ${tenant_args[@]+"${tenant_args[@]}"}

  if [[ "$RUN_DROP" = true ]]; then
    run_sql_script "$MIGRATION_DIR/999_drop_tenant_id_columns.sql"
  else
    echo "[sql] skipping DROP (use --with-drop to enable)"
  fi

  run_check
  echo "[done] tenant uuid backfill completed"
}

main "$@"
