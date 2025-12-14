[2025-12-09T04:57:08Z] Starting plugin_release quickstart dry-run at 2025-12-09T04:57:08Z.
[2025-12-09T04:57:08Z] Running proto verification + database migrations skipped (handled via CI).
[2025-12-09T04:57:08Z] Executing targeted go test suite for plugin_release transports...
[2025-12-09T04:57:11Z] Go tests finished successfully.
[2025-12-09T04:57:11Z] CLI sanity: printing help for px publish.
[2025-12-09T04:57:11Z] px publish --help executed.

## Summary
- Start time: 2025-12-09T04:57:08Z
- Tests: go test ./internal/service/plugin_release/... ./internal/transport/{http,grpc}/plugin_release ./tests/contract/plugin_release ./tests/integration/plugin_release
- CLI: px publish --help

