#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="powerx-backend.service"
DEFAULT_ROOT="${POWERX_LINKS_ROOT:-/opt/powerx}"

log() {
  printf '[powerx-precheck] %s\n' "$*"
}

die() {
  printf '[powerx-precheck][ERROR] %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

get_prop() {
  local key="$1"
  systemctl show "$SERVICE_NAME" -p "$key" --value
}

ensure_dir() {
  local dir="$1"
  [[ -n "$dir" ]] || return 0
  if [[ ! -d "$dir" ]]; then
    log "directory missing, creating: $dir"
    sudo mkdir -p "$dir"
  fi
}

main() {
  need_cmd systemctl
  need_cmd sudo

  local wd exec_start
  wd="$(get_prop WorkingDirectory | xargs)"
  exec_start="$(get_prop ExecStart | xargs)"

  log "service: $SERVICE_NAME"
  log "WorkingDirectory=$wd"
  log "ExecStart=$exec_start"

  [[ -n "$wd" ]] || die "WorkingDirectory is empty"
  ensure_dir "$wd"

  local backend_bin="${DEFAULT_ROOT}/backend/powerx"
  local backend_dir="${DEFAULT_ROOT}/backend"
  local releases_dir="${DEFAULT_ROOT}/releases"

  ensure_dir "$DEFAULT_ROOT"
  ensure_dir "$releases_dir"
  ensure_dir "$backend_dir"

  if [[ ! -e "$backend_bin" ]]; then
    log "backend binary not found at: $backend_bin"
    log "this may be expected before first deploy/switch; verify release symlink targets"
  else
    log "backend binary exists: $backend_bin"
  fi

  log "precheck passed"
}

main "$@"
