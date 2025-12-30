#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
ENV_FILE=""
FAILURES=0
SHOW_ALL=false

usage() {
  cat <<'USAGE'
Usage: scripts/ops/env-audit.sh [--env-file path] [--show-all]
说明：
  - 默认读取当前 shell 的环境变量；如提供 --env-file，会先 source 该文件再检测。
  - 检查重点：PX_HEADER_UUID_ONLY / PX_ALLOW_TENANT_ID_HEADER / PX_TENANT_COMPAT_MODE 等。
USAGE
}

log() {
  printf '[env-audit] %s\n' "$*"
}

warn() {
  printf '[env-audit][warn] %s\n' "$*" >&2
}

fail() {
  printf '[env-audit][fail] %s\n' "$*" >&2
  FAILURES=$((FAILURES + 1))
}

normalize_bool() {
  local val="${1:-}"
  val="${val,,}"
  case "$val" in
    true|1|yes|y|on) echo "true" ;;
    false|0|no|n|off|"") echo "false" ;;
    *) echo "$val" ;;
  esac
}

source_env_file() {
  local file="$1"
  if [[ ! -f "$file" ]]; then
    fail "env file not found: $file"
    return
  fi
  # shellcheck disable=SC1090
  set -a
  source "$file"
  set +a
  log "loaded env from $file"
}

check_true() {
  local var="$1"
  local raw="${!var-}"
  local norm
  norm=$(normalize_bool "$raw")
  if [[ "$norm" != "true" ]]; then
    fail "$var expected true but got '${raw:-<unset>}'"
  else
    log "$var = $raw ✅"
  fi
}

check_false_or_unset() {
  local var="$1"
  if [[ -z "${!var+x}" ]]; then
    log "$var is unset ✅"
    return
  fi
  local norm
  norm=$(normalize_bool "${!var}")
  if [[ "$norm" == "false" ]]; then
    log "$var=${!var} ✅"
  else
    fail "$var should be false/unset but got '${!var}'"
  fi
}

check_nonempty() {
  local var="$1"
  if [[ -z "${!var:-}" ]]; then
    warn "$var is empty"
  else
    log "$var=${!var}"
  fi
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --env-file)
        ENV_FILE=${2:-}
        shift 2
        ;;
      --env-file=*)
        ENV_FILE=${1#*=}
        shift 1
        ;;
      --show-all)
        SHOW_ALL=true
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        usage
        exit 1
        ;;
    esac
  done
}

main() {
  parse_args "$@"
  if [[ -n "$ENV_FILE" ]]; then
    source_env_file "$ENV_FILE"
  fi

  check_true "PX_HEADER_UUID_ONLY"
  check_false_or_unset "PX_ALLOW_TENANT_ID_HEADER"
  check_false_or_unset "PX_TENANT_COMPAT_MODE"
  check_false_or_unset "PX_ALLOW_TENANT_ID_LEGACY"
  check_nonempty "TENANT_TABLE"

  if [[ "$SHOW_ALL" == true ]]; then
    log "PX_HEADER_UUID_ONLY=${PX_HEADER_UUID_ONLY:-}"
    log "PX_ALLOW_TENANT_ID_HEADER=${PX_ALLOW_TENANT_ID_HEADER:-<unset>}"
    log "PX_TENANT_COMPAT_MODE=${PX_TENANT_COMPAT_MODE:-<unset>}"
    log "PX_ALLOW_TENANT_ID_LEGACY=${PX_ALLOW_TENANT_ID_LEGACY:-<unset>}"
    log "TENANT_TABLE=${TENANT_TABLE:-}"
  fi

  if [[ $FAILURES -gt 0 ]]; then
    log "completed with $FAILURES violations"
    exit 1
  fi
  log "env audit passed"
}

main "$@"
