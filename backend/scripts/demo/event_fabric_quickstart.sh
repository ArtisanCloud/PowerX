#!/usr/bin/env bash

set -euo pipefail

API_BASE="${EVENT_FABRIC_API_BASE:-http://localhost:8077/api/v1/admin/event-fabric}"
TENANT_UUID="${EVENT_FABRIC_TENANT_UUID:-00000000-0000-0000-0000-000000000001}"
SUBJECT_TYPE="${EVENT_FABRIC_SUBJECT_TYPE:-agent}"
SUBJECT_UUID="${EVENT_FABRIC_SUBJECT_UUID:-00000000-0000-0000-0000-000000000101}"
CAPABILITY_KEY="${EVENT_FABRIC_CAPABILITY:-event_fabric.publish}"
LOG_DIR="${EVENT_FABRIC_LOG_DIR:-reports}"
LOG_FILE="${LOG_DIR}/event_fabric_authorization_quickstart.log"
# 登录参数（可选）——若全部提供则脚本自动获取 AccessToken
LOGIN_ENDPOINT="${EVENT_FABRIC_LOGIN_ENDPOINT:-http://localhost:8077/api/v1/admin/user/auth/login}"
LOGIN_TENANT="${EVENT_FABRIC_LOGIN_TENANT:-system}"
LOGIN_IDENTIFIER="${EVENT_FABRIC_LOGIN_IDENTIFIER:-root}"
LOGIN_PASSWORD="${EVENT_FABRIC_LOGIN_PASSWORD:-}"
AUTH_HEADER="${EVENT_FABRIC_AUTH_HEADER:-}"

mkdir -p "${LOG_DIR}"
: >"${LOG_FILE}"

echo "[INFO] Quickstart 日志输出到 ${LOG_FILE}" | tee -a "${LOG_FILE}"

if [[ -z "${AUTH_HEADER}" && -n "${LOGIN_PASSWORD}" ]]; then
  echo "[STEP] 登录获取 AccessToken" | tee -a "${LOG_FILE}"
  login_payload=$(cat <<JSON
{"tenant":"${LOGIN_TENANT}","identifier":"${LOGIN_IDENTIFIER}","password":"${LOGIN_PASSWORD}"}
JSON
)
  login_status=$(curl -sS -o /tmp/event_fabric_login.json -w "%{http_code}" \
    -H "Content-Type: application/json" \
    -d "${login_payload}" \
    "${LOGIN_ENDPOINT}" || true)
  cat /tmp/event_fabric_login.json >> "${LOG_FILE}"
  if [[ "${login_status}" -ne 200 ]]; then
    echo "[ERROR] 登录失败，HTTP ${login_status}" | tee -a "${LOG_FILE}"
    exit 1
  fi
  token=$(python3 - <<'PY' /tmp/event_fabric_login.json 2>/tmp/event_fabric_login.err
import json, sys
try:
    with open(sys.argv[1], "r", encoding="utf-8") as fh:
        data = json.load(fh)
    token = data.get("data", {}).get("access_token")
    if not token:
        raise ValueError("access_token missing")
    print(token)
except Exception as exc:
    print(exc, file=sys.stderr)
    sys.exit(1)
PY
)
  if [[ $? -ne 0 || -z "${token}" ]]; then
    echo "[ERROR] 未能解析登录响应中的 access_token" | tee -a "${LOG_FILE}"
    cat /tmp/event_fabric_login.err >> "${LOG_FILE}"
    exit 1
  fi
  AUTH_HEADER="Authorization: Bearer ${token}"
  echo "[INFO] 登录成功，已注入 Authorization 头" | tee -a "${LOG_FILE}"
fi

curl_with_auth() {
  local method="$1"; shift
  local url="$1"; shift
  curl "${@}" ${AUTH_HEADER:+-H "${AUTH_HEADER}"} -X "${method}" "${url}"
}

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
cap_resp=$(curl_with_auth POST "${API_BASE}/capabilities" \
  -sS -o /tmp/capability.json -w "%{http_code}" \
  -H "Content-Type: application/json" \
  -d "${cap_body}" || true)
