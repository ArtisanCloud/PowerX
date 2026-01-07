# 任务列表：Knowledge Space Provisioning & Lifecycle Governance

**输入**：`/specs/011-knowledge-space/` 内的设计文档  
**前置依赖**：plan.md、spec.md、research.md、data-model.md、contracts/、quickstart.md

## 说明
- 任务格式：`[编号] [P?] [所属故事] 描述`
- `[P]` 代表可并行执行（不同文件、无依赖）
- 状态约定：`[ ]` = 待开发（含需按新方案重做/补齐），`[X]` = 已完成（与最新方案一致且可验收）
- 本任务清单需与以下方案对齐：`docs/plan/AI_engineering/knowledage_base.md`、`docs/plan/AI_engineering/rag.md`；若现有实现为 stub/占位或与方案不一致，应回退为“待开发”并更新描述。
- 故事标签：`Setup`、`Foundational`、`US1`（Web 管理台配置向导）、`US2`（多模态入库基线）、`US3`（多源融合策略管理）、`US4`（反馈驱动再加工与热更新 / SCN-KNOWLEDGE-UPDATE-FEEDBACK-001）、`US5`（QA 推理桥接）、`US6`（增量同步与版本治理 / SCN-KNOWLEDGE-UPDATE-SYNC-001）、`US7`（事件热更新 / SCN-KNOWLEDGE-UPDATE-EVENT-001）、`US8`（衰减巡检与空白治理 / SCN-KNOWLEDGE-UPDATE-DECAY-001）、`US9`（租户灰度发布 / SCN-KNOWLEDGE-UPDATE-TENANT-001）、`Polish`
- 所有路径均为仓库内真实路径，确保可直接执行

---

## 阶段 1：Setup（共享基础）

- [X] **T001 [Setup]** 按 plan.md 创建后端目录骨架：`backend/internal/service/knowledge_space`、`backend/internal/transport/http/{admin,openapi}/knowledge_space`、`backend/internal/transport/grpc/knowledge_space`、`backend/tests/{contract,integration}/knowledge_space`，并放置最小化 Go/README 以保持编译通过。
- [X] **T002 [P] [Setup]** 在 `web-admin/app/pages/knowledge-spaces/index.vue` 建立入口页并更新导航配置，暴露“知识空间”列表及“创建”入口。
- [X] **T003 [Setup]** 在 `backend/api/grpc/contracts/buf.yaml`、`backend/api/grpc/contracts/buf.gen.yaml` 和 `Makefile` 中注册 `powerx/knowledge/v1` proto 包，确保 `proto-gen` 目标输出到 `api/grpc/gen`.

---

## 阶段 2：Foundational（阻断性前置）

> 所有用户故事开始前必须完成，涵盖模型、仓储、配置、依赖注入与 proto。

- [X] **T004 [P] [Foundational]** 在 `backend/pkg/corex/db/persistence/model/knowledge/knowledge_space.go` 定义 `KnowledgeSpace` 模型，含租户级唯一约束、保留字段与审计列。
- [X] **T005 [P] [Foundational]** 在 `.../policy_template_version.go` 定义策略模版版本模型。
- [X] **T006 [P] [Foundational]** 在 `.../ingestion_job.go` 定义入库任务模型，记录重试计数与覆盖率指标。
- [X] **T007 [P] [Foundational]** 在 `.../fusion_strategy_version.go` 定义融合策略版本模型。
- [X] **T008 [P] [Foundational]** 在 `.../feedback_case.go` 定义反馈案例模型，携带 SLA 与匿名化字段。
- [X] **T009 [P] [Foundational]** 在 `.../iam_sync_task.go` 定义 IAM 同步任务模型。
- [X] **T010 [P] [Foundational]** 在 `.../audit_trail_entry.go` 定义审计轨迹模型。
- [X] **T011 [Foundational]** 将上述模型注册到 `backend/pkg/corex/db/database/migration.go` 与 `backend/cmd/database/migrate.go`，包括索引与排序。
- [X] **T012 [Foundational]** 在 `backend/pkg/corex/db/persistence/repository/knowledge/` 下实现各实体仓储（包含 KnowledgeSpace、PolicyTemplateVersion、IngestionJob、ArtifactBundle、FusionStrategyVersion、FeedbackCase、IAMSyncTask、AuditTrailEntry；继承 `BaseRepository`），提供 CRUD 与筛选接口。
- [X] **T012A [Foundational]** 在 `backend/pkg/corex/db/persistence/vectorstore/` 设计统一向量存储接口（抽象 CRUD、查询、批量 upsert、空间隔离等能力），并定义驱动注册/配置机制，供服务层按 driver 名称加载。
- [X] **T012B [Foundational]** 实现默认 `pgvector` 驱动：封装 `pgvector` schema/migration、连接池与批量写入 API，确保 `IngestionJob` / `ArtifactBundle` 可以将 embedding/chunk reference 交由驱动落地；同时预留 `milvus`, `pinecone` 等驱动骨架文件（空实现+TODO）以便后续扩展。
- [X] **T013 [Foundational]** 在 `backend/config/defaults.go`、`backend/etc/config.yaml`、`backend/config/config.go` 中新增 `knowledge_space` 配置段（SLA、保留期、Webhook 等）并完成校验。
- [X] **T014 [Foundational]** 在 `backend/internal/service/knowledge_space/instrumentation/` 构建指标封装（OpenTelemetry），暴露 provisioning p95、ingestion 覆盖率、fusion rollback、feedback SLA 等指标。
- [X] **T015 [Foundational]** 更新 `backend/internal/app/shared/deps.go`，注入 Redis key 前缀、事件总线、审计、通知依赖，供服务层使用。
- [X] **T016 [Foundational]** 编写 `api/grpc/contracts/powerx/knowledge/v1/knowledge_space.proto`，包含配置/入库/融合/反馈 RPC，并执行 `make proto-gen` 生成代码。

