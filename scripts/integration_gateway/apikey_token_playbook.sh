#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/integration_gateway/apikey_token_playbook.sh [options]

API Key / Token 联调脚本：
  1) 获取 ADMIN_TOKEN（优先读取参数/环境变量，缺失时自动登录）
  2) 为 Profile 绑定 permission_ids（最小权限）
  3) 创建 API Key（继承 Profile 权限）
  4) 使用 API Key 调用 ws-bus/register + publish
  5) 额外验证一条未授权 topic 会被拒绝（403）

Options:
  --base-url <url>          API 基地址（默认读 POWERX_BASE_URL）
  --admin-token <token>     直接指定管理员 token（可选）
  --tenant <tenant>         登录租户（默认 system）
  --identifier <id>         登录账号（默认 root）
  --password <pwd>          登录密码（默认 root）
  --tenant-uuid <uuid>      创建 API Key 使用的 tenant_uuid（默认自动从 me/context 读取）
  --profile-id <id>         API Key Profile ID（可选，默认自动发现）
  --topic <topic>           联调 topic（默认 _topic.system.notification）
  --forbidden-topic <topic> 未授权 topic（默认 _topic.forbidden.demo）
  --name <name>             API Key 名称（默认 ws-debug-key）
  --env-file <path>         指定 env 文件（默认自动尝试 backend/.env）
  -h, --help                显示帮助

USAGE
}

BASE_URL="${POWERX_BASE_URL:-}"
ADMIN_TOKEN_VALUE="${ADMIN_TOKEN:-}"
TENANT="${ADMIN_LOGIN_TENANT:-system}"
IDENTIFIER="${ADMIN_LOGIN_IDENTIFIER:-root}"
PASSWORD="${ADMIN_LOGIN_PASSWORD:-root}"
TENANT_UUID="${TENANT_UUID:-}"
PROFILE_ID=""
TOPIC="_topic.system.notification"
FORBIDDEN_TOPIC="_topic.forbidden.demo"
KEY_NAME="ws-debug-key"
ENV_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url) BASE_URL="$2"; shift 2 ;;
    --admin-token) ADMIN_TOKEN_VALUE="$2"; shift 2 ;;
    --tenant) TENANT="$2"; shift 2 ;;
    --identifier) IDENTIFIER="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --tenant-uuid) TENANT_UUID="$2"; shift 2 ;;
    --profile-id) PROFILE_ID="$2"; shift 2 ;;
    --topic) TOPIC="$2"; shift 2 ;;
    --forbidden-topic) FORBIDDEN_TOPIC="$2"; shift 2 ;;
    --name) KEY_NAME="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "[FAIL] 未知参数: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! command -v curl >/dev/null 2>&1; then
  echo "[FAIL] curl 不可用" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "[FAIL] jq 不可用" >&2
  exit 1
fi
if [[ -n "$PROFILE_ID" ]] && ! [[ "$PROFILE_ID" =~ ^[0-9]+$ ]]; then
  echo "[FAIL] --profile-id 必须是正整数" >&2
  exit 1
fi

load_env_file() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    return 0
  fi
  # shellcheck disable=SC1090
  set -a; source "$file"; set +a
}

if [[ -n "$ENV_FILE" ]]; then
  load_env_file "$ENV_FILE"
else
  load_env_file "backend/.env"
fi

if [[ -z "$BASE_URL" ]]; then
  BASE_URL="${POWERX_BASE_URL:-${API_BASE_URL:-${BASE_URL:-}}}"
fi
if [[ -n "$BASE_URL" && "$BASE_URL" != *"/api/v1" ]]; then
  BASE_URL="${BASE_URL%/}/api/v1"
fi
if [[ -z "$BASE_URL" ]]; then
  echo "[FAIL] 缺少 BASE_URL，请传 --base-url 或在 backend/.env 配置 POWERX_BASE_URL" >&2
  exit 1
fi

PASS_COUNT=0
WARN_COUNT=0
ts() { date '+%H:%M:%S'; }
info() { echo "[$(ts)] [INFO] $*"; }
pass() { PASS_COUNT=$((PASS_COUNT + 1)); echo "[$(ts)] [PASS] $*"; }
warn() { WARN_COUNT=$((WARN_COUNT + 1)); echo "[$(ts)] [WARN] $*"; }
fail() { echo "[$(ts)] [FAIL] $*" >&2; exit 1; }

request_json() {
  local method="$1"
  local path="$2"
  local auth_header="${3:-}"
  local body="${4:-}"
  local url="${BASE_URL}${path}"
  local response

  if [[ "$method" == "GET" ]]; then
    if [[ -n "$auth_header" ]]; then
      response="$(curl -sS -X GET -H "$auth_header" "$url" -w $'\n%{http_code}')" || return 1
    else
      response="$(curl -sS -X GET "$url" -w $'\n%{http_code}')" || return 1
    fi
  else
    if [[ -n "$auth_header" ]]; then
      response="$(curl -sS -X "$method" -H "$auth_header" -H "Content-Type: application/json" "$url" -d "$body" -w $'\n%{http_code}')" || return 1
    else
      response="$(curl -sS -X "$method" -H "Content-Type: application/json" "$url" -d "$body" -w $'\n%{http_code}')" || return 1
    fi
  fi

  local status
  status="$(printf '%s' "$response" | tail -n1)"
  local payload
  payload="$(printf '%s' "$response" | sed '$d')"
  printf '%s\n%s' "$payload" "$status"
}

