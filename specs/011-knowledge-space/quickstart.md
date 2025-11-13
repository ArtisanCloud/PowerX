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
Navigate to `/knowledge-spaces` for provisioning、`/knowledge-spaces/fusion` 管理融合策略、`/knowledge-spaces/feedback` 监控反馈闭环。使用 `.env` / `.env.local` 指向本地 API，组合式调用位于 `app/composables/useKnowledgeSpaces.ts`。

## 6. Execute tests
```bash
go test ./internal/service/knowledge_space/...
go test ./internal/transport/http/admin/knowledge_space/...
go test ./tests/contract/knowledge_space/...
go test ./tests/integration/knowledge_space/...
cd web-admin && npm run test:unit -- knowledge-spaces/ingestion.spec.ts
cd web-admin && npm run test:e2e -- --grep "knowledge-spaces"
cd web-admin && npm run test:e2e -- --grep "knowledge-spaces-fusion"
cd web-admin && npm run test:e2e -- --grep "knowledge-spaces-feedback"
```
Contract tests rely on `specs/011-docs-use-cases/contracts/http-openapi.yaml` and the gRPC proto in `api/grpc/contracts/powerx/knowledge/v1/`.

## 7. End-to-end smoke
1. Use the Nuxt Web Admin “Create Knowledge Space” wizard to submit a new space并确认 SLA + 审计徽章。
2. Trigger ingestion job via `POST /knowledge-spaces/{id}/ingestion-jobs`（或 UI CTA），使用 PDF/Excel 样本。
3. Publish a fusion strategy `POST /knowledge-spaces/{id}/fusion-strategies` 或 `/knowledge-spaces/fusion`，如需回滚执行 `node scripts/fusion/rollback_strategy.mjs <space> <strategy>`.
4. Submit feedback `POST /knowledge-spaces/{id}/feedback` 或 `/knowledge-spaces/feedback`，观察 SLA 倒计时与 `knowledge.feedback.reprocess` 事件。
5. 检查 `reports/_state/knowledge-spaces.json` 中 `ingestion` 与 `feedback` 节点均更新，Grafana `Knowledge Space` / `fusion-pipeline` / `feedback-loop` Dashboard 无红色告警。

更多运维/弹性细节见：
- [Knowledge Space Runbook](../../docs/guides/knowledge_space/runbook.md)
- [Perf & Resiliency Validation](../../docs/guides/knowledge_space/perf_validation.md)
- [Smoke Checklist](../../docs/guides/knowledge_space/smoke_checklist.md)
