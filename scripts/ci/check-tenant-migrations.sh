#!/usr/bin/env bash
# 校验 Tenant UUID 迁移脚本：SQL 规范、可执行性与一致性检查
set -euo pipefail

PROJECT_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
SQL_DIR=${SQL_DIR:-"$PROJECT_ROOT/scripts/migrations/tenant-uuid"}
CHECK_SQL=${CHECK_SQL:-"$PROJECT_ROOT/scripts/ops/checks/tenant_uuid_consistency.sql"}
SQLFLUFF_CONFIG=${SQLFLUFF_CONFIG:-"$PROJECT_ROOT/.sqlfluff"}
STRICT=${STRICT:-false}
RUN_PSQL_CHECK=${RUN_PSQL_CHECK:-true}
SUMMARY=()
FAILURES=0

log() {
  printf '[check-tenants] %s\n' "$*"
}

warn() {
  printf '[check-tenants][warn] %s\n' "$*" >&2
}

fail() {
  printf '[check-tenants][error] %s\n' "$*" >&2
  FAILURES=$((FAILURES + 1))
}

list_sql_files() {
  if [[ ! -d "$SQL_DIR" ]]; then
    fail "SQL 目录不存在: $SQL_DIR"
    return 1
  fi
  find "$SQL_DIR" -maxdepth 1 -name '*.sql' | sort
}

run_sqlfluff() {
  local files=("$@")
  if [[ ${#files[@]} -eq 0 ]]; then
    warn "未找到 SQL 文件，跳过 sqlfluff"
    SUMMARY+=("sqlfluff=skipped")
    return 0
  fi
  if ! command -v sqlfluff >/dev/null 2>&1; then
    local msg="sqlfluff 未安装，跳过语法/风格检查"
    if [[ "$STRICT" == true ]]; then
      fail "$msg（STRICT=true）"
      return 1
    fi
    warn "$msg"
    SUMMARY+=("sqlfluff=skipped")
    return 0
  fi
  log "运行 sqlfluff lint ..."
  if [[ -f "$SQLFLUFF_CONFIG" ]]; then
    SQLFLUFF_CFG="--config $SQLFLUFF_CONFIG"
  else
    SQLFLUFF_CFG=""
  fi
  sqlfluff lint $SQLFLUFF_CFG "${files[@]}"
  SUMMARY+=("sqlfluff=pass")
}

run_psql_check() {
  if [[ "$RUN_PSQL_CHECK" != true ]]; then
    SUMMARY+=("psql=skipped")
    return 0
  fi
  if ! command -v psql >/dev/null 2>&1; then
    local msg="psql 未安装，无法执行一致性检查"
    if [[ "$STRICT" == true ]]; then
      fail "$msg（STRICT=true）"
      return 1
    fi
    warn "$msg"
    SUMMARY+=("psql=skipped")
    return 0
  fi
  if [[ -z "${DATABASE_URL:-}" && -z "${PGHOST:-}" ]]; then
    local msg="未检测到数据库连接（缺少 DATABASE_URL 或 PGHOST），跳过 psql 检查"
    if [[ "$STRICT" == true ]]; then
      fail "$msg（STRICT=true）"
      return 1
    fi
    warn "$msg"
    SUMMARY+=("psql=skipped")
    return 0
  fi
  if [[ ! -f "$CHECK_SQL" ]]; then
    fail "一致性脚本不存在：$CHECK_SQL"
    return 1
  fi
  log "使用 psql 对 $CHECK_SQL 进行一次 dry-run"
  PSQL_CMD=(psql -v ON_ERROR_STOP=1)
  if [[ -n "${DATABASE_URL:-}" ]]; then
    PSQL_CMD+=("$DATABASE_URL")
  fi
  "${PSQL_CMD[@]}" <<SQL
BEGIN;
\\i $CHECK_SQL
ROLLBACK;
SQL
  SUMMARY+=("psql=pass")
}

report_summary() {
  log "检查完成：${SUMMARY[*]}"
  if [[ $FAILURES -gt 0 ]]; then
    log "共有 $FAILURES 项失败"
    exit 1
  fi
}

main() {
  log "SQL_DIR=$SQL_DIR"
  log "STRICT=$STRICT"
  mapfile -t SQL_FILES < <(list_sql_files)
  run_sqlfluff "${SQL_FILES[@]}"
  run_psql_check
  report_summary
}

main "$@"