**检查点**：核心模型、配置、依赖可用 → 可进入用户故事开发。

---

## 阶段 3：用户故事 US1（P1）— Web 管理台配置向导

**目标**：Nuxt4 向导收集租户/部门、策略、配额、IAM、告警信息，展示 SLA 指标与审计摘要，实现全闭环创建/更新/退役。  
**独立验证**：仅部署配置 API + 前端，完成创建流程并验证 IAM 待确认、审计记录与 SLA 计时。

### 测试（先于实现）

- [X] **T017 [P] [US1]** 在 `backend/tests/contract/knowledge_space/provisioning_http_test.go` 编写 HTTP 合同测试（创建、更新、退役、冲突 409）。
- [X] **T018 [P] [US1]** 在 `.../provisioning_grpc_test.go` 编写 gRPC 合同测试（Create/Update/Retire RPC）。
- [X] **T019 [P] [US1]** 在 `backend/tests/integration/knowledge_space/provisioning_flow_test.go` 编写集成测试，覆盖创建 → IAM Pending → 激活，并模拟同一租户并发创建以验证锁/队列生效。
- [X] **T020 [P] [US1]** 在 `web-admin/tests/e2e/knowledge-spaces.spec.ts` 使用 Playwright 覆盖多步骤向导（表单校验、IAM 待确认、成功提示）。

### 实现

- [X] **T021 [US1]** 在 `backend/internal/service/knowledge_space/provisioning.go` 实现业务逻辑：租户内唯一校验、配额校验、基于 Redis/DB 的串行锁、IAM 任务、13 个月只读计划。
- [X] **T022 [US1]** 在 `backend/internal/service/knowledge_space/audit_events.go` 实现审计/事件写入，发布 `knowledge.space.*` 事件。
- [X] **T023 [US1]** 在 `backend/internal/transport/http/admin/knowledge_space/handlers.go` + `dto.go` 实现 HTTP Admin 处理器及请求校验。
- [X] **T024 [US1]** 在 `backend/internal/transport/grpc/knowledge_space/service.go` 实现 gRPC 服务并注册到 `backend/internal/server/grpc/server.go`.
- [X] **T025 [US1]** 在 `backend/internal/transport/http/openapi/knowledge_space/routes.go` 挂载 OpenAPI 路由并同步 `contracts/http-openapi.yaml`.
- [X] **T026 [US1]** 在 `web-admin/app/pages/knowledge-spaces/create.vue` 及组件（`QuotaForm.vue`、`PolicySelector.vue`、`AuditPreview.vue`、`IamStatusBadge.vue`）实现多步骤向导。
- [X] **T027 [US1]** 在 `web-admin/app/stores/knowledgeSpaces.ts` 与 `app/composables/useKnowledgeSpaces.ts` 建立 Pinia + 组合式 API，处理 SLA 计时与冲突提示。

**检查点**：配置向导闭环完成，IAM Pending 状态可视化。

---

## 阶段 4：用户故事 US2（P2）— 多模态入库基线

**目标**：统一 orchestrator 支持企业多格式入库（PDF/Word/Markdown/Excel/CSV/HTML/SQL/图片；可选音视频转写），并通过可插拔 Processor（如 OCR/格式转换插件）完成解析→切块→增强→向量化→多索引落库（dense+sparse+hier+kg），保障 ≥95% 覆盖率、embedding 成功率与 100% 脱敏（按策略要求），自动重试并输出指标。  
**独立验证**：执行入库沙箱样例，查看 chunk/向量/图谱产物与指标，不依赖融合/反馈。

### 测试

- [X] **T028 [P] [US2]** 在 `backend/tests/contract/knowledge_space/ingestion_http_test.go` 编写 HTTP 合同测试（正常、重试、脱敏阻断、OCR/Processor 降级/阻塞）。
- [X] **T029 [P] [US2]** 在 `.../ingestion_grpc_test.go` 编写 gRPC 合同测试（同上）。
- [X] **T030 [P] [US2]** 在 `backend/tests/integration/knowledge_space/ingestion_flow_test.go` 编写集成测试：覆盖至少 PDF（文本层+扫描）、Word、Excel/CSV、HTML、SQL；断言多粒度 chunk（summary/section/chunk）、coverage、embedding、masking、artifact bundle URI（MinIO/S3）与 checksum。
- [X] **T031 [P] [US2]** 在 `web-admin/tests/unit/knowledge-spaces/ingestion.spec.ts` 使用 Vitest 覆盖入库触发组件（含 Processor 状态提示与降级原因）。

