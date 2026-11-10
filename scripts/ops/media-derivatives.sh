#!/bin/sh
# 参数与路径适配；清单、执行进度及所有业务逻辑均在 media_tool 内。
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
absolute_path() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$root" "$1" ;;
  esac
}
fail() { printf '%s\n' "$1" >&2; exit 2; }

[ -n "${MEDIA_MAINT_CONFIG:-}" ] || fail MEDIA_BACKFILL_CONFIG_REQUIRED
config=$(absolute_path "$MEDIA_MAINT_CONFIG")
set -- "${1:-}"
mode=$1
set -- derivatives-backfill --config "$config" --batch-size "${MEDIA_MAINT_BATCH:-50}" --concurrency "${MEDIA_MAINT_CONCURRENCY:-2}" --limit "${MEDIA_MAINT_LIMIT:-0}"
case "$mode" in
  scan)
    [ -n "${MEDIA_MAINT_REPORT:-}" ] || fail MEDIA_BACKFILL_REPORT_REQUIRED
    [ -z "${MEDIA_MAINT_PLAN:-}${MEDIA_MAINT_PROGRESS:-}" ] || fail MEDIA_BACKFILL_SCAN_SCOPE_INVALID
    [ "${MEDIA_MAINT_CONFIRM:-0}" = 0 ] || fail MEDIA_BACKFILL_SCAN_SCOPE_INVALID
    [ "${MEDIA_MAINT_RETRY:-0}" = 0 ] || fail MEDIA_BACKFILL_SCAN_SCOPE_INVALID
    set -- "$@" --dry-run=true --report "$(absolute_path "$MEDIA_MAINT_REPORT")" --after-id "${MEDIA_MAINT_AFTER:-0}" --through-id "${MEDIA_MAINT_THROUGH:-0}"
    if [ -n "${MEDIA_MAINT_TENANT:-}" ]; then set -- "$@" --tenant "$MEDIA_MAINT_TENANT"; fi
    if [ -n "${MEDIA_MAINT_ASSETS:-}" ]; then set -- "$@" --assets "$MEDIA_MAINT_ASSETS"; fi
    ;;
  apply)
    [ "${MEDIA_MAINT_CONFIRM:-0}" = 1 ] || fail MEDIA_BACKFILL_CONFIRM_REQUIRED
    [ -n "${MEDIA_MAINT_PLAN:-}" ] || fail MEDIA_BACKFILL_PLAN_REQUIRED
    [ -z "${MEDIA_MAINT_ASSETS:-}${MEDIA_MAINT_REPORT:-}" ] || fail MEDIA_BACKFILL_PLAN_SCOPE_OVERRIDE
    [ "${MEDIA_MAINT_AFTER:-0}" = 0 ] && [ "${MEDIA_MAINT_THROUGH:-0}" = 0 ] || fail MEDIA_BACKFILL_PLAN_SCOPE_OVERRIDE
    set -- "$@" --dry-run=false --confirm --plan "$(absolute_path "$MEDIA_MAINT_PLAN")"
    if [ -n "${MEDIA_MAINT_TENANT:-}" ]; then set -- "$@" --tenant "$MEDIA_MAINT_TENANT"; fi
    if [ -n "${MEDIA_MAINT_PROGRESS:-}" ]; then set -- "$@" --progress "$(absolute_path "$MEDIA_MAINT_PROGRESS")"; fi
    case "${MEDIA_MAINT_RETRY:-0}" in
      1) set -- "$@" --retry-failed=true ;;
      0) ;;
      *) fail MEDIA_BACKFILL_RETRY_FLAG_INVALID ;;
    esac
    ;;
  *) fail MEDIA_BACKFILL_MODE_INVALID ;;
esac
tool=${MEDIA_MAINT_TOOL:-}
if [ -n "$tool" ]; then tool=$(absolute_path "$tool"); fi
cd "$root/backend"
if [ -n "$tool" ]; then exec "$tool" "$@"; fi
exec "${MEDIA_MAINT_GO:-go}" run ./cmd/media_tool "$@"
