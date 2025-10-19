#!/usr/bin/env bash

set -euo pipefail

API_BASE="${EVENT_FABRIC_API_BASE:-http://localhost:8077/admin/event-fabric}"
TENANT_ID="${EVENT_FABRIC_TENANT:-tenant-demo}"
TOPIC_NAMESPACE="${EVENT_FABRIC_NAMESPACE:-corex.workflow}"
TOPIC_NAME="${EVENT_FABRIC_TOPIC_NAME:-approved}"
PUBLISH_PRINCIPAL="${EVENT_FABRIC_PUBLISH_PRINCIPAL:-svc-demo-wf}"
SUBSCRIBER_ID="${EVENT_FABRIC_SUBSCRIBER:-svc-demo-consumer}"
LOG_DIR="${EVENT_FABRIC_LOG_DIR:-reports}"
LOG_FILE="${LOG_DIR}/event_fabric_quickstart.log"

mkdir -p "${LOG_DIR}"
: >"${LOG_FILE}"

event_id="evt-quickstart-$(date +%s)"
full_topic="${TENANT_ID}.${TOPIC_NAMESPACE}.${TOPIC_NAME}"

echo "[INFO] Quickstart 日志输出到 ${LOG_FILE}" | tee -a "${LOG_FILE}"

echo "[STEP] 创建 Topic ${full_topic}" | tee -a "${LOG_FILE}"
create_resp=$(curl -sS -o /tmp/topic_create.json -w "%{http_code}" -H "Content-Type: application/json" -d "{\"tenant_id\":\"${TENANT_ID}\",\"namespace\":\"${TOPIC_NAMESPACE}\",\"name\":\"${TOPIC_NAME}\",\"payload_format\":\"json\",\"versioning_mode\":\"backward\",\"max_retry\":5,\"retention_policy\":\"{\\\"type\\\":\\\"time\\\",\\\"value\\\":\\\"7d\\\"}\"}" "${API_BASE}/topics" || true)
cat /tmp/topic_create.json >> "${LOG_FILE}"
if [[ "${create_resp}" -ne 200 && "${create_resp}" -ne 201 && "${create_resp}" -ne 409 ]]; then
  echo "[ERROR] Topic 创建失败，HTTP ${create_resp}" | tee -a "${LOG_FILE}"
  exit 1
fi

echo "[STEP] Upsert ACL" | tee -a "${LOG_FILE}"
acl_body=$(cat <<JSON
{
  "tenant_id": "${TENANT_ID}",
  "topic_full_name": "${full_topic}",
  "grants": [
    {"principal_type":"service","principal_id":"${PUBLISH_PRINCIPAL}","action":"publish"},
    {"principal_type":"service","principal_id":"${SUBSCRIBER_ID}","action":"subscribe"}
  ]
}
JSON
)
acl_resp=$(curl -sS -o /tmp/acl_upsert.json -w "%{http_code}" -H "Content-Type: application/json" -d "${acl_body}" "${API_BASE}/acl" || true)
cat /tmp/acl_upsert.json >> "${LOG_FILE}"
if [[ "${acl_resp}" -ne 200 && "${acl_resp}" -ne 201 ]]; then
  echo "[ERROR] ACL 设置失败，HTTP ${acl_resp}" | tee -a "${LOG_FILE}"
  exit 1
fi

echo "[STEP] 发布事件 ${event_id}" | tee -a "${LOG_FILE}"
payload_base64=$(printf '%s' '{"requestId":"req-123","status":"approved"}' | base64)
publish_body=$(cat <<JSON
{
  "tenant_id": "${TENANT_ID}",
  "topic": "${full_topic}",
  "event_id": "${event_id}",
  "trace_id": "trace-quickstart",
  "version": "v1",
  "payload": "${payload_base64}",
  "payload_format": "json",
  "attributes": {"principal_id":"${PUBLISH_PRINCIPAL}"}
}
JSON
)
publish_resp=$(curl -sS -o /tmp/publish.json -w "%{http_code}" -H "Content-Type: application/json" -d "${publish_body}" "${API_BASE}/events:publish" || true)
cat /tmp/publish.json >> "${LOG_FILE}"
if [[ "${publish_resp}" -ne 202 ]]; then
  echo "[ERROR] 发布事件失败，HTTP ${publish_resp}" | tee -a "${LOG_FILE}"
  exit 1
fi

echo "[STEP] 验证 DLQ 状态" | tee -a "${LOG_FILE}"
dlq_resp=$(curl -sS -o /tmp/dlq.json -w "%{http_code}" "${API_BASE}/dlq/messages?tenant_id=${TENANT_ID}&status=queued" || true)
cat /tmp/dlq.json >> "${LOG_FILE}"

echo "[DONE] Quickstart 完成。事件 ID: ${event_id}" | tee -a "${LOG_FILE}"
