#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/event_fabric/integration_playbook.sh [options]

Event Fabric 总体验证脚本（默认只读，不写库）：
  1) 基础连通性检查（overview / topics / task-queue/stats）
  2) 核心 topic / subscriber 分片检查
  3) 可选写入联调（Replay + Pipeline）
  4) 输出 PASS/FAIL 汇总，便于回归与排障

Options:
  --base-url <url>         API 基地址，默认读取 POWERX_BASE_URL
  --admin-token <token>    Root 管理员 token，默认读取 ADMIN_TOKEN
  --env-file <path>        指定 env 文件（默认自动尝试 backend/.env）
  --with-write             开启写入联调（会写 replay/task/notification 记录）
  --replay-topic <topic>   Replay 联调 topic（默认 _topic.knowledge.space.feedback.reprocess）
  --limit <n>              查询 limit（默认 30）
  -h, --help               显示帮助

Required env (若未通过参数传入):
  POWERX_BASE_URL
  ADMIN_TOKEN

Examples:
  scripts/event_fabric/integration_playbook.sh
  scripts/event_fabric/integration_playbook.sh --with-write
  scripts/event_fabric/integration_playbook.sh --with-write --replay-topic _topic.knowledge.space.feedback.reprocess
USAGE
}

BASE_URL="${POWERX_BASE_URL:-}"
ADMIN_TOKEN_VALUE="${ADMIN_TOKEN:-}"
ENV_FILE=""
WITH_WRITE=0
REPLAY_TOPIC="_topic.knowledge.space.feedback.reprocess"
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
    --replay-topic)
      REPLAY_TOPIC="$2"; shift 2 ;;
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
    echo "$payload" | jq . >/dev/null 2>&1 || true
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

contains_topic() {
  local topics_json="$1"
  local namespace="$2"
  local name="$3"
  local count
  count="$(printf '%s' "$topics_json" | jq --arg ns "$namespace" --arg name "$name" '[.data.items[]? | select(.namespace==$ns and .name==$name)] | length')"
  [[ "${count:-0}" -gt 0 ]]
}

contains_subscriber_pair() {
  local stats_json="$1"
  local tenant_key="$2"
  local subscriber_id="$3"
  local count
  count="$(printf '%s' "$stats_json" | jq --arg tenant "$tenant_key" --arg sub "$subscriber_id" '[.data.task_queue.by_subscriber[]? | select(.tenant_key==$tenant and .subscriber_id==$sub)] | length')"
  [[ "${count:-0}" -gt 0 ]]
}

info "========== Event Fabric Integration Playbook =========="
info "BASE_URL=${BASE_URL}"
if [[ "$WITH_WRITE" -eq 1 ]]; then
  info "已开启 --with-write：会创建 replay 任务与通知任务（会写入数据库）"
else
  info "当前模式：只读校验（不写库）"
fi

info "Step 1/6: 获取 overview"
OVERVIEW_JSON="$(request_json GET "/admin/event-fabric/overview?limit=${LIMIT}")"
assert_json_expr "$OVERVIEW_JSON" '.code == 200 and (.data.tenant_uuid | length > 0)' "overview 返回成功且带 tenant_uuid"
TENANT_UUID="$(printf '%s' "$OVERVIEW_JSON" | jq -r '.data.tenant_uuid // empty')"
[[ -n "$TENANT_UUID" ]] || fail "overview 中 tenant_uuid 为空"
pass "当前租户 tenant_uuid=${TENANT_UUID}"

info "Step 2/6: 获取 topic 列表并校验关键 topic"
TOPICS_JSON="$(request_json GET "/admin/event-fabric/topics?page=1&page_size=200")"
assert_json_expr "$TOPICS_JSON" '.code == 200' "topics 接口返回成功"
contains_topic "$TOPICS_JSON" "_topic.knowledge.space.feedback" "reprocess" \
  && pass "存在 topic: _topic.knowledge.space.feedback.reprocess" \
  || fail "缺少 topic: _topic.knowledge.space.feedback.reprocess"
contains_topic "$TOPICS_JSON" "_topic.system" "notification" \
  && pass "存在 topic: _topic.system.notification" \
  || fail "缺少 topic: _topic.system.notification"

info "Step 3/6: 获取 task-queue/stats 并校验关键分片"
STATS_JSON="$(request_json GET "/admin/event-fabric/task-queue/stats")"
assert_json_expr "$STATS_JSON" '.code == 200' "task-queue/stats 返回成功"

