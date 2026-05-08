#!/usr/bin/env bash
set -euo pipefail

# Loki maintenance helper for systemd deployment.
# Usage:
#   scripts/observability/loki-maintenance.sh status
#   scripts/observability/loki-maintenance.sh apply-config [SOURCE_CONFIG]
#   scripts/observability/loki-maintenance.sh purge --yes

LOKI_CONFIG_DEFAULT="/etc/loki/config.yml"
LOKI_DATA_DIR_DEFAULT="/var/loki"
SOURCE_CONFIG_DEFAULT="$(pwd)/deploy/observability/loki/loki-config.yaml"

log() {
  printf '[loki-maintenance] %s\n' "$*"
}

die() {
  printf '[loki-maintenance][ERROR] %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

status_cmd() {
  log "service status"
  sudo systemctl status loki --no-pager -l || true
  echo
  log "ready endpoint"
  curl -sS http://127.0.0.1:3100/ready || true
  echo
  log "effective retention config"
  sudo grep -nE 'retention_period|retention_enabled|path_prefix|working_directory' "$LOKI_CONFIG_DEFAULT" || true
}

apply_config_cmd() {
  local src="${1:-$SOURCE_CONFIG_DEFAULT}"
  [[ -f "$src" ]] || die "source config not found: $src"

  log "backup current config"
  local bak="${LOKI_CONFIG_DEFAULT}.bak.$(date +%F-%H%M%S)"
  sudo cp "$LOKI_CONFIG_DEFAULT" "$bak" || true
  log "backup -> $bak"

  log "apply config: $src -> $LOKI_CONFIG_DEFAULT"
  sudo cp "$src" "$LOKI_CONFIG_DEFAULT"

  log "restart loki"
  sudo systemctl restart loki

  log "wait for ready"
  sleep 1
  curl -sS http://127.0.0.1:3100/ready || true
}

purge_cmd() {
  local confirm="${1:-}"
  [[ "$confirm" == "--yes" ]] || die "purge is destructive; rerun with: purge --yes"

  log "stop loki"
  sudo systemctl stop loki

  log "remove old data dirs"
  sudo rm -rf \
    "$LOKI_DATA_DIR_DEFAULT/chunks" \
    "$LOKI_DATA_DIR_DEFAULT/index" \
    "$LOKI_DATA_DIR_DEFAULT/rules" \
    "$LOKI_DATA_DIR_DEFAULT/compactor" \
    "$LOKI_DATA_DIR_DEFAULT/wal"

  log "recreate required dirs"
  sudo mkdir -p \
    "$LOKI_DATA_DIR_DEFAULT/chunks" \
    "$LOKI_DATA_DIR_DEFAULT/rules" \
    "$LOKI_DATA_DIR_DEFAULT/compactor"

  log "fix ownership and permissions"
  # Some environments have no loki group; use user ownership only to avoid chown failure.
  sudo chown -R loki "$LOKI_DATA_DIR_DEFAULT"
  sudo chmod -R 755 "$LOKI_DATA_DIR_DEFAULT"

  log "start loki"
  sudo systemctl start loki

  log "ready endpoint"
  sleep 1
  curl -sS http://127.0.0.1:3100/ready || true
}

main() {
  need_cmd sudo
  need_cmd systemctl
  need_cmd curl

  local sub="${1:-}"
  case "$sub" in
    status)
      status_cmd
      ;;
    apply-config)
      shift || true
      apply_config_cmd "${1:-}"
      ;;
    purge)
      shift || true
      purge_cmd "${1:-}"
      ;;
    *)
      cat <<USAGE
Usage:
  $0 status
  $0 apply-config [SOURCE_CONFIG]
  $0 purge --yes

Examples:
  $0 status
  $0 apply-config
  $0 apply-config /opt/powerx/backend/deploy/observability/loki/loki-config.yaml
  $0 purge --yes
USAGE
      exit 1
      ;;
  esac
}

main "$@"