ensure_admin_token() {
  if [[ -n "${ADMIN_TOKEN_VALUE:-}" ]]; then
    pass "使用传入 ADMIN_TOKEN"
    return 0
  fi
  info "ADMIN_TOKEN 缺失，尝试自动登录获取"
  local login_body
  login_body="$(jq -cn --arg tenant "$TENANT" --arg identifier "$IDENTIFIER" --arg password "$PASSWORD" \
    '{tenant:$tenant,identifier:$identifier,password:$password}')"
  local result payload status token
  result="$(request_json POST "/admin/user/auth/login" "" "$login_body")"
  payload="$(printf '%s' "$result" | sed '$d')"
  status="$(printf '%s' "$result" | tail -n1)"
  if [[ ! "$status" =~ ^2[0-9][0-9]$ ]]; then
    fail "自动登录失败 HTTP ${status}：${payload}"
  fi
  token="$(printf '%s' "$payload" | jq -r '.data.access_token // empty')"
  [[ -n "$token" ]] || fail "登录成功但未返回 access_token"
  ADMIN_TOKEN_VALUE="$token"
  pass "自动登录成功并获取 ADMIN_TOKEN"
}

ensure_tenant_uuid() {
  if [[ -n "$TENANT_UUID" ]]; then
    pass "使用传入 tenant_uuid=${TENANT_UUID}"
    return 0
  fi
  local auth_header result payload status tenant_uuid
  auth_header="Authorization: Bearer ${ADMIN_TOKEN_VALUE}"
  result="$(request_json GET "/admin/user/auth/me/context" "$auth_header")"
  payload="$(printf '%s' "$result" | sed '$d')"
  status="$(printf '%s' "$result" | tail -n1)"
  if [[ ! "$status" =~ ^2[0-9][0-9]$ ]]; then
    fail "读取 me/context 失败 HTTP ${status}：${payload}"
  fi
  tenant_uuid="$(printf '%s' "$payload" | jq -r '.data.tenant_uuid // empty')"
  [[ -n "$tenant_uuid" ]] || fail "me/context 未返回 tenant_uuid"
  TENANT_UUID="$tenant_uuid"
  pass "自动获取 tenant_uuid=${TENANT_UUID}"
}

ensure_profile_id() {
  if [[ -n "$PROFILE_ID" ]]; then
    pass "使用传入 profile_id=${PROFILE_ID}"
    return 0
  fi
  info "profile_id 未指定，尝试自动发现可用 API Key Profile"
  local auth_header result payload status id
  auth_header="Authorization: Bearer ${ADMIN_TOKEN_VALUE}"
  result="$(request_json GET "/admin/integration/api-key-profiles?tenant_uuid=${TENANT_UUID}" "$auth_header")"
  payload="$(printf '%s' "$result" | sed '$d')"
  status="$(printf '%s' "$result" | tail -n1)"
  if [[ ! "$status" =~ ^2[0-9][0-9]$ ]]; then
    fail "查询 api key profiles 失败 HTTP ${status}：${payload}"
  fi
  id="$(printf '%s' "$payload" | jq -r '.data.items[]? | select(.status == 1) | .id' | head -n1)"
  if [[ -z "$id" ]]; then
    fail "未找到 status=1 的 api key profile，请先创建或手动传 --profile-id"
  fi
  PROFILE_ID="$id"
  pass "自动发现 profile_id=${PROFILE_ID}"
}

bind_profile_permissions() {
  local auth_header result payload status ids_json body
  auth_header="Authorization: Bearer ${ADMIN_TOKEN_VALUE}"
  result="$(request_json GET "/admin/integration/permissions/catalog" "$auth_header")"
  payload="$(printf '%s' "$result" | sed '$d')"
  status="$(printf '%s' "$result" | tail -n1)"
  [[ "$status" =~ ^2[0-9][0-9]$ ]] || fail "读取权限目录失败 HTTP ${status}：${payload}"

  ids_json="$(printf '%s' "$payload" | jq -c --arg topic "$TOPIC" '
    [
      (.data.items[]? | select(.meta.api_key.scope=="_scope.ws.topic.publish" and .meta.api_key.action=="publish" and .meta.api_key.resource_pattern==$topic) | .id),
      (.data.items[]? | select(.meta.api_key.scope=="_scope.ws.topic.subscribe" and .meta.api_key.action=="subscribe" and .meta.api_key.resource_pattern==$topic) | .id)
    ] | unique | map(select(. != null))
  ')"
  if [[ "$(printf '%s' "$ids_json" | jq 'length')" -lt 2 ]]; then
    fail "权限目录缺少 ws publish/subscribe 模板（topic=${TOPIC}），请先 migrate/seed 或改回默认 topic"
  fi

  body="$(jq -cn --argjson ids "$ids_json" '{permission_ids:$ids}')"
  result="$(request_json PUT "/admin/integration/api-key-profiles/${PROFILE_ID}/permissions" "$auth_header" "$body")"
  payload="$(printf '%s' "$result" | sed '$d')"
  status="$(printf '%s' "$result" | tail -n1)"
  [[ "$status" =~ ^2[0-9][0-9]$ ]] || fail "绑定 Profile 权限失败 HTTP ${status}：${payload}"
  pass "Profile 权限绑定成功 profile_id=${PROFILE_ID} permission_ids=$(printf '%s' "$ids_json" | jq -c '.')"
}

