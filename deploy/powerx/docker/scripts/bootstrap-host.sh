#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

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
  [[ -n "${value}" && "${value}" != "latest" && "${value}" != "CHANGE_ME" ]]
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

  candidate="$(get_env_key "POWERX_BACKEND_TAG")"
  if is_valid_tag "${candidate}"; then
    echo "${candidate}"
    return
  fi
  candidate="$(get_env_key "POWERX_RUNNER_TAG")"
  if is_valid_tag "${candidate}"; then
    echo "${candidate}"
    return
  fi
  candidate="$(get_env_key "POWERX_WEB_ADMIN_TAG")"
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

  echo ""
}

RESOLVED_IMAGE_TAG="$(infer_image_tag)"
if ! is_valid_tag "${RESOLVED_IMAGE_TAG}"; then
  echo "[docker-bootstrap] cannot resolve a valid image tag" >&2
  echo "[docker-bootstrap] set POWERX_IMAGE_TAG in .env once, then rerun ./scripts/bootstrap-host.sh" >&2
  exit 1
fi

set_env_key "POWERX_IMAGE_TAG" "${RESOLVED_IMAGE_TAG}"
set_env_key "POWERX_BACKEND_TAG" "${RESOLVED_IMAGE_TAG}"
set_env_key "POWERX_RUNNER_TAG" "${RESOLVED_IMAGE_TAG}"
set_env_key "POWERX_WEB_ADMIN_TAG" "${RESOLVED_IMAGE_TAG}"
echo "[docker-bootstrap] resolved image tag: ${RESOLVED_IMAGE_TAG}"

validate_required_tag() {
  local key="$1"
  local value
  value="$(grep -E "^${key}=" "${ENV_FILE}" | tail -n 1 | cut -d= -f2- || true)"
  value="${value%%#*}"
  value="$(echo "${value}" | xargs)"
  if [[ -z "${value}" || "${value}" == "latest" || "${value}" == "CHANGE_ME" ]]; then
    echo "[docker-bootstrap] invalid ${key}='${value:-<empty>}' in ${ENV_FILE}" >&2
    echo "[docker-bootstrap] set POWERX_IMAGE_TAG in .env, then rerun ./scripts/bootstrap-host.sh" >&2
    exit 1
  fi
}

validate_required_tag "POWERX_BACKEND_TAG"
validate_required_tag "POWERX_RUNNER_TAG"
validate_required_tag "POWERX_WEB_ADMIN_TAG"

sudo mkdir -p "${HOST_CONFIG_DIR}"
sudo mkdir -p "${HOST_DATA_DIR}/postgres" "${HOST_DATA_DIR}/redis" "${HOST_DATA_DIR}/uploads"
sudo chown -R 999:999 "${HOST_DATA_DIR}/postgres" "${HOST_DATA_DIR}/redis"

echo "[docker-bootstrap] done"
echo "[docker-bootstrap] next:"
echo "  cd ${DOCKER_DIR}"
echo "  ./scripts/up.sh"
