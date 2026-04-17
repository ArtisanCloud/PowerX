#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

BASELINE="scripts/ci/baseline/runtime-log-direct-output.txt"
CURRENT_FILE="$(mktemp)"
NEW_FILE="$(mktemp)"
trap 'rm -f "$CURRENT_FILE" "$NEW_FILE"' EXIT

rg -n "\\bfmt\\.(Print|Printf|Println)|\\blog\\.(Print|Printf|Println)" backend \
  --glob '*.go' \
  --glob '!**/*_test.go' \
  --glob '!backend/cmd/**' \
  --glob '!backend/tools/**' \
  --glob '!backend/tests/**' \
  | sort -u > "$CURRENT_FILE" || true

if [[ ! -f "$BASELINE" ]]; then
  echo "[log-unification] baseline missing: $BASELINE"
  echo "Run: make refresh-log-unification-baseline"
  exit 1
fi

comm -23 "$CURRENT_FILE" "$BASELINE" > "$NEW_FILE" || true

if [[ -s "$NEW_FILE" ]]; then
  echo "[log-unification] detected new direct stdout/stderr logging usages:"
  cat "$NEW_FILE"
  echo
  echo "Please migrate to pkg/utils/logger before merging."
  exit 1
fi

echo "[log-unification] no new direct-output logging usages."