cat /tmp/capability.json >> "${LOG_FILE}"
if [[ "${cap_resp}" -ne 201 && "${cap_resp}" -ne 409 ]]; then
  if [[ "${cap_resp}" -eq 400 ]]; then
    python3 - <<'PY' /tmp/capability.json 2>/tmp/event_fabric_cap.err
import json, sys
try:
    with open(sys.argv[1], "r", encoding="utf-8") as fh:
        data = json.load(fh)
    msg = (data.get("error") or data.get("message") or "").lower()
    if "already exists" in msg:
        sys.exit(0)
    raise ValueError(msg or "unknown error")
except Exception as exc:
    print(exc, file=sys.stderr)
    sys.exit(1)
PY
    if [[ $? -ne 0 ]]; then
      echo "[ERROR] 创建能力失败，HTTP ${cap_resp}" | tee -a "${LOG_FILE}"
      cat /tmp/event_fabric_cap.err >> "${LOG_FILE}"
      exit 1
    fi
  else
    echo "[ERROR] 创建能力失败，HTTP ${cap_resp}" | tee -a "${LOG_FILE}"
    exit 1
  fi
fi

echo "[STEP] 创建授权 Grant" | tee -a "${LOG_FILE}"
read -r -d '' grant_body <<JSON
{
  "tenant_id": "${TENANT_UUID}",
  "subject": {
    "type": "${SUBJECT_TYPE}",
    "id": "${SUBJECT_UUID}"
  },
  "capabilities": ["${namespace}.${action}"],
  "conditions": {
    "resources": ["topic://demo"],
    "context_tags": ["prod"]
  },
  "ttl_seconds": 7200
}
JSON
grant_resp=$(curl_with_auth POST "${API_BASE}/grants" \
  -sS -o /tmp/grant.json -w "%{http_code}" \
  -H "Content-Type: application/json" \
  -d "${grant_body}" || true)
cat /tmp/grant.json >> "${LOG_FILE}"
if [[ "${grant_resp}" -ne 201 && "${grant_resp}" -ne 202 ]]; then
  echo "[ERROR] 创建 Grant 失败，HTTP ${grant_resp}" | tee -a "${LOG_FILE}"
  exit 1
fi

echo "[STEP] 查询授权审计(JSON)" | tee -a "${LOG_FILE}"
now_utc=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
audit_url="${API_BASE}/audit/authorization?tenantId=${TENANT_UUID}&from=1970-01-01T00:00:00Z&to=${now_utc}&page=1&pageSize=20"
audit_resp=$(curl_with_auth GET "${audit_url}" \
  -sS -o "${LOG_DIR}/authorization_audit.json" -w "%{http_code}" || true)
if [[ "${audit_resp}" -ne 200 ]]; then
  echo "[WARN] 审计查询返回 HTTP ${audit_resp}" | tee -a "${LOG_FILE}"
else
  cat "${LOG_DIR}/authorization_audit.json" >> "${LOG_FILE}"
fi

echo "[STEP] 导出授权审计(CSV)" | tee -a "${LOG_FILE}"
csv_url="${audit_url}&format=csv"
csv_resp=$(curl_with_auth GET "${csv_url}" \
  -sS -o "${LOG_DIR}/authorization_audit.csv" -w "%{http_code}" || true)
if [[ "${csv_resp}" -ne 200 ]]; then
  echo "[WARN] CSV 导出返回 HTTP ${csv_resp}" | tee -a "${LOG_FILE}"
else
  head -n 5 "${LOG_DIR}/authorization_audit.csv" >> "${LOG_FILE}"
fi

echo "[DONE] Quickstart 完成。能力=${CAPABILITY_KEY}" | tee -a "${LOG_FILE}"
