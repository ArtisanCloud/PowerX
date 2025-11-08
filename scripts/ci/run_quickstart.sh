#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${REPO_ROOT}/backend/reports/plugin_release"
REPORT_FILE="${REPORT_DIR}/dry_run.md"
TS="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

mkdir -p "${REPORT_DIR}"
tmp_report="$(mktemp)"

log() {
  echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] $*" | tee -a "${tmp_report}"
}

log "Starting plugin_release quickstart dry-run at ${TS}."
log "Running proto verification + database migrations skipped (handled via CI)."

pushd "${REPO_ROOT}/backend" >/dev/null

log "Executing targeted go test suite for plugin_release transports..."
if go test ./internal/service/plugin_release/... \
  ./internal/transport/http/admin/plugin_release \
  ./internal/transport/http/openapi/plugin_release \
  ./internal/transport/grpc/plugin_release \
  ./tests/contract/plugin_release \
  ./tests/integration/plugin_release; then
  log "Go tests finished successfully."
else
  log "Go tests failed."
  popd >/dev/null
  cat "${tmp_report}" > "${REPORT_FILE}"
  exit 1
fi

popd >/dev/null

log "CLI sanity: printing help for powerx publish."
if command -v powerx >/dev/null 2>&1; then
  if powerx publish --help >/dev/null 2>&1; then
    log "powerx publish --help executed."
  else
    log "powerx publish --help failed."
  fi
else
  log "powerx binary not present in PATH, skipping CLI smoke."
fi

cat <<EOF >> "${tmp_report}"

## Summary
- Start time: ${TS}
- Tests: go test ./internal/service/plugin_release/... ./internal/transport/{http,grpc}/plugin_release ./tests/contract/plugin_release ./tests/integration/plugin_release
- CLI: powerx publish --help

EOF

mv "${tmp_report}" "${REPORT_FILE}"
log "Report written to ${REPORT_FILE}."
