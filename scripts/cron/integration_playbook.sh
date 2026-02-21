#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/cron/integration_playbook.sh [options]

Cron / Retry 联调脚本（默认只读）：
  1) 校验 cron jobs 列表
  2) 校验关键 job 是否存在
  3) 可选执行 run-now / pause / resume

Options:
  --base-url <url>         API 基地址，默认读取 POWERX_BASE_URL
  --admin-token <token>    管理员 token，默认读取 ADMIN_TOKEN
  --env-file <path>        指定 env 文件（默认自动尝试 backend/.env）
  --with-write             开启写入联调（会触发 run-now，并执行 pause/resume）
  --job-id <id>            指定单个 job（默认 event_fabric.retry_dispatch）
  --limit <n>              overview limit（默认 20）
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
JOB_ID="event_fabric.retry_dispatch"
LIMIT=20

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
    --job-id)
      JOB_ID="$2"; shift 2 ;;
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

job_exists() {
  local jobs_json="$1"
  local id="$2"
  local count
  count="$(printf '%s' "$jobs_json" | jq --arg id "$id" '[.data.items[]? | select(.id==$id)] | length')"
  [[ "${count:-0}" -gt 0 ]]
}

info "========== Cron Integration Playbook =========="
info "BASE_URL=${BASE_URL}"
if [[ "$WITH_WRITE" -eq 1 ]]; then
  info "已开启 --with-write：会触发 cron run-now + pause/resume"
else
  info "当前模式：只读校验（不修改作业状态）"
fi

info "Step 1/5: 获取 overview"
OVERVIEW_JSON="$(request_json GET "/admin/event-fabric/overview?limit=${LIMIT}")"
assert_json_expr "$OVERVIEW_JSON" '.code == 200 and (.data.tenant_uuid | length > 0)' "overview 返回成功且带 tenant_uuid"
TENANT_UUID="$(printf '%s' "$OVERVIEW_JSON" | jq -r '.data.tenant_uuid // empty')"
[[ -n "$TENANT_UUID" ]] || fail "overview 中 tenant_uuid 为空"
pass "当前租户 tenant_uuid=${TENANT_UUID}"

info "Step 2/5: 获取 cron jobs 列表"
JOBS_JSON="$(request_json GET "/admin/event-fabric/cron/jobs")"
assert_json_expr "$JOBS_JSON" '.code == 200 and (.data.items | length >= 1)' "cron/jobs 返回成功"

job_exists "$JOBS_JSON" "event_fabric.retry_dispatch" \
  && pass "存在作业: event_fabric.retry_dispatch" \
  || warn "未发现作业: event_fabric.retry_dispatch"
job_exists "$JOBS_JSON" "event_fabric.authorization_challenge_timeout" \
  && pass "存在作业: event_fabric.authorization_challenge_timeout" \
  || warn "未发现作业: event_fabric.authorization_challenge_timeout"

job_exists "$JOBS_JSON" "$JOB_ID" || fail "指定作业不存在: ${JOB_ID}"
pass "目标作业存在: ${JOB_ID}"

if [[ "$WITH_WRITE" -eq 1 ]]; then
  info "Step 3/5: 执行 run-now"
  RUN_JSON="$(request_json POST "/admin/event-fabric/cron/jobs/${JOB_ID}/run-now" "{}")"
  assert_json_expr "$RUN_JSON" '.code == 200 and (.data.id == "'"${JOB_ID}"'")' "run-now 返回成功"

  info "Step 4/5: 执行 pause/resume 并恢复运行态"
  PAUSE_JSON="$(request_json POST "/admin/event-fabric/cron/jobs/${JOB_ID}/pause" "{}")"
  assert_json_expr "$PAUSE_JSON" '.code == 200 and (.data.id == "'"${JOB_ID}"'")' "pause 返回成功"
  RESUME_JSON="$(request_json POST "/admin/event-fabric/cron/jobs/${JOB_ID}/resume" "{}")"
  assert_json_expr "$RESUME_JSON" '.code == 200 and (.data.id == "'"${JOB_ID}"'")' "resume 返回成功"

  info "Step 5/5: 再次读取作业状态"
  JOBS_AFTER_JSON="$(request_json GET "/admin/event-fabric/cron/jobs")"
  assert_json_expr "$JOBS_AFTER_JSON" '.code == 200' "cron/jobs 再次读取成功"
  final_status="$(printf '%s' "$JOBS_AFTER_JSON" | jq -r --arg id "$JOB_ID" '.data.items[]? | select(.id==$id) | .status' | head -n1)"
  if [[ "$final_status" == "running" || "$final_status" == "unavailable" ]]; then
    pass "作业状态可接受: ${final_status}"
  else
    warn "作业状态为 ${final_status:-<empty>}，请手工确认"
  fi
else
  info "Step 3/5: 跳过 run-now（默认只读）"
  info "Step 4/5: 跳过 pause/resume（默认只读）"
  info "Step 5/5: 跳过二次状态读取（默认只读）"
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