### 实现

- [X] **T032 [US2]** 在 `backend/internal/service/knowledge_space/ingestion_service.go` 实现真实 orchestrator：Loader/Parser/Transformer/Masking/Embedding/多索引写入；产出 ArtifactBundle（MinIO/S3 URI + checksum），并支持 retry/blocked/degraded（对齐 `docs/plan/AI_engineering/rag.md` 的 OCR/Processor 策略）。
- [X] **T033 [US2]** 在 `backend/internal/transport/http/admin/knowledge_space/ingestion_handlers.go` 实现 HTTP Handler + DTO 校验（包含 format、processor_profile、ocr_required、masking_profile 等字段）。
- [X] **T034 [US2]** 在 `backend/internal/transport/grpc/knowledge_space/ingestion_service.go` 实现 gRPC Handler（同上）。
- [X] **T035 [US2]** 在 `backend/internal/service/knowledge_space/ingestion_metrics.go` 输出监控指标并写入 `reports/_state/knowledge-spaces.json`（覆盖率、embedding 成功率、OCR 覆盖/置信度分布、脱敏覆盖率、degrade/block 计数）。
- [X] **T036 [US2]** 在 `web-admin/app/pages/knowledge-spaces/index.vue` 增加入库 CTA 与状态卡片：支持选择 Ingestion Profile、显示 Processor/OCR 状态、blocked/degraded 原因与修复指引。
- [X] **T032A [US2]** 在 `ingestion_service.go` 中接入 `deps.KnowledgeSpace.VectorStore.Upsert`，将 embedding（chunk UUID + metadata）写入向量驱动，并在失败时执行补偿（回滚/告警/降级）。
- [X] **T032B [US2]** 为 ArtifactBundle 退役/清理流程调用 `VectorStore.DeleteByChunkIDs` / `DropSpace`，并同步清理 sparse/hier/kg 资产（如启用）。
- [X] **T032C [US2]** 新增 Processor Registry（接口固化在底座，具体实现可由插件提供）：支持 OCR/格式转换（推荐 `com.powerx.plugin.data_forge`），并定义 `ocr_required=true` 时的 blocked 行为与非强制时 degraded 行为（对齐 `docs/plan/AI_engineering/rag.md:363`）。
- [X] **T032D [US2]** 增加“多格式解析策略”：Word/HTML/邮件/IM/SQL/图片（OCR）/表格行级抽取，统一 provenance（page/row/bbox/line_range/timecode）写入 chunk metadata（对齐 `docs/plan/AI_engineering/rag.md:33`）。

---

## 阶段 5：用户故事 US3（P2）— 多源融合策略管理

**目标**：配置 dense+sparse（BM25/FTS）+ hier + KG 的融合策略（含 Query Routing / Time-aware 约束的可选项），支持版本化、自动降级与 5 分钟内回滚；并为后续 rerank/CRAG/Self-RAG 提供可解释的候选集。  
**独立验证**：通过示例问题验证准确率提升 ≥15%，模拟 API 故障触发降级与回滚。

### 测试

- [X] **T037 [P] [US3]** 在 `backend/tests/contract/knowledge_space/fusion_http_test.go` 编写 HTTP 合同测试（发布、回滚、冲突队列、降级原因）。
- [X] **T038 [P] [US3]** 在 `.../fusion_grpc_test.go` 编写 gRPC 合同测试。
- [X] **T039 [P] [US3]** 在 `backend/tests/integration/knowledge_space/fusion_strategy_flow_test.go` 验证发布→（多源召回）→降级→回滚（至少覆盖 vector+bM25；可选 KG/hier）。
- [X] **T040 [P] [US3]** 在 `web-admin/tests/e2e/knowledge-spaces-fusion.spec.ts` 覆盖权重调节、降级提示、回滚按钮。

### 实现

- [X] **T041 [US3]** 在 `backend/internal/service/knowledge_space/fusion_service.go` 实现策略 CRUD、权重归一化、回滚令牌，并扩展到 multi-source（vector+sparse+hier+kg）可选融合与归一化。
- [X] **T042 [US3]** 在 `backend/internal/transport/http/admin/knowledge_space/fusion_handlers.go` 提供 HTTP 接口。
- [X] **T043 [US3]** 在 `backend/internal/transport/grpc/knowledge_space/fusion_service.go` 提供 gRPC 接口及降级触发。
- [X] **T044 [US3]** 在 `web-admin/app/pages/knowledge-spaces/fusion.vue` 构建策略管理界面：权重编辑、冲突队列、降级原因、版本对比与回滚确认。
- [X] **T045 [US3]** 添加 `scripts/fusion/rollback_strategy.mjs` 等运维脚本，并在后端事件/告警中接入 `fusion.source.failed`（携带 space_id/strategy_id/degrade_reason/trace_id）。
- [X] **T043A [US3]** 在服务层检索路径对接 `VectorStore.Query` + `SparseIndex.Query` + `KG.Query`（可选）并输出可解释候选集（source、raw_score、normalized_score、provenance），为 rerank/CRAG/Self-RAG 提供输入。

