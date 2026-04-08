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

- [X] T065 [P] [US4] 集成测试：实例迁移主流程（导出/导入/校验/切换/回切）`backend/tests/integration/ops/instance_migration_flow_test.go`
- [X] T066 [P] [US4] 契约测试：迁移执行记录查询与验收接口 `backend/tests/contract/ops/http_migration_contract_test.go`
- [X] T085 [P] [US4] 契约测试：迁移管理 gRPC 接口 `backend/tests/contract/ops/grpc_migration_contract_test.go`

### Implementation for User Story 4

- [X] T067 [US4] 实现迁移编排服务（区分 DB 迁移完成与实例迁移完成验收）`backend/internal/service/migration_ops/service.go`
- [X] T068 [US4] 实现迁移 HTTP Handler 与路由：`backend/internal/transport/http/admin/migration/{handler.go,routes.go}`
- [X] T069 [US4] 落地迁移脚本：`backend/scripts/ops/{export-instance.sh,import-instance.sh,verify-migration.sh,switch-traffic.sh,rollback-traffic.sh}`
- [X] T070 [US4] 前端 API Service：`web-admin/app/composables/api/services/migrationOpsService.ts`
- [X] T071 [US4] 迁移管理页面：`web-admin/app/pages/ops/migration.vue`
- [X] T072 [US4] E2E：A->B 迁移演练路径 `web-admin/tests/e2e/ops/instance-migration.spec.ts`
- [X] T086 [US4] 实现迁移 gRPC Handler 并挂载服务：`backend/internal/transport/grpc/ops/migration_handler.go` + `backend/internal/server/grpc/server.go`

**Checkpoint**: US4 低频高风险场景具备可演练、可回切、可审计能力

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 全局完善、质量门槛与文档回归

- [X] T073 [P] [Polish] 日志与指标补齐（deploy/plugin/backup/migration 四域）：`backend/internal/service/{deploy_ops,backup_ops,observability_ops,migration_ops}/instrumentation/*.go`
- [X] T074 [P] [Polish] 审批策略按环境可配置联调：`backend/internal/service/deploy_ops/approval_policy_service.go` + `web-admin/app/pages/ops/deploy.vue`
- [X] T075 [P] [Polish] 前端 RBAC 权限点收敛（按钮级与页面级）：`web-admin/app/pages/ops/{deploy,plugins,backup,migration}.vue`
- [X] T076 [Polish] 发布阻断脚本（验收清单未过禁止发布）：`backend/scripts/ops/pre-release-gate.sh` + `Makefile`
- [X] T077 [Polish] 覆盖率与性能门禁（>=80% / p95<200ms）：`backend/scripts/ci/{coverage-gate.sh,perf-smoke.sh}`
- [X] T078 [Polish] 运行 quickstart 验证并记录结果：`specs/025-powerx-docker-systemd/quickstart.md`
- [X] T079 [Polish] 更新部署文档引用与运维手册交叉链接：`docs/plan/deploy/*.md`
- [ ] T080 [Polish] 全量回归：`go test` 合同/集成 + `web-admin` E2E 回归（backend 回归已通过，待本机执行 E2E 后勾选）
- [X] T087 [Polish] `trace_id` 贯通验收（页面操作 -> API -> 审计 -> 日志）`backend/tests/integration/ops/traceability_e2e_test.go` + `web-admin/tests/e2e/ops/traceability.spec.ts`

---

## Phase 8: User Story 5 - 首次安装引导配置闭环 (Priority: P1)

**Goal**: 新安装实例在“无配置/无管理员”场景下自动进入引导页面，支持最小可用配置与完成标记，减少手工编辑配置文件出错率

**Independent Test**: 冷启动环境访问 `/` 自动进入 `/setup`；完成向导后可回到首页；已初始化环境不会再次被强制引导

### Tests for User Story 5 (TDD First)

