# Quickstart Regression Checklist – 2025-11-11

| Step | Command / Action | Status | Notes |
|------|------------------|--------|-------|
| QK-08 | `./scripts/ci/agent_model_hub_tests.sh` | ✗ FAIL | Buf lint stopped at legacy protobuf packages (`powerx/agent/v1`, `powerx/integration_gateway/v1`, `powerx/plugin_release/v1`) because enum/request naming does not follow current lint rules. Test portion not executed. Log: `tmp/quickstart-regression.log`. |

## Artifacts

- `tmp/quickstart-regression.log` — full console output captured at 2025-11-11 15:05 local time for future reference.

## Follow-ups

- Coordinate with owners of `powerx/agent` 与 `powerx/integration_gateway` proto bundles to reconcile lint rules or pin per-package overrides so CI script can proceed to Agent Model Hub contract/integration tests. Current failures are unrelated to the Agent Model Hub feature but block the quickstart flow.
