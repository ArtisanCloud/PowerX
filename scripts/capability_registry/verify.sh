#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/capability_registry/verify.sh [options]

Options:
  --artifact <path>          指定 .pxp 包目录（默认使用 make_files/dev.mk 中的 CAPABILITY_SYNC_ARTIFACTS）
  --plugin-id <id>           插件 ID（可使用环境变量 PLUGIN_ID）
  --capability-id <id>       能力 ID，POST /tenant/invocations 使用（可使用 CAPABILITY_ID）
  --tenant-uuid <uuid>       租户 UUID（可使用 TENANT_UUID）
  --preferred-protocol <p>   Preferred protocol，默认 mcp
  --skip-event-seed          跳过 Event Fabric manifest dry-run
  --event-seed-manifest <p>  覆盖默认 manifest 路径（传给 event_fabric_seed）
  -h, --help                 显示帮助

必须的环境变量：
  POWERX_BASE_URL    例如 https://powerx.dev/api/v1
  ADMIN_TOKEN        管理员 Token
  TENANT_TOKEN       租户 Token

脚本会：
  1. 触发 make capability-sync（如提供 --artifact 则覆盖同步目录）
  2. 调用 Admin/Tenant API 校验能力目录
  3. 触发一次租户调用，输出 trace_id
  4. 列出 Workflow 模板，提示需要手动升级的条目
USAGE
}

log() {
  printf '[%s] %s\n' "$(date '+%H:%M:%S')" "$*"
}

require_var() {
  local name="$1" value="${!1:-}"
  if [[ -z "$value" ]]; then
    echo "[ERROR] missing required variable: $name" >&2
    exit 1
  fi
}

ARTIFACT_DIR=""
PLUGIN_ID="${PLUGIN_ID:-}"
CAPABILITY_ID="${CAPABILITY_ID:-}"
TENANT_UUID="${TENANT_UUID:-}"
PREFERRED_PROTOCOL="mcp"
RUN_EVENT_SEED=1
EVENT_SEED_MANIFEST="${EVENT_SEED_MANIFEST:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact)
      ARTIFACT_DIR="$2"; shift 2 ;;
    --plugin-id)
      PLUGIN_ID="$2"; shift 2 ;;
    --capability-id)
      CAPABILITY_ID="$2"; shift 2 ;;
    --tenant-uuid)
      TENANT_UUID="$2"; shift 2 ;;
    --preferred-protocol)
      PREFERRED_PROTOCOL="$2"; shift 2 ;;
    --skip-event-seed)
      RUN_EVENT_SEED=0; shift ;;
    --event-seed-manifest)
      EVENT_SEED_MANIFEST="$2"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1 ;;
  esac
done

require_var POWERX_BASE_URL
require_var ADMIN_TOKEN
require_var TENANT_TOKEN
require_var TENANT_UUID
require_var PLUGIN_ID
require_var CAPABILITY_ID

# Step 1: capability sync
log "Run capability sync"
if [[ -n "$ARTIFACT_DIR" ]]; then
  CAPABILITY_SYNC_ARTIFACTS="$ARTIFACT_DIR" make capability-sync
else
  make capability-sync
fi

if [[ "$RUN_EVENT_SEED" -eq 1 ]]; then
  log "Dry-run Event Fabric seed for plugin=$PLUGIN_ID tenant=$TENANT_UUID"
  pushd backend >/dev/null
  SEED_CMD=(go run ./cmd/event_fabric_seed --tenant "$TENANT_UUID" --plugin "$PLUGIN_ID" --dry-run)
  if [[ -n "$EVENT_SEED_MANIFEST" ]]; then
    SEED_CMD+=(--manifest "$EVENT_SEED_MANIFEST")
  fi
  if ! "${SEED_CMD[@]}"; then
    echo "[ERROR] Event Fabric dry-run failed" >&2
    exit 1
  fi
  popd >/dev/null
fi

API_BASE="$POWERX_BASE_URL"

curl_json() {
  local method="$1" path="$2"; shift 2
  curl -sSf -X "$method" "$API_BASE$path" "$@"
}

log "Fetch Admin capability for plugin=$PLUGIN_ID"
ADMIN_JSON=$(curl_json GET "/admin/capabilities?plugin_id=$PLUGIN_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
if command -v jq >/dev/null 2>&1; then
  ADMIN_HASH=$(printf '%s' "$ADMIN_JSON" | jq -r '.data.capabilities_hash // .data[0].capabilities_hash // empty')
  if [[ -n "$ADMIN_HASH" ]]; then
    log "Admin capabilities_hash=$ADMIN_HASH"
  fi
fi

log "Fetch Tenant capabilities"
TENANT_JSON=$(curl_json GET "/tenant/capabilities?channel=agent" \
  -H "Authorization: Bearer $TENANT_TOKEN")
if command -v jq >/dev/null 2>&1; then
  TENANT_COUNT=$(printf '%s' "$TENANT_JSON" | jq '.data | length' 2>/dev/null || true)
  log "Tenant capability count: ${TENANT_COUNT:-unknown}"
fi

log "Invoke capability $CAPABILITY_ID"
IDEMPOTENCY_KEY="cap-reg-verify-$(date +%s)"
INVOCATION_JSON=$(curl_json POST "/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"capability_id\":\"$CAPABILITY_ID\",\"idempotency_key\":\"$IDEMPOTENCY_KEY\",\"preferred_protocol\":\"$PREFERRED_PROTOCOL\",\"payload\":{\"verify\":true}}")
TRACE_ID=""
if command -v jq >/dev/null 2>&1; then
  TRACE_ID=$(printf '%s' "$INVOCATION_JSON" | jq -r '.data.trace_id // empty')
else
  TRACE_ID=$(printf '%s' "$INVOCATION_JSON" | sed -n 's/.*"trace_id":"\([^"]*\)".*/\1/p')
fi
if [[ -n "$TRACE_ID" ]]; then
  log "Invocation trace_id: $TRACE_ID"
else
  log "Invocation response: $INVOCATION_JSON"
fi

log "List workflow templates"
WF_JSON=$(curl_json GET "/admin/workflow-templates?capability_id=$CAPABILITY_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
if command -v jq >/dev/null 2>&1; then
  NEEDS_UPGRADE=$(printf '%s' "$WF_JSON" | jq '[.data.items[] | select(.needs_upgrade==true)] | length' 2>/dev/null || echo 0)
  log "Workflow templates needing upgrade: $NEEDS_UPGRADE"
fi

log "Verification completed"
