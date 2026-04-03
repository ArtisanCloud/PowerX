#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <environment> <target_version> [mode]" >&2
  exit 1
fi

ENVIRONMENT="$1"
TARGET_VERSION="$2"
MODE="${3:-docker}"
BASE_URL="${POWERX_BASE_URL:-http://127.0.0.1:8080}"
API_PATH="${POWERX_DEPLOY_ROLLBACK_PATH:-/api/v1/admin/deploy/rollback}"
AUTH_HEADER="${POWERX_ADMIN_AUTH_HEADER:-}"

payload=$(cat <<JSON
{"environment":"${ENVIRONMENT}","target_version":"${TARGET_VERSION}"}
JSON
)

headers=(-H "Content-Type: application/json")
if [[ -n "${AUTH_HEADER}" ]]; then
  headers+=(-H "Authorization: ${AUTH_HEADER}")
fi

curl -sS -X POST "${BASE_URL}${API_PATH}?mode=${MODE}" \
  "${headers[@]}" \
  -d "${payload}"

echo
