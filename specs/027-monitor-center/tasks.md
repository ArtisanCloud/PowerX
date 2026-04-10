# Tasks: 自动备份闭环（Backup Center）

**Input**: Design documents from `/specs/027-monitor-center/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: 本特性未要求 TDD 先行；本清单以实现任务为主，保留阶段性验收与回归验证任务。  
**Organization**: 按用户故事分组，确保每个故事可独立实现与验收。

## Format: `[ID] [P?] [Story] Description`
- **[P]**: 可并行（不同文件、无直接依赖）
- **[Story]**: 所属故事（US1/US2/US3）或阶段（Setup/Foundation/Polish）

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 建立文档、配置与基础入口，不涉及业务逻辑。

- [X] T001 [Setup] 校对并冻结本特性规格文档：`specs/027-monitor-center/spec.md`
- [X] T002 [Setup] 校对并冻结实现计划：`specs/027-monitor-center/plan.md`
- [X] T003 [P] [Setup] 补充备份中心 API 合同细节（错误码/分页/过滤参数）：`specs/027-monitor-center/contracts/backup-center.openapi.yaml`
- [X] T004 [P] [Setup] 补充 quickstart 验收步骤（策略->作业->告警->演练）：`specs/027-monitor-center/quickstart.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 所有用户故事都依赖的基础能力。

**⚠️ CRITICAL**: 本阶段完成前不得开始任一用户故事实现。

- [ ] T005 [Foundation] 定义备份域持久化模型（Policy/Job/Artifact/Drill/Alert）：`backend/pkg/corex/db/persistence/model/backup_ops/*.go`
- [ ] T006 [Foundation] 将备份域模型挂载到 CoreX 统一迁移入口：`backend/pkg/corex/db/database/migration.go`
- [ ] T007 [P] [Foundation] 增加备份域 Repository（策略/作业/演练/告警查询与写入）：`backend/pkg/corex/db/persistence/repository/backup_ops/*.go`
- [ ] T008 [P] [Foundation] 在 `backup_ops` 服务层补齐统一错误码与错误翻译：`backend/internal/service/backup_ops/errors.go`
- [ ] T009 [Foundation] 为备份任务执行器补齐统一审计记录（操作者、动作、结果）：`backend/internal/service/backup_ops/*.go`
- [ ] T010 [Foundation] 在管理端路由注册备份域 API 分组：`backend/internal/transport/http/admin/backup/api.go` 与 `backend/internal/transport/http/admin/routes.go`
- [ ] T011 [P] [Foundation] 定义备份域 DTO（请求校验、列表响应、分页结构）：`backend/internal/transport/http/admin/backup/dto/*.go`
- [ ] T012 [Foundation] 更新系统菜单合同（监控中心一级菜单与备份入口关联校验）：`backend/internal/transport/http/admin/menu/system_menus_handler.go`
- [ ] T041 [Foundation] 实现备份调度器注册与触发 wiring（按 interval/timezone 计算 next_run 并注册到现有 scheduler）：`backend/internal/service/backup_ops/job_service.go`、`backend/internal/bootstrap/app.go`（或现有调度装配点）
- [ ] T042 [Foundation] 实现调度互斥与防重入（上一轮未完成时跳过/排队策略）并写入作业状态：`backend/internal/service/backup_ops/job_service.go`

**Checkpoint**: 基础模型、迁移、路由、DTO、审计框架已就绪。

---

## Phase 3: User Story 1 - 配置并启用自动备份策略 (Priority: P1) 🎯 MVP

**Goal**: Root 管理员可创建/编辑/启停自动备份策略（默认 6 小时、14 份、Asia/Shanghai）。

**Independent Test**: 在管理端或 API 完成策略创建并启用后，可查询到正确策略状态与默认值。

### Implementation for User Story 1