---

## 阶段 6：用户故事 US4（P3）— 反馈驱动再加工与热更新

**目标**：采集反馈、计算质量分、触发再加工、在 24 小时内热更新索引/图谱并留存审计，对齐 `docs/use_cases/_from_hub/SCN-KNOWLEDGE-UPDATE-001/SCN-KNOWLEDGE-UPDATE-FEEDBACK-001.md` 中关于 +25% 准确率提升与闭环通知的要求。  
**独立验证**：提交反馈→生成再加工任务→成功热更新；若失败则回滚并升级告警。

### 测试

- [ ] **T046 [P] [US4]** 在 `backend/tests/contract/knowledge_space/feedback_http_test.go` 编写 HTTP 合同测试：提交/列表/关闭/升级、退役空间阻断、审计字段与 trace_id 回传。
- [ ] **T047 [P] [US4]** 在 `.../feedback_grpc_test.go` 编写 gRPC 合同测试（同上）。
- [ ] **T048 [P] [US4]** 在 `backend/tests/integration/knowledge_space/feedback_loop_test.go` 验证闭环：反馈→再加工（重分块/重向量化/更新 KG/更新 sparse/hier）→热更新发布→失败回滚；覆盖“针对已删除/退役空间的反馈被拒绝并提示迁移”。
- [ ] **T049 [P] [US4]** 在 `web-admin/tests/e2e/knowledge-spaces-feedback.spec.ts` 覆盖反馈看板、SLA 倒计时、升级流程、回滚按钮与告警提示。

### 实现

- [ ] **T050 [US4]** 在 `backend/internal/service/knowledge_space/feedback_service.go` 实现反馈接收、质量评分、PII 处理，并对退役/已删除空间的反馈进行拦截与指引；将 `trace_id`、候选引用（chunk_id/citation）与 feedback case 关联，支撑回放。
- [ ] **T051 [US4]** 在 `backend/internal/transport/http/admin/knowledge_space/feedback_handlers.go` 实现 HTTP 接口（含筛选/导出/升级/关闭）。
- [ ] **T052 [US4]** 在 `backend/internal/transport/grpc/knowledge_space/feedback_service.go` 实现 gRPC 接口（同上）。
- [ ] **T053 [US4]** 在 `backend/internal/workflow/knowledge_space/reprocess_pipeline.go` 实现再加工与热更新编排：按 case 绑定的 chunk/source 执行重分块/重向量化/多索引写入（dense+sparse+hier+kg），产出新 ArtifactBundle，并支持失败回滚到上一 bundle/策略版本。
- [ ] **T054 [US4]** 在 `web-admin/app/pages/knowledge-spaces/feedback.vue` 增强：case 详情（trace/citations）、一键 reprocess、回滚/升级、SLA 解释与通知记录。
- [ ] **T055 [US4]** 将反馈与再加工指标写入 Grafana 与 `backend/reports/_state/knowledge-spaces.json`（如存在）/聚合 `reports/_state/knowledge-update.json`，并补齐 `knowledge.feedback.*` 指标的告警阈值。
- [ ] **T055A [US4]** 在 `backend/internal/service/knowledge_space/feedback_metrics.go` 维护 `knowledge.feedback.{loop_time,fix_accuracy,auto_rate,backlog}` 指标：落盘 `backend/reports/_state/knowledge-feedback.json`，并更新聚合 `reports/_state/knowledge-update.json`（与 Ops 脚本一致）。
- [ ] **T055B [US4]** 在 `backend/config/knowledge/feedback_playbook.yaml`、`scripts/ops/knowledge-feedback-loop.mjs` 固化严重等级→SLA→处理路线与回归脚本；要求输出审计链路（audit-ledger）与可复现报告（含 trace_id/space_id/case_id）。

---

## 阶段 7：用户故事 US5（P1）— QA 推理桥接

**目标**：让 QA Orchestrator / Agent Dialogue 能在 2 秒内拿到跨知识空间检索计划、对话记忆差异、工具链元数据与合规审计钩子，满足 `SCN-KNOWLEDGE-QA-REASON-001` 全链路 KPI。  
**独立验证**：从 QA Orchestrator 沙箱发送多租户、带标签的检索请求，确认返回计划含 citation 覆盖 ≥95%、降级原因、IAM/敏感校验；触发多轮追问与工具 failover，并核对 `qa.*` 指标、`audit.reasoning_steps`、`reports/_state/qa-reasoning.json`。

### 测试

