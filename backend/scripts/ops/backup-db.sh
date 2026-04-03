#!/usr/bin/env bash
set -euo pipefail

POLICY_ID="${1:-}"
if [[ -z "${POLICY_ID}" ]]; then
  echo "usage: $0 <policy_id>" >&2
  exit 1
fi

echo "[backup-db] start policy=${POLICY_ID} at $(date -u +%FT%TZ)"
# placeholder: pg_dump / snapshot command
sleep 0.1
echo "[backup-db] done policy=${POLICY_ID}"
