#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROXY_URL="${DOCKER_PROXY_URL:-${HTTPS_PROXY:-${https_proxy:-}}}"
NO_PROXY_VALUE="${DOCKER_NO_PROXY:-localhost,127.0.0.1,::1}"
DOCKER_MODE="${POWERX_DOCKER_MODE:-auto}" # auto | full | infra

echo "[docker-install] step 1/3: clean broken docker/env/data"
"${SCRIPT_DIR}/clean.sh" --yes

if [[ -n "${PROXY_URL}" ]]; then
  echo "[docker-install] setup docker daemon proxy: ${PROXY_URL}"
  sudo mkdir -p /etc/systemd/system/docker.service.d
  cat <<EOF | sudo tee /etc/systemd/system/docker.service.d/http-proxy.conf >/dev/null
[Service]
Environment="HTTP_PROXY=${PROXY_URL}"
Environment="HTTPS_PROXY=${PROXY_URL}"
Environment="NO_PROXY=${NO_PROXY_VALUE}"
EOF
  sudo systemctl daemon-reload
  sudo systemctl restart docker
fi

echo "[docker-install] mode=${DOCKER_MODE}, ghcr login not required (local source build)"

echo "[docker-install] step 2/3: bootstrap host dirs/env/tags"
"${SCRIPT_DIR}/bootstrap-host.sh"

echo "[docker-install] step 3/3: pull and start services"
"${SCRIPT_DIR}/up.sh"

echo "[docker-install] done"