- [X] T091 [P] [US5] 契约测试：首次安装状态与完成接口 `backend/tests/contract/system/http_setup_contract_test.go`
- [X] T092 [P] [US5] 集成测试：冷启动 -> 引导 -> 完成闭环 `backend/tests/integration/system/setup_bootstrap_flow_test.go`
- [X] T093 [P] [US5] E2E：`/` 自动跳转 `/setup` 与完成后回首页 `web-admin/tests/e2e/setup/first-install.spec.ts`

### Implementation for User Story 5

- [X] T088 [US5] 新增首次安装状态接口（公开）`GET /api/v1/admin/setup/status`：`backend/internal/transport/http/admin/system/setup_handler.go`
- [X] T089 [US5] 新增首次安装完成接口（公开）`POST /api/v1/admin/setup/complete` 并挂载路由：`backend/internal/transport/http/admin/system/{setup_handler.go,api.go}`
- [X] T090 [US5] 前端路由守卫接入首次安装重定向：`web-admin/app/middleware/auth.ts`
- [X] T094 [US5] 将 `/setup` 页面从演示态改为后端状态驱动：`web-admin/app/pages/setup/index.vue`
- [X] T095 [US5] 实现向导配置写入能力（DB 配置域 + 必填项校验）：`backend/internal/transport/http/admin/system/setup_handler.go` + `backend/internal/service/system/setting_service.go`
- [X] T096 [US5] 实现 `/setup` 配置提交与回显（域名/HTTPS/存储/缓存/邮件）`web-admin/app/pages/setup/index.vue` + `web-admin/app/composables/api/services/settingsService.ts`
- [X] T097 [US5] 补充部署文档“引导配置页”章节与故障排查：`docs/guides/deploy/025-powerx-docker-systemd/{docker,systemd}/*`

**Checkpoint**: US5 完成后，首次安装路径可执行且与文档一致

---

## Phase 9: User Story 6 - 端口默认策略与 Setup 端口配置 (Priority: P1)

**Goal**: 明确并落地开发/生产默认端口策略（dev: 3030/8077, prod: 3000/8080），并在首次安装向导中支持端口配置，避免环境切换时端口口径不一致

**Independent Test**: 在 `POWERX_ENV=dev/prod` 下未传端口变量时自动取对应默认值；传入端口变量后覆盖默认值；`/setup` 可回显与保存端口配置

### Tests for User Story 6 (TDD First)

- [X] T098 [P] [US6] 契约测试：`setup config` 包含端口字段并通过校验 `backend/tests/contract/system/http_setup_contract_test.go`
- [X] T099 [P] [US6] 集成测试：端口默认值与覆盖优先级 `backend/tests/integration/system/setup_bootstrap_flow_test.go`

### Implementation for User Story 6

- [X] T100 [US6] 后端实现端口默认策略与优先级解析：`config/*` + `backend/internal/transport/http/admin/system/setup_handler.go`
- [X] T101 [US6] 前端 setup 页面增加端口配置项与校验：`web-admin/app/pages/setup/index.vue`
- [X] T102 [US6] 文档与示例配置对齐端口策略：`docs/guides/deploy/025-powerx-docker-systemd/*` + `deploy/powerx/docker/.env.prod.example` + `backend/.env.example`

**Checkpoint**: US6 完成后，dev/prod 端口行为一致且 setup/文档/示例配置无偏差

---

## Phase 10: User Story 7 - YAML 首真相的安装状态机与硬拦截 (Priority: P1)

**Goal**: 建立“DB 未就绪也可启动”的首次安装机制，以 `config.install.status` 作为安装态首判定，确保 `/setup` 可完成从未安装到已安装的闭环切换

**Independent Test**: DB 不可用时系统仍可访问 `/setup` 与健康接口；未安装时业务 API 全量拦截；完成 setup 后自动切换为正常运行模式

### Tests for User Story 7 (TDD First)

