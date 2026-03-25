#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
THRESHOLD="${COVERAGE_THRESHOLD:-80}"
OUT_FILE="${COVERAGE_OUT:-$ROOT_DIR/tmp/coverage-gate.out}"
LOG_FILE="${COVERAGE_LOG:-$ROOT_DIR/tmp/coverage-gate.log}"
TEST_PACKAGES="${COVERAGE_TEST_PACKAGES:-./tests/contract/ops ./tests/integration/ops}"
COVER_PKG="${COVERAGE_PKG:-./internal/service/deploy_ops,./internal/service/backup_ops,./internal/service/migration_ops,./internal/service/observability_ops,./internal/transport/grpc/ops}"
mkdir -p "$(dirname "$OUT_FILE")"

(
  cd "$BACKEND_DIR"
  GOWORK=off GOCACHE="$BACKEND_DIR/.gocache" GOMODCACHE="$BACKEND_DIR/.gomodcache" go test $TEST_PACKAGES -coverpkg="$COVER_PKG" -coverprofile="$OUT_FILE" -count=1 > "$LOG_FILE"
)

pct=$(
  cd "$BACKEND_DIR"
  GOWORK=off GOCACHE="$BACKEND_DIR/.gocache" go tool cover -func="$OUT_FILE" | awk '/^total:/ {gsub("%","",$3); print $3}'
)
python - <<'PY' "$pct" "$THRESHOLD"
import sys
pct=float(sys.argv[1]); threshold=float(sys.argv[2])
if pct + 1e-9 < threshold:
    print(f"[coverage-gate] FAIL total={pct:.2f}% threshold={threshold:.2f}%")
    raise SystemExit(1)
print(f"[coverage-gate] PASS total={pct:.2f}% threshold={threshold:.2f}%")
PY
