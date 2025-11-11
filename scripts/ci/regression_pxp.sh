#!/usr/bin/env bash
# Regression harness for Plugin Release & Debug program (specs/009-install-plugin-pxp)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="${ROOT_DIR}/backend"

GO_TEST_FLAGS=${GO_TEST_FLAGS:-"-count=1"}

log() {
  printf "\n== %s ==\n" "$1"
}

run_go_suite() {
  local suite_name="$1"
  shift
  local packages=("$@")
  if [ ${#packages[@]} -eq 0 ]; then
    return
  fi
  log "$suite_name"
  (cd "${BACKEND_DIR}" && GOFLAGS="" go test ${GO_TEST_FLAGS} "${packages[@]}")
}

# ----------------------------------------------------------------------
# Suite definitions mapped to user scenarios / phases
# ----------------------------------------------------------------------

# Phase 9 — SCN-DEV-PLUGIN-INIT-001 / SCN-DEV-PLUGIN-PUBLISH-001
PHASE9_PKGS=(
  "./internal/service/plugin_bootstrap"
  "./internal/service/plugin_import"
  "./internal/service/plugin_release"
  "./cmd/px/commands/plugin"
  "./internal/transport/http/admin/plugin_dev"
)

# Phase 10 — SCN-DEV-PLUGIN-DEBUG-001
PHASE10_PKGS=(
  "./internal/service/plugin_debug/..."
  "./internal/service/plugin_sandbox/..."
  "./cmd/px/commands/host"
  "./internal/transport/http/admin/plugin_sandbox"
  "./internal/transport/http/admin/plugin_dev"
)

# Phase 11 — SCN-DEV-PLUGIN-VERSION-COMPAT-001
PHASE11_PKGS=(
  "./internal/service/plugin_governance"
  "./internal/service/plugin_compat"
  "./internal/transport/http/admin/version"
  "./cmd/px/commands/version"
)

# Allow callers to focus on a single suite via REGRESSION_FILTER
FILTER="${REGRESSION_FILTER:-}"

should_run() {
  local name="$1"
  if [[ -z "${FILTER}" ]]; then
    return 0
  fi
  [[ "${name}" == *"${FILTER}"* ]]
}

run_suite_if_needed() {
  local name="$1"
  shift
  if should_run "${name}"; then
    run_go_suite "${name}" "$@"
  else
    printf ">> Skipping %s (filtered)\n" "${name}"
  fi
}

run_suite_if_needed "Phase 9 · Init / Publish" "${PHASE9_PKGS[@]}"
run_suite_if_needed "Phase 10 · Debug / Sandbox" "${PHASE10_PKGS[@]}"
run_suite_if_needed "Phase 11 · Version Governance & Compat" "${PHASE11_PKGS[@]}"

printf "\n✅ Regression suites completed.\n"
