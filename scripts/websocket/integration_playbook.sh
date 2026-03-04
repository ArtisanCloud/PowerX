#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/websocket/integration_playbook.sh [options]

WebSocket 总线联调脚本（默认只读）：
  1) 校验租户上下文与基础接口可达
  2) 校验 ws-bus register/publish 接口
  3) 可选执行 queue 写入联调（pipeline debug）

Options:
  --base-url <url>         API 基地址，默认读取 POWERX_BASE_URL
  --admin-token <token>    管理员 token，默认读取 ADMIN_TOKEN
  --env-file <path>        指定 env 文件（默认自动尝试 backend/.env）
  --with-write             开启写入联调（会创建通知任务与历史）
  --topic <topic>          publish topic（默认 _topic.system.notification）
  --limit <n>              查询 limit（默认 30）
  -h, --help               显示帮助

Required env (若未通过参数传入):
  POWERX_BASE_URL
  ADMIN_TOKEN
USAGE
}

BASE_URL="${POWERX_BASE_URL:-}"
ADMIN_TOKEN_VALUE="${ADMIN_TOKEN:-}"
ENV_FILE=""
WITH_WRITE=0
TOPIC="_topic.system.notification"
LIMIT=30

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      BASE_URL="$2"; shift 2 ;;
    --admin-token)
      ADMIN_TOKEN_VALUE="$2"; shift 2 ;;
    --env-file)
      ENV_FILE="$2"; shift 2 ;;
    --with-write)
      WITH_WRITE=1; shift ;;
    --topic)
      TOPIC="$2"; shift 2 ;;
    --limit)
      LIMIT="$2"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "[FAIL] 未知参数: $1" >&2
      usage
      exit 1 ;;
  esac
done

if ! [[ "$LIMIT" =~ ^[0-9]+$ ]] || [[ "$LIMIT" -le 0 ]]; then
  echo "[FAIL] --limit 必须为正整数" >&2
  exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "[FAIL] curl 不可用" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "[FAIL] jq 不可用" >&2
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
if [[ -z "$ADMIN_TOKEN_VALUE" ]]; then
  ADMIN_TOKEN_VALUE="${ADMIN_TOKEN:-${ROOT_ADMIN_TOKEN:-}}"
fi
if [[ -n "$BASE_URL" && "$BASE_URL" != *"/api/v1" ]]; then
  BASE_URL="${BASE_URL%/}/api/v1"
fi

if [[ -z "$BASE_URL" ]]; then
  echo "[FAIL] 缺少 BASE_URL，请传 --base-url 或在 backend/.env 设置 POWERX_BASE_URL/API_BASE_URL" >&2
  exit 1
fi
if [[ -z "$ADMIN_TOKEN_VALUE" ]]; then
  echo "[FAIL] 缺少 ADMIN_TOKEN，请传 --admin-token 或在 backend/.env 设置 ADMIN_TOKEN/ROOT_ADMIN_TOKEN" >&2
  exit 1
fi

AUTH_HEADER="Authorization: Bearer ${ADMIN_TOKEN_VALUE}"
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
  local body="${3:-}"
  local url="${BASE_URL}${path}"

  local response
  if [[ "$method" == "GET" ]]; then
    response="$(curl -sS -X GET -H "$AUTH_HEADER" "$url" -w $'\n%{http_code}')" || return 1
  else
    response="$(curl -sS -X "$method" -H "$AUTH_HEADER" -H "Content-Type: application/json" "$url" -d "$body" -w $'\n%{http_code}')" || return 1
  fi
  local status
  status="$(printf '%s' "$response" | tail -n1)"
  local payload
  payload="$(printf '%s' "$response" | sed '$d')"
  if [[ ! "$status" =~ ^2[0-9][0-9]$ ]]; then
    fail "请求失败 ${method} ${path} -> HTTP ${status}；响应: ${payload}"
  fi
  printf '%s' "$payload"
}

assert_json_expr() {
  local json="$1"
  local expr="$2"
  local message="$3"
  local result
  result="$(printf '%s' "$json" | jq -r "$expr" 2>/dev/null || true)"
  if [[ "$result" == "true" ]]; then
    pass "$message"
  else
    fail "$message（断言失败: $expr）"
  fi
}

info "========== WebSocket Integration Playbook =========="
info "BASE_URL=${BASE_URL}"
if [[ "$WITH_WRITE" -eq 1 ]]; then
  info "已开启 --with-write：会创建通知任务并写入任务历史"
