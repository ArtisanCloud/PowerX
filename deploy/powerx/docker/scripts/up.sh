#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${DOCKER_DIR}/compose.prod.yaml"
ENV_FILE="${DOCKER_DIR}/.env"
MODE="${POWERX_DOCKER_MODE:-auto}" # auto | full | infra

compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
    return
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    docker-compose "$@"
    return
  fi
  echo "[docker-up] neither 'docker compose' nor 'docker-compose' is available" >&2
  exit 1
}

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

if [[ "${MODE}" == "infra" ]]; then
  echo "[docker-up] mode=infra, start postgres+redis only"
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" pull postgres redis
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" up -d postgres redis
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" ps
  exit 0
fi

if [[ "${MODE}" == "full" ]]; then
  echo "[docker-up] mode=full, pull infra images + build local app images"
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" pull postgres redis loki promtail grafana
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" build backend web-admin
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" up -d
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" ps
  exit 0
fi

echo "[docker-up] mode=auto -> full (local build)"
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" pull postgres redis loki promtail grafana
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" build backend web-admin
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" up -d
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" ps