- [ ] T013 [US1] 实现策略创建接口（含默认值注入与参数校验）：`backend/internal/transport/http/admin/backup/policy_handler.go`
- [ ] T014 [US1] 实现策略更新接口（频率/保留/时区/演练参数）：`backend/internal/transport/http/admin/backup/policy_handler.go`
- [ ] T015 [US1] 实现策略启用/停用接口：`backend/internal/transport/http/admin/backup/policy_handler.go`
- [ ] T016 [US1] 实现策略列表查询接口（按状态与关键字过滤）：`backend/internal/transport/http/admin/backup/policy_handler.go`
- [ ] T017 [US1] 在服务层实现策略校验规则（interval/retention/timezone/drill）：`backend/internal/service/backup_ops/policy_service.go`
- [ ] T018 [US1] 在服务层实现默认调度参数落库（6h,14份,Asia/Shanghai,周演练）：`backend/internal/service/backup_ops/policy_service.go`
- [ ] T019 [P] [US1] 在备份中心页面增加“策略管理”表单与列表：`web-admin/app/pages/ops/backup.vue`
- [ ] T020 [P] [US1] 新增备份策略 API 客户端方法：`web-admin/app/composables/api/services/deployOpsService.ts`（或新增 `backupService.ts`）
- [ ] T021 [US1] 前端表单校验与错误提示（非法频率/保留/时区）：`web-admin/app/components/ops/backup/*.vue`

**Checkpoint**: US1 可独立验收（策略可配、可启停、默认值正确）。

---

## Phase 4: User Story 2 - 监控备份任务状态与历史 (Priority: P1)

**Goal**: 管理员在监控中心看到备份任务状态、历史、失败摘要与高优先级告警。

**Independent Test**: 不依赖演练功能，仅通过备份作业执行即可在监控页看到状态与失败信息。

### Implementation for User Story 2

- [ ] T022 [US2] 实现备份作业历史查询接口（状态/时间范围/策略过滤）：`backend/internal/transport/http/admin/backup/job_handler.go`
- [ ] T023 [US2] 实现单作业详情接口（错误摘要、trace、耗时）：`backend/internal/transport/http/admin/backup/job_handler.go`
- [ ] T024 [US2] 实现告警查询与确认接口（含 high 级别筛选）：`backend/internal/transport/http/admin/backup/alert_handler.go`
- [ ] T025 [US2] 实现监控概览接口（next_run、last_result、连续失败次数）：`backend/internal/transport/http/admin/backup/monitor_handler.go`
- [ ] T026 [US2] 在服务层实现“连续 2 次失败升级高优先级告警”规则：`backend/internal/service/backup_ops/alert_service.go`
- [ ] T027 [US2] 在调度执行链路写入作业状态与失败摘要：`backend/internal/service/backup_ops/job_service.go`
- [ ] T028 [P] [US2] 监控中心 Task/Cron 页面接入备份任务状态卡片：`web-admin/app/components/monitor/MonitorCenterWorkspace.vue`
- [ ] T029 [P] [US2] 监控中心 Logs/Trace 页面接入备份链路摘要：`web-admin/app/components/monitor/MonitorCenterWorkspace.vue`
- [ ] T030 [US2] 备份中心页面接入作业历史与告警列表：`web-admin/app/pages/ops/backup.vue`
- [ ] T043 [US2] 实现过期备份清理执行逻辑（保留最近 N 份 + 不删除最新可用备份）：`backend/internal/service/backup_ops/artifact_cleanup_service.go`
- [ ] T044 [US2] 实现清理失败补偿与告警（删除失败不影响新备份落库，告警可见）：`backend/internal/service/backup_ops/artifact_cleanup_service.go`、`backend/internal/service/backup_ops/alert_service.go`

**Checkpoint**: US2 可独立验收（监控可见、失败可见、告警可见）。

---

## Phase 5: User Story 3 - 快速演练恢复可用性 (Priority: P2)

**Goal**: 支持从备份产物发起恢复演练，并持续可见演练状态与结论。

**Independent Test**: 在已有备份产物前提下，能发起演练并看到 success/failed 与原因。

### Implementation for User Story 3

- [ ] T031 [US3] 实现恢复演练创建接口（artifact 可用性校验）：`backend/internal/transport/http/admin/backup/drill_handler.go`
- [ ] T032 [US3] 实现恢复演练列表/详情接口：`backend/internal/transport/http/admin/backup/drill_handler.go`
- [ ] T033 [US3] 在服务层实现演练任务状态机（queued/running/success/failed）：`backend/internal/service/backup_ops/restore_drill_service.go`
- [ ] T034 [US3] 实现默认每周演练调度触发与策略覆盖：`backend/internal/service/backup_ops/job_service.go` 与 `backend/internal/service/backup_ops/restore_drill_service.go`
- [ ] T035 [P] [US3] 备份中心页面新增“发起演练”与“演练历史”区域：`web-admin/app/components/ops/backup/RestoreDrillPanel.vue` 与 `web-admin/app/pages/ops/backup.vue`
- [ ] T036 [P] [US3] 前端接入演练 API 客户端与 WebSocket/SSE 状态推送展示（轮询仅限临时诊断，不作为常驻方案）：`web-admin/app/composables/api/services/deployOpsService.ts`（或 `backupService.ts`）

