# 任务列表：Knowledge Space Provisioning & Lifecycle Governance

**输入**：`/specs/011-knowledge-space/` 内的设计文档  
**前置依赖**：plan.md、spec.md、research.md、data-model.md、contracts/、quickstart.md

## 说明
- 任务格式：`[编号] [P?] [所属故事] 描述`
- `[P]` 代表可并行执行（不同文件、无依赖）
- 故事标签：`Setup`、`Foundational`、`US1`（Web 管理台配置向导）、`US2`（多模态入库基线）、`US3`（多源融合策略管理）、`US4`（反馈驱动再加工与热更新）、`Polish`
- 所有路径均为仓库内真实路径，确保可直接执行

---

## 阶段 1：Setup（共享基础）

- [X] **T001 [Setup]** 按 plan.md 创建后端目录骨架：`backend/internal/service/knowledge_space`、`backend/internal/transport/http/{admin,openapi}/knowledge_space`、`backend/internal/transport/grpc/knowledge_space`、`backend/tests/{contract,integration}/knowledge_space`，并放置最小化 Go/README 以保持编译通过。
- [X] **T002 [P] [Setup]** 在 `web-admin/app/pages/knowledge-spaces/index.vue` 建立入口页并更新导航配置，暴露“知识空间”列表及“创建”入口。
- [X] **T003 [Setup]** 在 `backend/api/grpc/contracts/buf.yaml`、`backend/api/grpc/contracts/buf.gen.yaml` 和 `Makefile` 中注册 `powerx/knowledge/v1` proto 包，确保 `proto-gen` 目标输出到 `api/grpc/gen`.

---

## 阶段 2：Foundational（阻断性前置）

> 所有用户故事开始前必须完成，涵盖模型、仓储、配置、依赖注入与 proto。

- [ ] **T004 [P] [Foundational]** 在 `backend/pkg/corex/db/persistence/model/knowledge/knowledge_space.go` 定义 `KnowledgeSpace` 模型，含租户级唯一约束、保留字段与审计列。
- [ ] **T005 [P] [Foundational]** 在 `.../policy_template_version.go` 定义策略模版版本模型。
- [ ] **T006 [P] [Foundational]** 在 `.../ingestion_job.go` 定义入库任务模型，记录重试计数与覆盖率指标。
- [ ] **T007 [P] [Foundational]** 在 `.../fusion_strategy_version.go` 定义融合策略版本模型。
- [ ] **T008 [P] [Foundational]** 在 `.../feedback_case.go` 定义反馈案例模型，携带 SLA 与匿名化字段。
- [ ] **T009 [P] [Foundational]** 在 `.../iam_sync_task.go` 定义 IAM 同步任务模型。
- [ ] **T010 [P] [Foundational]** 在 `.../audit_trail_entry.go` 定义审计轨迹模型。
- [ ] **T011 [Foundational]** 将上述模型注册到 `backend/pkg/corex/db/database/migration.go` 与 `backend/cmd/database/migrate.go`，包括索引与排序。
- [ ] **T012 [Foundational]** 在 `backend/pkg/corex/db/persistence/repository/knowledge/` 下实现各实体仓储（包含 KnowledgeSpace、PolicyTemplateVersion、IngestionJob、ArtifactBundle、FusionStrategyVersion、FeedbackCase、IAMSyncTask、AuditTrailEntry；继承 `BaseRepository`），提供 CRUD 与筛选接口。
- [ ] **T013 [Foundational]** 在 `backend/config/defaults.go`、`backend/etc/config.yaml`、`backend/config/config.go` 中新增 `knowledge_space` 配置段（SLA、保留期、Webhook 等）并完成校验。
- [ ] **T014 [Foundational]** 在 `backend/internal/service/knowledge_space/instrumentation/` 构建指标封装（OpenTelemetry），暴露 provisioning p95、ingestion 覆盖率、fusion rollback、feedback SLA 等指标。
- [ ] **T015 [Foundational]** 更新 `backend/internal/app/shared/deps.go`，注入 Redis key 前缀、事件总线、审计、通知依赖，供服务层使用。
- [ ] **T016 [Foundational]** 编写 `api/grpc/contracts/powerx/knowledge/v1/knowledge_space.proto`，包含配置/入库/融合/反馈 RPC，并执行 `make proto-gen` 生成代码。

**检查点**：核心模型、配置、依赖可用 → 可进入用户故事开发。

---

## 阶段 3：用户故事 US1（P1）— Web 管理台配置向导

**目标**：Nuxt4 向导收集租户/部门、策略、配额、IAM、告警信息，展示 SLA 指标与审计摘要，实现全闭环创建/更新/退役。  
**独立验证**：仅部署配置 API + 前端，完成创建流程并验证 IAM 待确认、审计记录与 SLA 计时。

