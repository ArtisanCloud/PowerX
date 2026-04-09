#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${DOCKER_DIR}/compose.prod.yaml"
ENV_FILE="${DOCKER_DIR}/.env"

if [[ ! -f "${COMPOSE_FILE}" ]]; then
  echo "[docker-up] missing compose file: ${COMPOSE_FILE}" >&2
  exit 1
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "[docker-up] missing env file: ${ENV_FILE}" >&2
  echo "[docker-up] run: ${DOCKER_DIR}/scripts/bootstrap-host.sh" >&2
  exit 1
fi

cd "${DOCKER_DIR}"
docker compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" pull
docker compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" up -d
docker compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" ps