- [X] T103 [P] [US7] 契约测试：安装状态接口新增 `install_status/guard_mode` 字段与未安装统一错误码 `backend/tests/contract/system/http_install_state_contract_test.go`
- [X] T104 [P] [US7] 集成测试：未安装模式 API 白名单与全局硬拦截 `backend/tests/integration/system/install_guard_integration_test.go`
- [X] T105 [P] [US7] 集成测试：`setup/complete` 两阶段失败回滚与成功原子切换 `backend/tests/integration/system/setup_complete_atomicity_test.go`
- [X] T106 [P] [US7] E2E：未安装仅可走 `/setup`，安装成功后进入 `/home`，且不弹 AI 引导 `web-admin/tests/e2e/setup/install-guard.spec.ts`

### Implementation for User Story 7

- [X] T107 [US7] 配置模型新增 `install` 对象并加载默认值：`backend/config/{config.go,defaults.go}` + `backend/etc/{config.yaml,config_example.yaml}`
- [X] T108 [US7] 实现安装态守卫中间件并挂载到 HTTP 路由：`backend/internal/http/{router.go,install_guard_middleware.go}`
- [X] T109 [US7] 改造 setup 状态判定为“YAML 首真相 + 历史键兼容读取”并统一响应：`backend/internal/transport/http/admin/system/setup_handler.go`
- [X] T110 [US7] 实现 `setup/complete` 两阶段落地（写 dist 配置 -> migrate/seed -> 切 install.status）：`backend/internal/transport/http/admin/system/setup_handler.go` + `backend/internal/service/system/setting_service.go`
- [X] T111 [US7] 前端守卫与 AI 引导彻底解耦：`web-admin/app/middleware/auth.ts` + `web-admin/app/components/onboarding/WelcomeGuide.vue`
- [X] T112 [US7] 更新部署与安装机制文档并对齐 quickstart：`specs/025-powerx-docker-systemd/{install-mechanism.md,quickstart.md}` + `docs/guides/deploy/025-powerx-docker-systemd/*`

**Checkpoint**: US7 完成后，首次安装流程不再依赖“DB 先可用”，并具备可测试的状态机与拦截边界

---

## Phase 11: User Story 8 - 端口真源与生效一致性治理 (Priority: P1)

**Goal**: 建立“配置真源明确 + 生效状态可观测 + 重启语义清晰”的端口治理机制，避免 setup 页面、dist 产物、运行进程三者口径不一致导致的串端口问题

**Independent Test**: 在 `prod` 下 `3000` 前端只代理到 `8080`，`setup/status` 能同时返回 `desired/effective/restart_required`；修改端口后未重启时明确提示“未生效”，重启后状态一致

### Tests for User Story 8 (TDD First)

- [X] T113 [P] [US8] 契约测试：`GET /api/v1/admin/setup/status` 新增 `desired_ports/effective_ports/restart_required/config_source` 字段 `backend/tests/contract/system/http_setup_effective_status_contract_test.go`
- [X] T114 [P] [US8] 集成测试：`install.status=uninstalled/configuring` 时启动前置进入 setup-only 且不触发 DB 初始化 `backend/tests/integration/system/setup_preflight_no_db_bootstrap_test.go`
- [X] T115 [P] [US8] 集成测试：端口变更后“未重启=desired!=effective，重启后=desired==effective” `backend/tests/integration/system/setup_ports_apply_and_restart_test.go`
- [X] T116 [P] [US8] E2E：`prod` 场景下 web-admin 仅代理 `8080`，不会回落到 `8077` `web-admin/tests/e2e/setup/prod_port_proxy_consistency.spec.ts`

### Implementation for User Story 8