create_api_key() {
  local auth_header body result payload status code
  auth_header="Authorization: Bearer ${ADMIN_TOKEN_VALUE}"
  body="$(jq -cn \
    --arg tenant_uuid "$TENANT_UUID" \
    --argjson profile_id "$PROFILE_ID" \
    --arg name "$KEY_NAME" \
    '{
      tenant_uuid:$tenant_uuid,
      profile_id:$profile_id,
      name:$name
    }')"
  result="$(request_json POST "/admin/integration/api-keys" "$auth_header" "$body")"
  payload="$(printf '%s' "$result" | sed '$d')"
  status="$(printf '%s' "$result" | tail -n1)"
  if [[ ! "$status" =~ ^2[0-9][0-9]$ ]]; then
    fail "创建 API Key 失败 HTTP ${status}：${payload}"
  fi
  code="$(printf '%s' "$payload" | jq -r '.code // 0')"
  [[ "$code" == "200" || "$code" == "201" ]] || fail "创建 API Key 响应 code 非成功：$code"

  API_KEY_VALUE="$(printf '%s' "$payload" | jq -r '.data.plain_key // empty')"
  API_KEY_UUID="$(printf '%s' "$payload" | jq -r '.data.api_key.key_id // empty')"
  [[ -n "$API_KEY_VALUE" ]] || fail "创建成功但未返回 plain_key"
  [[ -n "$API_KEY_UUID" ]] || fail "创建成功但未返回 key_id"
  pass "创建 API Key 成功 key_id=${API_KEY_UUID}"
}

test_ws_register_publish() {
  local auth_header reg_body pub_body result payload status
  auth_header="Authorization: ApiKey ${API_KEY_VALUE}"

  reg_body="$(jq -cn --arg topic "$TOPIC" '{topics:[$topic],actions:["publish","subscribe"]}')"
  result="$(request_json POST "/internal/ws-bus/register" "$auth_header" "$reg_body")"
  payload="$(printf '%s' "$result" | sed '$d')"
  status="$(printf '%s' "$result" | tail -n1)"
  [[ "$status" =~ ^2[0-9][0-9]$ ]] || fail "ws-bus/register 失败 HTTP ${status}：${payload}"
  pass "ws-bus/register 通过"

  pub_body="$(jq -cn --arg topic "$TOPIC" '{topic:$topic,payload:{source:"apikey-token-playbook",message:"ok"}}')"
  result="$(request_json POST "/internal/ws-bus/publish" "$auth_header" "$pub_body")"
  payload="$(printf '%s' "$result" | sed '$d')"
  status="$(printf '%s' "$result" | tail -n1)"
  [[ "$status" =~ ^2[0-9][0-9]$ ]] || fail "ws-bus/publish 失败 HTTP ${status}：${payload}"
  pass "ws-bus/publish 通过"
}

test_forbidden_topic() {
  local auth_header pub_body result payload status
  auth_header="Authorization: ApiKey ${API_KEY_VALUE}"
  pub_body="$(jq -cn --arg topic "$FORBIDDEN_TOPIC" '{topic:$topic,payload:{source:"apikey-token-playbook",message:"deny-check"}}')"
  result="$(request_json POST "/internal/ws-bus/publish" "$auth_header" "$pub_body")"
  payload="$(printf '%s' "$result" | sed '$d')"
  status="$(printf '%s' "$result" | tail -n1)"
  if [[ "$status" == "403" ]]; then
    pass "未授权 topic 被正确拒绝（403）"
    return 0
  fi
  warn "未授权 topic 预期 403，实际 HTTP ${status}：${payload}"
}

info "========== API Key / Token Playbook =========="
info "BASE_URL=${BASE_URL}"
ensure_admin_token
ensure_tenant_uuid
ensure_profile_id
bind_profile_permissions
create_api_key
test_ws_register_publish
test_forbidden_topic

echo
info "========== Summary =========="
echo "PASS=${PASS_COUNT}"
echo "WARN=${WARN_COUNT}"
echo "TENANT_UUID=${TENANT_UUID}"
echo "API_KEY_ID=${API_KEY_UUID}"
echo "API_KEY_PLAIN=${API_KEY_VALUE}"
pass "脚本执行完成"
