#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${DOCKER_DIR}/compose.prod.yaml"
ENV_FILE="${DOCKER_DIR}/.env"

ASSUME_YES="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes)
      ASSUME_YES="true"
      shift
      ;;
    *)
      echo "[docker-clean] unknown argument: $1" >&2
      echo "[docker-clean] usage: ./scripts/clean.sh [--yes]" >&2
      exit 1
      ;;
  esac
done

compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
    return
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    docker-compose "$@"
    return
  fi
  echo "[docker-clean] neither 'docker compose' nor 'docker-compose' is available" >&2
  exit 1
}

read_env_or_default() {
  local key="$1"
  local default_value="$2"
  local value=""
  if [[ -f "${ENV_FILE}" ]]; then
    value="$(grep -E "^${key}=" "${ENV_FILE}" | tail -n 1 | cut -d= -f2- || true)"
    value="${value%%#*}"
    value="$(echo "${value}" | xargs)"
  fi
  if [[ -n "${value}" ]]; then
    echo "${value}"
    return
  fi
  echo "${default_value}"
}

HOST_CONFIG_DIR="${POWERX_HOST_CONFIG_DIR:-$(read_env_or_default POWERX_HOST_CONFIG_DIR /etc/powerx)}"
HOST_DATA_DIR="${POWERX_HOST_DATA_DIR:-$(read_env_or_default POWERX_HOST_DATA_DIR /var/lib/powerx)}"

echo "[docker-clean] docker dir: ${DOCKER_DIR}"
echo "[docker-clean] compose file: ${COMPOSE_FILE}"
echo "[docker-clean] env file: ${ENV_FILE}"
echo "[docker-clean] will remove host config dir: ${HOST_CONFIG_DIR}"
echo "[docker-clean] will remove host data dir: ${HOST_DATA_DIR}"

if [[ "${ASSUME_YES}" != "true" ]]; then
  echo "[docker-clean] destructive operation, type YES/yes/y to continue:"
  read -r confirm
  confirm="$(echo "${confirm}" | tr '[:upper:]' '[:lower:]' | xargs)"
  if [[ "${confirm}" != "yes" && "${confirm}" != "y" ]]; then
    echo "[docker-clean] cancelled"
    exit 1
  fi
fi

if [[ -f "${COMPOSE_FILE}" ]]; then
  if [[ -f "${ENV_FILE}" ]]; then
    compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" down -v --remove-orphans || true
  else
    compose -f "${COMPOSE_FILE}" down -v --remove-orphans || true
  fi
fi

sudo rm -rf "${HOST_CONFIG_DIR}" "${HOST_DATA_DIR}"
sudo rm -f "${ENV_FILE}" "${ENV_FILE}.bak"

# Optional image cleanup: local build images + base deps used by this compose.
docker image rm -f \
  powerx-backend \
  powerx-web-admin \
  pgvector/pgvector:pg16 \
  redis:7-alpine \
  grafana/loki:2.9.8 \
  grafana/promtail:2.9.8 \
  grafana/grafana:10.4.5 >/dev/null 2>&1 || true

echo "[docker-clean] done"
