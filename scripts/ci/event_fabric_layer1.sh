#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

PASS_COUNT=0
WARN_COUNT=0

ts() { date '+%H:%M:%S'; }
info() { echo "[$(ts)] [INFO] $*"; }
pass() { PASS_COUNT=$((PASS_COUNT + 1)); echo "[$(ts)] [PASS] $*"; }
warn() { WARN_COUNT=$((WARN_COUNT + 1)); echo "[$(ts)] [WARN] $*"; }
fail() { echo "[$(ts)] [FAIL] $*" >&2; exit 1; }

run() {
  local label="$1"; shift
  info "$label"
  "$@" || fail "$label"
  pass "$label"
}

info "========== Event Fabric Layer1 (No-Service CI) =========="
info "目标：不启动 backend/web-admin 服务，完成语法与编译级回归。"

run "Shell 脚本语法检查: integration_playbook" bash -n scripts/event_fabric/integration_playbook.sh
run "Shell 脚本语法检查: websocket integration_playbook" bash -n scripts/websocket/integration_playbook.sh
run "Shell 脚本语法检查: cron integration_playbook" bash -n scripts/cron/integration_playbook.sh

run "文档入口一致性检查" rg -n "integration_playbook\\.md" \
  docs/guides/async_runtime/README.md \
  docs/guides/async_runtime/event_fabric/README.md \
  docs/guides/async_runtime/testing/README.md \
  docs/guides/async_runtime/event_fabric/integration_playbook.md

mkdir -p tmp/gocache tmp/gomodcache
export GOCACHE="$ROOT_DIR/tmp/gocache"
export GOMODCACHE="$ROOT_DIR/tmp/gomodcache"
export GOTOOLCHAIN=go1.26.7

GO_VERSION_RAW="$(go version 2>/dev/null | awk '{print $3}' || true)"
[[ "$GO_VERSION_RAW" == "go1.26.7" ]] || \
  fail "Go 工具链必须为 go1.26.7，当前为 ${GO_VERSION_RAW:-unknown}"

run "Go 编译检查: admin event_fabric transport" \
  go -C backend test ./internal/transport/http/admin/event_fabric -count=1

run "Go 编译检查: admin runtime ws-bus transport" \
  go -C backend test ./internal/transport/http/admin/runtime -count=1

run "Go 编译检查: event_fabric replay service" \
  go -C backend test ./internal/service/event_fabric/replay -count=1

run "Go 编译检查: shared task history decorator" \
  go -C backend test ./internal/app/shared -run TestTaskHistory -count=1

echo
info "========== Summary =========="
echo "PASS=${PASS_COUNT}"
echo "WARN=${WARN_COUNT}"
pass "Layer1 回归完成（无服务）"
