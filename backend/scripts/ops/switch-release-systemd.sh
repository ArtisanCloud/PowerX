#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Usage:
  $0 <target_tag_or_version> [--with-runner] [--timeout-sec N]

Description:
  Switch /opt/powerx symlinks to /opt/powerx/releases/<target_tag_or_version>,
  restart systemd services, and run backend health check.
  If health check fails, auto rollback to previous symlink targets.
  Production should use immutable git tag names (e.g. v2.0.2).

Options:
  --with-runner            Also switch/restart powerx-runner.
  --timeout-sec N          Health check timeout seconds (default: 90).

Environment overrides:
  POWERX_RELEASES_ROOT     Default: /opt/powerx/releases
  POWERX_LINKS_ROOT        Default: /opt/powerx
  POWERX_HEALTH_URL        Default: http://127.0.0.1:8080/api/v1/health
  POWERX_HEALTH_EXPECT     Default: 200
USAGE
}

if [[ $# -lt 1 ]]; then
  usage >&2
  exit 1
fi

TARGET_REF="$1"
shift

WITH_RUNNER=0
TIMEOUT_SEC=90
while [[ $# -gt 0 ]]; do
  case "$1" in
    --with-runner)
      WITH_RUNNER=1
      shift
      ;;
    --timeout-sec)
      TIMEOUT_SEC="${2:-}"
      if [[ -z "$TIMEOUT_SEC" ]]; then
        echo "[switch-release] --timeout-sec requires a value" >&2
        exit 1
      fi
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[switch-release] unknown arg: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

RELEASES_ROOT="${POWERX_RELEASES_ROOT:-/opt/powerx/releases}"
LINKS_ROOT="${POWERX_LINKS_ROOT:-/opt/powerx}"
HEALTH_URL="${POWERX_HEALTH_URL:-http://127.0.0.1:8080/api/v1/health}"
HEALTH_EXPECT="${POWERX_HEALTH_EXPECT:-200}"
SERVICE_USER="${POWERX_SERVICE_USER:-powerx}"
SERVICE_GROUP="${POWERX_SERVICE_GROUP:-powerx}"

TARGET_ROOT="${RELEASES_ROOT}/${TARGET_REF}"
TARGET_BACKEND="${TARGET_ROOT}/backend"
TARGET_WEB_ADMIN="${TARGET_ROOT}/web-admin"
TARGET_RUNNER="${TARGET_ROOT}/runner"

LINK_BACKEND="${LINKS_ROOT}/backend"
LINK_WEB_ADMIN="${LINKS_ROOT}/web-admin"
LINK_RUNNER="${LINKS_ROOT}/runner"

if [[ "${EUID}" -ne 0 ]]; then
  echo "[switch-release] this script must run as root (use sudo)" >&2
  exit 1
fi

if [[ ! -d "$TARGET_BACKEND" ]]; then
  echo "[switch-release] missing target backend dir: $TARGET_BACKEND" >&2
  exit 1
fi
if [[ ! -d "$TARGET_WEB_ADMIN" ]]; then
  echo "[switch-release] missing target web-admin dir: $TARGET_WEB_ADMIN" >&2
  exit 1
fi
if [[ "$WITH_RUNNER" == "1" && ! -d "$TARGET_RUNNER" ]]; then
  echo "[switch-release] missing target runner dir: $TARGET_RUNNER" >&2
  exit 1
fi

PREV_BACKEND=""
PREV_WEB_ADMIN=""
PREV_RUNNER=""

if [[ -L "$LINK_BACKEND" ]]; then
  PREV_BACKEND="$(readlink "$LINK_BACKEND")"
fi
if [[ -L "$LINK_WEB_ADMIN" ]]; then
  PREV_WEB_ADMIN="$(readlink "$LINK_WEB_ADMIN")"
fi
if [[ "$WITH_RUNNER" == "1" && -L "$LINK_RUNNER" ]]; then
  PREV_RUNNER="$(readlink "$LINK_RUNNER")"
fi

echo "[switch-release] target ref: $TARGET_REF"
echo "[switch-release] releases root: $RELEASES_ROOT"
echo "[switch-release] links root: $LINKS_ROOT"

ensure_service_identity() {
  if ! getent group "${SERVICE_GROUP}" >/dev/null; then
    echo "[switch-release] create group: ${SERVICE_GROUP}"
    groupadd --system "${SERVICE_GROUP}"
  fi

  if ! id -u "${SERVICE_USER}" >/dev/null 2>&1; then
    echo "[switch-release] create user: ${SERVICE_USER}"
    useradd --system \
      --gid "${SERVICE_GROUP}" \
      --home "${LINKS_ROOT}" \
      --shell /usr/sbin/nologin \
      "${SERVICE_USER}"
  fi

  install -d -m 0755 "${LINKS_ROOT}" "${RELEASES_ROOT}"
  chown -R "${SERVICE_USER}:${SERVICE_GROUP}" "${TARGET_ROOT}"
  chown -h "${SERVICE_USER}:${SERVICE_GROUP}" "${LINK_BACKEND}" "${LINK_WEB_ADMIN}" 2>/dev/null || true
  if [[ "$WITH_RUNNER" == "1" ]]; then
    chown -h "${SERVICE_USER}:${SERVICE_GROUP}" "${LINK_RUNNER}" 2>/dev/null || true
  fi
}

rollback() {
  echo "[switch-release] rollback start"
  if [[ -n "$PREV_BACKEND" ]]; then
    ln -sfn "$PREV_BACKEND" "$LINK_BACKEND"
  fi
  if [[ -n "$PREV_WEB_ADMIN" ]]; then
    ln -sfn "$PREV_WEB_ADMIN" "$LINK_WEB_ADMIN"
  fi
  if [[ "$WITH_RUNNER" == "1" && -n "$PREV_RUNNER" ]]; then
    ln -sfn "$PREV_RUNNER" "$LINK_RUNNER"
  fi

  systemctl daemon-reload
  systemctl restart powerx-backend powerx-web-admin
  if [[ "$WITH_RUNNER" == "1" ]]; then
    systemctl restart powerx-runner
  fi
  echo "[switch-release] rollback done"
}

trap 'echo "[switch-release] error occurred" >&2; rollback' ERR

ln -sfn "$TARGET_BACKEND" "$LINK_BACKEND"
ln -sfn "$TARGET_WEB_ADMIN" "$LINK_WEB_ADMIN"
if [[ "$WITH_RUNNER" == "1" ]]; then
  ln -sfn "$TARGET_RUNNER" "$LINK_RUNNER"
fi

ensure_service_identity

systemctl daemon-reload
if [[ "$WITH_RUNNER" == "1" ]]; then
  systemctl enable powerx-backend powerx-web-admin powerx-runner
else
  systemctl enable powerx-backend powerx-web-admin
fi
systemctl restart powerx-backend powerx-web-admin
if [[ "$WITH_RUNNER" == "1" ]]; then
  systemctl restart powerx-runner
fi

START_TS="$(date +%s)"
while true; do
  CODE="$(curl -sS -o /tmp/powerx-switch-health.json -w '%{http_code}' "$HEALTH_URL" || true)"
  if [[ "$CODE" == "$HEALTH_EXPECT" ]]; then
    break
  fi
  NOW_TS="$(date +%s)"
  if (( NOW_TS - START_TS >= TIMEOUT_SEC )); then
    echo "[switch-release] health check timeout, url=$HEALTH_URL code=$CODE expect=$HEALTH_EXPECT" >&2
    exit 1
  fi
  sleep 2
done

trap - ERR

echo "[switch-release] success: switched to ${TARGET_REF}"
systemctl --no-pager --full status powerx-backend powerx-web-admin | sed -n '1,40p'
if [[ "$WITH_RUNNER" == "1" ]]; then
  systemctl --no-pager --full status powerx-runner | sed -n '1,20p'
fi