contains_subscriber_pair "$STATS_JSON" "$TENANT_UUID" "_subscriber.event_fabric.replay" \
  && pass "存在分片: ${TENANT_UUID} + _subscriber.event_fabric.replay" \
  || warn "未发现分片: ${TENANT_UUID} + _subscriber.event_fabric.replay（可能首次未触发）"

contains_subscriber_pair "$STATS_JSON" "global" "_subscriber.system.notification_dispatch" \
  && pass "存在分片: global + _subscriber.system.notification_dispatch" \
  || warn "未发现分片: global + _subscriber.system.notification_dispatch（可能首次未触发）"

contains_subscriber_pair "$STATS_JSON" "global" "_subscriber.authorization.challenge_timeout" \
  && pass "存在分片: global + _subscriber.authorization.challenge_timeout" \
  || warn "未发现分片: global + _subscriber.authorization.challenge_timeout"

if [[ "$WITH_WRITE" -eq 1 ]]; then
  info "Step 4/6: 执行 Replay 写入联调"
  REPLAY_REQ="$(jq -cn --arg topic "$REPLAY_TOPIC" '{topic:$topic, reason:"integration_playbook", shadow:true}')"
  REPLAY_JSON="$(request_json POST "/admin/event-fabric/replay/tasks" "$REPLAY_REQ")"
  assert_json_expr "$REPLAY_JSON" '.code == 200 and (.data.id | length > 0)' "Replay 任务创建成功"
  REPLAY_TASK_ID="$(printf '%s' "$REPLAY_JSON" | jq -r '.data.id // empty')"
  pass "Replay task_id=${REPLAY_TASK_ID}"

  info "Step 5/6: 执行 Pipeline 写入联调"
  PIPE_REQ="$(jq -cn '{title:"Integration Playbook",content:"pipeline debug",type:"system",category:"system"}')"
  PIPE_JSON="$(request_json POST "/admin/event-fabric/pipeline/tasks" "$PIPE_REQ")"
  assert_json_expr "$PIPE_JSON" '.code == 200 and (.data.task_id | length > 0)' "Pipeline 通知任务创建成功"
  PIPE_TASK_ID="$(printf '%s' "$PIPE_JSON" | jq -r '.data.task_id // empty')"
  pass "Pipeline task_id=${PIPE_TASK_ID}"

  info "Step 6/6: 校验两条分片历史（不做轮询）"
  REPLAY_MSG_JSON="$(request_json GET "/admin/event-fabric/task-queue/messages?tenant_key=${TENANT_UUID}&subscriber_id=_subscriber.event_fabric.replay&limit=${LIMIT}")"
  assert_json_expr "$REPLAY_MSG_JSON" '.code == 200' "Replay 分片消息接口可读"
  if [[ -n "$REPLAY_TASK_ID" ]]; then
    replay_found="$(printf '%s' "$REPLAY_MSG_JSON" | jq --arg id "$REPLAY_TASK_ID" '[.data.history[]? | select(.task_id==$id)] | length')"
    if [[ "${replay_found:-0}" -gt 0 ]]; then
      pass "Replay 历史命中 task_id=${REPLAY_TASK_ID}"
    else
      warn "Replay 历史暂未命中 task_id=${REPLAY_TASK_ID}（可稍后手工再查）"
    fi
  fi

  PIPE_MSG_JSON="$(request_json GET "/admin/event-fabric/task-queue/messages?tenant_key=global&subscriber_id=_subscriber.system.notification_dispatch&limit=${LIMIT}")"
  assert_json_expr "$PIPE_MSG_JSON" '.code == 200' "Pipeline 分片消息接口可读"
  if [[ -n "$PIPE_TASK_ID" ]]; then
    pipe_found="$(printf '%s' "$PIPE_MSG_JSON" | jq --arg id "$PIPE_TASK_ID" '[.data.history[]? | select(.task_id==$id)] | length')"
    if [[ "${pipe_found:-0}" -gt 0 ]]; then
      pass "Pipeline 历史命中 task_id=${PIPE_TASK_ID}"
    else
      warn "Pipeline 历史暂未命中 task_id=${PIPE_TASK_ID}（可稍后手工再查）"
    fi
  fi
else
  info "Step 4/6: 跳过写入联调（默认只读）"
  info "Step 5/6: 跳过写入联调（默认只读）"
  info "Step 6/6: 跳过写入联调（默认只读）"
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
