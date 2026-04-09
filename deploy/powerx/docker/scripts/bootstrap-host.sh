#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

HOST_CONFIG_DIR="${POWERX_HOST_CONFIG_DIR:-/etc/powerx}"
HOST_DATA_DIR="${POWERX_HOST_DATA_DIR:-/var/lib/powerx}"
ENV_FILE="${DOCKER_DIR}/.env"
ENV_EXAMPLE="${DOCKER_DIR}/.env.prod.example"

echo "[docker-bootstrap] docker dir: ${DOCKER_DIR}"
echo "[docker-bootstrap] host config dir: ${HOST_CONFIG_DIR}"
echo "[docker-bootstrap] host data dir: ${HOST_DATA_DIR}"

if [[ ! -f "${ENV_EXAMPLE}" ]]; then
  echo "[docker-bootstrap] missing env example: ${ENV_EXAMPLE}" >&2
  exit 1
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  cp "${ENV_EXAMPLE}" "${ENV_FILE}"
  echo "[docker-bootstrap] created ${ENV_FILE} from ${ENV_EXAMPLE}"
else
  echo "[docker-bootstrap] keep existing ${ENV_FILE}"
fi

sudo mkdir -p "${HOST_CONFIG_DIR}"
sudo mkdir -p "${HOST_DATA_DIR}/postgres" "${HOST_DATA_DIR}/redis" "${HOST_DATA_DIR}/uploads"
sudo chown -R 999:999 "${HOST_DATA_DIR}/postgres" "${HOST_DATA_DIR}/redis"

echo "[docker-bootstrap] done"
echo "[docker-bootstrap] next:"
echo "  cd ${DOCKER_DIR}"
echo "  ./scripts/up.sh"