- [ ] **T059 [P] [US5]** 在 `backend/tests/contract/knowledge_space/qa_bridge_http_test.go` 覆盖 `POST /knowledge-spaces/qa/retrieval-plan`、`/memory-snapshot` 的 SLA、降级、越权/敏感阻断返回，并新增对 Routing/Time-aware/Policy snapshot 的断言。
- [ ] **T060 [P] [US5]** 在 `backend/tests/contract/knowledge_space/qa_bridge_grpc_test.go` 覆盖 gRPC Planner（多空间路由、工具元数据、failover、策略快照）。
- [ ] **T061 [US5]** 在 `backend/tests/integration/knowledge_space/qa_reasoning_flow_test.go` 演练 Agent Session → 检索计划（含策略快照）→ 工具调用 → failover → 反馈闭环；断言 ≤2 秒 SLA、≥99% 工具成功、≥95% 引用覆盖与审计写入。
- [ ] **T062 [P] [US5]** 在 `web-admin/tests/unit/knowledge-spaces/qa-bridge-card.spec.ts` 校验 `QaBridgeStatusCard.vue` 渲染 QA 指标、降级告警与审计链接。

### 实现

- [ ] **T063 [US5]** 在 `backend/internal/service/knowledge_space/qa_bridge/service.go` 实现可解释检索计划：输出 `rewrite/recall/fusion/rerank/compress` 的 plan（含 Routing/Time-aware/ACL 过滤）与降级原因，并记录策略快照（对齐 `docs/plan/AI_engineering/rag.md`）。
- [ ] **T064 [US5]** 在 `backend/internal/service/knowledge_space/context_snapshot/store.go` 构建 Redis 的记忆快照/差异存储，提供 `Snapshot/Upsert` API，并把引用映射与 trace_id 关联（用于反馈闭环）。
- [ ] **T065 [US5]** 在 `backend/internal/service/knowledge_space/toolchain/registry.go` & `executor.go` 注册工具元数据与执行器：封装 IAM/ACL 校验、重试、缓存降级；失败写入 `qa.failover.count` 并在 plan 中体现。
- [ ] **T066 [US5]** 在 `backend/internal/service/knowledge_space/compliance/hooks.go` 统一接入 `security.AccessCheck`、敏感检测、`audit.reasoning_steps`；将 `must_cite_sources`、`min_evidence_chunks` 等 guardrails 落到服务层（不可仅靠 prompt）。
- [ ] **T067 [US5]** 在 `backend/internal/transport/http/openapi/knowledge_space/qa_bridge_handlers.go` 与 `grpc/knowledge_space/qa_bridge_service.go` 暴露 QA Bridge API：包含 policy_version_snapshot、degrade_reason、citations 映射。
- [ ] **T068 [US5]** 在 `backend/reports/_state/qa-reasoning.json` & Grafana 面板写入 `qa.retrieval.latency_ms`, `qa.cross_space.hit_rate`, `qa.tool.success_rate`, `qa.feedback.loop_time`, `qa.citation.coverage_pct`，并在 `web-admin/app/components/knowledge-spaces/QaBridgeStatusCard.vue` + `app/services/knowledge-spaces/qaBridgeClient.ts` 展示健康状态。

### 追加：RAG 策略产品化（Profile + Playground + Corpus Check）

- [ ] **T101 [US5]** 定义并落库三类 Profile：`IngestionProfile`、`IndexProfile`、`RAGProfile`（可版本化/可回滚），并与 `KnowledgeSpace` 绑定默认 profile（对齐 `docs/plan/AI_engineering/rag.md:251`）。
- [ ] **T102 [US5]** 实现 `Corpus Check`（语料体检）作业：统计格式占比、OCR 占比、表格/代码占比、语言分布、重复率，并输出推荐策略卡片（规则集）与成本/风险提示（对齐 `docs/plan/AI_engineering/rag.md:301`）。
- [ ] **T103 [US5]** 增加 `Retrieval Playground` API：给定 `space_id + rag_profile_id + query + filters` 返回 `RetrievalPlan + candidates + context_pack + trace_id`。
- [ ] **T104 [US5]** 在 `web-admin/app/pages/knowledge-spaces/playground.vue` 新增 Playground：支持选择 profile、A/B 对比（默认 vs 草稿）、展示各阶段耗时/候选数/降级原因、候选来源（vector/bm25/kg/hier）与最终 citations。
- [ ] **T105 [US5]** 在 `web-admin/app/pages/knowledge-spaces/create.vue` 向导中增加“场景模板/默认策略”选择，并在导入样本文档后触发 Corpus Check，引导用户采用推荐组合（默认式 + 引导式）。
- [ ] **T106 [US5]** 将 OCR/Processor 能力与 UI 串联：当 Corpus Check 检测到扫描占比高时提示启用 OCR 扩展（推荐 `com.powerx.plugin.data_forge`），并在 blocked/degraded 时给出修复指引（对齐 `docs/plan/AI_engineering/rag.md:363`）。

---

## 阶段 8：用户故事 US6（P1）— 增量同步与版本治理

