# Quickstart — Knowledge Space Provisioning & Lifecycle Governance

## 1. Prerequisites
- Go 1.24+ toolchain + `buf` CLI + `powerx` CLI installed.
- Node 20 + npm (per constitution) for the Web Admin workspace.
- Local PostgreSQL + Redis instances (see `config/docker-compose.*`), plus MinIO for artifact staging.
- 确认 `backend/etc/config.yaml` 已配置可用的 DB/Redis，并启用 `feature_gate.enable_knowledge_space: true`。
- 如需验收 PDF 入库的“真实正文”与/或扫描件 OCR，请先安装系统依赖并配置开关（见 `docs/guides/deploy/knowledge_pdf_ocr.md:1`）。

### 1.1 本地（macOS）可选安装

```bash
brew install poppler tesseract
# 如缺少中文简体模型（chi_sim），再安装：
brew install tesseract-lang
```

### 1.2 配置开关（config.yaml）

`backend/etc/config.yaml`：

```yaml
knowledge_space:
  ingestion_processors:
    pdf_text_available: null # true/false/null(自动探测)
    ocr_available: null      # true/false/null(自动探测)
```

## 2. Generate contracts & mocks
```bash
make proto-gen
make proto-lint
```
OpenAPI contract lives at `specs/011-knowledge-space/contracts/http-openapi.yaml`.

## 3. Apply database migrations
```bash
make db-migrate
make db-seed
```
This registers new models (KnowledgeSpace, PolicyTemplateVersion, etc.) inside the CoreX migration pipeline.

### 3.1 Vector store (pgvector) & KG assist tables

若需要在本地直接看到并使用向量表（默认 `public.knowledge_vectors`）与 KG 协助表（`public.knowledge_kg_nodes` / `public.knowledge_kg_edges`），`make db-migrate` 需要包含相应迁移（幂等、可重复执行）。

#### 3.1.1 Embedding 默认值与模型选择

- 默认安装会配置 `ai.defaults.embedding` 为 OpenAI `text-embedding-3-small`（1536 维）；需要你在 Web Admin 的 **AI Settings**（或 `config.yaml`）里补齐 `api_key` 才会真正生成语义向量。
- 若未配置 embedding（例如缺少 api_key / 未设置 active profile），入库会被阻断并提示 `embedding_not_configured`（请先在 AI Settings 完成配置与测试）。
- 维度必须对齐：`knowledge_space.vector_store.pgvector.dimensions` 必须等于 embedding 模型输出维度（例如 OpenAI `text-embedding-3-small` 为 1536）。不一致时会在入库任务里报 `embedding_dim_mismatch` 并提示如何修复。

规格与 DDL 说明：
- `specs/011-knowledge-space/db-migrations.md`

同理，若你启用了 `index.sparse`（hybrid/BM25/FTS）或 `index.hier`（层次化检索/邻接扩展）并选择 Postgres-backed 实现，则应一并创建 `public.knowledge_chunks`（以及可选 `public.knowledge_chunk_links`）。

## 4. Run targeted services
```bash
cd backend
go run ./cmd/app
```
Ensure `backend/internal/app/shared/deps.go` wiring includes Redis, EventBus, Audit, Telemetry for the new module.

## 5. Launch Web Admin workspace
```bash
cd web-admin
npm install
npm run dev
```
默认端口：
- Admin API：`http://127.0.0.1:8077/api/v1/admin`
- Web Admin：`http://127.0.0.1:3030`

常用页面：
- `/knowledge-spaces`：空间总览（入库 / Playground / 策略 / 数据源入口）
- `/knowledge-spaces/create`：创建空间
- `/knowledge-spaces/strategy`：策略包（A0–O）配置 + 依赖校验 + 场景适配说明 + Corpus Check 推荐
- `/knowledge-spaces/playground`：Retrieval Playground（Profile A/B 对比）
- `/knowledge-spaces/release`：租户灰度发布
- `/knowledge-spaces/:spaceId/sources`：连接数据源（Notion/飞书等鉴权接入的占位入口）

组合式调用位于 `web-admin/app/composables/useKnowledgeSpaces.ts`。

