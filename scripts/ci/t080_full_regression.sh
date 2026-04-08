#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SKIP_E2E="${SKIP_E2E:-0}"
E2E_PROJECT="${E2E_PROJECT:-chromium}"

echo "==> [T080] Full regression start"
echo "    ROOT_DIR=${ROOT_DIR}"
echo "    SKIP_E2E=${SKIP_E2E}"
echo "    E2E_PROJECT=${E2E_PROJECT}"

echo "==> [T080] Run backend contract/integration suites (ops + setup)"
(
  cd "${ROOT_DIR}/backend"
  mkdir -p .gocache .gomodcache
  export GOCACHE="${PWD}/.gocache"
  export GOMODCACHE="${PWD}/.gomodcache"
  go test \
    ./tests/contract/ops \
    ./tests/integration/ops \
    ./tests/contract/system \
    ./tests/integration/system \
    -count=1
)

if [[ "${SKIP_E2E}" == "1" ]]; then
  echo "==> [T080] Skip web-admin E2E by SKIP_E2E=1"
  echo "==> [T080] Full regression passed (without E2E)"
  exit 0
fi

echo "==> [T080] Run web-admin E2E suites (ops + setup)"
(
  cd "${ROOT_DIR}/web-admin"
  node ./scripts/prepare-playwright.mjs
  NUXT_PUBLIC_E2E_SKIP_AUTH=true \
    NO_PROXY=127.0.0.1,localhost,::1 \
    no_proxy=127.0.0.1,localhost,::1 \
    npx playwright test tests/e2e/ops tests/e2e/setup/first-install.spec.ts --project="${E2E_PROJECT}"
)

echo "==> [T080] Full regression passed"

