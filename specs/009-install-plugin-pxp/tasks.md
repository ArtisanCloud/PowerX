# Tasks: Plugin Release & Marketplace Publishing Foundation

**Input**: `specs/001-install-plugin-pxp/`

## Format: `[ID] [P?] [Story] Description`
- `[P]` → safe to execute in parallel（不同文件、无顺序依赖）
- `[US#]` / `[FR#]` → 对应用户故事或关键需求，确保可追溯
- 描述中包含明确文件路径与依赖提示

---

## Phase 1: Setup（共享基线）

- [X] T001 确立 plugin_release 配置 schema 与默认 Feature Gate（编辑 `backend/config/schema/plugin_release.yaml`，更新 `backend/internal/bootstrap/app.go` / `backend/internal/app/shared/deps.go` 注册）
- [X] T002 注册 plugin_release proto 包至 Buf/Makefile 工具链（修改 `backend/api/grpc/contracts/buf.yaml`、`backend/api/grpc/contracts/buf.gen.yaml`、`backend/Makefile`）
- [X] T003 初始化 plugin_release 包结构与占位依赖（创建 `backend/internal/service/plugin_release/`、`backend/internal/transport/http/{admin,openapi}/plugin_release/`、`backend/internal/transport/grpc/plugin_release/`；在 `backend/internal/bootstrap/app.go` 与 `backend/internal/app/shared/deps.go` 中注册占位依赖）
- [X] T004 生成示例发布计划 `examples/release-plan.json`（放置 `specs/001-install-plugin-pxp/examples/`，用于 quickstart 与合同测试；在 README/Quickstart 引用）
- [X] T004a 生成 plugin_release gRPC 契约并执行 `make proto-gen`（`specs/001-install-plugin-pxp/contracts/plugin_release.proto` -> `api/grpc/gen/go/powerx/plugin_release/v1`，依赖 T002）
- [X] T004b 扩展 HTTP OpenAPI 契约覆盖本地热更新/审批/发布流程（`specs/001-install-plugin-pxp/contracts/http-openapi.yaml`，依赖 T003）

---

## Phase 2: Foundational（全局阻塞前置）

- [X] T005 准备持久化与 DI 脚手架（创建 `backend/pkg/corex/db/persistence/model/plugin_release/` 与 `repository/plugin_release/` 基础目录、更新 `backend/internal/app/shared/deps.go` 以暴露占位依赖）
- [X] T006 [P] 定义 PluginReleaseCandidate 模型与校验（`backend/pkg/corex/db/persistence/model/plugin_release/release_candidate.go`，依赖 T005）
- [X] T007 [P] 定义 ReleasePlan 模型含灰度批次 Schema（`backend/pkg/corex/db/persistence/model/plugin_release/release_plan.go`，依赖 T005）
- [X] T008 [P] 定义 CanaryDeploymentRecord 模型并加 GIN 索引（`backend/pkg/corex/db/persistence/model/plugin_release/canary_record.go`，依赖 T005）
- [X] T009 [P] 定义 OfflineDistributionPackage 模型（`backend/pkg/corex/db/persistence/model/plugin_release/offline_package.go`，依赖 T005）
- [X] T010 [P] 定义 MarketplaceListing 模型含升级字段（`backend/pkg/corex/db/persistence/model/plugin_release/marketplace_listing.go`，依赖 T005）
- [X] T011 [P] 定义 LocalInstallSession 模型支撑热更新（`backend/pkg/corex/db/persistence/model/plugin_release/local_install_session.go`，依赖 T005）
- [X] T012 将全部 plugin_release 模型加入 AutoMigrate（更新 `backend/pkg/corex/db/database/migration.go`，依赖 T006-T011）
- [X] T013 建立 `mv_plugin_release_status` 物化视图迁移 + 指标查询索引（新增迁移脚本或自动迁移钩子，位于 `backend/pkg/corex/db/migration/plugin_release_status.go`，依赖 T012）
- [X] T014 实现 `plugin_release_candidates` 月度分区与清理逻辑（扩展 `backend/pkg/corex/db/database/migration.go` 与定时脚本，依赖 T012）
- [X] T015 [P] 创建 ReleaseCandidateRepository（`backend/pkg/corex/db/persistence/repository/plugin_release/release_candidate_repository.go`，依赖 T006, T005）
- [X] T016 [P] 创建 ReleasePlanRepository（`backend/pkg/corex/db/persistence/repository/plugin_release/release_plan_repository.go`，依赖 T007-T008）
- [X] T017 [P] 创建 DistributionRepository（`backend/pkg/corex/db/persistence/repository/plugin_release/distribution_repository.go`，涵盖离线包 & Marketplace，依赖 T009-T010）
- [X] T018 [P] 创建 LocalInstallSessionRepository（`backend/pkg/corex/db/persistence/repository/plugin_release/local_install_session_repository.go`，依赖 T011）
- [X] T019 建立 plugin_release instrumentation 脚手架（`backend/internal/service/plugin_release/instrumentation/metrics.go`、`tracing.go`，依赖 T015-T018）
- [X] T020 组装 PluginReleaseService 工厂并注册 DI（新增 `backend/internal/service/plugin_release/service.go`，修改 `backend/internal/app/shared/deps.go`，依赖 T015-T019）
- [X] T021 脚手架 gRPC Server 并接入全局 server（`backend/internal/transport/grpc/plugin_release/server.go` 初始实现，更新 `backend/internal/server/grpc/server.go`，依赖 T020）
- [X] T022 建立 HTTP Admin/OpenAPI 路由占位（新增 `backend/internal/transport/http/admin/plugin_release/routes.go`、`openapi/plugin_release/routes.go`，更新上层 `routes.go`，依赖 T020）
- [X] T023 种子 CLI 命令骨架（创建 `backend/cmd/powerx/commands/publish/plugin_release_root.go` 并挂载至 `backend/cmd/powerx/commands/publish/root.go`，依赖 T020）

