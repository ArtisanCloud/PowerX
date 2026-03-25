#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
FEATURE_DIR="$ROOT_DIR/specs/025-powerx-docker-systemd"
TASKS_FILE="$FEATURE_DIR/tasks.md"

if [ ! -f "$TASKS_FILE" ]; then
  echo "[gate] tasks.md not found: $TASKS_FILE"
  exit 1
fi

incomplete_checklists=$(python - <<'PY' "$FEATURE_DIR/checklists"
import os,re,sys
base=sys.argv[1]
if not os.path.isdir(base):
    print(0); raise SystemExit
n=0
for fn in os.listdir(base):
    p=os.path.join(base,fn)
    if not os.path.isfile(p):
        continue
    for line in open(p,encoding='utf-8'):
        if re.search(r'^- \[ \]', line):
            n+=1
print(n)
PY
)

if [ "$incomplete_checklists" != "0" ]; then
  echo "[gate] checklists incomplete items: $incomplete_checklists"
  exit 1
fi

required_ids=(T023 T024 T025 T026 T038 T039 T046 T047 T048 T049 T065 T066 T085)
missing=0
for id in "${required_ids[@]}"; do
  if ! rg -q "^- \[X\] ${id} " "$TASKS_FILE"; then
    echo "[gate] required task not completed: ${id}"
    missing=1
  fi
done
if [ "$missing" -ne 0 ]; then
  exit 1
fi

echo "[gate] pre-release checks passed"
