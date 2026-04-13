#!/usr/bin/env bash
set -euo pipefail

POLICY_ID="${1:-}"
OUTPUT_PATH="${2:-}"
if [[ -z "${POLICY_ID}" || -z "${OUTPUT_PATH}" ]]; then
  echo "usage: $0 <policy_id> <output_path>" >&2
  exit 1
fi

SOURCE_DSN="${POWERX_OPS_BACKUP_SOURCE_DSN:-}"
if [[ -z "${SOURCE_DSN}" ]]; then
  SOURCE_DSN="${POWERX_DB_DSN:-}"
fi
if [[ -z "${SOURCE_DSN}" ]]; then
  echo "[backup-db] missing source dsn (POWERX_OPS_BACKUP_SOURCE_DSN/POWERX_DB_DSN)" >&2
  exit 1
fi
if ! command -v pg_dump >/dev/null 2>&1; then
  echo "[backup-db] pg_dump not found in PATH" >&2
  exit 127
fi

OUTPUT_DIR="$(dirname "${OUTPUT_PATH}")"
mkdir -p "${OUTPUT_DIR}"
TMP_FILE="${OUTPUT_PATH}.tmp.$$"
trap 'rm -f "${TMP_FILE}"' EXIT

echo "[backup-db] start policy=${POLICY_ID} output=${OUTPUT_PATH} at $(date -u +%FT%TZ)"
pg_dump \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file "${TMP_FILE}" \
  "${SOURCE_DSN}"

if [[ ! -s "${TMP_FILE}" ]]; then
  echo "[backup-db] dump file is empty: ${TMP_FILE}" >&2
  exit 1
fi

mv "${TMP_FILE}" "${OUTPUT_PATH}"
trap - EXIT
chmod 640 "${OUTPUT_PATH}" || true
SIZE_BYTES="$(wc -c < "${OUTPUT_PATH}" | tr -d ' ')"
echo "[backup-db] done policy=${POLICY_ID} bytes=${SIZE_BYTES} output=${OUTPUT_PATH}"
