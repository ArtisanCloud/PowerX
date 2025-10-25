#!/usr/bin/env bash

set -euo pipefail

TARGET_URL="${EVENT_FABRIC_URL:-https://localhost:8077/admin/event-fabric/events:publish}"
HTTP_URL_OVERRIDE="${EVENT_FABRIC_HTTP_URL:-}"
TENANT_ID="${EVENT_FABRIC_TENANT:-tenant-bench}"
TOPIC_FULL="${EVENT_FABRIC_TOPIC:-tenant-bench.corex.workflow.approved}"
SIGN_SECRET="${EVENT_FABRIC_SIGNATURE_SECRET:-}"
SIGN_KEY_ID="${EVENT_FABRIC_SIGNATURE_KEY_ID:-event-fabric}"
INSECURE="${EVENT_FABRIC_INSECURE:-0}"
LOG_DIR="${EVENT_FABRIC_LOG_DIR:-reports}"
LOG_FILE="${LOG_DIR}/event_fabric_security.log"

mkdir -p "${LOG_DIR}"

if [[ -z "${SIGN_SECRET}" ]]; then
  echo "[ERROR] 必须通过 EVENT_FABRIC_SIGNATURE_SECRET 提供签名密钥" | tee -a "${LOG_FILE}"
  exit 1
fi

tmpdir="$(mktemp -d)"
cleanup() { rm -rf "${tmpdir}"; }
trap cleanup EXIT

payload_base64="$(printf '%s' '{"security":"self-check"}' | base64)"
body_template=$(printf '{"tenant_id":"%s","topic":"%s","event_id":"selfcheck-%d","trace_id":"security-check","version":"v1","payload":"%s","payload_format":"json","attributes":{"principal_id":"svc-security-check"}}' \
  "${TENANT_ID}" "${TOPIC_FULL}" "$(date +%s)" "${payload_base64}")

# --- Step 1: HTTP (非 TLS) 请求应被拒绝 ---
http_target="${HTTP_URL_OVERRIDE}"
if [[ -z "${http_target}" ]]; then
  http_target="${TARGET_URL/https:\/\//http://}"
fi
http_resp="${tmpdir}/http_fail.json"
http_status=$(curl -sS -o "${http_resp}" -w "%{http_code}" -H "Content-Type: application/json" -d "${body_template}" "${http_target}" || true)
echo "[INFO] HTTP plain request status=${http_status}" | tee -a "${LOG_FILE}"
if [[ "${http_status}" -lt 400 ]]; then
  echo "[ERROR] 预期 HTTP 明文请求被拒绝，但返回状态 ${http_status}" | tee -a "${LOG_FILE}"
  exit 1
fi

# --- Step 2: HTTPS + 签名校验 ---
canonical_path="$(printf '%s' "${TARGET_URL}" | sed -E 's#https?://[^/]+##')"
if [[ -z "${canonical_path}" ]]; then
  canonical_path="/"
fi
timestamp="$(date -u +"%Y-%m-%dT%H:%M:%S.%NZ")"
canonical_payload="$(printf "%s\n%s\n%s\n%s" "${timestamp}" "POST" "${canonical_path}" "${body_template}")"
signature_hex="$(printf "%s" "${canonical_payload}" | openssl dgst -sha256 -hmac "${SIGN_SECRET}" -binary | xxd -p -c 256)"
signature_header="${SIGN_KEY_ID}:${signature_hex}"

curl_flags=()
if [[ "${INSECURE}" == "1" ]]; then
  curl_flags+=(-k)
fi

https_resp="${tmpdir}/https_success.json"
https_status=$(curl "${curl_flags[@]}" -sS -o "${https_resp}" -w "%{http_code}" \
  -H "Content-Type: application/json" \
  -H "X-PowerX-Timestamp: ${timestamp}" \
  -H "X-PowerX-Signature: ${signature_header}" \
  -d "${body_template}" \
  "${TARGET_URL}")

echo "[INFO] HTTPS signed request status=${https_status}" | tee -a "${LOG_FILE}"
if [[ "${https_status}" -ne 202 && "${https_status}" -ne 200 ]]; then
  echo "[ERROR] HTTPS+签名请求预期成功，但返回状态 ${https_status}" | tee -a "${LOG_FILE}"
  cat "${https_resp}" >> "${LOG_FILE}"
  exit 1
fi

echo "[OK] 事件骨干安全自检通过。日志: ${LOG_FILE}" | tee -a "${LOG_FILE}"
