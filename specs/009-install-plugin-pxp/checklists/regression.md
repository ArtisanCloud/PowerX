# Regression Checklist — Plugin Release & Debug

This checklist traces the automated regression suites back to the scenarios in
`docs/use_cases/_from_hub` and Phase 9–11 tasks. Execute via
`make regression-pxp` (default runs all suites) or filter with
`REGRESSION_FILTER=<Phase>`/`REGRESSION_FILTER=SCN-DEV-PLUGIN-DEBUG-001`.

| Suite / Command | Related Use Cases & Tasks | What It Verifies |
|-----------------|---------------------------|------------------|
| `make regression-pxp` → Phase 9 block | SCN-DEV-PLUGIN-INIT-001, SCN-DEV-PLUGIN-PUBLISH-001, Tasks T071–T075 | Compiles/tests plugin bootstrap/import services, release service scaffolding, CLI `px plugin *`, admin plugin_dev routes — ensures init/doctor/import pipelines stay healthy. |
| `make regression-pxp` → Phase 10 block | SCN-DEV-PLUGIN-DEBUG-001, SCN-DEV-PLUGIN-SANDBOX-VALIDATION-001, Tasks T074–T076a | Exercises `plugin_debug` host/diagnostics, sandbox orchestrator, admin sandbox routes, CLI `px host start` to prevent regressions in hot reload + sandbox validation workflow. |
| `make regression-pxp` → Phase 11 block | SCN-DEV-PLUGIN-VERSION-COMPAT-001, Tasks T077–T079 | Runs governance + compatibility services, admin `/internal/version/*`, CLI `px version *` to guard version scan/board/exception paths. |
| `go test ./tests/integration/dev_hotload` | SCN-PUBLISH-HUB-001 (Dev API Gateway), Tasks T081–T085 | Spins up in-memory Dev Hotload API，依次调用 `POST/GET/DELETE /api/internal/dev/plugins/*` + SSE `stream`，确保 CLI `--dev-api` 所依赖的 register/reload/terminate 流程与事件推送稳定。 |

Manual + E2E complements (execute when changing APIs / workflows):

1. `px-plugin dev --watch --host-api ...` against a running admin API (SCN-DEV-PLUGIN-DEBUG-001 acceptance step 1–3).  
2. Sandbox orchestrator smoke: `POST /internal/sandbox/test/run` with `suiteId=order-sync-regression` (UC-DEV-PLUGIN-SANDBOX-VALIDATION-001).  
3. Governance board UI smoke: open `/admin/plugin-release/governance` and verify multi-tenant cards for at least one tenant drift scenario.  
4. Marketplace/publish flow: `px publish create` + `px publish deploy` on staging tenant (SCN-DEV-PLUGIN-PUBLISH-001) — required before production cutover.  
5. Dev API Gateway smoke：在本地 CoreX 打开 `PX_DEV_PLUGIN_HOTLOAD=1` 后执行 `px-plugin dev --watch --dev-api http://localhost:8077/api/v1 ...`，观察 `~/.px-plugin/sessions/*.json` 与 `GET /api/internal/dev/plugins/stream`，验证 CLI 新模式与 Admin SSE 面板一致。

Document the evidence (logs, artifacts, Grafana screenshots) in each release train’s report to prove coverage for SCN-derived acceptance criteria.
