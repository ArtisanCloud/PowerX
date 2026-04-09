#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${DOCKER_DIR}/compose.prod.yaml"
ENV_FILE="${DOCKER_DIR}/.env"

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

check_required_tag() {
  local key="$1"
  local value
  value="$(grep -E "^${key}=" "${ENV_FILE}" | tail -n 1 | cut -d= -f2- || true)"
  value="${value%%#*}"
  value="$(echo "${value}" | xargs)"
  if [[ -z "${value}" || "${value}" == "latest" || "${value}" == "CHANGE_ME" ]]; then
    echo "[docker-up] invalid ${key}='${value:-<empty>}' in ${ENV_FILE}" >&2
    echo "[docker-up] please set a real image tag before startup, e.g. ${key}=v2.0.1" >&2
    exit 1
  fi
}

check_required_tag "POWERX_BACKEND_TAG"
check_required_tag "POWERX_RUNNER_TAG"
check_required_tag "POWERX_WEB_ADMIN_TAG"

cd "${DOCKER_DIR}"
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" pull
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" up -d
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" ps