**目标**：实现 `docs/use_cases/_from_hub/SCN-KNOWLEDGE-UPDATE-001/SCN-KNOWLEDGE-UPDATE-SYNC-001.md` 中的增量抓取、差异报告、审批、部分发布、回滚闭环，确保 ≤30 分钟 SLA、≥98% 差异准确率、全量审计与 `knowledge.delta.*` 指标。  
**独立验证**：通过 `scripts/ops/knowledge-delta-job.mjs` 生成增量包，跑审批→部分发布→回滚→审计流程，并核对 `backend/reports/_state/knowledge-delta.json`。
**约束**：开启 `PX_KNOWLEDGE_DELTA_SYNC`、`PX_KNOWLEDGE_VERSIONED_STORAGE`、`PX_KNOWLEDGE_PARTIAL_RELEASE` flag，所有 embedding/向量读写依赖 `backend/pkg/corex/db/persistence/vectorstore` 多驱动注册表，禁止在 delta service 内重复实现驱动；审批 & 回滚动作必须写入 `audit-ledger` 且更新 `reports/_state/knowledge-update.json` 聚合。

### 测试

- [ ] **T069 [P] [US6]** 在 `backend/tests/contract/knowledge_space/delta_http_test.go` 覆盖 `POST /knowledge/delta/jobs`、`GET /knowledge/delta/reports/:id`、`POST /knowledge/delta/publish`、`POST /knowledge/version/rollback` 的成功、冲突、部分发布、审计分支（含策略快照与 bundle lineage）。
- [ ] **T070 [P] [US6]** 在 `backend/tests/contract/knowledge_space/delta_grpc_test.go` 覆盖对应 RPC 接口与 SLA 断言。
- [ ] **T071 [US6]** 在 `backend/tests/integration/knowledge_space/delta_sync_flow_test.go` 演练多源抓取→diff（chunk级）→审批→部分发布→回滚；校验差异准确率 ≥98%、≤30 分钟 SLA 与 `knowledge.delta.*` 指标写入。

### 实现

- [ ] **T072 [US6]** 在 `backend/internal/service/knowledge_space/delta/service.go` 实现 orchestrator：抓取/拆包、chunk 级 diff、审批、版本落地（ArtifactBundle lineage）、部分发布、回滚命令；并写入 `audit-ledger` 与聚合 `reports/_state/knowledge-update.json`。
- [ ] **T073 [US6]** 在 `backend/internal/transport/http/admin/knowledge_space/delta_handlers.go` 实现 HTTP Handler，支持审批签名、payload hash 校验、幂等键。
- [ ] **T074 [US6]** 在 `backend/internal/transport/grpc/knowledge_space/delta_service.go` 实现 gRPC Handler 与 Stream 报告输出。
- [ ] **T075 [US6]** 校验并增强 `scripts/ops/knowledge-delta-job.mjs`、`scripts/ops/knowledge-diff-report.mjs`：支持 dry-run、拆包、回滚 drill；输出与服务端一致的 report schema。
- [ ] **T076 [US6]** 对齐配置：`backend/config/knowledge/delta_sources.yaml`、`backend/config/knowledge/partial_release.yaml`；更新 `backend/etc/config.yaml`、`backend/config/config.go` 校验逻辑与 feature flag 依赖。
- [ ] **T077 [US6]** 在 `backend/internal/service/knowledge_space/instrumentation/delta_metrics.go` 输出 `knowledge.delta.{sla,approval_time,diff_accuracy,rollback_count,partial_release}`，生成 `backend/reports/_state/knowledge-delta.json` 并更新 Grafana《Knowledge Delta Sync》。

---

## 阶段 9：用户故事 US7（P1）— 事件热更新与 Agent 通知

**目标**：落实 `SCN-KNOWLEDGE-UPDATE-EVENT-001.md` 的事件订阅、策略匹配、≤5 分钟热修与 Agent 权重刷新，具备幂等控制与失败回放脚本。  
**独立验证**：向事件总线注入法规/价格事件，观察 `knowledge.event.latency ≤5m`、重复事件被幂等跳过、Agent 权重成功刷新并记录审计。
**约束**：启用 `PX_KNOWLEDGE_EVENT_HOTFIX`、`PX_KNOWLEDGE_EVENT_IDEMPOTENT`、`PX_AGENT_WEIGHT_REFRESH` flag，对所有事件执行签名校验与幂等键去重；HTTP/gRPC/CLI 需复用现有 vectorstore/agent 通知依赖，输出 `backend/reports/_state/knowledge-event.json` 并回填 `reports/_state/knowledge-update.json`。

### 测试

- [ ] **T078 [P] [US7]** 在 `backend/tests/contract/knowledge_space/event_http_test.go` 覆盖 `POST /knowledge/events/apply`、`POST /knowledge/events/retry`、`POST /knowledge/index/hot-update`、`POST /agent/weights/refresh`（含签名校验/幂等/重放窗口）。
- [ ] **T079 [P] [US7]** 在 `backend/tests/contract/knowledge_space/event_grpc_test.go` 覆盖 gRPC 事件处理接口与幂等键冲突。
- [ ] **T080 [US7]** 在 `backend/tests/integration/knowledge_space/event_hotfix_flow_test.go` 模拟事件→策略→热修（bundle/索引更新）→Agent 通知→失败重试→幂等忽略，并断言 ≤5m latency 与审计写入。

