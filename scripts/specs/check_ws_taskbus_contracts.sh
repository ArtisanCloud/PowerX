#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "[doc-check] FAIL: $1" >&2
  exit 1
}

pass() {
  echo "[doc-check] OK: $1"
}

REQ_FILES=(
  "specs/004-eventbus-message-fabric/spec.md"
  "specs/023-websocket-notify/spec.md"
  "specs/023-websocket-notify/contracts/http-openapi.yaml"
  "specs/004-eventbus-message-fabric/checklists/doc-consistency.md"
)

for f in "${REQ_FILES[@]}"; do
  [[ -f "$f" ]] || fail "missing file: $f"
done
pass "required files exist"

CONTRACT_FILE="specs/023-websocket-notify/contracts/http-openapi.yaml"
SPEC004_FILE="specs/004-eventbus-message-fabric/spec.md"
SPEC023_FILE="specs/023-websocket-notify/spec.md"

rg -n "^\s*/api/v1/internal/ws-bus/register:" "$CONTRACT_FILE" >/dev/null || fail "missing register path"
rg -n "^\s*/api/v1/internal/ws-bus/publish:" "$CONTRACT_FILE" >/dev/null || fail "missing publish path"
pass "required internal ws-bus paths exist"

for field in topic type payload ts trace_id; do
  rg -n "^\s*${field}:\s*$" "$CONTRACT_FILE" >/dev/null || fail "WSBusEnvelope missing field: ${field}"
done
pass "WSBusEnvelope fields exist"

rg -n "^## Normative Reference" "$SPEC023_FILE" >/dev/null || fail "missing Normative Reference section in 023"
rg -n "specs/004-eventbus-message-fabric/spec.md" "$SPEC023_FILE" >/dev/null || fail "023 does not reference 004 authority spec"
pass "023 references 004 authority"

rg -n "FR-016" "$SPEC023_FILE" >/dev/null || fail "missing FR-016 protected-topic guard in 023"
rg -n "受保护 topic" "$SPEC023_FILE" >/dev/null || fail "missing protected topic rule text in 023"
pass "protected topic anti-bypass rule exists"

for section in "规范治理策略" "三类契约冻结" "文档防漂移执行要求"; do
  rg -n "$section" "$SPEC004_FILE" >/dev/null || fail "004 missing section: $section"
done
pass "004 governance sections exist"

echo "[doc-check] PASS: ws/taskbus contracts are consistent"
