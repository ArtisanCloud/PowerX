# Tasks: 监控中心闭环（Backup + Logs）

**Input**: Design documents from `/specs/027-monitor-center/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: 本特性未要求 TDD 先行；本清单以实现任务为主，保留阶段性验收与回归验证任务。  
**Organization**: 按用户故事分组，确保每个故事可独立实现与验收。

**2026-10-15 范围更新**：US8 本地开发与 macOS 原生整站恢复／切换回归已完成。Linux 运行验收、生产插件业务验收及部署后真实 scheduled 包的独立还原仍待执行；原有日志任务不属于本次快照实现范围。

**2026-10-28 范围更新**：新增 [US9 双环境存储治理](#us9-storage-governance)，任务 T101～T165；本文件是唯一任务状态来源。已有文档登记及本地只读页面切片；完整治理与部署验收待执行，US6／US8 原有任务状态不变。

## Format: `[ID] [P?] [Story] Description`
- **[P]**: 可并行（不同文件、无直接依赖）
- **[Story]**: 所属故事（US1～US9，按下文现有编号）或阶段（Setup/Foundation/Polish）

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

- [X] T005 [Foundation] 定义备份域持久化模型（Policy/Job/Artifact/Drill/Alert）：`backend/pkg/corex/db/persistence/model/backup_ops/*.go`
- [X] T006 [Foundation] 将备份域模型挂载到 CoreX 统一迁移入口：`backend/pkg/corex/db/database/migration.go`
- [X] T007 [P] [Foundation] 增加备份域 Repository（策略/作业/演练/告警查询与写入）：`backend/pkg/corex/db/persistence/repository/backup_ops/*.go`
- [X] T008 [P] [Foundation] 在 `backup_ops` 服务层补齐统一错误码与错误翻译：`backend/internal/service/backup_ops/errors.go`
- [X] T009 [Foundation] 为备份任务执行器补齐统一审计记录（操作者、动作、结果）：`backend/internal/service/backup_ops/*.go`
- [X] T010 [Foundation] 在管理端路由注册备份域 API 分组：`backend/internal/transport/http/admin/backup/api.go` 与 `backend/internal/transport/http/admin/routes.go`
- [X] T011 [P] [Foundation] 定义备份域 DTO（请求校验、列表响应、分页结构）：`backend/internal/transport/http/admin/backup/dto/*.go`
- [X] T012 [Foundation] 更新系统菜单合同（监控中心一级菜单与备份入口关联校验）：`backend/internal/transport/http/admin/menu/system_menus_handler.go`
- [X] T041 [Foundation] 实现备份调度器注册与触发 wiring（按 interval/timezone 计算 next_run 并注册到现有 scheduler）：`backend/internal/service/backup_ops/job_service.go`、`backend/internal/bootstrap/app.go`（或现有调度装配点）
- [X] T042 [Foundation] 实现调度互斥与防重入（上一轮未完成时跳过/排队策略）并写入作业状态：`backend/internal/service/backup_ops/job_service.go`

**Checkpoint**: 基础模型、迁移、路由、DTO、审计框架已就绪。

---

## Phase 3: User Story 1 - 配置并启用自动备份策略 (Priority: P1) 🎯 MVP

**Goal**: 管理员可创建/编辑/启停数据库备份策略；当前新建默认 1 小时、7 天、168 份、Asia/Shanghai，策略和自动验证默认停用。历史线上策略保持原值。

**Independent Test**: 在管理端或 API 完成策略创建并启用后，可查询到正确策略状态与默认值。

### Implementation for User Story 1

- [X] T013 [US1] 实现策略创建接口（含默认值注入与参数校验）：`backend/internal/transport/http/admin/backup/policy_handler.go`
- [X] T014 [US1] 实现策略更新接口（频率/保留/时区/演练参数）：`backend/internal/transport/http/admin/backup/policy_handler.go`
- [X] T015 [US1] 实现策略启用/停用接口：`backend/internal/transport/http/admin/backup/policy_handler.go`
- [X] T016 [US1] 实现策略列表查询接口（按状态与关键字过滤）：`backend/internal/transport/http/admin/backup/policy_handler.go`
- [X] T017 [US1] 在服务层实现策略校验规则（interval/retention/timezone/drill）：`backend/internal/service/backup_ops/policy_service.go`
- [X] T018 [US1] 在服务层实现默认调度参数落库（当前 1h／7天／168份、Asia/Shanghai，自动验证默认关闭；取代早期 6h／14份基线）：`backend/internal/service/backup_ops/policy_service.go`
- [X] T019 [P] [US1] 在备份中心页面增加“策略管理”表单与列表：`web-admin/app/pages/ops/backup.vue`
- [X] T020 [P] [US1] 新增备份策略 API 客户端方法：`web-admin/app/composables/api/services/deployOpsService.ts`（或新增 `backupService.ts`）
- [X] T021 [US1] 前端表单校验与错误提示（非法频率/保留/时区）：`web-admin/app/components/ops/backup/*.vue`

**Checkpoint**: US1 可独立验收（策略可配、可启停、默认值正确）。

---

## Phase 4: User Story 2 - 监控备份任务状态与历史 (Priority: P1)

**Goal**: 管理员在监控中心看到备份任务状态、历史、失败摘要与高优先级告警。

**Independent Test**: 不依赖演练功能，仅通过备份作业执行即可在监控页看到状态与失败信息。

### Implementation for User Story 2

- [X] T022 [US2] 实现备份作业历史查询接口（状态/时间范围/策略过滤）：`backend/internal/transport/http/admin/backup/job_handler.go`
- [X] T023 [US2] 实现单作业详情接口（错误摘要、trace、耗时）：`backend/internal/transport/http/admin/backup/job_handler.go`
- [X] T024 [US2] 实现告警查询与确认接口（含 high 级别筛选）：`backend/internal/transport/http/admin/backup/alert_handler.go`
- [X] T025 [US2] 实现监控概览接口（next_run、last_result、连续失败次数）：`backend/internal/transport/http/admin/backup/monitor_handler.go`
- [X] T026 [US2] 在服务层实现“连续 2 次失败升级高优先级告警”规则：`backend/internal/service/backup_ops/alert_service.go`
- [X] T027 [US2] 在调度执行链路写入作业状态与失败摘要：`backend/internal/service/backup_ops/job_service.go`
- [X] T028 [P] [US2] 监控中心 Task/Cron 页面接入备份任务状态卡片：`web-admin/app/components/monitor/MonitorCenterWorkspace.vue`
- [X] T029 [P] [US2] 监控中心 Logs/Trace 页面接入备份链路摘要：`web-admin/app/components/monitor/MonitorCenterWorkspace.vue`
- [X] T030 [US2] 备份中心页面接入作业历史与告警列表：`web-admin/app/pages/ops/backup.vue`
- [X] T043 [US2] 实现过期备份清理执行逻辑（保留最近 N 份 + 不删除最新可用备份）：`backend/internal/service/backup_ops/artifact_cleanup_service.go`
- [X] T044 [US2] 实现清理失败补偿与告警（删除失败不影响新备份落库，告警可见）：`backend/internal/service/backup_ops/artifact_cleanup_service.go`、`backend/internal/service/backup_ops/alert_service.go`

**Checkpoint**: US2 可独立验收（监控可见、失败可见、告警可见）。

---

## Phase 5: User Story 3 - 快速演练恢复可用性 (Priority: P2)

**Goal**: 支持从备份产物发起恢复演练，并持续可见演练状态与结论。

**Independent Test**: 在已有备份产物前提下，能发起演练并看到 success/failed 与原因。

### Implementation for User Story 3

- [X] T031 [US3] 实现恢复演练创建接口（artifact 可用性校验）：`backend/internal/transport/http/admin/backup/handler.go`
- [X] T032 [US3] 实现恢复演练列表/详情接口：`backend/internal/transport/http/admin/backup/handler.go`
- [X] T033 [US3] 在服务层实现演练任务状态机（queued/running/success/failed）：`backend/internal/service/backup_ops/restore_drill_service.go`
- [X] T034 [US3] 实现默认每周演练调度触发与策略覆盖：`backend/internal/service/backup_ops/job_service.go` 与 `backend/internal/service/backup_ops/restore_drill_service.go`
- [X] T035 [P] [US3] 备份中心页面新增“发起演练”与“演练历史”区域：`web-admin/app/components/ops/backup/RestoreDrillPanel.vue` 与 `web-admin/app/pages/ops/backup.vue`
- [X] T036 [P] [US3] 前端接入演练 API 客户端与 WebSocket/SSE 状态推送展示（轮询仅限临时诊断，不作为常驻方案）：`web-admin/app/composables/api/services/backupOpsService.ts`

**Checkpoint**: US3 可独立验收（演练可发起、可跟踪、可判定）。

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 全链路一致性、文档与回归校验。

- [X] T037 [Polish] 对齐 OpenAPI 与 Handler 实际返回字段（含错误码）：`specs/027-monitor-center/contracts/backup-center.openapi.yaml` 与 `backend/internal/transport/http/admin/backup/*.go`
- [X] T038 [Polish] 完成 quickstart 全流程实测并修订：`specs/027-monitor-center/quickstart.md`
- [X] T039 [Polish] 补充运维脚本说明（备份、清理、演练、回滚）：`backend/scripts/ops/*.sh` 与 `specs/027-monitor-center/quickstart.md`
- [X] T040 [Polish] 执行后端与前端构建 + 质量门禁回归（测试覆盖、OTel Trace、关键指标）并记录结果：`backend`、`web-admin`
- [X] T045 [Polish] 补充服务层测试（策略校验、调度防重入、连续失败升级、保留清理幂等）：`backend/internal/service/backup_ops/*_test.go`
- [X] T046 [Polish] 补充前端 E2E smoke（策略启停、作业可见、告警可见、演练状态推送）：`web-admin/tests/e2e/ops/backup-center.spec.ts`
- [X] T047 [Polish] 补充可观测性埋点（结构化日志字段 + 指标：成功率/失败率/延迟）：`backend/internal/service/backup_ops/*.go`、`backend/internal/transport/http/admin/backup/*.go`
- [X] T048 [Polish] 补充 OTel 全链路验证步骤（策略操作 -> 调度执行 -> 告警/演练）：`specs/027-monitor-center/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1（Setup）可立即开始。
- Phase 2（Foundation）依赖 Phase 1，且阻塞所有用户故事。
- T041/T042 属于 Foundation 补充阻塞项，需在 US1 前完成。
- Phase 3/4/5（US1/US2/US3）均依赖 Phase 2 完成。
- Phase 6（Polish）依赖已交付的用户故事阶段。
- Phase 7（US4 日志监控）依赖 Phase 2，建议在 US2 后执行以复用作业与告警上下文。

### User Story Dependencies

- **US1 (P1)**: 无其他故事依赖，Foundation 后可先做（MVP）。
- **US2 (P1)**: 依赖基础调度/作业写入能力；建议在 US1 后执行。
- **US3 (P2)**: 依赖可用备份产物与作业记录；建议在 US1+US2 后执行。
- **US4 (P1)**: 依赖监控页基础框架与作业上下文；建议在 US2 后执行。

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

---

## Phase 7: User Story 4 - 日志与链路追踪监控 (Priority: P1)

**Goal**: 监控中心 Logs/Trace 页面支持 `loki/file/stdio` 三驱动能力感知，提供统一检索与可操作排障入口。

**Independent Test**: 仅启用监控日志能力时，管理员可以在日志页按 `trace_id/job_id/policy_id` 查询并获得正确驱动提示。

### Implementation for User Story 4

- [X] T049 [US4] 新增日志配置查询接口（返回 driver + capabilities + grafana_link_enabled）：`backend/internal/transport/http/admin/monitor/log_config_handler.go`
- [X] T050 [US4] 新增统一日志查询接口（trace/job/policy/time_range/keyword）：`backend/internal/transport/http/admin/monitor/log_query_handler.go`
- [X] T051 [US4] 实现日志驱动适配层（loki/file/stdio dispatch）：`backend/internal/service/monitor_logs/*.go`
- [X] T052 [US4] 实现 Loki 查询与 Grafana 深链生成：`backend/internal/service/monitor_logs/loki_provider.go`
- [X] T053 [US4] 实现 File 驱动查询（时间窗口 + 关键字 + 结构化映射）：`backend/internal/service/monitor_logs/file_provider.go`
- [X] T054 [US4] 实现 Stdio ring buffer 查询与窗口限制提示：`backend/internal/service/monitor_logs/stdio_provider.go`
- [X] T055 [P] [US4] 监控中心 Logs/Trace 页面改为能力感知 UI（禁用态+提示文案+深链按钮）：`web-admin/app/components/monitor/MonitorCenterWorkspace.vue`
- [X] T056 [P] [US4] 新增 monitor logs API client 与 store：`web-admin/app/composables/api/services/monitorService.ts`、`web-admin/app/stores/monitorLogs.ts`
- [X] T057 [US4] 日志查询与深链操作审计埋点（操作人/筛选摘要/结果状态）：`backend/internal/transport/http/admin/monitor/*.go`
- [X] T058 [US4] quickstart 补充三驱动验收步骤与故障排查：`specs/027-monitor-center/quickstart.md`

**Checkpoint**: US4 可独立验收（多驱动可用、能力提示清晰、排障链路完整）。

---

## Phase 8: User Story 5 - 统一日志保留与定时清理 (Priority: P1)

**Goal**: 在 `log.retention` 下统一治理文件日志与数据库日志保留，避免磁盘与日志表无限增长。

**Independent Test**: 启用 `log.retention` 后，管理员可观察到定时清理执行记录，且文件/DB 日志都按策略删除过期数据。

### Implementation for User Story 5

- [X] T059 [US5] 扩展日志配置模型与校验（`log.retention` 配置项、默认值、合法性校验）：`backend/config/config.go`、`backend/config/defaults.go`、`backend/config/validator.go`
- [X] T060 [US5] 实现统一日志保留调度器（cron 触发、批量删除上限、失败重试与状态记录）：`backend/internal/service/monitor_logs/retention_service.go`
- [X] T061 [P] [US5] 实现文件日志清理器（多目录、按 mtime + retention_days 清理）：`backend/internal/service/monitor_logs/file_retention_provider.go`
- [X] T062 [P] [US5] 实现数据库日志清理器（`audit_event` + 兼容日志表，分批 DELETE）：`backend/internal/service/monitor_logs/db_retention_provider.go`
- [X] T063 [US5] 增加日志保留执行审计与结构化日志字段（source/deleted_count/duration/status/error）：`backend/internal/service/monitor_logs/retention_service.go`
- [X] T064 [P] [US5] 监控中心 Logs/Trace 页面增加“日志保留任务最近执行”展示：`web-admin/app/components/monitor/MonitorCenterWorkspace.vue`
- [X] T065 [US5] quickstart 增加 `log.retention` 配置样例与验收步骤：`specs/027-monitor-center/quickstart.md`

**Checkpoint**: US5 可独立验收（统一策略生效、执行可见、失败可排障）。

---

## Phase 9: User Story 6 - 插件日志统一接入（Host 模式）(Priority: P1)

**Goal**: 宿主模式下插件日志稳定进入 PowerX 统一日志链路，并支持平台化 `policy/probe` 编排与审计。

**Independent Test**: 启用宿主模式插件后，在监控中心可检索插件日志字段；执行 policy/probe 可返回并审计结果。

### Implementation for User Story 6

- [ ] T066 [US6] 调整插件 supervisor 默认 stdout/stderr 透传策略，确保插件日志进入宿主采集链路：`backend/internal/infra/plugin/manager/supervisor/supervisor.go`、`config/powerx.env.example`
- [ ] T067 [US6] systemd 部署模板补充 `POWERX_SUPERVISOR_FORWARD_STDIO` 推荐值与说明：`deploy/powerx/systemd/powerx.env.example`、`docs/guides/deploy/*`
- [ ] T068 [US6] 为 Promtail 增加插件 JSON pipeline 与低基数标签提取：`deploy/observability/promtail/promtail-config.yaml`、`deploy/powerx/docker/observability/promtail-config.yaml`
- [ ] T069 [US6] 新增插件日志策略编排服务（GET/PUT policy + POST probe）并记录审计：`backend/internal/service/monitor_logs/*`、`backend/internal/transport/http/admin/monitor/*`
- [ ] T070 [P] [US6] 监控中心 Logs/Trace 页面增加“插件策略探测结果”可视化区块：`web-admin/app/components/monitor/MonitorCenterWorkspace.vue`
- [ ] T071 [US6] quickstart 补充插件日志联调验收与回滚步骤：`specs/027-monitor-center/quickstart.md`
- [ ] T072 [US6] 在宿主 HTTP 最外层统一实现 `plugin_id` 注入，覆盖 `/_p/<plugin_id>/...` 与 `/api/v1/integration/<slug>/...`（含 slug->plugin_id 映射）：`backend/internal/transport/http/middleware/*`、`backend/internal/infra/plugin/manager/router/*`
- [ ] T073 [US6] 对齐 `http_request` 与 `audit_event` 字段读取策略，统一从上下文输出 `request_id/trace_id/plugin_id/tenant_uuid`（禁止仅依赖 meta）：`backend/internal/transport/http/middleware/*`、`backend/pkg/corex/audit/*`
- [ ] T074 [US6] 对齐 `_p` 网关链路日志（`API-IN/GATE-*/PROXY-*`）字段注入与命名，补齐 `upstream_request_id` 回填：`backend/internal/infra/plugin/manager/router/*`
- [ ] T075 [US6] 对齐 `wsbus.*` 与异步 worker（cron/queue/retry/event-fabric）上下文透传，移除无条件 `context.Background()` 断链点：`backend/internal/transport/websocket/*`、`backend/internal/app/shared/*`、`backend/internal/app/shared/workers/*`
- [ ] T076 [US6] 新增日志上下文字段回归用例：同一 `request_id` 串联 `http_request`、`audit_event`、`API-IN/GATE/PROXY-*`、`wsbus.*`，且 `plugin_id` 非空：`backend/tests/integration/ops/*`

**Checkpoint**: US6 可独立验收（插件日志接入稳定、策略可编排、结果可审计）。

---

## Phase 10: User Story 8 - 系统快照与统一恢复 (Priority: P1，本地实现完成，部署验收待执行)

**Goal**: 管理员选择一个完整恢复点，系统统一处理数据库、媒体、配置和版本；完整包可带离服务器恢复，正式切换有维护边界和整套回退。

**Design**: [系统快照计划](../../docs/plan/deploy/system-snapshot-and-restore.md)；[US8 验收](../../docs/guides/deploy/027-monitor-center/usecase-us8-system-snapshot-acceptance.md)。

当前操作见[托管整站恢复手册](../../docs/guides/deploy/027-monitor-center/usecase-us8-managed-recovery.md)、[实际合同](../../docs/contracts/system-snapshots.openapi.yaml)及[本地验证报告](../../docs/contracts/evidence/system-snapshot-managed-validation-20261015.json)。历史包／数据副本仍保留。本地代码完成不代表正式站已升级。

- [x] M8.1 统一父任务、业务库之外账本、文件锁、Root typed API 与持久化审计；重启将活动任务识别为中断。
- [x] M8.2 PostgreSQL／本地媒体同 MVCC 采集与 SHA256 引用清单；配置密文、程序文件及独立完整包；不复用旧 CAS 引用。
- [x] M8.3 口令加密、目标主密钥重封装、导入路径／内容／空间检查、离线列出／导出／导入工具。
- [x] M8.4 多套命名策略表格维护、唯一默认启用及原子切换；真实周期、固定调度锚点及失败提示，默认关闭，旧策略按原值兼容。历史任务记录策略身份，保留清理按所属策略分组。
- [x] M8.5 Root 页面与确认弹层；统一等待数据库／文件副本准备，`prepared` 与业务／正式状态分开。
- [x] M8.6 原生 PostgreSQL A/B 与源依赖移除回归；服务竞态、JWT HTTP、32 项前端单测、3 条模拟 API 浏览器流程与发布构建。证据：[第一阶段报告](../../docs/contracts/evidence/system-snapshot-package-validation-20261015.json)。
- [x] M8.7 托管写入者维护、目标映射、启动前隔离、真实 Core／Web／鉴权／媒体验证、独立切换及 SIGKILL 后整套回退；前端 37 项单测和 4 条模拟 API 浏览器流程通过。
- [ ] M8.8 Linux bubblewrap 实机、已启用生产插件业务探针，以及部署后真实定时完整包的跨环境第二轮验收。

- [x] T077 [US8] 冻结快照／组件／导入导出／恢复 typed 合同、权限、异步与幂等、错误和状态机；不将新路径提前宣称已发布。
- [x] T078 [US8] 实现业务库之外的快照清单、恢复账本、源／目标身份及重启中断识别；敏感字段不得公开。
- [x] T079 [US8] 实现父任务统一调度、真实周期／下一次执行及调度失败页面告警；旧独立策略保持原配置和类型。
- [x] T080 [US8] 普通采集改为在线同 MVCC 数据库／ready 媒体引用、逐级安全打开、摘要验证及同任务最多 4 次退避；不暂停 HTTP、不停止服务。配置／插件生命周期指纹继续校验；正式切换回退点保留受控停写，在线包无同点 Redis 合同明确不可直接整站切换。
- [x] T081 [US8] 复用同 MVCC dump 与媒体引用，校验不可变文件版本、大小和摘要；并发删除／覆盖必须阻断完成。
- [x] T082 [US8] 实现生效配置允许清单、插件 registry／绑定、Core／插件版本和迁移／扩展依赖采集。
- [x] T083 [US8] 实现配置／必要密钥材料加密、独立解密路径和目标环境映射，避免只保存源库中的唯一密钥。
- [x] T084 [US8] 封存父 manifest 并聚合必需组件结果，分别记录本地完整性、异地、导出和业务验收状态。
- [x] T085 [US8] 实现跨系统快照、恢复和导出任务的对象引用保护及整套保留；适配旧 CAS 清理器后再复用对象。
- [x] T086 [US8] 实现独立完整包导出与安装产物解析，空目标且源实例不可访问时仍具备全部必需内容。
- [x] T087 [US8] 实现导入的格式、路径安全、解密、摘要、空间、版本／平台／驱动及扩展预检查；不支持项明确阻断。
- [x] T088 [US8] 实现新库、新媒体根目录、独立配置和目标部署身份／插件数据库绑定迁移计划。
- [x] T089 [US8] 在 Core／插件／worker 启动前落实恢复测试隔离：禁止生产连接与外部发送，停用 Core 后台任务、只读插件角色；正式开放写入前重置验证产生的 Redis 状态。
- [x] T090 [US8] 聚合数据库、媒体、配置和应用业务验证，区分准备完成、验证通过、待确认切换和正式生效。
- [x] T091 [US8] 实现独立持久化切换协调器、维护确认、全部停写、当前环境回退点及跨组件切换检查点。
- [x] T092 [US8] 实现切换失败／中断／重复提交的整套补偿和幂等；写入恢复后的回退不自动丢弃新数据。
- [x] T093 [US8] 实现仅 Root 用户 JWT 的导出／导入／恢复／切换权限和审计，拒绝 STS／API Key 及任意 shell／SQL／DSN／路径输入。
- [x] T094 [US8] 页面增加“系统快照”主入口，优先展示完整恢复点与主要操作，组件／摘要／Trace 在详情分层呈现。
- [x] T095 [US8] 页面实现统一恢复向导与父子进度；旧任务标注仅数据库／媒体，不占用整站成功指标。
  - 2026-10-17：移除快照、归档、恢复副本及协调器进度的定时状态查询；使用文件变更通知驱动的共享 SSE 和独立单任务 SSE，覆盖断线重连、离开页面、身份切换与 Node 代理取消。
- [x] T096 [US8] 原生数据库及媒体集成验证：状态 A 采集后形成 B，空目标独立恢复 A，源 B 不变化。
- [x] T097 [US8] 覆盖缺文件、坏摘要、缺配置／密钥、不兼容、空间不足、并发变更、旧 GC 和导入路径逃逸失败用例。
- [x] T098 [US8] 在自建完整测试实例覆盖启动前隔离、正式切换、进程中断、阶段失败及整套回退；清理自建进程与资源。
- [ ] T099 [US8] 本地通过后，由运维部署并取得至少一份真实 scheduled 完整包，带到独立本地／开发实例完成第二轮业务验收。
- [x] T100 [US8] 按真实实现冻结 API／CLI 与页面文案，更新操作手册和脱敏本地验收报告，明确 Linux／真实插件／生产定时包的待验收范围。

**Checkpoint**: 本地实现与回归完成；T099 和 M8.8 尚未完成。正式交付需两轮证据及目标 Linux／实际插件验收，不能以本地测试代替线上恢复能力。


## 2026-10-16：按需工具与高级组件策略

- [x] 日常整站备份继续以系统快照为入口；数据库和媒体默认进入“按需备份与导出”，额外周期移入各自“高级设置”，已启用策略保持可发现。
- [x] Root 无需创建策略即可生成一次数据库备份并下载；登记文件在保留锁内验证路径、大小和 SHA256，拒绝 STS／API Key／非 Root 导出。
- [x] 已登记本地媒体来源可直接生成仅媒体导出包，不保存策略、不附带数据库，保留请求幂等和失败反馈。
- [x] 按需任务纳入默认 7 天／168 份保留，最新和受保护文件保留；不自动变更原有定时策略。
- [x] 修复恢复验证长报告写入失败；报告列通过现有集中模型迁移升级为 text。
- [x] 本地单元、原生 PostgreSQL、桌面／窄屏受控 API 浏览器流程和构建验证；完整证据见 [按需备份验收](../../docs/contracts/evidence/backup-on-demand-validation-20261016.json)。

本轮不替代 T099／M8.8 的 Linux、正式插件和生产定时完整包验收。

---

<a id="us9-storage-governance"></a>

## Phase 11: User Story 9 - 双环境存储治理 (Priority: P1，只读切片已本地实现，其余待实施与部署验收)

**Goal**: 在 120 GiB 同机正式／开发环境管理容量、历史窗口及自动生命周期，保持业务引用、日志与可用恢复点保护。

**Requirements**: `spec.md` 的 US9、FR-049～FR-068、SC-017～SC-024；实施顺序见 `plan.md` 的 US9。

**Design**: [总体方案](../../docs/plan/deploy/storage-governance.md)、[UI 设计](../../docs/plan/deploy/storage-governance-ui.md)、[实施验收](../../docs/guides/deploy/027-monitor-center/usecase-us9-storage-governance.md)。

**Authority**: 本节为存储治理任务状态的唯一来源。SG 编号保留为设计阶段交叉引用，不在 docs 重复维护勾选。落点中标为“拟新增”的是设计路径，不代表文件、正式合同或能力已发布；最终结构由 T104～T109 冻结。

**Independent Test**: 两实例／共享资源只读对账、预算缺口、跨进程预留竞争、一轮受控清理、保护项、真实归档取回及桌面／窄屏推送均可独立核验，再推进自动周期和 XDocker 对接。

### 实施顺序与阶段门禁

```text
SG-00 文档
  → SG-01 合同／资源身份
  → SG-02 只读观测与估算
  → SG-03 全局预留与租约
  → SG-04 日志封存、压缩与异地覆盖
  → SG-05 文件生命周期／SG-06 终态数据／SG-07 恢复点治理
  → SG-08 UI 与事件推送真实接入
  → SG-09 双环境上线验收
  → SG-10 XDocker 接管验收
```

UI 布局可使用明确标记的 fixtures 提前验证，自动执行不能跳过 SG-03、相应资源适配器与验收。可先交付只读观测，但必须明确“清理未启用”，不得以只读页面交付代表自动治理完成。

| 阶段 | 负责人角色 | 前提 | 阶段退出条件 |
|---|---|---|---|
| P0：SG-01～02 | Core 后端／运维 | 本文基线 | 双环境来源可核对、整盘账目不重复、估算和未知状态真实 |
| P1：SG-03～04 | Core 后端／运维 | P0 合同与来源 | 跨进程预算不超卖、保护条件完整、归档校验／取回证据 |
| P2：SG-05～07 | Core 后端；插件协作 | P1 | 每类预览、受控执行、崩溃恢复和误删防护通过 |
| P3：SG-08～09 | 前端／QA／运维 | 对应后端合同 | 真实页面推送、权限、自动周期及故障注入通过 |
| P4：SG-10 | XDocker／Core／QA | 主机协作合同冻结 | 无重复控制器，卷与物理空间一致，部署迁移后规则不变 |

### US9 任务清单

<a id="us9-sg-00"></a>

#### SG-00：文档基线

- [x] T101 [US9] [SG-00.1] 记录 120 GiB 双环境容量池、目标天数／容量参数、生命周期、UI 和开发清单。 落点：`docs/plan/deploy/storage-governance.md`、`docs/plan/deploy/storage-governance-ui.md`。
- [x] T102 [US9] [SG-00.2] 将 US9 规格、实施计划与完整任务登记到 specs，并在部署／监控文档建立索引；specs 为任务状态唯一来源。 落点：`specs/027-monitor-center/spec.md`、`specs/027-monitor-center/plan.md`、`specs/027-monitor-center/tasks.md`。
- [ ] T103 [US9] [SG-00.3] 项目负责人／运维确认来源归属、远端目标、关键审计留存要求、允许的保护例外和维护窗口。未确认前不配置自动删除。 落点：`docs/guides/deploy/027-monitor-center/usecase-us9-storage-governance.md`。

<a id="us9-sg-01"></a>

#### SG-01：合同、身份与兼容策略

- [x] T104 [US9] [SG-01.1] 定义 host／filesystem／deployment instance／tenant／resource 身份；复用实际环境合同，登记共享 PG、journal、Loki，不从路径猜身份。 落点：`backend/internal/service/storage_governance/types.go`。
- [ ] T105 [US9] [SG-01.2] 冻结整数 bytes、逻辑／物理／压缩量、数据库内部可复用量、覆盖缺口及样本口径；定义归因与唯一对象去重。 落点：`backend/internal/service/storage_governance/types.go`。
- [x] T106 [US9] [SG-01.3] 定义带 revision 的主机预算与资源策略 DTO／存储模型；定义天数、份数、最低验证点、保护项、启停与草稿状态；旧策略保持原值。 落点：`backend/internal/service/storage_governance/policy.go`、`backend/pkg/corex/db/database/migration.go`。
- [ ] T107 [US9] [SG-01.4] 冻结估算、预览、幂等执行、任务详情、启停／保护的 Admin OpenAPI 和错误码；首期 Root 用户 JWT，按真实路由测试授权。 落点：`specs/027-monitor-center/contracts/storage-governance.openapi.yaml`、`backend/internal/transport/http/admin/storage/api.go`。
- [x] T108 [US9] [SG-01.5] 明确配置权威和优先级：部署拥有主机身份、路径及凭据；UI 管理策略；持久化模式和 schema 版本明确。未知来源／错误配置不得静默启用。 落点：`backend/config/config.go`、`backend/config/loader.go`。
- [ ] T109 [US9] [SG-01.6] 新旧清理器资源所有权互斥：接管前检测旧 Cron／timer／应用清理任务，迁移记录可回退，不同时清同一来源。 落点：`backend/internal/service/storage_governance/ownership.go`（拟新增）。

**交付物**：正式合同、迁移设计、权限矩阵、资源登记 schema、配置说明。预计涉及 `backend/config/`、`internal/service/monitor_logs/`、新治理域、集中 migration、`docs/contracts/`；新增代码路径在合同评审后确定。

<a id="us9-sg-02"></a>

#### SG-02：只读采集、容量总账与估算

- [ ] T110 [US9] [SG-02.1] 实现文件系统普通可用／预留／已用、未纳管目录、已删除仍占用、挂载和权限覆盖采集；不得为扫描读取或输出凭据内容。 落点：`backend/internal/service/storage_governance/filesystem_collector.go`（拟新增）。
- [ ] T111 [US9] [SG-02.2] 实现文件实际分配、压缩后大小、硬链接／CAS 去重与共享资源归因；子额度不重复计入总账。 落点：`backend/internal/service/storage_governance/file_collector.go`（拟新增）。
- [x] T112 [US9] [SG-02.3] 实现数据库表／索引／TOAST／WAL 计量、环境归属及来源不可读状态；禁止把 DB 无权限报为 0。 落点：`backend/internal/infra/storage_observer/postgres.go`、`backend/pkg/corex/db/persistence/repository/storage_governance/postgres_metrics_repo.go`。
- [ ] T113 [US9] [SG-02.4] 记录清理前新增量和压缩率，构建 14 天样本，样本数／断档／口径变更可见；计算 `D × R95 × 1.3 + P` 和预算缺口。 落点：`backend/internal/service/storage_governance/estimate.go`、`backend/internal/service/storage_governance/samples.go`（拟新增）。
- [ ] T114 [US9] [SG-02.5] 实现备份／快照按唯一对象、成功份数及保护额外占用估算；不能以单份平均值保证可用恢复点数量。 落点：`backend/internal/service/storage_governance/estimate.go`。
- [ ] T115 [US9] [SG-02.6] 实现本地／远端实际历史覆盖与缺段视图；目标保留天数、实际窗口和预测天数分别返回。 落点：`backend/internal/service/storage_governance/coverage.go`（拟新增）。
- [ ] T116 [US9] [SG-02.7] 实现 20／10 GiB、80%／90%、预算缺口、采集失联的告警与恢复通知；采集策略默认只读。 落点：`backend/internal/service/storage_governance/alerts.go`（拟新增）。

**验收**：构造两实例共享 PG／journal／CAS、独立挂载、采集失败和并发写入；账目无重复，估算示例 7 天为 2.73 GiB、14 天为 5.46 GiB；未知来源使覆盖不完整而非绿色正常。

<a id="us9-sg-03"></a>

#### SG-03：跨环境预算预留与持久租约

- [ ] T117 [US9] [SG-03.1] 实现每文件系统唯一预算协调者，跨 Core／worker／正式／开发原子预留，支持独立于业务 DB 的持久状态和锁。 落点：`backend/internal/service/storage_governance/budget.go`（拟新增）。
- [ ] T118 [US9] [SG-03.2] 定义预留尚未写出部分与实际已写部分的转换，续租、超时接管、取消释放、重启恢复、幂等操作和审计。 落点：`backend/internal/service/storage_governance/reservation.go`（拟新增）。
- [ ] T119 [US9] [SG-03.3] dump、归档压缩、上传、导入／导出、下载取回、快照创建／恢复均接入峰值空间预检；替代单一 128 MiB 局部假设，保留原有内容大小检查。 落点：`backend/internal/service/snapshot_ops/preflight.go`、`backend/internal/service/storage_governance/admission.go`（拟新增）。
- [ ] T120 [US9] [SG-03.4] 实现 20 GiB 余量准入、10 GiB 紧急行为和每盘初始并发 1；保护中的业务不因容量告警被任意终止。 落点：`backend/internal/service/storage_governance/admission.go`（拟新增）。
- [ ] T121 [US9] [SG-03.5] 建立候选预览指纹、revision、有效期和执行前复核；提供作业状态机、取消和“部分完成／待核验”。 落点：`backend/internal/service/storage_governance/cleanup_plan.go`（拟新增）、`backend/internal/service/storage_governance/cleanup_run.go`（拟新增）。
- [ ] T122 [US9] [SG-03.6] 审计与任务账本自身有容量／保留限制，避免自动治理循环产生日志撑满盘。 落点：`backend/internal/service/storage_governance/audit.go`（拟新增）。

**验收**：两个环境同时申请超过余量的大任务只能安全接纳允许的任务；Core／业务 DB 不可用、进程崩溃、重复请求、预览后新增引用均不导致超卖或误删；多盘任务分别预留。

<a id="us9-sg-04"></a>

#### SG-04：PostgreSQL、journal、Loki 与异地覆盖

- [ ] T123 [US9] [SG-04.1] 运维核实 PG 实际日志目的地与 collector／轮转、journal 配置和两环境共享关系；collector 变更另定维护窗口，不在开发测试中重启线上 PG。 落点：`docs/guides/deploy/027-monitor-center/usecase-us7-remote-log-archives.md`。
- [ ] T124 [US9] [SG-04.2] 给归档 CAS 增加压缩对象编码版本、内容与存储哈希、实际物理计量；兼容读取旧未压缩对象、恢复与远端协议，不原地覆盖有效对象。 落点：`backend/internal/service/archive_ops/types.go`、`backend/internal/service/archive_ops/capture.go`、`backend/internal/service/archive_ops/encoding.go`（拟新增）。
- [ ] T125 [US9] [SG-04.3] 增加 PG 源日志回收握手：活动文件排除、封存稳定检测、远端校验、源与 CAS 双份计量、过期候选与保护。 落点：`backend/internal/service/archive_ops/capture.go`、`backend/internal/service/storage_governance/postgres_log_lifecycle.go`（拟新增）。
- [ ] T126 [US9] [SG-04.4] 增加 journal 专用来源：游标／boot 边界、持续导出、封段、远端确认、断线追赶与覆盖缺口；原生容量清理与归档存在竞争时如实记录缺失。 落点：`backend/internal/service/archive_ops/journal.go`（拟新增）。
- [ ] T127 [US9] [SG-04.5] 证明“本地 7／2 天、远端 90 天”的目标与实际覆盖独立；实现远端独立生命周期任务、namespace 安全、离线重试、清单保留及取回验证。 落点：`backend/internal/service/archive_ops/remote.go`、`backend/internal/service/storage_governance/coverage.go`（拟新增）。
- [ ] T128 [US9] [SG-04.6] 对齐独立 archive-worker 与 Core 的来源和锁，业务库不可用时已有封存日志仍可归档；版本／实例信息可核对。 落点：`backend/cmd/archive_worker/main.go`。
- [ ] T129 [US9] [SG-04.7] Loki 本地 1 天／1 GiB 的实际组件配置、WAL／索引／chunk 与采集副本计量；不把该额度冒充原生硬上限。 落点：`deploy/observability/loki/loki-config.yaml`、`deploy/powerx/docker/observability/loki-config.yaml`。
- [x] T130 [US9] [SG-04.8] 普通日志转发采用有界异步交付，断网／Loki 拒绝不拖慢业务请求；可靠审计单独定义保证，丢弃／积压可见且不形成自我日志循环。 落点：`backend/pkg/utils/logger/writer/loki.writer.go`。

**验收**：真实回环 SSH／固定接收器、错误指纹、断网、磁盘不足、源变化、活动文件、重试、旧压缩协议兼容、远端取回；停业务 HTTP 不影响封存归档。journal 离线超过本地窗口需报缺口，不能声称全部保护。不得用删除 `pg_wal`、truncate 活动日志或关闭日志通过验收。

<a id="us9-sg-05"></a>

#### SG-05：临时文件、上传与附件

- [ ] T131 [US9] [SG-05.1] 盘点本仓库 multipart／导入／发布／导出入口，统一成功、绑定失败、超限、取消、panic 收尾；核实异步任务移交后不再依赖请求临时文件。 落点：`backend/internal/transport/http/admin/plugin/install_handler.go`、`backend/internal/transport/http/admin/plugin_release/publish_handler.go`。
- [ ] T132 [US9] [SG-05.2] 受控任务工作区接入容量限制与 SG-03 租约，普通源目录不接受通用按 mtime 删除；崩溃接管与符号链接防护通过。 落点：`backend/internal/service/storage_governance/workspace.go`（拟新增）。
- [ ] T133 [US9] [SG-05.3] 定义插件／Framework 的暂存登记、持久化确认、引用与释放合同；明确邮件附件归插件所有，外仓实现单独跟踪。 落点：`docs/contracts/temporary-workspace-lifecycle.md`（拟新增）、`backend/internal/service/storage_governance/workspace.go`（拟新增）。
- [ ] T134 [US9] [SG-05.4] 旧未登记 multipart／附件进入“待核实／隔离保留”队列；只有来源、引用和回退证据充分时迁移，不把历史目录自动纳入清理。 落点：`backend/internal/service/storage_governance/legacy_inventory.go`（拟新增）。
- [ ] T135 [US9] [SG-05.5] 正式 2.75／开发 1.25 GiB 临时池和共享 2 GiB 可重建缓存接入；缓存须证明可重建和无引用，VS Code／源码不自动处理。 落点：`backend/internal/service/storage_governance/cache_lifecycle.go`（拟新增）。

**验收**：大文件边界、请求失败／取消、异步移交、进程终止后接管、附件持久化失败、任务仍在使用的旧文件全部覆盖，业务持久文件校验不变。

<a id="us9-sg-06"></a>

#### SG-06：追踪、事件与审计数据库生命周期

- [ ] T136 [US9] [SG-06.1] 核对调用追踪、事件发布、审计真实模型与查询／重试／幂等／外键依赖，冻结终态、业务时间、最小窗口和租户边界。 落点：`backend/internal/service/storage_governance/db_adapter.go`（拟新增）。
- [ ] T137 [US9] [SG-06.2] 实现类型化 preview／cleanup 适配器，复核终态与无引用；拒绝用普通 `db_tables` 时间规则替代业务保护。 落点：`backend/internal/service/storage_governance/db_lifecycle.go`（拟新增）、`backend/internal/service/monitor_logs/db_retention_provider.go`。
- [ ] T138 [US9] [SG-06.3] 分批删除、取消、超时、锁等待、死锁重试和 WAL／副本压力限制；适用索引在独立测试数据验证。 落点：`backend/internal/service/storage_governance/db_lifecycle.go`（拟新增）。
- [ ] T139 [US9] [SG-06.4] 接入正式／开发追踪、终态事件与普通审计子额度；安全／法律保留优先，保护项导致超额时告警不删。 落点：`backend/internal/service/storage_governance/policy.go`。
- [ ] T140 [US9] [SG-06.5] 报告删除行数、库内可复用量和物理释放差异；维护 autovacuum 与 WAL／长事务／复制槽观测，不自动跑 VACUUM FULL。 落点：`backend/internal/infra/storage_observer/postgres.go`、`backend/pkg/corex/db/persistence/repository/storage_governance/postgres_metrics_repo.go`。
- [ ] T141 [US9] [SG-06.6] 评审时间分区及在线迁移方案，验证唯一键／外键、查询、在途数据、历史回填、锁窗口、额外空间和回退；这属于独立结构优化门禁。 落点：`backend/pkg/corex/db/database/migration.go`、`docs/plan/deploy/storage-governance.md`。

**验收**：使用真实临时 PostgreSQL 数据库验证 pending／retry／inflight／回执待完成与已终态混合数据、租户隔离和并发引用；物理空间未下降时不得报告释放成功。分区移除不能带走仍需保护的记录。

<a id="us9-sg-07"></a>

#### SG-07：备份、系统快照与 CAS 保护

- [ ] T142 [US9] [SG-07.1] 在现有按时间／份数规则上增加字节预算和最低成功备份数；最新成功、显式保护及运行引用优先，存量配置不静默改值。 落点：`backend/internal/service/backup_ops/retention.go`。
- [ ] T143 [US9] [SG-07.2] 系统快照区分生成就绪、内容存在、恢复能力与实际恢复验证；保护正式 2／开发 1 个已验证恢复点的目标，不把旧 ready 记录直接补成已验证。 落点：`backend/internal/service/snapshot_ops/policies.go`。
- [ ] T144 [US9] [SG-07.3] 统一 backup／archive／snapshot 对象引用与锁，独立清理器不得破坏系统快照；GC 覆盖远端、恢复、导出、下载引用及未发布临时对象。 落点：`backend/internal/service/archive_ops/restore.go`、`backend/internal/service/snapshot_ops/delete.go`。
- [ ] T145 [US9] [SG-07.4] 为未产出新备份的策略提供到期检查，协调旧“成功作业后清理”的触发方式，避免双执行。 落点：`backend/internal/service/monitor_logs/retention_service.go`、`backend/internal/service/storage_governance/ownership.go`（拟新增）。
- [ ] T146 [US9] [SG-07.5] 本地清理后保留远端清单、历史与可取回状态；验证最新完整包带离服务器后独立恢复。 落点：`backend/internal/service/archive_ops/remote.go`、`backend/internal/service/snapshot_ops/package.go`。

**验收**：保护项超过额度时报冲突；没有已验证恢复点时页面明确显示缺口；压缩／本地 GC／异地过期不破坏被引用恢复点；完成独立恢复与业务核对。

<a id="us9-sg-08"></a>

#### SG-08：UI、API 与事件推送

- [x] T147 [US9] [SG-08.1] 按 UI 文档增加 `/monitor/storage` 工作区与菜单授权；默认概览、内部纵向分区、窄屏选择器，不把所有功能堆成一页。 落点：`web-admin/app/pages/monitor/storage.vue`、`web-admin/app/components/monitor/MonitorCenterWorkspace.vue`。
- [ ] T148 [US9] [SG-08.2] 新 API 客户端／store 接入资源范围、实占／目标／估算／历史覆盖，加载／空／错误／过期样本分别处理。 落点：`web-admin/app/composables/api/services/storageGovernanceService.ts`、`web-admin/app/stores/storageGovernance.ts`（拟新增）。
- [ ] T149 [US9] [SG-08.3] 策略抽屉实现草稿、版本冲突、GiB→bytes、份数／天数、估算缺口、保存／启用分离；复用权威备份／归档配置。 落点：`web-admin/app/components/monitor/storage/StoragePolicyDrawer.vue`（拟新增）。
- [ ] T150 [US9] [SG-08.4] 归档视图实现实际覆盖、缺段、校验与取回状态，携带上下文进入 `/ops/backup` 详情，不复制策略。 落点：`web-admin/app/components/monitor/storage/ArchiveCoveragePanel.vue`（拟新增）、`web-admin/app/pages/ops/backup.vue`。
- [ ] T151 [US9] [SG-08.5] 清理预览／任务抽屉实现保护与跳过原因、部分完成、实际释放与审计；取消后已完成部分保持真实。 落点：`web-admin/app/components/monitor/storage/CleanupRunDrawer.vue`（拟新增）。
- [ ] T152 [US9] [SG-08.6] 复用 TaskBus 与事件总线接入 WS／SSE；事件去重、版本顺序、重连快照、断线提示、身份切换释放连接；无状态轮询。 落点：`web-admin/app/composables/useStorageGovernanceEvents.ts`（拟新增）、`web-admin/app/composables/useBackupEvents.ts`。
- [ ] T153 [US9] [SG-08.7] 告警在所有分区可发现；详情不泄露目录凭据或其他租户内容；i18n、键盘、深浅主题与无障碍一起交付。 落点：`web-admin/app/components/monitor/storage/StorageGovernanceWorkspace.vue`。

**验收**：真实路由／真实 API／浏览器请求记录，而非仅 fixtures。检查桌面、390 px、320 px、主操作、分区草稿保留、失败反馈和授权；持续静置时无周期状态请求。

<a id="us9-sg-09"></a>

#### SG-09：双环境试运行与交付

- [ ] T154 [US9] [SG-09.1] 重新采集主机总账与策略、记录版本／进程／服务基线和来源权限；只读阶段不改变原任务。 落点：`docs/guides/deploy/027-monitor-center/usecase-us9-storage-governance.md`。
- [ ] T155 [US9] [SG-09.2] 选择独立且长期在线的远端目标，配置凭据引用和已核对主机指纹，完成真实传输、校验和取回。 落点：`docs/contracts/evidence/storage-governance-archive-validation.json`（拟新增）。
- [ ] T156 [US9] [SG-09.3] 逐资源 observe → preview → 受控 execute → enforce；每类单独记录启用版本、操作者与回退办法。 落点：`docs/guides/deploy/027-monitor-center/usecase-us9-storage-governance.md`。
- [ ] T157 [US9] [SG-09.4] 连续至少 7 天自动周期验证，14 天样本后复核预算；分别保存正式／开发结果及共享计量，不能等待 7 天后默认标成功。 落点：`docs/contracts/evidence/storage-governance-runtime-validation.json`（拟新增）。
- [ ] T158 [US9] [SG-09.5] 故障注入：断网、目标满、DB 不可用、Core／worker 重启、过期预览、重复任务、活动引用、采集断线、资源超预算和物理盘紧张。 落点：`backend/internal/service/storage_governance/fault_injection_test.go`（拟新增）、`web-admin/tests/e2e/ops/storage-governance.spec.ts`（拟新增）。
- [ ] T159 [US9] [SG-09.6] 核对业务／插件请求、备份恢复、日志查询、历史缺口与物理空间；需要重启／迁移的操作另取得授权并记录窗口。 落点：`docs/contracts/evidence/storage-governance-business-validation.json`（拟新增）。
- [ ] T160 [US9] [SG-09.7] 发布正式合同、配置迁移说明、验证证据、完整修复 SHA／PR／CI、实际部署版本和未完成项；未部署不得写“线上已解决”。 落点：`specs/027-monitor-center/contracts/storage-governance.openapi.yaml`、`docs/guides/deploy/027-monitor-center/usecase-us9-storage-governance.md`。

<a id="us9-sg-10"></a>

#### SG-10：XDocker 后续对接（外部协作）

- [ ] T161 [US9] [SG-10.1] 与 XDocker 冻结主机／文件系统／卷／容器／实例身份、采集范围、可用空间和预留合同；未核对能力前不写成已支持。 落点：`docs/contracts/storage-host-delegation.v1.md`（拟新增）。
- [ ] T162 [US9] [SG-10.2] 明确物理卷、容器日志、镜像层、构建缓存和 release 的所有者及清理资格，防止 PowerX 与 XDocker 双重计算／执行。 落点：`docs/contracts/storage-host-delegation.v1.md`（拟新增）。
- [ ] T163 [US9] [SG-10.3] 实现主机唯一预留权威的接管与退回，迁移过程不丢失有效任务租约；XDocker 断联时拒绝未知空间的新大任务并保留业务服务。 落点：`backend/internal/service/storage_governance/host_delegation.go`（拟新增）。
- [ ] T164 [US9] [SG-10.4] 若消费 Core 服务能力，补正式最小权限 `core_internal` 合同和对应 API Key／STS 授权；运行相关 capability／grant 检查，不开放任意命令代理。 落点：`backend/config/platform_capabilities/storage_governance.yaml`（拟新增）、`docs/contracts/storage-host-delegation.v1.md`（拟新增）。
- [ ] T165 [US9] [SG-10.5] 宿主机、容器、卷内采样对账；整机和 PowerX 视图一致，迁移后保留原策略、引用、审计和恢复能力。 落点：`docs/contracts/evidence/storage-governance-xdocker-validation.json`（拟新增）。

### US9 测试与证据矩阵

| 类型 | 核心证据 | 不能替代它的证据 |
|---|---|---|
| 估算／规则单测 | 单位、样本不足、保护项、份数、边界和冲突 | UI 中手写一个正确数字 |
| 文件系统集成 | 临时独立目录、租约竞争、CAS、符号链接、真实分配变化 | 仅 mock 删除函数 |
| PostgreSQL 集成 | 临时库真实状态／事务／索引／WAL及数据核对 | 仅直接调用过滤函数 |
| 真实路由授权 | Root／无权 Admin／服务身份及跨实例边界 | 手动注入已授权上下文 |
| 归档集成 | 真实回环接收器、哈希、离线重试和取回 | SSH 连通或 HTTP 202 |
| Web Admin E2E | 桌面／窄屏、真实 API、请求记录、状态推送 | 构建或原型截图 |
| 运行验收 | 正式／开发真实自动周期、完整版本和数据快照 | 本地单测通过 |
| 恢复验收 | 带离源机恢复与业务核对 | 文件 ready 或 dump 可解析 |

新增测试放在对应域的 service／repository／handler 与 `web-admin/tests` 中，覆盖实际不变量，不以重复实现的断言充数。现有可参考测试位于 `archive_ops/ssh_test.go`、`backup_ops/artifact_cleanup_service_test.go`、`snapshot_ops/delete_test.go` 及 `backend/tests/integration/ops/logging_loki_retention_test.go`。

每份运行证据记录时间／时区、提交与构建版本、环境／实例、模式、来源、策略版本、任务、实际计量、保护项、错误、归档／取回结论和业务检查；不保存密钥、口令或认证 URL。后续新增的证据文件名和正式合同路径由实施 PR 一并登记。

### US9 文档交付边界

已完成 T101～T102、T104／T106／T108／T112 与 T147。2026-10-29 新增稳定部署资源身份、平台级 revision 草稿／事务审计、严格配置 schema、当前连接下重点表／索引／TOAST／共享 WAL 计量。T106 当前启停合同固定禁用，实际启用仍由 T107／T152 后续实现。T112 当前覆盖两张重点表的固定 allowlist，不承诺对任意插件表完整覆盖。

T113 已补独立五分钟占用采样、15 天有界元数据及 14 天缺失／不可用／口径变化视图；清理前新增量、压缩率与真实 R95 尚未埋点，故 T113 不勾选。T116 只有 20/10 GiB、整盘使用率 80%/90% 的当前快照提示；预算缺口及失联／恢复通知未完成。T148／T149 已接草稿保存／读回、资源级人工测算及环境上下文；T150 仅有占用历史，不是归档覆盖。实际产物估算、归档覆盖、备份权威策略复用与远端证据仍未满足整项，保持未勾选。

初次实现仅本地开发与临时库验收。后续按用户授权，已定向迁移本地 `powerx.public` 的三张治理元数据表；没有执行完整 Core／IAM／Agent 迁移或 seed。远程、既有配置和用户服务均未改动，不安装系统 timer、不删除业务文件。

### US9 需求／任务映射

| 需求 | 主任务范围 | 验收口径 |
|---|---|---|
| FR-049～FR-051、FR-064、FR-067 | T104～T109 | 来源／单位／合同、权限与新旧兼容 |
| FR-050～FR-053 | T110～T116 | SC-017～SC-018：实占、覆盖、估算与未知状态 |
| FR-060～FR-063 | T117～T122 | SC-019～SC-020：预算、持久租约、预览与真实结果 |
| FR-057～FR-058、FR-063、FR-067 | T123～T130 | SC-021：封存、压缩、journal 缺口、异地及非阻塞 |
| FR-054、FR-056 | T131～T135 | SC-020：暂存移交、引用与崩溃收尾 |
| FR-054～FR-055、FR-063 | T136～T141 | SC-020：终态、租户、WAL 与逻辑／物理区别 |
| FR-054、FR-059、FR-063 | T142～T146 | SC-020～SC-021：恢复点、CAS 引用与取回 |
| FR-052～FR-053、FR-062、FR-064～FR-066 | T147～T153 | SC-018、SC-022：参数、权限、分区与推送 |
| FR-068 | T154～T160 | SC-023：双环境自动周期、14 天复核和交付证据 |
| FR-064、FR-067 | T161～T165 | SC-024：XDocker 唯一权威、接管／退回 |

**Checkpoint**: T101／T102 为文档登记，T104／T106／T108／T112 为身份、策略草稿、配置与 PG 计量，T147 为页面入口与任务分区；其余未勾选项待完整交付。占用采样不是新增量和归档覆盖，当前风险提示不是通知投递，自动保留与 XDocker 未完成。原 US6 与 US8 未完成任务及 T099／M8.8 保持原状态；任何局部交付不自动完成本节或既有运行验收。

本地只读切片的验证范围和待办见[2026-10-28 验证记录](../../docs/contracts/evidence/storage-governance-observe-validation-20261028.json)。浏览器数据为受控 fixtures；Go 路由测试读取真实临时目录。未进行远程部署或自动清理。

2026-10-29 实现与验证范围见[策略及计量验证记录](../../docs/contracts/evidence/storage-governance-state-validation-20261029.json)。新增元数据迁移仅在 Agent 临时 PG 验证；用户数据库和远程服务未改动。

2026-10-29 用户授权本地 migrate：三张治理表和索引已创建，原非治理 schema／IAM 数量不变；限定入口、临时 PG 幂等／隔离回归和实际结果见[本地迁移证据](../../docs/contracts/evidence/storage-governance-local-migration-20261029.json)。本地迁移当时主机／文件系统配置及指标／采样开关未设置；后续配置准备结果见下条记录。Core 重启及运行验收尚待完成，不勾选完整运行验收任务。

2026-10-29 本地验收配置准备：按用户“继续实现缺失”的授权，仅在私有 `backend/etc/config.yaml` 新增观察配置。主机标识取本机硬件 UUID 的 SHA-256 派生值，文件系统登记实际 APFS 卷 UUID，PG 集群登记当前连接的 `pg_control_system()` 标识；复用原安装实例身份。PG 只读指标与 300 秒占用采样已配置，自动清理保持关闭。真实本地 JWT／Storage 模块验证数据库、两张表、WAL、草稿保存／读回／冲突及占用写入；测试草稿事务已回滚。新增 `sampling` 状态区分配置和 worker 准入，不声称进程已运行或已有完整历史。

详见[本地验收准备证据](../../docs/contracts/evidence/storage-governance-local-ready-20261029.json)。原 Core 未重启；正常定时周期与实际用户页面验收待用户重启后执行。T113 的新增量／压缩率／R95、T115 的归档覆盖及其他未勾选任务仍未完成，不能将此次配置接通算作完整 US9 完成。

### 2026-10-29：环境与资源策略具象化（v0.3）

- T106 合同补充：可选 `retention_mode=days/count/protected`，旧无 mode 组合值原样读取／保存；新响应的 `catalog_version=1/catalog` 是运行期部署目录和建议，不写入策略／审计 JSON。本次没有模型／DDL 变化，无新 migrate。
- T148／T149 局部交付：按登记环境／实例再选资源，明确显示正式／开发／共享意图；开发备份 24 份／1 GiB、正式 2 份／6 GiB，快照分别至少 1／2 个已验证恢复点，日志按天，媒体／归档对象只按引用保护。主机预算独立弹层和保存；当前资源保存不夹带其他未保存编辑。主机建议使用实际容量及预留，旧值不自动覆盖。
- 测算新增可信 resource_id 解析，不接受调用者覆盖环境／实例／路径；按天仍是人工假设，按份数明确 `artifact_inventory_required`，引用保护为 `retention_not_applicable`。T114 的真实产物清单、T115 覆盖仍待实现，不以新增模式勾选完成。
- 验证：18 项前端单测，25 项临时原生 PG／路由／服务 race 测试，1440／390／320 三种宽度的真实 Nuxt 构建＋受控 API 流程，隔离构建、vet、能力审计通过。完整前端类型检查仍被原有 `menuConfig.ts:165/186` 语法错误阻塞。运行环境与远程验收未替代。
- 证据及进程收尾见[环境策略验证记录](../../docs/contracts/evidence/storage-governance-context-validation-20261029.json)。没有重启用户 Core、变更私有配置／用户数据库或操作远程；所有本轮临时服务已退出，自动清理关闭。用户重启 Core 加载 v0.3 后继续实际页面验收。

### 2026-10-29：PowerX 与整盘范围澄清

- T148／T150 展示边界修正：概览默认查看 PowerX 数据库与非共享登记目录，总量明确未完整统计；整盘指标和共享来源单列。历史导航改为“用量历史”，默认当前数据库记录，整盘记录另选，不把两种范围放在同一表格中。
- 明确未完成内容：当前摘要采样没有目录历史、插件独立库完整计量或完整 PowerX 总量；T110／T113／T115／T150 等不因命名和分区修正而完成。采样详情按需展开，失败和范围变化保持可发现，无页面轮询。
- 本次仅前端展示、四种语言、回归和操作说明，无后端合同／迁移变化。范围隔离、分区保留和窄屏检查见[范围澄清验收](../../docs/contracts/evidence/storage-governance-scope-validation-20261029.json)。刷新前端即可，无需为此变更重启 Core。

### 2026-10-29 v0.4：普通应用日志手动闭环及逐来源历史

- T110／T148 局部交付：从实际日志配置发现普通日志，仅匹配对应活动和正式轮转文件；自动登记本地媒体、已启用的调试／审计目录。覆盖清单列未接入、禁用和平台不适用；没有宣称插件独立库／临时附件等已覆盖。
- T113／T148 局部交付：摘要保存逐来源占用、文件数、状态、失败原因和归因指纹，最多 32 KiB／槽，仍保留 15 天；每个来源可看历史与相邻可比净变化。真实流入、压缩率和 R95 未埋点，T113 保持未勾选。
- T120～T122、T151 局部交付：Root 的普通应用日志预览绑定实例／操作者／指纹／revision／有效期；最多 32 项的同步批次逐项复核、先记意图、再删除、再记结果，幂等回读。活动、最新轮转、链接、所有权不符和近期修改受保护。数据库故障后未知结果不重删、不宣称物理释放。覆盖所有资源的保护、持久租约、真实释放和异步任务仍未完成，相关整项不勾选。
- T123／T125／T145 局部交付：与原生日志 writer 共用目录 inode 锁，不额外生成锁文件、不增加自动定时器、不改旧保留值。异步轮转清理使用触发时刻快照，避免读取被测试时钟替换的全局变量。旧通用保留器启用则阻止新执行；正式／预发和要求验证归档的来源当前只预览。全面移交、PG／journal 归档仍待完成。
- 四张治理元数据表统一注册，新表 `ops_storage_cleanup_runs` 已经授权的本地限定迁移创建；其他表结构／索引和 IAM 数量读回一致。没有执行实际日志删除、重启用户服务或操作远程。
- 页面默认实际日志保留／预览流程，主机预算不是必填。其他来源规划在高级折叠中，旧入口继续可用；记录只覆盖本模块的日志批次。受控浏览器、原生 PG 的保护／故障回归、本地只读日志识别及完整边界见[验证记录](../../docs/contracts/evidence/storage-governance-logs-validation-20261029.json)。

### 2026-10-29 v0.4.1：真实启动路径与有界扫描修正

- T110／T148 局部修正：启动登记时解析可信配置的现存目录别名（含 macOS `/var → /private/var` 和 shell PWD），固定目标后仍不跟随内部／后来替换的软链；缺失子目录不创建。打开失败分别说明缺失、权限和软链。
- 扫描仍为 20000 条／2 秒总上限，后面的来源可以使用前面未用完的额度。UI 单独标注未完成的已扫描字节，不冒充完整去重占用。
- 本机只读验收复现实际 PWD 别名，读取普通日志 8 文件、调试 73、审计 76、媒体 299、快照 10018 文件。未写采样／草稿、未清理用户文件、未迁移或重启服务；其他 US9 未完成项不因此勾选。详见[路径修正证据](../../docs/contracts/evidence/storage-governance-paths-validation-20261029.json)。


### 2026-10-30 v0.5 文件治理与共享预算交付（本地，未部署）

- T117／T118／T120 局部交付：`internal/infra/storage_budget` 以同盘私有账本、flock、原子落盘支持权威预算、revision、未写出预留、已写转换、续租、到期恢复和取消释放。两个实例竞争、初始默认冲突、损坏账本和写前扩张有受控测试；未宣称全部外部工具和所有恢复后的 DB 写入均受控。
- T119 局部交付：本地媒体、数据库备份、归档、配套 dump、快照捕获／导入导出、归档恢复／下载和恢复包上传接入共享准入；archive-worker 运行配置可携带同一预算身份。受控 Writer 在扩张前准入，脚本 dump 有文件大小上限。原生 rsync 下载及服务端恢复增长仍需补齐适配，因此整项保持未完成。
- T121／T122 局部交付：运行策略预览、已保存 revision 执行、UUID 幂等、32 文件批次、1000 项扫描、最多 100 条执行元数据；删除意图先落盘，逐项最终策略与文件核对在共享锁内；中断结果未知并阻止重做。来源暂停且超过两分钟后可人工解除陈旧锁，保留未知历史；完整通用异步作业与取消机制尚未覆盖。
- T125／T128 局部交付：封存 numeric／lumberjack／dated／journal 文件归档，支持 PG stderr 数字轮转；删除前重新验证本文件的异地清单和对象哈希，活动、最新、变化、链接、属主不符均跳过。旧 collector current_logfiles 路径保持兼容；真实主机移交与独立 worker 现场配置仍待验收。
- T130 本地实现完成：Loki 普通日志采用有界内存、批量、有限重试、请求与退出超时，计量丢弃／失败／积压，恢复后清除当前降级，不递归写失败出口。受控故障服务验证写入延迟与内存边界；不承诺审计可靠交付，也没有升级线上服务。
- T116／T148／T149／T151～T153 局部交付：六分区中的自动治理弹层、运行预算与草稿分离、readback、失联提示、SSE 真实文件变更与分钟重鉴权；部署显式接收人通知，固定通知身份重试、失败冷却和恢复信号。1440／390／320 下验证表单保留、按事件更新和无状态轮询；全部归档覆盖／图表／生产通知验收仍未满足整项。
- 数据库删除、CAS 压缩编码、journal 游标和覆盖缺口、真实流入／R95、备份／快照按字节淘汰、完整临时附件与 XDocker 不属于已完成能力。7 天运行、14 天完整样本和正式安装验收保持待完成。证据：`docs/contracts/evidence/storage-governance-runtime-validation-20261030.json`。

- v0.5 移交加固：`manage_native_retention` 明确移交 Core 普通／审计文件原生保留权，关闭按天／份数删除但继续轮转压缩；未明确移交不能允许执行。旧通用清理器在生命周期登记后拒绝破坏性操作和重新启用，保证本轮数据库事件只监控。PG／journal 外部原生配置仍需现场对齐。最终候选先原子移入同盘私有隔离区再核对 inode，轮转替换冲突保留两份文件并阻止继续；隔离项未处理不能解除中断锁。

- 2026-10-30 真实隔离实例补验：实际 Core／PG／Redis／Nuxt 接通，无存储 API mocks；修正观察耗时导致错过清理边界的问题，独立十五分钟定时器实测约 900.28 秒触发，自动删除 1 MiB 过期封存日志。暂停／链接／业务文件和旧 pending 数据保留。补齐旧 HTTP 上传入口的共享预算、原子暂存与流式 multipart，低空间拒绝写入。证据见 `docs/contracts/evidence/storage-governance-live-validation-20261030.json`。日常实例和线上未切换；T157 的七天／十四天及 SG-09 双环境条件仍待完成。


### 2026-10-31：归档 UI 控制与数据库期限本地实现（不替代线上验收）

- T119～T130／T149／T151 局部实现：显式 `cleanup_mode`，旧无模式策略停用；新配置、验证和归档启用分开。独立 S3 凭据引用及执行节点指纹，私有桶／确切版本完整读回；直接到期删除仍走文件保护。临时守护协议 2、可信部署绑定及逐来源接管工具已写入源码，尚未部署旧安装副本。
- T123～T130／T144～T146：持久历史目录、受限 gzip 预览、临时／原生清单导入和索引重建、来源保留的异步迁移、按引用与确切版本到期、独立每小时检查和只读未确认结果核对。旧 Mac 对象仍保留，存量生产凭据匹配／迁移和退役尚未执行。
- T136～T141 局部实现：调用通知发布确认与终态分类，独立数据库期限、旧 pending 的同租户／链路／负载核验隔离、终态到期与引用检查、有界事务和幂等意图；可显式选择预算满前拒绝新的能力调用。未知记录不删；不运行 VACUUM FULL 或删除 WAL。大表列／索引生产迁移、普通 VACUUM 与物理复用、长期容量跟踪保持待验收。
- T147～T153：存储治理与备份中心复用同一归档面板，按任务分区和弹层配置；身份变化销毁旧上下文，SSE 驱动，无详情／列表常驻轮询。真实 Nuxt 产物的 1440／390／320 浏览器检查通过；API fixtures 仅证明页面，未代替日常实例／生产链路。
- 真实隔离 MinIO 通过具体版本读回、源与 CAS 消失后下载／预览、目录重建、迁移（原件保留）和远端确切版本到期；真实隔离 PostgreSQL 通过终态／引用保护与旧 pending 隔离。测试及部署边界见 `docs/contracts/evidence/storage-archive-controls-validation-20261031.json`，操作见 `docs/guides/deploy/027-monitor-center/usecase-us9-storage-archive-controls.md`。
- 未执行生产部署／重启／数据库迁移／旧 Mac 对象清理。正式任务的七天／十四天、双环境、真实存量旧归档及原生 PG collector 激活仍未满足；不因本地代码交付勾选这些完整任务。

### 2026-11-01：v0.6 开发收尾（运行验收随后安排）

- 已补齐 `migrate -scope capability-retention`：通过集中 `cmd/database/migrate.go → pkg/corex/db/database/migration.go` 增量增加两表 5 列／3 索引，不夹带完整 AutoMigrate、seed、旧数据分类或清理；仅 dev/test，统一升级锁、事务和超时。独立真实 PG 验证幂等、保留旧状态／正文、缺表拒绝和索引失败整批回滚。原 `storage-governance` 范围仍只处理监控元数据，语义不变。
- 数据库期限状态在观察模式返回 Root 可读的运行／授权／迁移就绪信息，不再把未配置运行目录一律当作服务故障；不创建状态或隐式启用。页面使用简短阻塞状态与“查看准备步骤”弹层，部署与迁移可分别定位，刷新成功清除旧错误；保留无轮询、表单状态和卸载取消。
- 补齐 100 批未确认结果停止新写批次的门禁，保留全部意图、状态和只读预览；不通过丢弃运行证据控制目录增长。
- 开发文档、OpenAPI、升级载荷校验和临时计划已同步；本地运行和十五分钟周期安排在交付后，操作者何时启动日常服务不再作为开发工作的等待条件。
- 本次未执行日常／生产迁移、策略启用、业务记录删除、真实旧归档处置或用户服务重启。TMP-SG-01～11 的代码和开发侧验证见 `docs/contracts/evidence/storage-completion-validation-20261101.json`；真实存量、TMP-SG-12、生产及长期验收继续作为运行任务，不能提前勾选。

### 2026-11-01 后续：本地已准备为可重启验收状态

- 按用户 `fix` 备份现有本地配置及 `powerx` 库，39.4 MiB 自定义格式 dump 在 Agent 自建私有 socket PostgreSQL 完整恢复，231 张表可恢复；隔离服务已关闭。限定 `capability-retention` 迁移完成，新增 5 列／3 索引，没有 seed 或其他迁移。
- 备份与迁移后逐条比较旧字段：3472 条追踪、4780 条事件均存在且内容不变，保持未分类与空终态；数据库 SQL 写日志／采集器开启且无待重启配置。私有运行目录 0700、同盘身份、部署门禁和余量保护已配置，没有开启清理／归档策略。
- 真实配置的只读组装检查 `TestStorageLocalRuntimePreparedReadOnlyAcceptance` 及 bootstrap vet 通过：复用原安装身份，结构／授权／运行状态就绪；不启动 worker 或用户服务。证据为 `docs/contracts/evidence/storage-local-ready-validation-20261101.json`。下一步仅需操作者重新加载 Core、刷新前端并从 UI 明确启用测试策略，再开始完整十五分钟运行验收；不是生产部署完成。

### 2026-11-01 本地重启后观测（监控通过，清理尚未启用）

- 操作者已于 18:57:54 重新加载 Core（8077 PID 20488），工作目录与配置校验值匹配本地准备记录；健康检查和 3030 页面均返回 200。19:16 检查时重启已超过 15 分钟，19:03／19:08／19:13 三次实际采样成功，间隔约 300 秒，容量观测账本持续更新。
- PostgreSQL 仍保留 3472 条 trace、4780 条 event；SQL 写操作记录与 collector 保持开启。可用空间约 145.4 GiB，高于 20／10 GiB 余量门槛；这只是本地进程可见文件系统，不能表示生产容量或受保护数据总上限已验收。
- 文件／数据库清理、调用拒绝和归档策略均未开启，无清理批次。因此只将实际重启后的监控采样记为通过，不将其替代自动删除、UI 取回或磁盘耗尽准入测试。浏览器访问宿主暂不可用，未伪造会话或绕过 Root UI 授权；启用测试策略后再验自动清理周期。证据为 `docs/contracts/evidence/storage-postrestart-validation-20261101.json`。

### 2026-11-01 真实 Root 页面与自动文件批次验收

- 使用用户提供的既有账号正常登录，没有伪造会话。定位并修复 2025 年缓存的菜单 301 与当前规范路径形成循环：菜单网络读取使用 `cache: no-store`，既有应用内短缓存保留。实际菜单请求恢复 200；相关 49 项前端测试及隔离生产构建通过。
- 在真实 `/monitor/storage` 页面预览仅命中 1 份自建过期日志，显式启用临时 3650 天／10 GiB 的 `expire_delete` 策略。后台 scheduled 批次于 20:25:59 删除 1 份（319488 字节），保护测试文件及 6 份真实封存日志保留、哈希一致；后续批次删除数为 0，未调用手动执行接口。
- 最后通过正常登录会话的同源正式 Root API 暂停策略，并从实际 UI 读回 Paused、总开关关闭；清理剩余自有测试文件，未开启归档或数据库删除。备份中的 3472 条 trace 和 4780 条 event 的原始 ID 全部仍在，运行期间另外各新增 8 条；PostgreSQL 日志保持开启。证据为 `docs/contracts/evidence/storage-canary-live-validation-20261101.json`。
- 电源日志确认 Mac 于 20:38:23 Idle Sleep、21:25:57 Wake，与采样间断吻合。因此真实自动删除／保护验收通过，连续十五分钟在线节奏、生产、归档取回及长期验收保持未通过／未执行状态，不能用本次结果代替。

### 同日补测：防休眠窗口与后端重启接管

- 临时 `caffeinate -i` 仅作用于本次测试，没有修改永久电源配置，结束已退出。第二份自有过期日志在 21:57:48 被 scheduled 批次自动删除，保护样例和 6 份原封存日志哈希仍一致；样例已清理，最终策略 revision=4、enabled=false。
- 窗口中后端由 PID 46181 切为 30958（21:57:43 启动），新进程沿用策略并于启动约 5 秒后完成自动接管。21:48／21:53／21:57:48／22:02:48 采样间隔约 287～300 秒；重启接管通过，但不宣称同一进程持续十五分钟未中断。
- 前端相关 49 项测试、独立构建及 diff 检查通过。当前整仓升级载荷校验存在差异，未通过、未在本前端修复中盲目重新生成；生产发布仍需单独处理。最终证据继续为 `docs/contracts/evidence/storage-canary-live-validation-20261101.json`，未改变归档／数据库删除停用边界。

### 同日最终稳定窗口：连续十五分钟文件治理验收通过

- 用户确认保持服务运行后，23:38:06～23:53:11 策略有效开启 905.08 秒，全程保持同一 Core PID 30958；墙钟与单调时钟差异约 0.23 秒，没有此前的休眠／进程切换中断。
- 23:42:48 scheduled 批次自动删除 1 份自有过期日志（315392 字节），未调用手动执行；23:42:48／23:47:48／23:52:48 三次采样间隔约 300 秒。保护样例及 6 份真实封存日志哈希一致；数据库旧分类记录保留，SQL 写操作记录和 collector 开启。
- 一次性测试结束暂停成功，最终策略 revision=6、enabled=false；自有测试文件清理完毕，临时防休眠进程已退出，未改变永久电源设置。记录为 `docs/contracts/evidence/storage-stable-window-validation-20261101.json`。
- 这一项仅关闭本地文件治理连续窗口验收；生产、归档取回、数据库到期、容量不足拒绝以及七／十四天等完整验收门槛保持独立，不据此全部勾选。

### 2026-11-02 数据库期限与容量保护组合验收

- 复现整盘占用上限漏检：隔离 PG 中将自有预算设为 1 byte，修复前 HTTP 仍返回 200 并执行业务。调用准入现同时检查关系物理占用、主机 critical 余量和整盘上限（包含在途预留）；检查位于路由／业务副作用及新增调用审计行之前。gRPC 不再把容量拒绝降为通用调用失败，返回 `Unavailable` 和 `STORAGE_CAPACITY_EXHAUSTED`。缺失目标表的物理占用显式返回未知，保持拒绝保护。
- 新增 `scripts/ops/validate-storage-database.py`：自建 PostgreSQL、随机私有 Unix socket、禁用 TCP、不读日常配置，并在写入前校验集群标识。真实 Admin HTTP、Selector、调用服务、审计及 gRPC 组合验证；身份上下文、路由登记和业务适配器是受控 fixture，不混称真实用户登录或日常实例验收。
- 默认关闭／只读不落策略、暂停删除而保护独立开启、真实 64 MiB 关系超限、整盘上限／critical／预留拒绝、不同成功／失败期限、终态时间、pending／未知／租户／引用保护、旧通知核验与宽限、批次幂等均通过。真实 worker 启动周期按 2 行批次推进 5 行并发布变化事件；连续十五分钟日常数据库运行仍单独验收。
- 28 条大体积自有记录删除前后关系均为 81649664 字节；普通 `VACUUM (ANALYZE)` 后为 1318912 字节，不使用 `VACUUM FULL`、清表或 WAL 删除，不对生产承诺相同回收量。临时 PG 的 collector／SQL mod 日志持续开启，SQL 日志实际记录 DELETE，正文随临时集群清理而未公开。
- 三项 native／gRPC 测试命令及四个受影响包 race、vet 通过。自建集群进程退出、socket 释放、临时数据移除；日常 Core PID 30958／Nuxt PID 11794 未重启，日常配置校验值仍一致、数据库策略文件不存在、文件策略暂停，SQL collector／mod 日志开启。证据：`docs/contracts/evidence/storage-database-admission-validation-20261102.json`。
- 日常／生产数据库清理和拒绝未开启，真实旧归档未动；归档实际 UI 取回、生产发布与长期验收仍待执行。整仓 `UPGRADE_PAYLOAD_CHANGED` 仍为发布门禁，不因本轮局部测试通过而勾选部署完成。

### 2026-11-02 后续：归档原生 API 与 UI 组合验收完成

- 日常 Root 会话实际发现归档接口 503：默认相对 `logs/audit` 未在 bootstrap 转为绝对路径。新增相对审计／应用／额外来源注册回归并修复；操作者重新加载后的 Core PID 23733 已返回 200，归档调度器运行、4 个来源就绪，真实页面显示 0 个开启策略。
- 实际产物点击“归档一批”没有请求，编译代码证明模板将 `crypto.randomUUID` 解析为组件上下文属性。手动归档和迁移改为脚本动作生成 UUID；回归先失败后通过，手动调用实际返回 202。现有分区与表单结构保持不变。
- 新增自有原生浏览器 fixture 与 `scripts/ops/validate-storage-archive-ui.py`：实际 Nuxt 产物、正式归档 HTTP／JWT／SSE、生命周期及独立私有版本 MinIO 组合验收。页面身份和 Root JWT 明确为受控 fixture，导航外壳依赖 fixture，归档接口与对象内容不伪造。
- UI 草稿不传输、未验证目标拒绝启用、验证后明确开启、实际上传与清单版本、按远端证明回收自有原件、本地 CAS 消失后的原名检索／预览／tar 下载与 SHA-256、关闭后继续下载均通过。1440／390／320 宽度、分区筛选保留、真实 SSE 更新及空闲无周期详情请求通过；截图和证据见 `docs/contracts/evidence/storage-archive-native-ui-validation-20261102.json`。
- 42 项前端相关测试、独立生产构建、3 个相关 Go 包 race／vet 通过。临时 MinIO、Go HTTP、Nuxt／fixture 全部退出，端口和临时数据清理；日常 6 份封存日志哈希仍一致，PostgreSQL collector／mod 日志保持开启，日常／生产数据及 Mac 已有对象未删除。
- 升级载荷注册表的 core／catalog 校验和已按当前完整工作树同步，`--check` 与升级 CLI 参数保护测试通过；未执行迁移，也不将内容一致性扩大为所有无关迁移语义已验收。生产部署、实际目标启用、旧脚本接管、Mac 历史和长期运行仍保持独立任务。
- 本次重启后的日常监控也已连续采样：00:45／00:50／00:55／01:00 四个实际槽位均存在，约 5 分钟间隔。该证据说明停用清理策略时仍持续监控，不代表日常归档或数据库删除已启用。

### 2026-11-02 当前日常实例：正常 Root 页面手动闭环验收通过

- 用户再次委托验证后，在 `3030 → 8077` 实际日常实例使用原有正常 Root 会话操作，没有 fixture 身份、伪造 JWT 或另起 Core。通过真实页面保存临时独立凭据／目标／归档草稿，验证后明确开启并手动归档；草稿阶段任务数为 0。目标为 Agent 自有 `19400` 私有版本 MinIO，原有 `9000` 未改动。草稿阶段外部 Core 重载后，保存的配置仍能回读并完成测试，执行 PID 为 42377。
- 真实来源按整目录封存规则归档 6 项、420449194 字节，取得远端具体清单版本并完整验证。仅自建 59 字节日志满足本次 3650 天测试期限；实际 UI 预览清楚列出唯一候选，保存 `archive_then_delete` 后手动执行只删除该样例。原有 6 份封存原件哈希不变，文件策略立即恢复暂停 revision=8。
- 移除样例的自有本地 CAS 后，关闭归档策略，仍在实际 UI 按原名检索、选中样例并预览，远端恢复的 59 字节 SHA-256 一致。正常 Root 的正式下载接口返回 200；浏览器流式解压 gzip／tar，样例哈希再次吻合；其他日志正文未输出或另存 Downloads。
- 验收后先确认所有测试策略停用，再按本轮具体 ID、属主／哈希、基线及剩余引用核对移除测试配置与缓存副本。当前策略／目标／任务／凭据均恢复 0，约 401 MiB 新建 CAS 副本和自有 MinIO 数据清理，临时 PID／端口退出；操作审计和 59 字节清理批次继续保留。数据库行期限／调用拒绝保持未启用，SQL collector／mod 日志仍开启；01:10／01:15／01:20／01:25 监控采样持续存在。
- 证据：`docs/contracts/evidence/storage-installed-root-validation-20261102.json`。这是日常实例的手动归档／清理／取回证明，不将其扩大为新一轮连续十五分钟自动归档或生产部署；长期正式目标／期限、原有 Mac 历史和业务数据库清理仍未擅自启用。

### 2026-11-02 下一步：原生 PostgreSQL 来源与通知准备

- [x] 新增原生 `postgresql` 封存格式和受信任 `current_logfiles` 配置，动态活动集与目录／属主／权限／链接／命名验证；清理及归档共用活动／最新／稳定保护。原子捕获后复查打开句柄，工具缺失、权限不足、警告及超时一律保留；清单变化或复原冲突保留原件／隔离意图。
- [x] 本地共享集群文件来源和有效 Root 成员的站内通知接收配置准备完成，其他模块配置及安装身份未变。只读实际目录接线验收通过，不启动日常 worker、不写数据库样本／来源／通知。
- [x] 针对性 Go race／vet、前端 10 项测试、Core 构建及升级载荷检查通过；范例、四语错误原因、操作与回滚文档同步。
- [x] 日常 Core 重新加载后的真实 PG 来源、采样、站内告警交付及状态验收：操作者重启后的 PID 93664 读取新配置；Root 实际页面来源暂停、通知配置有效，归档来源 ready。正常 04:28／04:33 两轮采样含 PG 来源且间隔约 300 秒；04:23 交付的存储告警在通知中心实页可见、仍为一条未读，运行账本无交付错误。
- [ ] 长期开页右上角通知浮层跨后端重启的自动同步复查：本轮曾显示空，而后台未读接口已有告警；刷新后通知中心可见。仅记录现象，尚未修复／证明重连根因，不以通知落库代替全部前端实时同步验收。
- [ ] 长期实际期限／预算／归档目标经 UI 明确配置后启用；当前文件删除暂停、归档配置为 0、数据库期限策略未写入。生产和旧 Mac 对象仍按既有独立任务执行。
- 证据：`docs/contracts/evidence/storage-native-observation-validation-20261102.json`；临时任务清单第十三节。原生 SQL 日志持续开启，未改变数据库日志设置，也未重启用户服务。
- 后验收边界：本机整盘约 94% 已用／55.7 GiB 可用，容量风险告警与观测一致；PG 实际只读预览 0 候选，活动 1／最新 1／期限内 17。文件删除、归档及数据库期限仍停用，既有 5 条文件运行记录保留；没有写入 PG 来源清理策略或改变生产／旧 Mac 对象。
