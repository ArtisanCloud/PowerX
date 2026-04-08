#!/usr/bin/env bash
set -euo pipefail

SOURCE_JOB_ID="${1:-}"
if [[ -z "${SOURCE_JOB_ID}" ]]; then
  echo "usage: $0 <source_job_id>" >&2
  exit 1
fi

echo "[restore-drill] start source_job_id=${SOURCE_JOB_ID} at $(date -u +%FT%TZ)"
# placeholder: restore drill command
sleep 0.1
echo "[restore-drill] done source_job_id=${SOURCE_JOB_ID}"
