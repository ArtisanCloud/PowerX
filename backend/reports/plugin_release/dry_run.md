[2025-11-07T07:01:38Z] Starting plugin_release quickstart dry-run at 2025-11-07T07:01:38Z.
[2025-11-07T07:01:38Z] Running proto verification + database migrations skipped (handled via CI).
[2025-11-07T07:01:38Z] Executing targeted go test suite for plugin_release transports...
[2025-11-07T07:01:39Z] Go tests finished successfully.
[2025-11-07T07:01:39Z] CLI sanity: printing help for px publish.
[2025-11-07T07:01:39Z] powerx binary not present in PATH, skipping CLI smoke.

## Summary
- Start time: 2025-11-07T07:01:38Z
- Tests: go test ./internal/service/plugin_release/... ./internal/transport/{http,grpc}/plugin_release ./tests/contract/plugin_release ./tests/integration/plugin_release
- CLI: px publish --help

