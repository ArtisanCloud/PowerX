#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Usage:
  $0 <target_tag_or_version> [switch-release-systemd options]

Description:
  Switch the isolated PowerX develop/dev systemd environment.
  This is a thin wrapper around switch-release-systemd.sh with dev defaults:
    - /opt/powerx-dev
    - /etc/powerx-dev
    - powerx-dev-backend / powerx-dev-web-admin / powerx-dev-runner
    - health check on 127.0.0.1:8081
    - no release systemd unit sync

Environment overrides:
  POWERX_DEV_BACKEND_PORT  Default: 8081
  POWERX_RELEASES_ROOT     Default: /opt/powerx-dev/releases
  POWERX_LINKS_ROOT        Default: /opt/powerx-dev
  POWERX_RUNTIME_ROOT      Default: /etc/powerx-dev
  POWERX_STORAGE_ROOT      Default: /opt/powerx-dev/storage
  POWERX_PLUGIN_RUNTIME_ROOT
                            Default: /opt/powerx-dev/plugins
  POWERX_HEALTH_URL        Default: http://127.0.0.1:\${POWERX_DEV_BACKEND_PORT}/api/v1/health
  POWERX_BACKEND_SERVICE   Default: powerx-dev-backend
  POWERX_WEB_ADMIN_SERVICE Default: powerx-dev-web-admin
  POWERX_RUNNER_SERVICE    Default: powerx-dev-runner
  POWERX_SYNC_SYSTEMD_UNITS
                            Default: 0
USAGE
}

if [[ $# -lt 1 ]]; then
  usage >&2
  exit 1
fi

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEV_BACKEND_PORT="${POWERX_DEV_BACKEND_PORT:-8081}"

export POWERX_RELEASES_ROOT="${POWERX_RELEASES_ROOT:-/opt/powerx-dev/releases}"
export POWERX_LINKS_ROOT="${POWERX_LINKS_ROOT:-/opt/powerx-dev}"
export POWERX_RUNTIME_ROOT="${POWERX_RUNTIME_ROOT:-/etc/powerx-dev}"
export POWERX_STORAGE_ROOT="${POWERX_STORAGE_ROOT:-/opt/powerx-dev/storage}"
export POWERX_PLUGIN_RUNTIME_ROOT="${POWERX_PLUGIN_RUNTIME_ROOT:-/opt/powerx-dev/plugins}"
export POWERX_HEALTH_URL="${POWERX_HEALTH_URL:-http://127.0.0.1:${DEV_BACKEND_PORT}/api/v1/health}"
export POWERX_BACKEND_SERVICE="${POWERX_BACKEND_SERVICE:-powerx-dev-backend}"
export POWERX_WEB_ADMIN_SERVICE="${POWERX_WEB_ADMIN_SERVICE:-powerx-dev-web-admin}"
export POWERX_RUNNER_SERVICE="${POWERX_RUNNER_SERVICE:-powerx-dev-runner}"
export POWERX_SYNC_SYSTEMD_UNITS="${POWERX_SYNC_SYSTEMD_UNITS:-0}"

exec "${SCRIPT_DIR}/switch-release-systemd.sh" "$@"