- [X] T117 [US8] 后端启动日志增强：输出 `config_path/install_status/effective_ports`，用于排查“读错配置文件” `backend/cmd/app/main.go` + `backend/config/config.go`
- [X] T118 [US8] 扩展 setup 状态接口：返回 `desired_ports/effective_ports/restart_required/config_source` `backend/internal/transport/http/admin/system/setup_handler.go`
- [X] T119 [US8] setup 配置应用器：统一写入 `dist/.../config/powerx.env`、`web-admin.env` 与 `backend/etc/config.yaml`（端口相关）`backend/internal/transport/http/admin/system/setup_handler.go` + `backend/internal/service/system/setting_service.go`
- [X] T120 [US8] 前端 setup 页增加“目标值/当前生效值”双列与“需重启”提示 `web-admin/app/pages/setup/index.vue` + `web-admin/app/composables/api/services/settingsService.ts`
- [X] T121 [US8] 打包链路固化 prod 端口与 upstream（防止 dist 产物内联 dev 默认）`make_files/dist.mk` + `web-admin/nuxt.config.ts`
- [X] T122 [US8] systemd/docker 环境模板补齐 `POWERX_CONFIG/POWERX_BACKEND/WS_UPSTREAM` 真源说明与示例 `deploy/powerx/systemd/*.service` + `deploy/powerx/docker/*` + `dist/systemd/*/config/*.env*`
- [X] T123 [US8] 文档与排障手册统一端口矩阵与“重启生效”语义 `specs/025-powerx-docker-systemd/{install-mechanism.md,quickstart.md}` + `docs/guides/deploy/025-powerx-docker-systemd/*`

**Checkpoint**: US8 完成后，端口改动的“配置写入 -> 重启 -> 生效”路径可观测、可验证、可回归，不再出现 3000/3030 与 8080/8077 串路问题

---

## Phase 12: Post-Install Upgrade Guardrails（安装后升级护栏）

**Goal**: 已安装实例默认走“代码升级 + 显式 migrate”路径，禁止误触 `/setup` 造成重装风险；运行配置从版本目录解耦为外置真源

**Independent Test**: `install_status=installed` 时访问 `/setup` 自动回 `/home`，调用 setup 写接口返回 409；切版本后 `/etc/powerx/config.yaml` 保持不变

- [X] T124 [US8] 运行时配置外置：switch-release 首次迁移并后续保留 `/etc/powerx/config.yaml` 与可选 `setup.wizard.config.json` `backend/scripts/ops/switch-release-systemd.sh`
- [X] T125 [US8] systemd 单元改为优先读取外置运行配置：`deploy/powerx/systemd/{powerx-backend.service,powerx-web-admin.service}`
- [X] T126 [US8] 后端 setup 写接口增加 installed 重入保护（默认 409，支持紧急开关）`backend/internal/transport/http/admin/system/setup_handler.go`
- [X] T127 [US8] 前端路由守卫收敛：仅未安装态强制 `/setup`，已安装态访问 `/setup` 跳 `/home` `web-admin/app/middleware/01-auth.global.ts` + `web-admin/app/plugins/{api.ts,auth-init.client.ts}`

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 -> Phase 2 -> Phase 3/4/5/6 -> Phase 7 -> Phase 8 -> Phase 9 -> Phase 10 -> Phase 11
- User Stories 必须在 Phase 2 完成后开始
- US1/US2/US3/US4/US5/US6 可并行推进（团队资源允许时）

### User Story Dependencies

- **US1 (P1)**: 依赖 Foundational 完成，无需依赖其他用户故事
- **US2 (P1)**: 依赖 Foundational 完成，可与 US1 并行
- **US3 (P2)**: 依赖 Foundational 完成，可与 US1/US2 并行
- **US4 (P3)**: 依赖 Foundational 完成，可与 US1/US2/US3 并行
- **US5 (P1)**: 依赖 Foundational 完成，可与 US1 并行；建议在发布前完成以保障首装体验
- **US6 (P1)**: 依赖 US5（setup 闭环）完成后推进；用于统一 dev/prod 端口策略并补齐 setup 端口配置
- **US7 (P1)**: 依赖 US5/US6，重构安装态真相与拦截策略；完成后才能标记 setup 闭环“生产可用”
- **US8 (P1)**: 依赖 US6/US7，收敛端口真源与生效状态模型，补齐 dist 构建与运行态一致性

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
- US5 测试任务 T091/T092/T093 可并行
- US6 测试任务 T098/T099 可并行
- US8 测试任务 T113/T114/T115/T116 可并行

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