### 实现

- [ ] **T081 [US7]** 在 `backend/internal/service/knowledge_space/event_hotfix/service.go` 实现事件 intake、策略匹配、热更新（对索引/ArtifactBundle 产生可追溯变更）、幂等/重试控制与 `audit-ledger` 写入；支持 YAML/JSON policy 解析一致。
- [ ] **T082 [US7]** 在 `backend/internal/transport/http/admin/knowledge_space/event_handlers.go` 实现 HTTP Handler，校验事件签名与 payload schema，并把幂等结果/审计 ID 返回给调用方。
- [ ] **T083 [US7]** 在 `backend/internal/transport/grpc/knowledge_space/event_service.go` 实现 gRPC Handler + 订阅注册，注入事件总线。
- [ ] **T084 [US7]** 在 `backend/internal/service/knowledge_space/event_hotfix/agent_notifier.go` 实现真正的 Agent 刷新动作（权重/模板/路由缓存），并将结果写入 `agent.refresh.success_rate` 与审计。
- [ ] **T085 [US7]** 校验并增强 `backend/config/knowledge/event_hotfix_policies.yaml`、`backend/config/knowledge/agent_weight_matrix.yaml`、`scripts/ops/knowledge-event-replay.mjs`；生成 `backend/reports/_state/knowledge-event.json` 并更新 Grafana《Event Hotfix》。

---

## 阶段 10：用户故事 US8（P2）— 衰减巡检与空白治理

**目标**：根据 `SCN-KNOWLEDGE-UPDATE-DECAY-001.md` 建立 100% 覆盖的巡检、空白识别、任务派发、误判恢复（≤10 分钟）与 7 天 SLA 的补齐流程。  
**独立验证**：运行 `scripts/ops/knowledge-decay-scan.mjs`，确认 `knowledge.decay.*` 指标、任务、恢复、`backend/reports/_state/knowledge-decay.json` 与 `reports/_state/knowledge-update.json` 更新。
**约束**：Flag `PX_KNOWLEDGE_DECAY_GUARD`、`PX_KNOWLEDGE_GAP_ALERT`、`PX_KNOWLEDGE_RESTORE_FLOW` 必须在 CI/ops 场景可控；巡检阈值读取 `backend/config/knowledge/decay_thresholds.yaml`，任务派发复用 `task-center` / 审批流程，恢复路径必须记录审批人、误判理由并写入审计。

### 测试

- [ ] **T086 [P] [US8]** 在 `backend/tests/contract/knowledge_space/decay_http_test.go` 覆盖 `POST /knowledge/decay/tasks`、`POST /knowledge/decay/restore`、`GET /knowledge/decay/status`，含租户隔离、误判恢复 ≤10 分钟、`knowledge.decay.*` 指标写入断言。
- [ ] **T087 [US8]** 在 `backend/tests/integration/knowledge_space/decay_guard_flow_test.go` 演练巡检→任务→补齐→误判撤回，验证 7 天 SLA 计算、false-positive <10% 告警与 `task-center` 审批联动。

### 实现

- [ ] **T088 [US8]** 在 `backend/internal/service/knowledge_space/decay_guard/service.go` 实现巡检调度、阈值计算、任务派发、恢复/误判处理、audit 记录，并复用 `task-center`/`audit-ledger`/`vectorstore` 依赖注入模式，确保 `reports/_state/knowledge-update.json` 聚合更新。
- [ ] **T089 [US8]** 在 `backend/internal/transport/http/admin/knowledge_space/decay_handlers.go` 实现 HTTP API（含严重度/租户过滤、批量导出、flag 校验），返回任务 ID、SLA 倒计时与 `knowledge.decay` 指标片段。
- [ ] **T090 [US8]** 在 `backend/internal/transport/grpc/knowledge_space/decay_service.go` 实现 gRPC API，供任务中心与 Workflow 调用，包括 Run/List/Restore 方法及租户隔离校验。
- [ ] **T091 [US8]** 校验并增强 `scripts/ops/knowledge-decay-scan.mjs`，并补齐 `docs/ops/gap_task_template.md`：任务模板、审批字段、恢复/误判剧本、dry-run 与报告导出。
- [ ] **T092 [US8]** 对齐 `backend/config/knowledge/decay_thresholds.yaml`，生成/更新 `backend/reports/_state/knowledge-decay.json`，输出 `knowledge.decay.{detected,false_positive,gap_backlog,fill_time}` 并更新聚合 `knowledge-update.json`。

---

## 阶段 11：用户故事 US9（P1）— 租户灰度发布与治理

