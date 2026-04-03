#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
THRESHOLD_MS="${PERF_P95_THRESHOLD_MS:-200}"
JSON_FILE="${PERF_JSON_OUT:-$ROOT_DIR/tmp/perf-smoke.json}"
mkdir -p "$(dirname "$JSON_FILE")"

(
  cd "$BACKEND_DIR"
  GOCACHE="$BACKEND_DIR/.gocache" GOMODCACHE="$BACKEND_DIR/.gomodcache" go test ./tests/integration/ops -run Test -count=1 -json > "$JSON_FILE"
)

python - <<'PY' "$JSON_FILE" "$THRESHOLD_MS"
import json,sys,math
path=sys.argv[1]; threshold=float(sys.argv[2])
values=[]
with open(path,encoding='utf-8') as f:
    for line in f:
        line=line.strip()
        if not line:
            continue
        try:
            o=json.loads(line)
        except Exception:
            continue
        if o.get('Action')=='pass' and o.get('Test') and isinstance(o.get('Elapsed'), (int,float)):
            values.append(float(o['Elapsed'])*1000.0)
if not values:
    print('[perf-smoke] no test elapsed metrics found')
    raise SystemExit(1)
values.sort()
idx=max(0, math.ceil(0.95*len(values))-1)
p95=values[idx]
if p95 > threshold:
    print(f'[perf-smoke] FAIL p95={p95:.2f}ms threshold={threshold:.2f}ms n={len(values)}')
    raise SystemExit(1)
print(f'[perf-smoke] PASS p95={p95:.2f}ms threshold={threshold:.2f}ms n={len(values)}')
PY