### 测试（先于实现）

- [ ] **T017 [P] [US1]** 在 `backend/tests/contract/knowledge_space/provisioning_http_test.go` 编写 HTTP 合同测试（创建、更新、退役、冲突 409）。
- [ ] **T018 [P] [US1]** 在 `.../provisioning_grpc_test.go` 编写 gRPC 合同测试（Create/Update/Retire RPC）。
- [ ] **T019 [P] [US1]** 在 `backend/tests/integration/knowledge_space/provisioning_flow_test.go` 编写集成测试，覆盖创建 → IAM Pending → 激活，并模拟同一租户并发创建以验证锁/队列生效。
- [ ] **T020 [P] [US1]** 在 `web-admin/tests/e2e/knowledge-spaces.spec.ts` 使用 Playwright 覆盖多步骤向导（表单校验、IAM 待确认、成功提示）。

### 实现

- [ ] **T021 [US1]** 在 `backend/internal/service/knowledge_space/provisioning.go` 实现业务逻辑：租户内唯一校验、配额校验、基于 Redis/DB 的串行锁、IAM 任务、13 个月只读计划。
- [ ] **T022 [US1]** 在 `backend/internal/service/knowledge_space/audit_events.go` 实现审计/事件写入，发布 `knowledge.space.*` 事件。
- [ ] **T023 [US1]** 在 `backend/internal/transport/http/admin/knowledge_space/handlers.go` + `dto.go` 实现 HTTP Admin 处理器及请求校验。
- [ ] **T024 [US1]** 在 `backend/internal/transport/grpc/knowledge_space/service.go` 实现 gRPC 服务并注册到 `backend/internal/server/grpc/server.go`.
- [ ] **T025 [US1]** 在 `backend/internal/transport/http/openapi/knowledge_space/routes.go` 挂载 OpenAPI 路由并同步 `contracts/http-openapi.yaml`.
- [ ] **T026 [US1]** 在 `web-admin/app/pages/knowledge-spaces/create.vue` 及组件（`QuotaForm.vue`、`PolicySelector.vue`、`AuditPreview.vue`、`IamStatusBadge.vue`）实现多步骤向导。
- [ ] **T027 [US1]** 在 `web-admin/app/stores/knowledgeSpaces.ts` 与 `app/composables/useKnowledgeSpaces.ts` 建立 Pinia + 组合式 API，处理 SLA 计时与冲突提示。

**检查点**：配置向导闭环完成，IAM Pending 状态可视化。

---

## 阶段 4：用户故事 US2（P2）— 多模态入库基线

**目标**：统一 orchestrator 支持 PDF/Markdown/Excel/API 入库，保障 ≥95% 覆盖率与 100% 脱敏，自动重试并输出指标。  
**独立验证**：执行入库沙箱样例，查看 chunk/向量/图谱产物与指标，不依赖融合/反馈。

### 测试

- [ ] **T028 [P] [US2]** 在 `backend/tests/contract/knowledge_space/ingestion_http_test.go` 编写 HTTP 合同测试（正常、重试、脱敏阻断）。
- [ ] **T029 [P] [US2]** 在 `.../ingestion_grpc_test.go` 编写 gRPC 合同测试。
- [ ] **T030 [P] [US2]** 在 `backend/tests/integration/knowledge_space/ingestion_flow_test.go` 编写集成测试，模拟多源数据与事件上报，并断言双粒度 chunk（≈800/≈300 token）及覆盖率/嵌入/脱敏指标。
- [ ] **T031 [P] [US2]** 在 `web-admin/tests/unit/knowledge-spaces/ingestion.spec.ts` 使用 Vitest 覆盖入库触发组件。

### 实现

- [ ] **T032 [US2]** 在 `backend/internal/service/knowledge_space/ingestion_service.go` 实现 orchestrator、双粒度 chunk 构建（含 ArtifactBundle 写入）、重试策略、事件上报。
- [ ] **T033 [US2]** 在 `backend/internal/transport/http/admin/knowledge_space/ingestion_handlers.go` 实现 HTTP Handler + DTO 校验。
- [ ] **T034 [US2]** 在 `backend/internal/transport/grpc/knowledge_space/ingestion_service.go` 实现 gRPC Handler。
- [ ] **T035 [US2]** 在 `backend/internal/service/knowledge_space/ingestion_metrics.go` 输出监控指标并写入 `reports/_state/knowledge-spaces.json`.
- [ ] **T036 [US2]** 在 `web-admin/app/pages/knowledge-spaces/index.vue` 增加入库 CTA 与状态卡片，支持上传文件/API 配置与脱敏告警。

---

## 阶段 5：用户故事 US3（P2）— 多源融合策略管理

