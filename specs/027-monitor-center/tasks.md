# Tasks: 监控中心闭环（Backup + Logs）

**Input**: Design documents from `/specs/027-monitor-center/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: 本特性未要求 TDD 先行；本清单以实现任务为主，保留阶段性验收与回归验证任务。  
**Organization**: 按用户故事分组，确保每个故事可独立实现与验收。

**2026-10-15 范围更新**：US8 本地开发与 macOS 原生整站恢复／切换回归已完成。Linux 运行验收、生产插件业务验收及部署后真实 scheduled 包的独立还原仍待执行；原有日志任务不属于本次快照实现范围。

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