else
  info "当前模式：只读校验（不写库）"
fi

info "Step 1/5: 获取 overview"
OVERVIEW_JSON="$(request_json GET "/admin/event-fabric/overview?limit=${LIMIT}")"
assert_json_expr "$OVERVIEW_JSON" '.code == 200 and (.data.tenant_uuid | length > 0)' "overview 返回成功且带 tenant_uuid"
TENANT_UUID="$(printf '%s' "$OVERVIEW_JSON" | jq -r '.data.tenant_uuid // empty')"
[[ -n "$TENANT_UUID" ]] || fail "overview 中 tenant_uuid 为空"
pass "当前租户 tenant_uuid=${TENANT_UUID}"

info "Step 2/5: 注册 ws-bus topic（register）"
REGISTER_REQ="$(jq -cn --arg topic "$TOPIC" '{topics:[$topic],actions:["publish","subscribe"]}')"
REGISTER_JSON="$(request_json POST "/internal/ws-bus/register" "$REGISTER_REQ")"
assert_json_expr "$REGISTER_JSON" '.code == 200 and (.data.topics | length > 0)' "ws-bus/register 返回成功"
REG_MODE="$(printf '%s' "$REGISTER_JSON" | jq -r '.data.mode // empty')"
if [[ "$REG_MODE" == "registry_acl" || "$REG_MODE" == "compat_dynamic" ]]; then
  pass "register 模式=${REG_MODE}"
else
  warn "register 模式异常：${REG_MODE:-<empty>}"
fi

info "Step 3/5: 发布 ws-bus 消息（publish）"
TRACE_ID="ws-playbook.$(date +%s)"
PUBLISH_REQ="$(jq -cn --arg topic "$TOPIC" --arg trace "$TRACE_ID" '{topic:$topic,trace_id:$trace,payload:{source:"integration_playbook.websocket",message:"ping"}}')"
PUBLISH_JSON="$(request_json POST "/internal/ws-bus/publish" "$PUBLISH_REQ")"
assert_json_expr "$PUBLISH_JSON" '.code == 200 and (.data.topic | length > 0)' "ws-bus/publish 返回成功"
pass "publish topic=${TOPIC} trace_id=${TRACE_ID}"

info "Step 4/5: 检查 Queue 统计接口"
STATS_JSON="$(request_json GET "/admin/event-fabric/task-queue/stats")"
assert_json_expr "$STATS_JSON" '.code == 200' "task-queue/stats 返回成功"

if [[ "$WITH_WRITE" -eq 1 ]]; then
  info "Step 5/5: 执行 Pipeline 写入联调并校验历史"
  PIPE_REQ="$(jq -cn '{title:"WS Integration Playbook",content:"pipeline debug",type:"system",category:"system"}')"
  PIPE_JSON="$(request_json POST "/admin/event-fabric/pipeline/tasks" "$PIPE_REQ")"
  assert_json_expr "$PIPE_JSON" '.code == 200 and (.data.task_id | length > 0)' "Pipeline 通知任务创建成功"
  PIPE_TASK_ID="$(printf '%s' "$PIPE_JSON" | jq -r '.data.task_id // empty')"
  pass "Pipeline task_id=${PIPE_TASK_ID}"

  PIPE_MSG_JSON="$(request_json GET "/admin/event-fabric/task-queue/messages?tenant_key=global&subscriber_id=_subscriber.system.notification_dispatch&limit=${LIMIT}")"
  assert_json_expr "$PIPE_MSG_JSON" '.code == 200' "Pipeline 分片消息接口可读"
  pipe_found="$(printf '%s' "$PIPE_MSG_JSON" | jq --arg id "$PIPE_TASK_ID" '[.data.history[]? | select(.task_id==$id)] | length')"
  if [[ "${pipe_found:-0}" -gt 0 ]]; then
    pass "Pipeline 历史命中 task_id=${PIPE_TASK_ID}"
  else
    warn "Pipeline 历史暂未命中 task_id=${PIPE_TASK_ID}（可稍后手工再查）"
  fi
else
  info "Step 5/5: 跳过写入联调（默认只读）"
fi

echo
info "========== Summary =========="
echo "PASS=${PASS_COUNT}"
echo "WARN=${WARN_COUNT}"
if [[ "$WITH_WRITE" -eq 1 ]]; then
  echo "MODE=read+write"
else
  echo "MODE=read-only"
fi
pass "脚本执行完成"