**目标**：配置 BM25 + 向量 + 图谱的融合策略，支持版本化、自动降级与 5 分钟内回滚。  
**独立验证**：通过示例问题验证准确率提升 ≥15%，模拟 API 故障触发降级与回滚。

### 测试

- [ ] **T037 [P] [US3]** 在 `backend/tests/contract/knowledge_space/fusion_http_test.go` 编写 HTTP 合同测试（发布、回滚、冲突队列）。
- [ ] **T038 [P] [US3]** 在 `.../fusion_grpc_test.go` 编写 gRPC 合同测试。
- [ ] **T039 [P] [US3]** 在 `backend/tests/integration/knowledge_space/fusion_strategy_flow_test.go` 验证发布→降级→回滚。
- [ ] **T040 [P] [US3]** 在 `web-admin/tests/e2e/knowledge-spaces-fusion.spec.ts` 覆盖权重调节、降级提示、回滚按钮。

### 实现

- [ ] **T041 [US3]** 在 `backend/internal/service/knowledge_space/fusion_service.go` 实现策略 CRUD、权重归一化、回滚令牌。
- [ ] **T042 [US3]** 在 `backend/internal/transport/http/admin/knowledge_space/fusion_handlers.go` 提供 HTTP 接口。
- [ ] **T043 [US3]** 在 `backend/internal/transport/grpc/knowledge_space/fusion_service.go` 提供 gRPC 接口及降级触发。
- [ ] **T044 [US3]** 在 `web-admin/app/pages/knowledge-spaces/fusion.vue` 构建策略管理界面，含冲突队列与缓存模式提示。
- [ ] **T045 [US3]** 添加 `scripts/fusion/rollback_strategy.mjs` 等运维脚本，并在后端 CLI/告警中接入 `fusion.source.failed`.

---

## 阶段 6：用户故事 US4（P3）— 反馈驱动再加工与热更新

**目标**：采集反馈、计算质量分、触发再加工、在 24 小时内热更新索引/图谱并留存审计。  
**独立验证**：提交反馈→生成再加工任务→成功热更新；若失败则回滚并升级告警。

### 测试

- [ ] **T046 [P] [US4]** 在 `backend/tests/contract/knowledge_space/feedback_http_test.go` 编写 HTTP 合同测试。
- [ ] **T047 [P] [US4]** 在 `.../feedback_grpc_test.go` 编写 gRPC 合同测试。
- [ ] **T048 [P] [US4]** 在 `backend/tests/integration/knowledge_space/feedback_loop_test.go` 验证反馈→再加工→热更新→失败回滚，并覆盖“针对已删除/退役空间的反馈被拒绝并提示迁移”场景。
- [ ] **T049 [P] [US4]** 在 `web-admin/tests/e2e/knowledge-spaces-feedback.spec.ts` 覆盖反馈看板、SLA 倒计时、升级流程。

### 实现

- [ ] **T050 [US4]** 在 `backend/internal/service/knowledge_space/feedback_service.go` 实现反馈接收、质量评分、PII 处理，并对退役/已删除空间的反馈进行拦截与指引。
- [ ] **T051 [US4]** 在 `backend/internal/transport/http/admin/knowledge_space/feedback_handlers.go` 实现 HTTP 接口。
- [ ] **T052 [US4]** 在 `backend/internal/transport/grpc/knowledge_space/feedback_service.go` 实现 gRPC 接口。
- [ ] **T053 [US4]** 在 `backend/internal/workflow/knowledge_space/reprocess_pipeline.go` 构建再加工与热更新编排（含回滚逻辑）。
- [ ] **T054 [US4]** 在 `web-admin/app/pages/knowledge-spaces/feedback.vue` 及相关组件实现反馈看板、SLA 徽章、升级弹窗。
- [ ] **T055 [US4]** 将反馈与再加工指标写入 Grafana 与 `backend/reports/_state/knowledge-spaces.json`。

---

## 阶段 7：Polish & Cross-Cutting

- [ ] **T056 [P] [Polish]** 更新 quickstart.md、README、Runbook，确保命令（npm、make、Grafana 看板）与最终实现一致。
- [ ] **T057 [Polish]** 进行性能 / 弹性验证（批量创建/入库、模拟融合 API 故障）并调整告警阈值。
- [ ] **T058 [Polish]** 按 quickstart 执行全链路冒烟（后端 + Nuxt + Playwright），并验证关键指标/告警 <5 分钟触发、`reports/_state/knowledge-spaces.json` / 审计日志完整性，输出报告供 QA / 发布使用。

---

## 依赖与执行顺序

- Setup → Foundational → 各用户故事 → Polish；Foundational 未完成前，任何用户故事不得开始。
- 每个故事内部遵循：合同测试 → 集成/E2E → 服务层 → 传输层 → 前端界面。
- US1 完成后，其余故事可按优先级并行推进，但需复用共享组件。

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
