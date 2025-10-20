#!/usr/bin/env bash

set -euo pipefail

API_BASE="${EVENT_FABRIC_API_BASE:-http://localhost:8077/api/v1/admin/event-fabric}"
TENANT_UUID="${EVENT_FABRIC_TENANT_UUID:-00000000-0000-0000-0000-000000000001}"
SUBJECT_TYPE="${EVENT_FABRIC_SUBJECT_TYPE:-agent}"
SUBJECT_UUID="${EVENT_FABRIC_SUBJECT_UUID:-00000000-0000-0000-0000-000000000101}"
CAPABILITY_KEY="${EVENT_FABRIC_CAPABILITY:-event_fabric.publish}"
LOG_DIR="${EVENT_FABRIC_LOG_DIR:-reports}"
LOG_FILE="${LOG_DIR}/event_fabric_authorization_quickstart.log"

mkdir -p "${LOG_DIR}"
: >"${LOG_FILE}"

echo "[INFO] Quickstart 日志输出到 ${LOG_FILE}" | tee -a "${LOG_FILE}"

echo "[STEP] 创建能力 ${CAPABILITY_KEY}" | tee -a "${LOG_FILE}"
namespace="${CAPABILITY_KEY%%.*}"
action="${CAPABILITY_KEY#*.}"
cap_body=$(cat <<JSON
{
  "namespace": "${namespace}",
  "action": "${action}",
  "description": "Demo capability for quickstart",
  "risk_level": "medium"
}
JSON
)
cap_resp=$(curl -sS -o /tmp/capability.json -w "%{http_code}" -H "Content-Type: application/json" -d "${cap_body}" "${API_BASE}/capabilities" || true)
cat /tmp/capability.json >> "${LOG_FILE}"
if [[ "${cap_resp}" -ne 201 && "${cap_resp}" -ne 409 ]]; then
  echo "[ERROR] 创建能力失败，HTTP ${cap_resp}" | tee -a "${LOG_FILE}"
  exit 1
fi

echo "[STEP] 创建授权 Grant" | tee -a "${LOG_FILE}"
grant_body=$(cat <<JSON
{
  "tenant_id": "${TENANT_UUID}",
  "subject": {
    "type": "${SUBJECT_TYPE}",
    "id": "${SUBJECT_UUID}"
  },
  "capabilities": ["${CAPABILITY_KEY}"],
  "conditions": {
    "resources": ["topic://demo"],
    "context_tags": ["prod"]
  },
  "ttl_seconds": 7200
}
JSON
)
grant_resp=$(curl -sS -o /tmp/grant.json -w "%{http_code}" -H "Content-Type: application/json" -d "${grant_body}" "${API_BASE}/grants" || true)
cat /tmp/grant.json >> "${LOG_FILE}"
if [[ "${grant_resp}" -ne 201 && "${grant_resp}" -ne 202 ]]; then
  echo "[ERROR] 创建 Grant 失败，HTTP ${grant_resp}" | tee -a "${LOG_FILE}"
  exit 1
fi

echo "[STEP] 查询授权审计(JSON)" | tee -a "${LOG_FILE}"
now_utc=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
audit_url="${API_BASE}/audit/authorization?tenantId=${TENANT_UUID}&from=1970-01-01T00:00:00Z&to=${now_utc}&page=1&pageSize=20"
audit_resp=$(curl -sS -o "${LOG_DIR}/authorization_audit.json" -w "%{http_code}" "${audit_url}" || true)
if [[ "${audit_resp}" -ne 200 ]]; then
  echo "[WARN] 审计查询返回 HTTP ${audit_resp}" | tee -a "${LOG_FILE}"
else
  cat "${LOG_DIR}/authorization_audit.json" >> "${LOG_FILE}"
fi

echo "[STEP] 导出授权审计(CSV)" | tee -a "${LOG_FILE}"
csv_url="${audit_url}&format=csv"
csv_resp=$(curl -sS -o "${LOG_DIR}/authorization_audit.csv" -w "%{http_code}" "${csv_url}" || true)
if [[ "${csv_resp}" -ne 200 ]]; then
  echo "[WARN] CSV 导出返回 HTTP ${csv_resp}" | tee -a "${LOG_FILE}"
else
  head -n 5 "${LOG_DIR}/authorization_audit.csv" >> "${LOG_FILE}"
fi

echo "[DONE] Quickstart 完成。能力=${CAPABILITY_KEY}" | tee -a "${LOG_FILE}"
