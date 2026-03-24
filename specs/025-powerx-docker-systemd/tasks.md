# Tasks: PowerX 部署与运维治理基线

**Input**: Design documents from `/specs/025-powerx-docker-systemd/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

## Format: `[ID] [P?] [Story] Description`
- **[P]**: 可并行执行（不同文件且无直接依赖）
- **[Story]**: 对应用户故事（US1/US2/US3/US4）
- 每条任务都包含明确文件路径，保证可直接执行

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 初始化运维治理功能的目录、路由入口与测试骨架

- [X] T001 [P] [Shared] 创建后端运维域目录骨架：`backend/internal/service/{deploy_ops,backup_ops,observability_ops,migration_ops}`、`backend/internal/transport/http/admin/{deploy,backup,observability,migration}`、`backend/internal/transport/grpc/ops`
- [X] T002 [P] [Shared] 创建持久化目录骨架：`backend/pkg/corex/db/persistence/model/ops`、`backend/pkg/corex/db/persistence/repository/ops`
- [X] T003 [P] [Shared] 创建测试目录骨架：`backend/tests/contract/ops`、`backend/tests/integration/ops`、`web-admin/tests/e2e/ops`
- [X] T004 [Shared] 在 `backend/internal/http/router.go` 与相关 `api.go` 中预留 ops 管理路由挂载点（仅注册空路由组）
- [X] T005 [Shared] 在 `web-admin/app/pages/ops` 下创建页面占位：`deploy.vue`、`plugins.vue`、`backup.vue`、`migration.vue`
- [X] T081 [Shared] 创建 ops 域 DTO 骨架：`backend/internal/dto/ops/*.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 完成所有用户故事共享的数据层、审计、权限、契约与调度基础

**⚠️ CRITICAL**: 本阶段未完成前，不得进入任何用户故事实现

- [X] T006 [P] [Shared] 创建实体模型 `DeployReleaseRecord`：`backend/pkg/corex/db/persistence/model/ops/deploy_release_record.go`
- [X] T007 [P] [Shared] 创建实体模型 `PluginLifecycleAudit`：`backend/pkg/corex/db/persistence/model/ops/plugin_lifecycle_audit.go`
- [X] T008 [P] [Shared] 创建实体模型 `BackupPolicy`：`backend/pkg/corex/db/persistence/model/ops/backup_policy.go`
- [X] T009 [P] [Shared] 创建实体模型 `BackupJob`：`backend/pkg/corex/db/persistence/model/ops/backup_job.go`
- [X] T010 [P] [Shared] 创建实体模型 `BackupArtifact`：`backend/pkg/corex/db/persistence/model/ops/backup_artifact.go`
- [X] T011 [P] [Shared] 创建实体模型 `RestoreDrillRecord`：`backend/pkg/corex/db/persistence/model/ops/restore_drill_record.go`
- [X] T012 [P] [Shared] 创建实体模型 `ApprovalPolicyProfile`：`backend/pkg/corex/db/persistence/model/ops/approval_policy_profile.go`
- [X] T013 [Shared] 在 `backend/pkg/corex/db/database/migration.go` 挂载 ops 域模型迁移入口（遵循 MigrateCoreModels 约束）
- [X] T014 [P] [Shared] 为 ops 模型创建 Repository：`backend/pkg/corex/db/persistence/repository/ops/*.go`
- [X] T015 [Shared] 实现统一审批策略读取与校验中间层：`backend/internal/service/deploy_ops/approval_policy_service.go`
- [X] T016 [Shared] 实现统一审计写入器（部署/插件/备份/迁移复用）：`backend/internal/service/observability_ops/audit_writer.go`
- [X] T017 [Shared] 实现任务状态机与通用错误映射：`backend/internal/service/backup_ops/job_state_machine.go`
- [X] T018 [Shared] 接入脚本执行抽象（备份/清理/演练/迁移）：`backend/internal/service/backup_ops/script_runner.go`
- [X] T019 [Shared] 新增 gRPC 契约权威源：`backend/api/grpc/contracts/powerx/platform_ops/v1/ops_admin.proto`
- [X] T020 [Shared] 接入 Buf 生成链路并生成代码：`backend/api/grpc/contracts/{buf.yaml,buf.gen.yaml}` + `backend/api/grpc/gen/go/powerx/platform_ops/v1/*`
- [X] T021 [Shared] 补齐 proto 工具链目标：`Makefile` 中 `proto-gen` / `proto-lint` / `proto-clean`
- [X] T022 [Shared] 实现 Ops 域 RBAC 权限点与高风险操作鉴权中间件：`backend/internal/transport/http/admin/deploy/authorization.go` + `backend/internal/service/iam/rbac_service.go`

**Checkpoint**: Foundation 完成，US1/US2/US3/US4 可并行推进

---

## Phase 3: User Story 1 - 平台管理员完成双模式生产部署 (Priority: P1) 🎯 MVP

**Goal**: 提供部署发布中心，支持发布记录、健康聚合、回滚触发；同时落地 Docker/systemd 双模式部署资产

**Independent Test**: 通过 API + 页面完成一次发布记录查询与一次回滚触发，状态可追踪；分别验证 Docker/systemd 发布与回滚

### Tests for User Story 1 (TDD First)

- [X] T023 [P] [US1] 契约测试（HTTP OpenAPI: deploy 相关路径）`backend/tests/contract/ops/http_deploy_contract_test.go`（来源：`specs/025-powerx-docker-systemd/contracts/http-openapi.yaml`）
- [X] T024 [P] [US1] 契约测试（gRPC: deploy 相关 RPC）`backend/tests/contract/ops/grpc_deploy_contract_test.go`（来源：`backend/api/grpc/contracts/powerx/platform_ops/v1/ops_admin.proto`）
- [X] T025 [P] [US1] 集成测试：发布与回滚主流程 `backend/tests/integration/ops/deploy_release_flow_test.go`
- [X] T026 [P] [US1] 集成测试：双模式发布与回滚演练 `backend/tests/integration/ops/deploy_mode_parity_test.go`

### Implementation for User Story 1

- [X] T027 [US1] 实现 Deploy Service（发布/回滚/状态聚合）`backend/internal/service/deploy_ops/service.go`
- [X] T028 [US1] 实现 Deploy HTTP Handler：`backend/internal/transport/http/admin/deploy/handler.go`
- [X] T029 [US1] 注册 Deploy 路由：`backend/internal/transport/http/admin/deploy/routes.go`
- [X] T030 [US1] 实现 Deploy gRPC Handler：`backend/internal/transport/grpc/ops/deploy_handler.go`
- [X] T031 [US1] 在全局 gRPC server 挂载 Ops deploy 服务：`backend/internal/server/grpc/server.go`
- [X] T032 [US1] 前端 API Service：`web-admin/app/composables/api/services/deployOpsService.ts`
- [X] T033 [US1] 部署发布中心页面实现：`web-admin/app/pages/ops/deploy.vue`
- [X] T034 [US1] E2E：部署中心查询+回滚流程 `web-admin/tests/e2e/ops/deploy-center.spec.ts`
- [X] T035 [US1] 落地 Docker 生产部署资产：`deploy/powerx/docker/compose.prod.yaml` + `deploy/powerx/docker/.env.prod.example`
- [X] T036 [US1] 落地 systemd 生产部署资产：`deploy/powerx/systemd/{powerx-backend.service,powerx-runner.service,powerx-web-admin.service}`
- [X] T037 [US1] 落地健康检查与快速回滚脚本：`backend/scripts/ops/{deploy-check.sh,rollback-release.sh}`

**Checkpoint**: US1 可独立演示并满足 MVP

---

## Phase 4: User Story 2 - 运维人员完成插件无市场阶段平滑升级 (Priority: P1)

**Goal**: 提供插件生命周期中心，展示版本状态、执行切换/回滚、可审计

**Independent Test**: 页面触发一次插件版本切换并回滚，审计时间线可见

### Tests for User Story 2 (TDD First)

- [X] T038 [P] [US2] 集成测试：插件切换与回滚流程 `backend/tests/integration/ops/plugin_lifecycle_flow_test.go`
- [X] T039 [P] [US2] 契约测试：插件审计查询接口 `backend/tests/contract/ops/http_plugin_audit_contract_test.go`
- [X] T082 [P] [US2] 契约测试：插件生命周期 gRPC 接口 `backend/tests/contract/ops/grpc_plugin_lifecycle_contract_test.go`

### Implementation for User Story 2

- [X] T040 [US2] 实现插件生命周期审计服务：`backend/internal/service/deploy_ops/plugin_lifecycle_service.go`
- [X] T041 [US2] 实现插件生命周期 HTTP Handler：`backend/internal/transport/http/admin/deploy/plugin_lifecycle_handler.go`
- [X] T042 [US2] 前端 API Service：`web-admin/app/composables/api/services/pluginOpsService.ts`
- [X] T043 [US2] 插件生命周期中心页面：`web-admin/app/pages/ops/plugins.vue`
- [X] T044 [US2] 插件审计时间线组件：`web-admin/app/components/ops/plugins/PluginAuditTimeline.vue`
- [X] T045 [US2] E2E：插件切换+回滚+审计可见 `web-admin/tests/e2e/ops/plugin-lifecycle.spec.ts`
- [X] T083 [US2] 实现插件生命周期 gRPC Handler：`backend/internal/transport/grpc/ops/plugin_lifecycle_handler.go`
- [X] T084 [US2] 在全局 gRPC server 挂载插件生命周期服务：`backend/internal/server/grpc/server.go`

**Checkpoint**: US2 与 US1 可并行使用且互不阻塞

---

## Phase 5: User Story 3 - 运维负责人查看日志、备份与恢复状态 (Priority: P2)

**Goal**: 提供备份恢复中心与日志入口，支持策略、任务、演练闭环

**Independent Test**: 手动触发备份任务并可查询结果；恢复演练结果可展示；日志保留与告警状态可见

### Tests for User Story 3 (TDD First)

- [X] T046 [P] [US3] 契约测试（HTTP OpenAPI: backup 相关路径）`backend/tests/contract/ops/http_backup_contract_test.go`
- [X] T047 [P] [US3] 契约测试（gRPC: backup 相关 RPC）`backend/tests/contract/ops/grpc_backup_contract_test.go`
- [X] T048 [P] [US3] 集成测试：备份触发/清理/演练流程 `backend/tests/integration/ops/backup_restore_flow_test.go`
- [X] T049 [P] [US3] 集成测试：Loki 检索与 30 天保留策略状态校验 `backend/tests/integration/ops/logging_loki_retention_test.go`

### Implementation for User Story 3

- [X] T050 [US3] 实现备份策略服务：`backend/internal/service/backup_ops/policy_service.go`
- [X] T051 [US3] 实现备份任务服务：`backend/internal/service/backup_ops/job_service.go`
- [X] T052 [US3] 实现恢复演练服务：`backend/internal/service/backup_ops/restore_drill_service.go`
- [X] T053 [US3] 实现备份 HTTP Handler 与路由：`backend/internal/transport/http/admin/backup/{handler.go,routes.go}`
- [X] T054 [US3] 实现备份 gRPC Handler：`backend/internal/transport/grpc/ops/backup_handler.go`
- [X] T055 [US3] 挂载备份脚本到 `backend/scripts/ops/{backup-db.sh,cleanup-backups.sh,restore-drill.sh}` 并在服务层接入
- [X] T056 [US3] 前端 API Service：`web-admin/app/composables/api/services/backupOpsService.ts`
- [X] T057 [US3] 备份恢复中心页面：`web-admin/app/pages/ops/backup.vue`
- [X] T058 [US3] 备份任务列表组件：`web-admin/app/components/ops/backup/BackupJobTable.vue`
- [X] T059 [US3] 演练结果组件：`web-admin/app/components/ops/backup/RestoreDrillPanel.vue`
- [X] T060 [US3] 落地 Loki 30 天保留配置：`deploy/observability/loki/loki-config.yaml`
- [X] T061 [US3] 落地 Promtail 采集与标签规范：`deploy/observability/promtail/promtail-config.yaml`
- [X] T062 [US3] 落地 Grafana 数据源/仪表盘/告警规则：`deploy/observability/grafana/provisioning/{datasources,dashboards,alerting}/*`
- [X] T063 [US3] 日志观测面板与保留策略可视化：`web-admin/app/components/ops/backup/LogObservabilityPanel.vue`
- [X] T064 [US3] E2E：备份中心+日志可观测主流程 `web-admin/tests/e2e/ops/backup-center.spec.ts`

**Checkpoint**: US3 完成后，P0 三域闭环达成

---

## Phase 6: User Story 4 - 项目负责人完成 A->B 环境迁移 (Priority: P3)

**Goal**: 提供实例迁移 runbook、导入校验、切换与回切可追踪流程

**Independent Test**: 完成一次 A->B 演练迁移，含导出、导入、验收、切换、回切

### Tests for User Story 4 (TDD First)

- [ ] T065 [P] [US4] 集成测试：实例迁移主流程（导出/导入/校验/切换/回切）`backend/tests/integration/ops/instance_migration_flow_test.go`
- [ ] T066 [P] [US4] 契约测试：迁移执行记录查询与验收接口 `backend/tests/contract/ops/http_migration_contract_test.go`
- [ ] T085 [P] [US4] 契约测试：迁移管理 gRPC 接口 `backend/tests/contract/ops/grpc_migration_contract_test.go`

### Implementation for User Story 4

- [ ] T067 [US4] 实现迁移编排服务（区分 DB 迁移完成与实例迁移完成验收）`backend/internal/service/migration_ops/service.go`
- [ ] T068 [US4] 实现迁移 HTTP Handler 与路由：`backend/internal/transport/http/admin/migration/{handler.go,routes.go}`
- [ ] T069 [US4] 落地迁移脚本：`backend/scripts/ops/{export-instance.sh,import-instance.sh,verify-migration.sh,switch-traffic.sh,rollback-traffic.sh}`
- [ ] T070 [US4] 前端 API Service：`web-admin/app/composables/api/services/migrationOpsService.ts`
- [ ] T071 [US4] 迁移管理页面：`web-admin/app/pages/ops/migration.vue`
- [ ] T072 [US4] E2E：A->B 迁移演练路径 `web-admin/tests/e2e/ops/instance-migration.spec.ts`
- [ ] T086 [US4] 实现迁移 gRPC Handler 并挂载服务：`backend/internal/transport/grpc/ops/migration_handler.go` + `backend/internal/server/grpc/server.go`

**Checkpoint**: US4 低频高风险场景具备可演练、可回切、可审计能力

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 全局完善、质量门槛与文档回归

- [ ] T073 [P] [Polish] 日志与指标补齐（deploy/plugin/backup/migration 四域）：`backend/internal/service/{deploy_ops,backup_ops,observability_ops,migration_ops}/instrumentation/*.go`
- [ ] T074 [P] [Polish] 审批策略按环境可配置联调：`backend/internal/service/deploy_ops/approval_policy_service.go` + `web-admin/app/pages/ops/deploy.vue`
- [ ] T075 [P] [Polish] 前端 RBAC 权限点收敛（按钮级与页面级）：`web-admin/app/pages/ops/{deploy,plugins,backup,migration}.vue`
- [ ] T076 [Polish] 发布阻断脚本（验收清单未过禁止发布）：`backend/scripts/ops/pre-release-gate.sh` + `Makefile`
- [ ] T077 [Polish] 覆盖率与性能门禁（>=80% / p95<200ms）：`backend/scripts/ci/{coverage-gate.sh,perf-smoke.sh}`
- [ ] T078 [Polish] 运行 quickstart 验证并记录结果：`specs/025-powerx-docker-systemd/quickstart.md`
- [ ] T079 [Polish] 更新部署文档引用与运维手册交叉链接：`docs/plan/deploy/*.md`
- [ ] T080 [Polish] 全量回归：`go test` 合同/集成 + `web-admin` E2E 回归
- [ ] T087 [Polish] `trace_id` 贯通验收（页面操作 -> API -> 审计 -> 日志）`backend/tests/integration/ops/traceability_e2e_test.go` + `web-admin/tests/e2e/ops/traceability.spec.ts`

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 -> Phase 2 -> Phase 3/4/5/6 -> Phase 7
- User Stories 必须在 Phase 2 完成后开始
- US1/US2/US3/US4 可并行推进（团队资源允许时）

### User Story Dependencies

- **US1 (P1)**: 依赖 Foundational 完成，无需依赖其他用户故事
- **US2 (P1)**: 依赖 Foundational 完成，可与 US1 并行
- **US3 (P2)**: 依赖 Foundational 完成，可与 US1/US2 并行
- **US4 (P3)**: 依赖 Foundational 完成，可与 US1/US2/US3 并行

### Within Each User Story

- 先测试任务（契约/集成）再实现任务（TDD）
- 先 service 再 transport handler，再前端页面
- 跨层联调后再进入 E2E

---

## Parallel Opportunities

- Phase 1 中 T001/T002/T003 可并行
- Phase 2 中模型任务 T006~T012 可并行
- US1 契约与集成测试 T023~T026 可并行
- US2 测试 T038/T039/T082 可并行
- US3 测试 T046~T049 可并行
- US4 测试 T065/T066/T085 可并行
- US3 观测配置 T060/T061/T062 可并行

## Parallel Example

```bash
# Foundational: 并行创建 ops 域实体模型
Task: "T006 DeployReleaseRecord model"
Task: "T007 PluginLifecycleAudit model"
Task: "T008 BackupPolicy model"
Task: "T009 BackupJob model"
Task: "T010 BackupArtifact model"
Task: "T011 RestoreDrillRecord model"
Task: "T012 ApprovalPolicyProfile model"

# US1: 并行执行契约/集成测试定义
Task: "T023 HTTP deploy contract test"
Task: "T024 gRPC deploy contract test"
Task: "T025 deploy integration flow test"
Task: "T026 deploy mode parity test"
```

---

## Implementation Strategy

### MVP First (US1)

1. 完成 Setup + Foundational
2. 完成 US1（Deploy 中心 + Docker/systemd 资产）
3. 先验收发布/回滚闭环

### Incremental Delivery

1. US1 完成并验收
2. 并行推进 US2（插件生命周期中心）
3. 完成 US3（备份恢复 + 日志观测）
4. 完成 US4（实例迁移 runbook）
5. 进入全局 polish 与回归

### Notes

- 所有高风险操作必须带 `trace_id` 与审计记录
- 避免在同一文件上并行提交冲突任务
- 任务完成后按逻辑分组提交，确保可回滚
