#!/usr/bin/env bash
set -euo pipefail

SOURCE_JOB_ID="${1:-}"
ARTIFACT_PATH="${2:-}"
if [[ -z "${SOURCE_JOB_ID}" || -z "${ARTIFACT_PATH}" ]]; then
  echo "usage: $0 <source_job_id> <artifact_path>" >&2
  exit 1
fi

if [[ ! -f "${ARTIFACT_PATH}" ]]; then
  echo "[restore-drill] artifact not found: ${ARTIFACT_PATH}" >&2
  exit 1
fi
if ! command -v pg_restore >/dev/null 2>&1; then
  echo "[restore-drill] pg_restore not found in PATH" >&2
  exit 127
fi

PROBE_DB_PREFIX="${POWERX_OPS_RESTORE_PROBE_DB_PREFIX:-powerx_restore_probe}"
PROBE_DB_NAME="${POWERX_OPS_RESTORE_PROBE_DB:-${PROBE_DB_PREFIX}_${SOURCE_JOB_ID}}"
KEEP_PROBE_DB="${POWERX_OPS_RESTORE_KEEP_DB:-1}"

echo "[restore-drill] start source_job_id=${SOURCE_JOB_ID} db=${PROBE_DB_NAME} artifact=${ARTIFACT_PATH} at $(date -u +%FT%TZ)"

# 先验证 dump 目录可读，避免无效文件继续执行恢复。
pg_restore -l "${ARTIFACT_PATH}" >/dev/null

if ! command -v createdb >/dev/null 2>&1; then
  echo "[restore-drill] createdb not found in PATH" >&2
  exit 127
fi
if ! command -v dropdb >/dev/null 2>&1; then
  echo "[restore-drill] dropdb not found in PATH" >&2
  exit 127
fi
if ! command -v psql >/dev/null 2>&1; then
  echo "[restore-drill] psql not found in PATH" >&2
  exit 127
fi

# 每次演练覆盖同名 probe 库，保证幂等。
dropdb --if-exists "${PROBE_DB_NAME}" >/dev/null 2>&1 || true
createdb "${PROBE_DB_NAME}"

pg_restore \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  -d "${PROBE_DB_NAME}" \
  "${ARTIFACT_PATH}"

TABLE_COUNT="$(psql -d "${PROBE_DB_NAME}" -Atqc "select count(*) from pg_catalog.pg_tables where schemaname not in ('pg_catalog','information_schema');" | tr -d ' ')"
if [[ -z "${TABLE_COUNT}" ]]; then
  TABLE_COUNT="0"
fi

if [[ "${KEEP_PROBE_DB}" != "1" ]]; then
  dropdb --if-exists "${PROBE_DB_NAME}" >/dev/null 2>&1 || true
fi

echo "[restore-drill] done source_job_id=${SOURCE_JOB_ID} db=${PROBE_DB_NAME} tables=${TABLE_COUNT} keep_db=${KEEP_PROBE_DB}"
