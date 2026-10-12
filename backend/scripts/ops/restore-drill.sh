#!/usr/bin/env bash
set -euo pipefail
umask 077
SOURCE_JOB_ID="${1:-}"
ARTIFACT_PATH="${2:-}"
if [[ ! "${SOURCE_JOB_ID}" =~ ^[0-9]+$ || ! -s "${ARTIFACT_PATH}" || -L "${ARTIFACT_PATH}" ]]; then
  echo "[restore-drill] invalid source job or artifact" >&2; exit 1
fi
if [[ -n "${POWERX_OPS_RESTORE_PROBE_DB:-}" || -n "${POWERX_OPS_RESTORE_PROBE_DB_PREFIX:-}" ]]; then
  echo "[restore-drill] custom target databases are forbidden; only a new isolated database is allowed" >&2; exit 1
fi
if [[ -z "${PGDATABASE:-}" || -z "${PGHOST:-}" || -z "${PGUSER:-}" ]]; then
  echo "[restore-drill] missing source connection; use the Core restore service" >&2; exit 1
fi
for tool in pg_restore createdb dropdb psql; do
  if ! command -v "${tool}" >/dev/null 2>&1; then echo "[restore-drill] missing ${tool}" >&2; exit 127; fi
done
KEEP_PROBE_DB="${POWERX_OPS_RESTORE_KEEP_DB:-0}"
if [[ "${KEEP_PROBE_DB}" != "0" && "${KEEP_PROBE_DB}" != "1" ]]; then echo "[restore-drill] keep flag must be 0 or 1" >&2; exit 1; fi
PROBE_DB_NAME="powerx_restore_probe_${SOURCE_JOB_ID}_$(date -u +%Y%m%d%H%M%S)_$$_${RANDOM}"
if [[ "${#PROBE_DB_NAME}" -gt 63 || "${PROBE_DB_NAME}" == "${PGDATABASE}" ]]; then echo "[restore-drill] unsafe target name" >&2; exit 1; fi
# createdb 是原子创建；同名库已经存在时必须失败，绝不先删除它。
pg_restore -l "${ARTIFACT_PATH}" >/dev/null
createdb --maintenance-db="${PGDATABASE}" "${PROBE_DB_NAME}"
cleanup() {
  if [[ "${KEEP_PROBE_DB}" == "0" ]]; then dropdb --maintenance-db="${PGDATABASE}" "${PROBE_DB_NAME}"; fi
}
trap cleanup EXIT
echo "[restore-drill] start source_job_id=${SOURCE_JOB_ID} db=${PROBE_DB_NAME} artifact=${ARTIFACT_PATH}"
pg_restore --exit-on-error --no-owner --no-privileges -d "${PROBE_DB_NAME}" "${ARTIFACT_PATH}"
TABLE_COUNT="$(psql -X -v ON_ERROR_STOP=1 -d "${PROBE_DB_NAME}" -Atqc "select count(*) from pg_catalog.pg_tables where schemaname not in ('pg_catalog','information_schema');")"
if [[ ! "${TABLE_COUNT}" =~ ^[0-9]+$ ]]; then echo "[restore-drill] invalid verification result" >&2; exit 1; fi
cleanup
trap - EXIT
echo "[restore-drill] done source_job_id=${SOURCE_JOB_ID} db=${PROBE_DB_NAME} tables=${TABLE_COUNT} keep_db=${KEEP_PROBE_DB} artifact=${ARTIFACT_PATH}"
