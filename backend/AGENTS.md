# Repository Guidelines

## Project Structure & Module Organization

- `cmd/` — Binaries. Examples: `cmd/agent/`, `cmd/demo/`, `cmd/test_agent_api/`.
- `api/` — HTTP/gRPC endpoints and handlers.
- `internal/` — Application modules not intended for external reuse.
- `pkg/` — Reusable libraries and Makefile includes (`pkg/make_files/*.mk`).
- `config/` — Configuration loading, defaults, validation, secrets helpers.
- `domain/` — Core domain models and logic.
- `plugins/`, `extensions/` — Optional plugin implementations.
- `docs/` — API and code docs; `deploy/`, `scripts/`, `tools/` for ops and utilities.

## Build, Test, and Development Commands

- `make dev` — Run local demo server (logs to stdout). Env: `DEV_PORT=8077 LOG_LEVEL=debug`.
- `make build` — Build all binaries into `bin/` (`agent`, `demo`, tools).
- `make unit-test` — Run Go unit tests (`go test ./...`).
- `make test-all` — Exercise HTTP API via curl against running server.
- `make format && make vet` — Format and static analysis. Use `make check-all` for full lint if `golangci-lint` is installed.
- `make deps-tidy` — Sync `go.mod`/`go.sum`. `make docs-api` — Generate Swagger under `docs/api/` (requires `swag`).
- Optional: `make docker-build` / `docker-run` for container workflows.

## Coding Style & Naming Conventions

- Go 1.x. Format with `go fmt`; keep imports tidy. Prefer small packages; names are lowercase, no underscores.
- Files: snake_case; tests end with `_test.go`. Exported identifiers require doc comments.
- Lint: `go vet` required; `golangci-lint` recommended when available.

## Testing Guidelines

- Framework: standard `testing` package. Place tests alongside code: `pkg/foo/foo_test.go`.
- Run `make unit-test` before PRs; optional coverage via `make test-coverage` (outputs `reports/coverage.html`).
- API checks: ensure server is up (`make dev`) then `make test-all` or targeted `make test-health`.

## Commit & Pull Request Guidelines

- Use Conventional Commits where possible: `feat:`, `fix:`, `chore:`, `refactor:`, etc. Keep messages imperative and scoped (e.g., `feat(api): add chat stream`).
- PRs: include purpose, scope, screenshots/logs for API changes, and linked issues. Note config or schema changes.
- Pre-submit: `make format vet unit-test` must pass; update docs or examples if endpoints/config changed.

## Security & Configuration Tips

- Do not commit secrets. Use env vars (e.g., `LOG_LEVEL`, `DEV_PORT`) and provider creds via local env. See `config/` and `pkg/make_files/secret.mk` for helpers.
- Validate config with `make test-verify`; prefer `defaults.go` patterns for sane defaults.

# Communication

- 默认使用简体中文与我交流和解释。
- 生成代码时：注释、README、commit message 优先中文，必要时双语（中 + 英术语）。

# Style

- 终端命令给出可复制的一行版本；危险操作前先解释风险并征询确认。