**目标**：落实 `SCN-KNOWLEDGE-UPDATE-TENANT-001.md` 的租户策略、灰度排期、指标监控、自动扩散/回滚、审计追踪，保障跨租户隔离。  
**独立验证**：配置 `backend/config/knowledge/tenant_release_matrix.yaml`，通过 Web Admin + CLI 完成试点→扩散→指标异常→回滚流程，并核对 `backend/reports/_state/knowledge-release.json` 与聚合 `reports/_state/knowledge-update.json` 的版本轨迹。
**约束**：`PX_KNOWLEDGE_GRAY_RELEASE`、`PX_TENANT_RELEASE_MATRIX`、`PX_KNOWLEDGE_RELEASE_GUARD` flag 必须可控；发布策略需写入/导出 `release_guardrails.md`，所有扩散/暂停/回滚动作写 `audit-ledger` 并推送 IM 通知；指标与 CLI/脚本复用共享依赖（version-store、notifications、metrics-gateway），不得重复实现监控或嵌套版本存储逻辑。

### 测试

- [ ] **T093 [P] [US9]** 在 `backend/tests/contract/knowledge_space/release_http_test.go` 覆盖 `POST /knowledge/release/policies`、`POST /knowledge/release/publish`、`POST /knowledge/release/promote`、`POST /knowledge/release/rollback`，含策略冲突、租户隔离、指标未达标触发 `release.gray.alert` 的分支。
- [ ] **T094 [P] [US9]** 在 `backend/tests/contract/knowledge_space/release_grpc_test.go` 覆盖 gRPC 接口，断言审批 ID、滚动窗口策略、批次 token，以及版本 drift ≤1 的守卫。
- [ ] **T095 [US9]** 在 `backend/tests/integration/knowledge_space/tenant_release_flow_test.go` 演练试点→扩散→指标异常→自动暂停→回滚→审计报告，校验 `knowledge.release.*` 指标、IM 通知与 `knowledge-release.json` 快照。

### 实现

- [ ] **T096 [US9]** 在 `backend/internal/service/knowledge_space/tenant_release/service.go` 实现策略管理、灰度调度、扩散/暂停/回滚状态机、audit 写入，并输出 `knowledge.release.*` 指标（含 drift、alerts）。
- [ ] **T097 [US9]** 在 `backend/internal/transport/http/admin/knowledge_space/tenant_release_handlers.go` 实现 HTTP Handler；补齐 `web-admin/app/pages/knowledge-spaces/release.vue`（当前缺失）用于展示策略、指标、批次推进与回滚按钮。
- [ ] **T098 [US9]** 在 `backend/internal/transport/grpc/knowledge_space/tenant_release_service.go` 实现 gRPC API，供 CLI/Workflow 调用，返回批次 token、版本号、租户覆盖率。
- [ ] **T099 [US9]** 校验并增强 `cmd/knowledge/release.go`（如缺失则新增）与 `scripts/ops/knowledge-release-matrix.mjs`：支持策略校验、批次推进、报告导出，并引用同一配置/Flag 管理。
- [ ] **T100 [US9]** 对齐 `backend/config/knowledge/tenant_release_matrix.yaml`、补齐 `release_guardrails.md`；生成 `backend/reports/_state/knowledge-release.json` 并写入 `reports/_state/knowledge-update.json` 聚合，同时记录 version drift 报表供审计。

---

## 阶段 12：Polish & Cross-Cutting

- [ ] **T056 [P] [Polish]** 更新 quickstart.md、README、Runbook，确保命令（npm、make、Grafana 看板）与最终实现一致，并补齐新引入的 Profile/Playground/OCR 插件化说明。
- [ ] **T057 [Polish]** 进行性能 / 弹性验证：批量创建/入库、多索引融合、OCR/Processor 降级、事件热修、回滚演练；并调整告警阈值与 SLO。
- [ ] **T058 [Polish]** 按 quickstart 执行全链路冒烟（后端 + Nuxt + Playwright），并验证关键指标/告警 <5 分钟触发、`reports/_state/*` 与审计日志完整性，输出报告供 QA / 发布使用。

---

## 依赖与执行顺序

- Setup → Foundational → US1 → US2/US3/US4（可并行但依赖共享模型）→ US5（需要 US2–US4 的数据/策略/反馈能力）→ US6（复用 US2/US3/US4 的模型与审计）→ US7（依赖 US6 的版本/指标）→ US8（依赖 US6/US7 的监控与任务数据）→ US9（依赖 US6–US8 的版本与告警) → Polish；Foundational 未完成前，任何用户故事不得开始。
- 每个故事内部遵循：合同测试 → 集成/E2E → 服务层 → 传输层 → 前端界面。
- US1 完成后，其余故事可按优先级并行推进；US5 需等 US2–US4 的 API/指标稳定后再启动，US6 需要 US2/US3/US4/US5 的实体与告警落位，US7 依赖 US6 的版本/监控，US8 依赖 US6/US7 的数据资产，US9 依赖 US6–US8 的指标/审计能力，以避免重复实现。

### 并行示例

```bash
# 并行创建模型
/task run T004 &
/task run T005 &
/task run T006 &
/task run T007 &
/task run T008 &
/task run T009 &
/task run T010 &
wait

# 并行编写 US1 合同测试
/task run T017 &
/task run T018 &
wait
```

确保所有任务严格遵循依赖关系，保持每个用户故事可独立测试与交付。