**Checkpoint**：模型、仓储、DI、观测与双传输骨架就绪。

---

## Phase 3: Local Hotload Debug Loop（FR-001）

### Tests（先写再实现）
- [X] T024 [P] [FR-001] gRPC 合同测试：本地安装/热更新流 (`backend/tests/contract/plugin_release/grpc_local_hotload_test.go`，依赖 T021)
- [X] T025 [P] [FR-001] OpenAPI 合同测试：租户本地导入入口 (`backend/tests/contract/plugin_release/http_openapi_local_install_test.go`，依赖 T022)
- [X] T026 [P] [FR-001] 集成测试：`px-plugin dev --watch` → Web Admin 热更新闭环 (`backend/tests/integration/plugin_release/test_local_hotload_flow_test.go`，依赖 T020)

### 实现
- [X] T027 [FR-001] 构建 LocalInstall service（签名/权限/缓存逻辑）于 `backend/internal/service/plugin_release/local/install_service.go`（依赖 T018, T020）
- [X] T028 [FR-001] 实现 OpenAPI Handler（启动、取消、日志查询）于 `backend/internal/transport/http/openapi/plugin_release/local_install_handler.go`（依赖 T027, T022）
- [X] T029 [FR-001] 扩展 gRPC Server：`StartLocalInstall`/`StreamHotReload` 双向流 (`backend/internal/transport/grpc/plugin_release/server.go`，依赖 T027, T021)
- [X] T030 [FR-001] 增强 CLI `px-plugin dev --watch` 推送与日志回传（更新 `backend/cmd/powerx/commands/plugin/dev_watch.go`，依赖 T023, T029）
- [X] T031 [FR-001] 增加热更新审计与日志聚合 (`backend/internal/service/plugin_release/local/audit_hooks.go`，依赖 T027, T019)

**Checkpoint**：FR-001 闭环可执行，CLI ↔ Web Admin 热更新成功。

---

## Phase 4: User Story 1 – End-to-end Release Guardrail（Priority P1）

### Tests
- [X] T032 [P] [US1] gRPC 合同测试：CreateReleaseCandidate / RunQualityGates / GenerateReleasePlan (`backend/tests/contract/plugin_release/grpc_release_guardrail_test.go`，依赖 T021)
- [X] T033 [P] [US1] HTTP Admin 合同测试：`/candidates` & `/plans` (`backend/tests/contract/plugin_release/http_admin_release_guardrail_test.go`，依赖 T022)
- [X] T034 [P] [US1] 集成测试：本地构建 → 流水线审批 → 发布计划 (`backend/tests/integration/plugin_release/test_release_guardrail_flow.go`，依赖 T020)

