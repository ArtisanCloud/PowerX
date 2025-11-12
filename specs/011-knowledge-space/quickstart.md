# Quickstart — Knowledge Space Provisioning & Lifecycle Governance

## 1. Prerequisites
- Go 1.24 toolchain + `buf` CLI + `powerx` CLI installed.
- Node 20 + npm (per constitution) for the Web Admin workspace.
- Local PostgreSQL + Redis instances (see `config/docker-compose.*`), plus MinIO for artifact staging.
- Feature flags enabled in `config/config.yaml`: `knowledge-space-v1`, `knowledge-ingestion`, `structured-ingestion`, `fusion.pipeline`, `feedback.loop`.

## 2. Generate contracts & mocks
```bash
make proto-gen
make proto-lint
```
OpenAPI contract lives at `specs/011-docs-use-cases/contracts/http-openapi.yaml`. Regenerate SDKs via `make api-http`.

## 3. Apply database migrations
```bash
go run ./cmd/database migrate --modules knowledge_space
```
This registers new models (KnowledgeSpace, PolicyTemplateVersion, etc.) inside the CoreX migration pipeline.

## 4. Run targeted services
```bash
go run ./cmd/server --modules knowledge_space
```
Ensure `internal/app/shared/deps.go` wiring includes Redis, EventBus, Audit, Telemetry for the new module.

## 5. Launch Web Admin workspace
```bash
cd web-admin
npm install
npm run dev
```
Navigate to `/knowledge-spaces` to access the provisioning wizard. Use `.env` or `.env.local` to point API calls at the local server; SDK/composable bindings reside in `app/services/knowledge-spaces/client.ts` and `app/composables/useKnowledgeSpaces.ts`.

## 6. Execute tests
```bash
go test ./internal/service/knowledge_space/...
go test ./internal/transport/http/admin/knowledge_space/...
go test ./tests/contract/knowledge_space/...
go test ./tests/integration/knowledge_space/...
cd web-admin && npm run test:unit
cd web-admin && npm run test:e2e -- --grep \"knowledge-spaces\"
```
Contract tests rely on `specs/011-docs-use-cases/contracts/http-openapi.yaml` and the gRPC proto in `api/grpc/contracts/powerx/knowledge/v1/`.

## 7. End-to-end smoke
1. Use the Nuxt Web Admin “Create Knowledge Space” wizard to submit a new space and confirm SLA + audit badges.
2. Trigger ingestion job via `POST /knowledge-spaces/{id}/ingestion-jobs` (or the UI CTA) using sandbox PDF + Excel fixtures.
3. Publish a fusion strategy `POST /knowledge-spaces/{id}/fusion-strategies` or the Fusion tab in Web Admin.
4. Submit feedback `POST /knowledge-spaces/{id}/feedback` or the Feedback board, then verify SLA timers plus reprocess job creation.

Monitor Grafana dashboards (`Knowledge Space`, `fusion-pipeline`, `feedback-loop`) and confirm `reports/_state/knowledge-spaces.json` updates after each step.
