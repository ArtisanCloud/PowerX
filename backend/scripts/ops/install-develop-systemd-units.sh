#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Usage:
  $0 [--with-runner|--without-runner]

Description:
  Install or refresh isolated PowerX develop/dev systemd unit files.
  This script only writes powerx-dev-* units. It does not switch releases.

Environment overrides:
  POWERX_DEV_RUNTIME_ROOT  Default: /etc/powerx-dev
  POWERX_DEV_LINKS_ROOT    Default: /opt/powerx-dev
  POWERX_DEV_BACKEND_SERVICE
                            Default: powerx-dev-backend
  POWERX_DEV_WEB_ADMIN_SERVICE
                            Default: powerx-dev-web-admin
  POWERX_DEV_RUNNER_SERVICE
                            Default: powerx-dev-runner
  POWERX_SERVICE_USER      Default: powerx
  POWERX_SERVICE_GROUP     Default: powerx
USAGE
}

WITH_RUNNER=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    --with-runner)
      WITH_RUNNER=1
      shift
      ;;
    --without-runner)
      WITH_RUNNER=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[install-dev-units] unknown arg: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ "${EUID}" -ne 0 ]]; then
  echo "[install-dev-units] this script must run as root (use sudo)" >&2
  exit 1
fi

RUNTIME_ROOT="${POWERX_DEV_RUNTIME_ROOT:-/etc/powerx-dev}"
LINKS_ROOT="${POWERX_DEV_LINKS_ROOT:-/opt/powerx-dev}"
BACKEND_SERVICE="${POWERX_DEV_BACKEND_SERVICE:-powerx-dev-backend}"
WEB_ADMIN_SERVICE="${POWERX_DEV_WEB_ADMIN_SERVICE:-powerx-dev-web-admin}"
RUNNER_SERVICE="${POWERX_DEV_RUNNER_SERVICE:-powerx-dev-runner}"
SERVICE_USER="${POWERX_SERVICE_USER:-powerx}"
SERVICE_GROUP="${POWERX_SERVICE_GROUP:-powerx}"
ENV_FILE="${RUNTIME_ROOT}/powerx.env"

install -d -m 0755 "${RUNTIME_ROOT}" "${LINKS_ROOT}"
if [[ ! -f "${ENV_FILE}" ]]; then
  touch "${ENV_FILE}"
fi
chown root:root "${ENV_FILE}"
chmod 0644 "${ENV_FILE}"

cat > "/etc/systemd/system/${BACKEND_SERVICE}.service" <<EOF
[Unit]
Description=PowerX Dev Backend Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
WorkingDirectory=/
EnvironmentFile=-${ENV_FILE}
ExecStart=/bin/sh -c 'ROOT=\${POWERX_LINKS_ROOT:-${LINKS_ROOT}}; exec "\$ROOT/backend/powerx"'
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

cat > "/etc/systemd/system/${WEB_ADMIN_SERVICE}.service" <<EOF
[Unit]
Description=PowerX Dev Web Admin Service
After=${BACKEND_SERVICE}.service
Requires=${BACKEND_SERVICE}.service

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
WorkingDirectory=/
EnvironmentFile=-${ENV_FILE}
ExecStart=/bin/sh -c 'ROOT=\${POWERX_RUNTIME_ROOT:-${RUNTIME_ROOT}}; CFG=\${POWERX_CONFIG:-\$ROOT/config.yaml}; [ -f "\$CFG" ] || CFG=\${POWERX_LINKS_ROOT:-${LINKS_ROOT}}/backend/etc/config.yaml; BPORT=\$(awk "/^[[:space:]]*server:[[:space:]]*/{in_server=1; next} in_server && /^[[:space:]]*port:[[:space:]]*[0-9]+/{print; exit} in_server && /^[^[:space:]]/{in_server=0}" "\$CFG" | tr -cd "0-9"); WPORT=\$(awk "/^[[:space:]]*web_admin_port:[[:space:]]*[0-9]+/{print; exit}" "\$CFG" | tr -cd "0-9"); [ -z "\$BPORT" ] && BPORT=8081; [ -z "\$WPORT" ] && WPORT=3001; export POWERX_BACKEND=http://127.0.0.1:\$BPORT; export NUXT_PUBLIC_POWERX_CORE_BASE=http://127.0.0.1:\$BPORT; export NUXT_PUBLIC_WS_ORIGIN=ws://127.0.0.1:\$BPORT; export NUXT_PUBLIC_WS_PATH=/api/ws; PORT=\$WPORT exec "\${NODE_BIN:-/usr/bin/node}" \${POWERX_LINKS_ROOT:-${LINKS_ROOT}}/web-admin/.output/server/index.mjs'
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

if [[ "${WITH_RUNNER}" == "1" ]]; then
  cat > "/etc/systemd/system/${RUNNER_SERVICE}.service" <<EOF
[Unit]
Description=PowerX Dev Runner Service
After=${BACKEND_SERVICE}.service
Requires=${BACKEND_SERVICE}.service

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
WorkingDirectory=/
EnvironmentFile=-${ENV_FILE}
ExecCondition=/bin/sh -c 'test -f "\${POWERX_LINKS_ROOT:-${LINKS_ROOT}}/runner/dist/main.js"'
ExecStart=/bin/sh -c 'ROOT=\${POWERX_LINKS_ROOT:-${LINKS_ROOT}}; exec "\${NODE_BIN:-/usr/bin/node}" "\$ROOT/runner/dist/main.js"'
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
fi

systemctl daemon-reload
if [[ "${WITH_RUNNER}" == "1" ]]; then
  systemctl enable "${BACKEND_SERVICE}" "${WEB_ADMIN_SERVICE}" "${RUNNER_SERVICE}"
else
  systemctl enable "${BACKEND_SERVICE}" "${WEB_ADMIN_SERVICE}"
fi

echo "[install-dev-units] installed: ${BACKEND_SERVICE}.service ${WEB_ADMIN_SERVICE}.service"
if [[ "${WITH_RUNNER}" == "1" ]]; then
  echo "[install-dev-units] installed: ${RUNNER_SERVICE}.service"
fi
echo "[install-dev-units] env file: ${ENV_FILE}"