### 实现
- [X] T035 [US1] 实现流水线 orchestration（构建元数据、Workflow 调用、通知）`backend/internal/service/plugin_release/pipeline/service.go`（依赖 T015, T020, T027）
- [X] T036 [US1] 实现质量门禁执行器（覆盖率、安全、许可证）`backend/internal/service/plugin_release/pipeline/gate_runner.go`（依赖 T035）
- [X] T037 [US1] 实现 Admin HTTP Handler（候选与计划 CRUD）`backend/internal/transport/http/admin/plugin_release/release_guardrail_handler.go`（依赖 T035-T036, T022）
- [X] T038 [US1] 实现 gRPC 方法（CreateReleaseCandidate/RunQualityGates/GenerateReleasePlan）`backend/internal/transport/grpc/plugin_release/server.go`（依赖 T036-T037, T021）
- [X] T039 [US1] 加强 CLI `powerx publish create` 上传与提交逻辑（`backend/cmd/powerx/commands/publish/create.go`，依赖 T023, T035）
- [X] T040 [US1] 补充审计与失败通知链路 `backend/internal/service/plugin_release/pipeline/audit_hooks.go`（依赖 T035-T036, T019）

**Checkpoint**：US1 流水线具备门禁、审批与审计能力。

---

## Phase 5: User Story 2 – Controlled Production Rollout（Priority P2）

### Tests
- [X] T041 [P] [US2] gRPC 合同测试：TriggerCanary / FinalizeDeployment (`backend/tests/contract/plugin_release/grpc_canary_rollout_test.go`，依赖 T038)
- [X] T042 [P] [US2] HTTP Admin 合同测试：`/plans/{id}/deploy/*` (`backend/tests/contract/plugin_release/http_admin_canary_deploy_test.go`，依赖 T037)
- [X] T043 [P] [US2] 集成测试：灰度部署与 5 分钟自动回滚 (`backend/tests/integration/plugin_release/test_canary_rollback_flow.go`，依赖 T035)

### 实现
- [X] T044 [US2] 实现 runtime service（批次 orchestration、指标阈值判断）`backend/internal/service/plugin_release/runtime/service.go`（依赖 T035-T036, T019）
- [X] T045 [US2] 扩展指标采集与告警（Prometheus/Grafana 规则）`backend/internal/service/plugin_release/instrumentation/runtime_metrics.go`（依赖 T044, T019）
- [X] T046 [US2] 实现 Admin HTTP 部署 Handler (`backend/internal/transport/http/admin/plugin_release/deployment_handler.go`，依赖 T044, T022)
- [X] T047 [US2] 实现 gRPC TriggerCanary/FinalizeDeployment 流 (`backend/internal/transport/grpc/plugin_release/server.go`，依赖 T044, T038, T021)
- [X] T048 [US2] 更新 CLI `powerx publish deploy` 流式进度与回滚命令 (`backend/cmd/powerx/commands/publish/deploy.go`，依赖 T023, T047)
- [X] T049 [US2] 集成事件总线与回滚自动化钩子 (`backend/internal/service/plugin_release/runtime/event_hooks.go`，依赖 T044, T019)

**Checkpoint**：US2 灰度部署具备监控、告警、自动回滚能力。

---

## Phase 6: User Story 3 – Multi-channel Distribution & Marketplace Visibility（Priority P3）

### Tests
- [X] T050 [P] [US3] gRPC 合同测试：UploadOfflinePackage / SubmitMarketplaceListing / ImportOfflinePackage (`backend/tests/contract/plugin_release/grpc_distribution_test.go`，依赖 T047)
- [X] T051 [P] [US3] HTTP Admin 合同测试：离线上传与 Marketplace 审核 (`backend/tests/contract/plugin_release/http_admin_distribution_test.go`，依赖 T046)
- [X] T052 [P] [US3] 集成测试：离线包审核 → 租户导入闭环 (`backend/tests/integration/plugin_release/test_offline_distribution_flow.go`，依赖 T044)

