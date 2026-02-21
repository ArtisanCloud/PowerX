#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

function usage() {
  cat <<'EOF'
集成网关端到端验证脚本
---------------------------------
依赖环境变量：
  ADMIN_BASE   管理端 API 基址，例如 http://localhost:8077/api/v1/admin
  OPEN_BASE    租户开放 API 基址，例如 http://localhost:8077/api/v1/tenant
  ADMIN_TOKEN  管理员 Token
  TENANT_TOKEN 租户 Token

可选环境变量：
  TENANT_ID    目标租户 ID（默认 tenant-verify）
  ROUTE_SLUG   路由别名（默认 demo-sync）
  CAPABILITY_ID 能力 ID（默认 cap.demo.echo）
  TOOL_GRANT_ID Tool Grant（默认 grant-demo-echo）

脚本步骤：
  1. 管理员创建 / 更新路由（若已存在则跳过错误）
  2. 租户发起 HTTP 调用
  3. MCP 工具调用（integration.route.list + integration.route.invoke）
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

for var in ADMIN_BASE OPEN_BASE ADMIN_TOKEN TENANT_TOKEN; do
  if [[ -z "${!var:-}" ]]; then
    echo "缺少环境变量 $var，请先设置。" >&2
    exit 1
  fi
done

TENANT_ID="${TENANT_ID:-tenant-verify}"
ROUTE_SLUG="${ROUTE_SLUG:-demo-sync}"
CAPABILITY_ID="${CAPABILITY_ID:-cap.demo.echo}"
TOOL_GRANT_ID="${TOOL_GRANT_ID:-grant-demo-echo}"

echo "==> Step 1: 管理员创建/更新路由 ${ROUTE_SLUG}"
create_payload=$(cat <<EOF
{
  "tenant_id": "${TENANT_ID}",
  "route_slug": "${ROUTE_SLUG}",
  "capability_id": "${CAPABILITY_ID}",
  "tool_grant_ids": ["${TOOL_GRANT_ID}"],
  "channels": ["http","mcp"],
  "rate_limit": {"limit": 60, "burst": 60, "window_seconds": 60},
  "event_topics": {
    "invocation_succeeded": "integration.gateway.invocation.succeeded",
    "invocation_failed": "integration.gateway.invocation.failed"
  }
}
EOF
)

create_resp=$(curl -sS -w "\n%{http_code}" -X POST "${ADMIN_BASE}/integration/routes" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${create_payload}" || true)

create_body=$(echo "${create_resp}" | head -n1)
create_code=$(echo "${create_resp}" | tail -n1)

if [[ "${create_code}" != "201" && "${create_code}" != "409" ]]; then
  echo "创建路由失败：HTTP ${create_code}"
  echo "${create_body}"
  exit 1
fi

route_id=$(echo "${create_body}" | jq -r '.data.route_id // empty')
if [[ -z "${route_id}" ]]; then
  # 409 时需要查询 route id
  get_resp=$(curl -sS -X GET "${ADMIN_BASE}/integration/routes?tenant_id=${TENANT_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
  route_id=$(echo "${get_resp}" | jq -r ".data.items[] | select(.route_slug==\"${ROUTE_SLUG}\") | .route_id")
fi

etag=$(echo "${create_body}" | jq -r '.data.current_version // 0')
echo "  -> route_id: ${route_id}, version: ${etag}"

echo "==> Step 2: 租户调用 HTTP 接口"
invoke_resp=$(curl -sS -w "\n%{http_code}" -X POST "${OPEN_BASE}/integration/routes/${ROUTE_SLUG}/invoke" \
  -H "Authorization: Bearer ${TENANT_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"payload":{"demo":"ok"}}' || true)

invoke_body=$(echo "${invoke_resp}" | head -n1)
invoke_code=$(echo "${invoke_resp}" | tail -n1)
echo "  -> HTTP status: ${invoke_code}"
echo "${invoke_body}" | jq '.' || echo "${invoke_body}"

echo "==> Step 3: MCP 工具调用"
list_req='{"tool":"integration.route.list","arguments":{"tenant_id":"'"${TENANT_ID}"'"}}'
invoke_req='{"tool":"integration.route.invoke","arguments":{"tenant_id":"'"${TENANT_ID}"'","route_slug":"'"${ROUTE_SLUG}"'","payload":{"demo":"mcp"}}}'

list_resp=$(curl -sS -X POST "${OPEN_BASE}/integration/mcp/tools" \
  -H "Authorization: Bearer ${TENANT_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${list_req}")

echo "  -> MCP list response:"
echo "${list_resp}" | jq '.' || echo "${list_resp}"

invoke_mcp_resp=$(curl -sS -X POST "${OPEN_BASE}/integration/mcp/tools" \
  -H "Authorization: Bearer ${TENANT_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${invoke_req}")

echo "  -> MCP invoke response:"
echo "${invoke_mcp_resp}" | jq '.' || echo "${invoke_mcp_resp}"

echo "🎉 验证完成。请根据输出的 trace_id 进一步串联日志或事件。"

