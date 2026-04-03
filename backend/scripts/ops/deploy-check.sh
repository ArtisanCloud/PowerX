#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${POWERX_BASE_URL:-http://127.0.0.1:8080}"
HEALTH_PATH="${POWERX_HEALTH_PATH:-/api/v1/health}"

echo "[deploy-check] probing ${BASE_URL}${HEALTH_PATH}"
status_code=$(curl -sS -o /tmp/powerx-deploy-health.json -w "%{http_code}" "${BASE_URL}${HEALTH_PATH}")

if [[ "${status_code}" != "200" ]]; then
  echo "[deploy-check] health check failed, status=${status_code}" >&2
  cat /tmp/powerx-deploy-health.json >&2 || true
  exit 1
fi

echo "[deploy-check] healthy"
cat /tmp/powerx-deploy-health.json || true