### 实现
- [X] T053 [US3] 实现 distribution service（离线包入库、渠道分发）`backend/internal/service/plugin_release/distribution/service.go`（依赖 T017, T044, T019）
- [X] T054 [US3] 实现签名/许可证校验工具 `backend/internal/service/plugin_release/distribution/validator.go`（依赖 T053）
- [X] T055 [US3] 实现 Admin HTTP distribution handler（离线包与 Marketplace 升级）`backend/internal/transport/http/admin/plugin_release/distribution_handler.go`（依赖 T053-T054, T022）
- [X] T055a [US3] 实现 Marketplace 列表查询 API（分页/筛选）`backend/internal/transport/http/admin/plugin_release/distribution_handler.go:getMarketplaceListings`（依赖 T053-T055）
- [X] T056 [US3] 实现 OpenAPI Handler：企业租户离线导入 `backend/internal/transport/http/openapi/plugin_release/offline_import_handler.go`（依赖 T053-T054, T022）
- [X] T057 [US3] 扩展 gRPC Server：UploadOfflinePackage / SubmitMarketplaceListing / ImportOfflinePackage (`backend/internal/transport/grpc/plugin_release/server.go`，依赖 T053-T056, T047, T021)
- [X] T058 [US3] 更新 CLI：`powerx publish package --offline` 与 `powerx plugin import --offline` (`backend/cmd/powerx/commands/publish/package_offline.go`、`backend/cmd/powerx/commands/plugin/import_offline.go`，依赖 T023, T057)
- [X] T059 [US3] 实现补件升级与通知审计 `backend/internal/service/plugin_release/distribution/audit_hooks.go`（依赖 T053-T054, T019）

**Checkpoint**：US3 完成在线/离线渠道发布与合规治理。

---

## Phase 7: Polish & Cross-Cutting

- [X] T060 [P] 完成 Prometheus/Grafana 仪表盘与告警规则（`backend/internal/service/plugin_release/instrumentation/alerts.go`，`docs/guides/plugin_release/observability.md`，依赖 T045, T059）
- [X] T061 更新 Quickstart 与业务文档（`specs/001-install-plugin-pxp/quickstart.md`、`docs/use_cases/_from_hub/SCN-PUBLISH-HUB-001/*.md`，依赖 T030, T048, T058）
- [X] T062 运行 quickstart 演练，产出报告 (`scripts/ci/run_quickstart.sh` + `backend/reports/plugin_release/dry_run.md`，依赖 T031, T040, T049, T059)
- [X] T063 [P] 补充单元测试以达到覆盖率目标（`backend/internal/service/plugin_release/pipeline/service_test.go`、`runtime/service_test.go`、`distribution/service_test.go`、`local/install_service_test.go`，依赖 T027, T035, T044, T053）
- [X] T064 安全加固巡检（RBAC、审计保留、配置基线），覆盖 `backend/internal/service/plugin_release/*` 与 `backend/internal/transport/http/*`（依赖 T060-T063）

**Final Checkpoint**：准备进入 `/implement` 阶段，满足 Constitution 与 FR/NFR 要求。

---

## Phase 8: Admin Web UI – Marketplace Ops Console（Priority P3）

- [X] T065 设计并实现离线包入库页面，提供表单上传与结果列表（`web-admin/app/pages/plugin-release/OfflinePackages.vue`，调用 `POST /api/admin/plugin-release/offline-packages`，依赖 T055）
- [X] T066 开发 Marketplace 审核列表页，展示补件次数/SLA 并消费 `GET /api/admin/plugin-release/marketplace/listings`（`web-admin/app/pages/plugin-release/MarketplaceListings.vue`，依赖 T055a）
- [X] T067 实现审核详情与操作视图，调用 `POST /api/admin/plugin-release/marketplace/listings/:id/reviews` 并处理 need_fix/approved/rejected 结果（`web-admin/app/pages/plugin-release/ReviewDetail.vue`，依赖 T066）
- [X] T068 为三处页面补充 service 层与单元测试（`web-admin/app/services/pluginRelease.ts`，`web-admin/app/services/__tests__/pluginRelease.test.ts`），确保异常时展示后端返回的审计 reference（依赖 T065-T067）
- [X] T069 配置前端菜单与路由注册（`web-admin/app/services/menuConfig.ts`，提供菜单创建脚本和说明），将新页面挂载到管理员菜单（依赖 T065-T068）
- [X] T070 编写 E2E 测试覆盖提交→审核→回滚流程（`web-admin/cypress/e2e/plugin-release.cy.ts`，依赖 T065-T069）

**Checkpoint**：运营可在 Web Admin 内完成离线包登记与 Marketplace 审核，无需额外 CLI。
