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

compose_pull_with_retry() {
  local max_retries="${1:-3}"
  shift
  local attempt=1
  local delay=2
  while true; do
    if compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" pull "$@"; then
      return 0
    fi
    if [[ "${attempt}" -ge "${max_retries}" ]]; then
      echo "[docker-up] pull failed after ${attempt} attempts" >&2
      return 1
    fi
    echo "[docker-up] pull attempt ${attempt} failed, retry in ${delay}s..." >&2
    sleep "${delay}"
    attempt=$((attempt + 1))
    delay=$((delay * 2))
  done
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
  compose_pull_with_retry 3 postgres redis
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" up -d postgres redis
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" ps
  exit 0
fi

if [[ "${MODE}" == "full" ]]; then
  echo "[docker-up] mode=full, pull infra images + build local app images"
  compose_pull_with_retry 3 postgres redis loki promtail grafana
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" build backend web-admin
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" up -d
  compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" ps
  exit 0
fi

echo "[docker-up] mode=auto -> full (local build)"
compose_pull_with_retry 3 postgres redis loki promtail grafana
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" build backend web-admin
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" up -d
compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" ps
