#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT_DIR"

echo "[log-check] scanning PowerX backend logging conventions..."

tmp1="$(mktemp)"
tmp2="$(mktemp)"
trap 'rm -f "$tmp1" "$tmp2"' EXIT

# 1) 禁止 logger/pxlog 直接使用 context.Background() 打业务日志
rg -n \
  --glob 'backend/**/*.go' \
  --glob '!backend/**/*_test.go' \
  --glob '!backend/api/grpc/gen/**' \
  '(logger|pxlog)\.(Info|Warn|Error|Debug|InfoF|WarnF|ErrorF|DebugF)\(context\.Background\(\)' \
  >"$tmp1" || true

# 2) 禁止在核心代码中直接 stdout/stderr 打印（测试文件除外）
rg -n \
  --glob 'backend/cmd/**/*.go' \
  --glob 'backend/internal/**/*.go' \
  --glob 'backend/pkg/**/*.go' \
  --glob 'backend/tools/**/*.go' \
  --glob '!backend/**/*_test.go' \
  --glob '!backend/api/grpc/gen/**' \
  '\bfmt\.Print(f|ln)?\(|\blog\.Print(f|ln)?\(|\bprintln\(' \
  >"$tmp2" || true

failed=0

if [[ -s "$tmp1" ]]; then
  failed=1
  echo
  echo "[log-check] ERROR: logger/pxlog calls with context.Background() detected:"
  cat "$tmp1"
fi

if [[ -s "$tmp2" ]]; then
  failed=1
  echo
  echo "[log-check] ERROR: raw print calls detected (use unified logger instead):"
  cat "$tmp2"
fi

if [[ $failed -ne 0 ]]; then
  echo
  echo "[log-check] FAILED"
  exit 1
fi

echo "[log-check] PASSED"
