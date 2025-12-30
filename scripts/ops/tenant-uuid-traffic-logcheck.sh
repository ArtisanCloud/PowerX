#!/usr/bin/env bash
set -euo pipefail

# Scan ingress/app logs for legacy tenant headers.
# Usage:
#   scripts/ops/tenant-uuid-traffic-logcheck.sh --path /var/log/powerx/api.log

PATTERN='(X-Tenant-ID|X-PowerX-Tenant)'

usage() {
  cat <<'USAGE'
Usage: scripts/ops/tenant-uuid-traffic-logcheck.sh --path <logfile> [--summary-only]

Options:
  --path <file>       Path to log file or directory (required)
  --summary-only      Only print summary count, suppress matching lines
USAGE
}

LOG_PATH=""
SUMMARY_ONLY="false"

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --path)
        LOG_PATH="$2"; shift 2 ;;
      --summary-only)
        SUMMARY_ONLY="true"; shift ;;
      -h|--help)
        usage; exit 0 ;;
      *)
        echo "unknown option: $1" >&2
        usage; exit 1 ;;
    esac
  done
  if [[ -z "$LOG_PATH" ]]; then
    echo "--path is required" >&2
    usage
    exit 1
  fi
}

scan_file() {
  local target="$1"
  if [[ ! -e "$target" ]]; then
    echo "[warn] path not found: $target" >&2
    return
  fi
  if [[ -d "$target" ]]; then
    rg --color=never -n "$PATTERN" "$target" || true
  else
    rg --color=never -n "$PATTERN" "$target" || true
  fi
}

count_matches() {
  local target="$1"
  if [[ ! -e "$target" ]]; then
    echo 0
    return
  fi
  local counts
  counts=$(rg -c "$PATTERN" "$target" 2>/dev/null || true)
  if [[ -z "$counts" ]]; then
    echo 0
    return
  fi
  echo "$counts" | awk -F: '{sum+=$2} END {print sum+0}'
}

parse_args "$@"
count=$(count_matches "$LOG_PATH")
if [[ "$SUMMARY_ONLY" == "true" ]]; then
  echo "[summary] legacy header occurrences: $count"
else
  echo "[summary] legacy header occurrences: $count"
  if [[ "$count" -gt 0 ]]; then
    echo "[details] matching lines:"
    scan_file "$LOG_PATH"
  else
    echo "[details] no matches found"
  fi
fi
