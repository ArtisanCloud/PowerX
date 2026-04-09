#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${DOCKER_DIR}/../../.." && pwd)"

HOST_CONFIG_DIR="${POWERX_HOST_CONFIG_DIR:-/etc/powerx}"
HOST_DATA_DIR="${POWERX_HOST_DATA_DIR:-/var/lib/powerx}"
ENV_FILE="${DOCKER_DIR}/.env"
ENV_EXAMPLE="${DOCKER_DIR}/.env.prod.example"
IMAGE_TAG=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image-tag)
      IMAGE_TAG="${2:-}"
      shift 2
      ;;
    *)
      echo "[docker-bootstrap] unknown argument: $1" >&2
      echo "[docker-bootstrap] usage: ./scripts/bootstrap-host.sh [--image-tag <tag>]" >&2
      exit 1
      ;;
  esac
done

echo "[docker-bootstrap] docker dir: ${DOCKER_DIR}"
echo "[docker-bootstrap] repo root: ${REPO_ROOT}"
echo "[docker-bootstrap] host config dir: ${HOST_CONFIG_DIR}"
echo "[docker-bootstrap] host data dir: ${HOST_DATA_DIR}"

if [[ ! -f "${REPO_ROOT}/backend/go.mod" ]]; then
  echo "[docker-bootstrap] missing backend/go.mod under repo root: ${REPO_ROOT}" >&2
  exit 1
fi
if [[ ! -f "${REPO_ROOT}/backend/go.sum" ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "[docker-bootstrap] backend/go.sum missing and 'go' not found" >&2
    echo "[docker-bootstrap] install Go, then run: cd ${REPO_ROOT}/backend && go mod tidy" >&2
    exit 1
  fi
  echo "[docker-bootstrap] backend/go.sum missing, run go mod tidy"
  (
    cd "${REPO_ROOT}/backend"
    go mod tidy
  )
fi

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

set_env_key() {
  local key="$1"
  local value="$2"
  if grep -qE "^${key}=" "${ENV_FILE}"; then
    sed -i.bak -E "s|^${key}=.*|${key}=${value}|" "${ENV_FILE}"
  else
    echo "${key}=${value}" >> "${ENV_FILE}"
  fi
}

get_env_key() {
  local key="$1"
  local value
  value="$(grep -E "^${key}=" "${ENV_FILE}" | tail -n 1 | cut -d= -f2- || true)"
  value="${value%%#*}"
  echo "$(echo "${value}" | xargs)"
}

is_valid_tag() {
  local value="$1"
  [[ -n "${value}" && "${value}" != "CHANGE_ME" ]]
}

infer_image_tag() {
  local candidate=""
  if is_valid_tag "${IMAGE_TAG}"; then
    echo "${IMAGE_TAG}"
    return
  fi
  if is_valid_tag "${POWERX_IMAGE_TAG:-}"; then
    echo "${POWERX_IMAGE_TAG}"
    return
  fi
  if is_valid_tag "${POWERX_VERSION:-}"; then
    echo "${POWERX_VERSION}"
    return
  fi

  candidate="$(get_env_key "POWERX_IMAGE_TAG")"
  if is_valid_tag "${candidate}"; then
    echo "${candidate}"
    return
  fi

  local repo_dir git_tag=""
  repo_dir="$(cd "${DOCKER_DIR}/../../.." && pwd)"
  if command -v git >/dev/null 2>&1 && git -C "${repo_dir}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git_tag="$(git -C "${repo_dir}" describe --tags --abbrev=0 2>/dev/null || true)"
    if is_valid_tag "${git_tag}"; then
      echo "${git_tag}"
      return
    fi
  fi

  echo "local"
}

RESOLVED_IMAGE_TAG="$(infer_image_tag)"
set_env_key "POWERX_IMAGE_TAG" "${RESOLVED_IMAGE_TAG}"
echo "[docker-bootstrap] resolved image tag: ${RESOLVED_IMAGE_TAG}"

sudo mkdir -p "${HOST_CONFIG_DIR}"
sudo mkdir -p "${HOST_DATA_DIR}/postgres" "${HOST_DATA_DIR}/redis" "${HOST_DATA_DIR}/uploads" "${HOST_DATA_DIR}/loki" "${HOST_DATA_DIR}/promtail" "${HOST_DATA_DIR}/grafana"
sudo chown -R 999:999 "${HOST_DATA_DIR}/postgres" "${HOST_DATA_DIR}/redis"
sudo chown -R 472:472 "${HOST_DATA_DIR}/grafana"

echo "[docker-bootstrap] done"
echo "[docker-bootstrap] next:"
echo "  cd ${DOCKER_DIR}"
echo "  ./scripts/up.sh"