## 6. Execute tests
```bash
cd backend
go test ./tests/contract/knowledge_space/...
go test ./tests/integration/knowledge_space/...
cd web-admin && npm run test:unit -- tests/unit/knowledge-spaces/ingestion.spec.ts
cd web-admin && npm run test:e2e -- --grep "knowledge-spaces"
cd web-admin && npm run test:e2e -- --grep "knowledge-spaces-fusion"
cd web-admin && npm run test:e2e -- --grep "knowledge-spaces-feedback"
```
Contract tests rely on `specs/011-knowledge-space/contracts/http-openapi.yaml` and the gRPC proto in `backend/api/grpc/contracts/powerx/knowledge/v1/`.

## 7. End-to-end smoke
1. Use the Nuxt Web Admin “Create Knowledge Space” wizard to submit a new space并确认 SLA + 审计徽章。
2. Trigger ingestion job via `POST /api/v1/admin/knowledge-spaces/{spaceId}/ingestion-jobs`（或 UI CTA），使用 PDF/Excel 样本。
   - 入库完成后会自动触发一次 Corpus Check（推荐场景/策略包与成本/风险提示），可在 `/knowledge-spaces/strategy` 查看与一键应用。
   - 若提示需要 OCR：建议安装 `com.powerx.plugin.data_forge`，或在入库高级设置中启用/关闭 `OCR required`。
   - 验收入库质量（切块预览/编辑）：
     - UI（推荐）：入库记录 ` /knowledge-spaces/{spaceId}/ingestions` → 切块预览/编辑 ` /knowledge-spaces/{spaceId}/ingestions/{jobId}`
     - API：`GET /api/v1/admin/knowledge-spaces/{spaceId}/ingestion-jobs?limit=20`、`GET /api/v1/admin/knowledge-spaces/{spaceId}/ingestion-jobs/{jobId}/chunks?page=1&pageSize=50`、`PATCH /api/v1/admin/knowledge-spaces/{spaceId}/ingestion-jobs/{jobId}/chunks/{chunkId}`、`GET /api/v1/admin/knowledge-spaces/{spaceId}/ingestion-jobs/{jobId}/pages/{pageNumber}/image`（bbox 叠框预览用）
   - WS 进度验证（可选）：打开 DevTools → Network → WS，确认 `/ws` 或 `/api/ws` 为 101，保持切块预览页打开，进度条应实时更新；断网后应回退轮询，恢复后继续推送。
3. Publish a fusion strategy `POST /knowledge-spaces/{id}/fusion-strategies` 或 `/knowledge-spaces/fusion`，如需回滚执行 `node scripts/fusion/rollback_strategy.mjs <space> <strategy>`.
4. Submit feedback `POST /knowledge-spaces/{id}/feedback` 或 `/knowledge-spaces/feedback`，观察 SLA 倒计时与 `knowledge.feedback.reprocess` 事件。
5. 运行 US6–US9 的 ops 脚本（可选但建议）：
   - `node scripts/ops/knowledge-delta-job.mjs --space=<space> --source=default`
   - `node scripts/ops/knowledge-event-replay.mjs`
   - `node scripts/ops/knowledge-decay-scan.mjs --dry-run --space=<space> --detected=3`
   - `node scripts/ops/knowledge-release-matrix.mjs --matrix=backend/config/knowledge/tenant_release_matrix.yaml`
6. 检查 `reports/_state/knowledge-spaces.json`、`reports/_state/knowledge-update.json` 的相关段落更新；并确认 Grafana `Knowledge Space` / `fusion-pipeline` / `feedback-loop` / `Knowledge Delta Sync` / `Event Hotfix` / `Knowledge Decay Monitor` / `Tenant Release Control` 无红色告警。

更多运维/弹性细节见：
- [Knowledge Space Runbook](../../docs/guides/knowledge_space/runbook.md)
- [Perf & Resiliency Validation](../../docs/guides/knowledge_space/perf_validation.md)
- [Smoke Checklist](../../docs/guides/knowledge_space/smoke_checklist.md)

策略设计参考（策略包 → 场景映射）：
- [RAG Strategy Modules](../../docs/plan/ai_engineering/knowledge/rag.md)
- [Strategy Package → Scene Mapping](../../docs/plan/ai_engineering/knowledge/rag_scene_strategy_mode.md)