**Checkpoint**: US3 可独立验收（演练可发起、可跟踪、可判定）。

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 全链路一致性、文档与回归校验。

- [ ] T037 [Polish] 对齐 OpenAPI 与 Handler 实际返回字段（含错误码）：`specs/027-monitor-center/contracts/backup-center.openapi.yaml` 与 `backend/internal/transport/http/admin/backup/*.go`
- [ ] T038 [Polish] 完成 quickstart 全流程实测并修订：`specs/027-monitor-center/quickstart.md`
- [ ] T039 [Polish] 补充运维脚本说明（备份、清理、演练、回滚）：`backend/scripts/ops/*.sh` 与 `specs/027-monitor-center/quickstart.md`
- [ ] T040 [Polish] 执行后端与前端构建 + 质量门禁回归（测试覆盖、OTel Trace、关键指标）并记录结果：`backend`、`web-admin`
- [ ] T045 [Polish] 补充服务层测试（策略校验、调度防重入、连续失败升级、保留清理幂等）：`backend/internal/service/backup_ops/*_test.go`
- [ ] T046 [Polish] 补充前端 E2E smoke（策略启停、作业可见、告警可见、演练状态推送）：`web-admin/tests/e2e/backup-center.spec.ts`（或现有测试目录）
- [ ] T047 [Polish] 补充可观测性埋点（结构化日志字段 + 指标：成功率/失败率/延迟）：`backend/internal/service/backup_ops/*.go`、`backend/internal/transport/http/admin/backup/*.go`
- [ ] T048 [Polish] 补充 OTel 全链路验证步骤（策略操作 -> 调度执行 -> 告警/演练）：`specs/027-monitor-center/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1（Setup）可立即开始。
- Phase 2（Foundation）依赖 Phase 1，且阻塞所有用户故事。
- T041/T042 属于 Foundation 补充阻塞项，需在 US1 前完成。
- Phase 3/4/5（US1/US2/US3）均依赖 Phase 2 完成。
- Phase 6（Polish）依赖已交付的用户故事阶段。

### User Story Dependencies

- **US1 (P1)**: 无其他故事依赖，Foundation 后可先做（MVP）。
- **US2 (P1)**: 依赖基础调度/作业写入能力；建议在 US1 后执行。
- **US3 (P2)**: 依赖可用备份产物与作业记录；建议在 US1+US2 后执行。

### Within Each User Story

- 先后端接口/服务，再前端接入。
- 同一文件任务顺序执行；不同文件可并行（标记 `[P]`）。

### Parallel Opportunities

- Setup: T003 与 T004 可并行。
- Foundation: T007 与 T008 可并行；T011 可与部分服务任务并行。
- US1: T019 与 T020 可并行。
- US2: T028 与 T029 可并行。
- US3: T035 与 T036 可并行。

---

## Parallel Example: User Story 2

```bash
# 并行处理监控展示与日志摘要
Task T028: MonitorCenterWorkspace 接入备份状态卡片
Task T029: MonitorCenterWorkspace 接入备份日志摘要
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. 完成 Phase 1 + Phase 2。
2. 完成 US1（T013-T021）。
3. 按 quickstart 验证策略可创建、可启停、默认值正确。
4. 可先行交付“自动备份基础能力”。

### Incremental Delivery

1. MVP（US1）上线。
2. 增量交付 US2（监控与告警）。
3. 增量交付 US3（恢复演练）。
4. 最后执行 Polish 阶段统一回归。

### Parallel Team Strategy

1. 一组负责 backend（policy/job/drill/alert）。
2. 一组负责 web-admin（backup page + monitor page）。
3. Foundation 完成后，按 US 分工并行推进前后端。

---

## Notes

- 每个用户故事都可独立验收，避免跨故事强耦合。
- 先保证“任务可观察”，再扩展“高级分析”。
- 任何涉及调度、清理、演练的改动都必须保留审计轨迹。
