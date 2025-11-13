#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "${REPO_ROOT}"

echo "[agent-model-hub] running buf lint + proto gen"
make proto-lint proto-gen

pushd backend >/dev/null

go test \
  ./internal/tests/http/admin/agent/... \
  ./internal/tests/integration/agent_model_hub/...

popd >/dev/null

echo "[agent-model-hub] contract + integration tests completed"
