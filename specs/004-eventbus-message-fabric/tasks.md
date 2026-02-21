# Tasks: EventBus & Message Fabric

**Input**: Design documents from `/specs/004-eventbus-message-fabric/`
**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/`

**Tests**: 规格未要求 TDD，但每个用户故事均包含关键合同或集成测试以保障验收。

**Organization**: 任务按用户故事分组，确保每个故事都可独立实现与验证。

## Format: `[ID] [P?] [Story] Description`
- **[P]**: 表示可与其他任务并行（不同文件、无直接依赖）
- **[Story]**: 该任务对应的用户故事（US1/US2/US3/US4）
- 描述中包含精确文件路径，便于直接实施

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 建立事件骨干的目录与基础配置，确保后续任务可落地。

- [x] **T001 [P] [Setup]** 在 `pkg/corex/db/persistence/model/event_fabric/`, `pkg/corex/db/persistence/repository/event_fabric/`, `internal/service/event_fabric/{directory,acl,delivery,dlq,replay,audit}`, `internal/transport/http/admin/event_fabric/`, `internal/transport/grpc/event_fabric/` 创建空的 `doc.go`/占位文件与 `go` 包声明。
- [x] **T002 [Setup]** 更新 `api/grpc/contracts/buf.yaml` 与 `api/grpc/contracts/buf.gen.yaml` 注册 `powerx/event_fabric/v1` Proto 包，校验 `managed.go_package_prefix`、`api/grpc/gen` 输出路径，并在 `Makefile` 新增 `proto-event-fabric` 目标触发代码生成。
- [x] **T003 [Setup]** 扩展 `config/config.go`, `config/validator.go`，新增 `EventFabricConfig`（含重试/ACK 超时/Redis 配置），并在 `.env.sample` 说明默认值。
- [x] **T004 [Setup]** 在 `internal/transport/http/admin/routes.go` 注册 `/event-fabric` 路由分组与占位 Handler，便于后续故事挂载。
- [x] **T005 [Setup]** 在 `api/grpc/contracts/powerx/event_fabric/v1/event_fabric.proto` 定义基础消息与服务骨架（目录、ACL、Publish/Subscribe、Replay、DLQ），确保契约覆盖 FR-003~FR-009 并能通过 `buf lint`。
- [x] **T006 [Setup]** 在 `specs/004-eventbus-message-fabric/contracts/event_fabric_admin.openapi.yaml` 起草 Admin REST OpenAPI 合同骨架，包含 `/topics`, `/acl`, `/publish`, `/dlq`, `/replay` 路由与 `pkg/dto` 响应格式。

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 跨用户故事共享的核心能力，完成前禁止进入任何故事实现。

- [x] **T007 [Foundation]** 在 `internal/app/shared/deps.go`、`internal/bootstrap/app.go` 建立 `EventFabricDeps` 注入结构，初始化 Redis 客户端、默认 EventBus、Postgres Repo 工厂。
- [x] **T008 [Foundation]** 定义 `internal/service/event_fabric/shared/constants.go`、`errors.go`（多租户、状态、错误码、默认重试 5 次、ACK 超时 30s），供各故事统一引用。
- [x] **T009 [Foundation]** 在 `cmd/database/migrate.go`、`pkg/corex/db/database/migration.go` 注册 `migrateEventFabricModels` 钩子，并生成空迁移文件模板 `pkg/corex/db/migration/202510170000_event_fabric.go` 以承载后续表结构。

**Checkpoint**: 依赖注入、配置与迁移框架就绪，可开始各用户故事。

---

## Phase 3: User Story 1 - 统一主题治理与发现 (Priority: P1) 🎯 MVP

**Goal**: 提供主题目录 API，支持命名校验、生命周期管理与事件通知。
**Independent Test**: 通过 Admin REST 创建/更新/查询主题，校验命名冲突、生命周期变更事件与审计日志。

### Implementation

- [x] **T010 [US1]** 在 `pkg/corex/db/migration/202510170001_create_event_topics.go` 定义 Topic 表；2026-02 规范更新后统一以 `event_topics` 作为 Topic 注册与治理真相源，不再按租户复制 Topic 实例。
- [x] **T011 [US1]** 新增模型 `pkg/corex/db/persistence/model/event_fabric/topic_definition.go`，实现 GORM 标签、唯一索引与生命周期时间戳。
- [x] **T012 [US1]** 在 `pkg/corex/db/persistence/repository/event_fabric/topic_repository.go` 实现查询、分页、乐观锁更新与生命周期状态过滤。
- [x] **T013 [US1]** 实现 `internal/service/event_fabric/directory/validator.go` 与 `service.go`，覆盖命名规则校验、版本化快照、事件发布 `TopicLifecycleChanged`。
- [x] **T014 [P] [US1]** 编写 Admin REST Handler `internal/transport/http/admin/event_fabric/directory_handler.go` 与 DTO 映射，挂载至 `/topics` 系列路由，并同步更新 OpenAPI 合同中相关路径。
- [x] **T015 [P] [US1]** 在 `internal/tests/http/admin/event_fabric/topic_contract_test.go` 编写合同测试：创建、命名冲突、生命周期转换、审计校验。
- [x] **T016 [US1]** 更新 `quickstart.md` 并在 `docs/event_fabric/operations.md` 新增章节，记录主题目录操作流程与事件命名规范。

**Checkpoint**: 主题目录独立可用，可完成用户故事 1 的验收脚本。

---

## Phase 4: User Story 2 - 精细化事件访问控制 (Priority: P1)

**Goal**: 基于租户/角色管理发布与订阅授权，输出审计记录。
**Independent Test**: 创建授予与撤销操作；未授权主体调用 Publish/Subscribe 时被拒并产生审计事件。

### Implementation

- [x] **T017 [US2]** 在迁移文件中补充 `event_acl_bindings` 表结构，包含唯一键 (`scope/topic_id/principal_id/action`) 与过期时间；ACL 基于 Topic 注册表 ID，不依赖租户复制 Topic。
- [x] **T018 [US2]** 增加模型 `pkg/corex/db/persistence/model/event_fabric/acl_binding.go`。
- [x] **T019 [US2]** 编写仓储 `pkg/corex/db/persistence/repository/event_fabric/acl_repository.go`，支持批量授予/撤销与有效期过滤。
- [x] **T020 [US2]** 实现 `internal/service/event_fabric/acl/acl_service.go`：整合权限校验、授予/撤销与冲突检测。
- [x] **T021 [P] [US2]** 完成 REST Handler `internal/transport/http/admin/event_fabric/acl_handler.go`，覆盖 `/acl` 批量接口与查询，并更新 OpenAPI 契约。
- [x] **T022 [P] [US2]** 在 `internal/tests/http/admin/event_fabric/acl_contract_test.go` 添加授权/撤销合同测试。
- [x] **T023 [US2]** 新增 `internal/service/event_fabric/acl/enforcer.go` 并在 `internal/app/shared/deps.go` 注入运行时校验能力。

**Checkpoint**: ACL 管道生效，可独立验证授权与审计逻辑。

---

## Phase 5: User Story 3 - 高可靠投递与死信补偿 (Priority: P1)

**Goal**: 实现至少一次投递、重试/退避、DLQ 与 gRPC 长连接订阅。
**Independent Test**: 模拟发布/消费失败，验证重试 5 次后写入 DLQ；通过回放/补偿 API 成功恢复。

### Implementation

- [x] **T024 [US3]** 在新迁移 `pkg/corex/db/migration/202510170003_create_event_delivery_tables.go` 创建 `event_envelopes`, `event_delivery_attempts`, `event_dlq_messages`, `event_subscription_offsets` 表。
- [x] **T025 [US3]** 增加对应模型文件（`event_envelope.go`, `delivery_attempt.go`, `dlq_message.go`, `subscription_offset.go`）于 `pkg/corex/db/persistence/model/event_fabric/`。
- [x] **T026 [US3]** 构建仓储 `pkg/corex/db/persistence/repository/event_fabric/{envelope_repository.go,delivery_repository.go,dlq_repository.go}`，封装投递与状态变更。
- [x] **T027 [US3]** 扩展 `pkg/event_bus`：新增接口支持 Ack/Nack、重试计划、幂等窗口（包含订阅者并发/超时配置），默认 Redis 实现位于 `pkg/event_bus/redis_retry.go`。
- [x] **T028 [US3]** 编写 `internal/service/event_fabric/delivery/backoff_scheduler.go` 使用 Redis Sorted Set 计算指数退避与抖动。
- [x] **T029 [US3]** 实现 `internal/service/event_fabric/delivery/service.go`：发布入库、推送订阅者、重试决策、订阅幂等策略、DLQ 写入。
- [x] **T030 [US3]** 构建 `internal/service/event_fabric/audit/service.go`，封装发布/订阅审计写入（调用审计客户端、覆盖成功/失败场景），并在 `internal/app/shared/deps.go` 注册。
- [x] **T031 [US3]** 在 `internal/service/event_fabric/delivery/service.go`、`internal/transport/{grpc,http}/event_fabric/*` 集成审计记录；于 `internal/tests/{grpc,http}/event_fabric/audit_contract_test.go` 编写测试验证发布/订阅审计流水与错误回滚。
- [x] **T032 [US3]** 创建 DLQ 服务 `internal/service/event_fabric/dlq/service.go`，支持查询、重放、告警钩子。
- [x] **T033 [P] [US3]** 实现 gRPC 服务 `internal/transport/grpc/event_fabric/publisher_server.go` 与 `subscriber_server.go`（使用 `api/grpc/contracts/powerx/event_fabric/v1/event_fabric.proto`），接入拦截器并更新 Proto 契约中的 RPC 定义。
- [x] **T034 [P] [US3]** 编写 Admin REST Handler `internal/transport/http/admin/event_fabric/dlq_handler.go` 与 `/publish` 端点（调用 delivery 服务），同步扩充 OpenAPI 合同。
- [x] **T035 [P] [US3]** 在 `internal/tests/grpc/event_fabric/delivery_contract_test.go` 覆盖 Publish/Ack/Nack/重试 流程；在 `internal/tests/http/admin/event_fabric/dlq_contract_test.go` 验证 DLQ 操作与审计。
- [x] **T036 [US3]** 新增后台 worker（可挂载于 `internal/app/shared/workers/event_fabric_retry.go`）周期拉取 Redis 重试队列并调用 delivery 服务。
- [x] **T037 [US3]** 更新 `internal/server/mcp/templates/usecase.tmpl`（或相关模板）以支持事件发布用例示例。

**Checkpoint**: 完成至少一次投递、重试、DLQ 与审计流水接入，gRPC/REST 通路均可独立测试。

---

## Phase 6: User Story 4 - 事件契约与回放能力 (Priority: P2)

**Goal**: 提供事件回放 API，支持版本协商与影子通路。
**Independent Test**: 针对指定时间窗口与 Trace ID 发起回放，确认订阅者收到 `replay=true` 标记且不影响实时流量；订阅者可声明可接受的事件版本并正确协商。

### Implementation

- [x] **T038 [US4]** 扩展 `internal/service/event_fabric/delivery/version_negotiator.go` 与 `delivery/service.go`，实现事件版本协商策略（strict/backward/any）并记录兼容结果。
- [x] **T039 [US4]** 更新订阅注册入口：在 `internal/transport/grpc/event_fabric/subscriber_server.go` 与 Admin `/topics` Handler 中允许消费者声明兼容策略与支持版本列表，并同步更新 Proto/OpenAPI 契约。
- [x] **T040 [P] [US4]** 编写 `internal/tests/grpc/event_fabric/version_negotiation_test.go` 与 `internal/tests/http/admin/eventFabric/topic_version_contract_test.go` 验证版本协商行为。
- [x] **T041 [US4]** 在迁移 `pkg/corex/db/migration/202510170004_create_event_replay_tables.go` 定义 `event_replay_requests` 表。
- [x] **T042 [US4]** 添加模型与仓储 `pkg/corex/db/persistence/model/event_fabric/replay_request.go`、`repository/event_fabric/replay_repository.go`。
- [x] **T043 [US4]** 实现 `internal/service/event_fabric/replay/service.go`：支持回放任务调度、状态机、影子通路（确保不触发业务副作用）。
- [x] **T044 [P] [US4]** 在 `internal/transport/http/admin/event_fabric/replay_handler.go` 提供回放 API；更新 `routes.go` 挂载并扩充 OpenAPI 合同。
- [x] **T045 [P] [US4]** 实现 gRPC `internal/transport/grpc/event_fabric/replay_server.go` 对应 `EventReplayService`，同步维护 Proto 定义。
- [x] **T046 [P] [US4]** 编写测试 `internal/tests/http/admin/event_fabric/replay_contract_test.go` 与 `internal/tests/grpc/event_fabric/replay_contract_test.go` 验证回放行为。
- [x] **T047 [US4]** 更新 `quickstart.md` 增补回放与版本协商示例，确保影子通路操作说明。

**Checkpoint**: 回放与版本协商能力上线，P1 + P2 用户故事均可独立演示。

---

## Phase 7: Polish & Hardening

**Purpose**: 收尾性能、可观测性、安全与发布准备工作。

- [X] **T048 [P] [Polish]** 在 `internal/service/event_fabric/metrics/metrics.go` 输出投递成功率、重试延迟、DLQ 积压、回放耗时指标；更新 `internal/app/shared/deps.go` 注册。
- [X] **T049 [Polish]** 扩展 `pkg/dto/error.go`、`pkg/dto/base.go` 增加事件 Fabric 错误码与统一响应。
- [X] **T050 [P] [Polish]** 在 `docs/event_fabric/operations.md` 编写运维与高可用指南（多副本部署、复制存储、故障演练），满足 NFR-004。
- [X] **T051 [P] [Polish]** 在 `internal/tests/perf/event_fabric/latency_benchmark_test.go` 与 `internal/tests/perf/event_fabric/throughput_benchmark_test.go` 编写性能基准，校验 NFR-001/002。
- [X] **T052 [P] [Polish]** 新增 `tools/event_fabric_loadtest/` 或脚本（Go/Make target）以运行压力测试并生成报告，记录在 `docs/event_fabric/operations.md`。
- [X] **T053 [P] [Polish]** 在 `internal/service/event_fabric/security` 新增 TLS/签名校验中间件，更新 `config/config.go` 支持密钥配置，并编写自检脚本覆盖 NFR-005。
- [X] **T054 [P] [Polish]** 编写并执行 `scripts/demo/event_fabric_quickstart.sh`，依照 `quickstart.md` 完成端到端验证并保存日志。
- [X] **T055 [Polish]** 运行 `make format vet unit-test proto-event-fabric` 确认 CI 通过，整理提交说明。

---

## Phase 8: Governance & Contract Alignment（新增）

**Purpose**: 固化“单一权威文档 + 三类契约 + 防漂移”治理策略，支撑插件 Framework 的 task/event 接入。

- [x] **T056 [Governance]** 在 `specs/004-eventbus-message-fabric/spec.md` 增加单一权威文档声明，明确 `004` 为 EventBus/TaskBus 契约源头，`023` 仅承载 WS 传输实现。
- [x] **T057 [Governance]** 在 `specs/004-eventbus-message-fabric/spec.md` 补齐“三类契约冻结”（连接/发布/事件）与租户、鉴权优先级及 `proxy=1` 责任边界。
- [x] **T058 [Governance]** 在 `specs/004-eventbus-message-fabric/quickstart.md` 增加联调检查矩阵，覆盖 `local+proxy=0/1` 与 `mode=taskbus/dual/fallback`，并写明预期响应与日志字段。
- [x] **T059 [Governance]** 在 `specs/004-eventbus-message-fabric/spec.md` 增加发布与迁移章节（版本变更记录、插件最小改动清单）。
- [x] **T060 [Governance]** 在 `specs/023-websocket-notify/spec.md` 增加对 `004` 的规范引用（Normative Reference），避免重复定义事件语义。
- [x] **T061 [Governance]** 新增 `specs/004-eventbus-message-fabric/checklists/doc-consistency.md`，沉淀 PR 检查项与 CI 一致性最小规则（topic、路径、envelope、受保护 topic）。
- [x] **T062 [Governance]** 落地一致性检查脚本 `scripts/specs/check_ws_taskbus_contracts.sh`，将规则脚本化（`rg` 校验关键路径/字段，缺失时非 0 退出）。
- [x] **T063 [Governance]** 在 `.github/workflows/` 新增或扩展 CI Job，执行 `T062` 脚本并在 PR 阶段阻断文档漂移。
- [x] **T064 [Governance]** 统一 PR 模板（新增“改 WS/TaskBus 代码必须同步主契约文档”检查项）。

**Checkpoint**: 契约治理流程可执行，代码与文档变更可在 PR/CI 阶段被自动校验。

---

## Phase 9: Queue Driver Strategy（新增）

**Purpose**: 以 Redis 作为默认任务驱动，支持 Kafka/RabbitMQ/NATS 扩展，并将数据库收敛为 fallback，避免高频刷表。

- [x] **T065 [Queue]** 在 `pkg/event_bus/` 与 `internal/service/event_fabric/delivery/` 抽象统一任务驱动接口（enqueue/dequeue/ack/nack/retry），定义 driver capability 与 fallback 契约。
- [x] **T066 [Queue]** 在 `pkg/event_bus/` 实现 Redis 默认驱动的阻塞消费路径（优先 BRPOP/XREADGROUP），并在 `internal/app/shared/deps.go` 完成默认装配与指标埋点。
- [x] **T067 [Queue]** 在 `pkg/event_bus/drivers/kafka/` 落地 Kafka 驱动适配（consumer group、offset 提交、重平衡处理），并接入 `config/config.go` 驱动选择配置。
- [x] **T068 [Queue]** 在 `pkg/event_bus/drivers/rabbitmq/` 与 `pkg/event_bus/drivers/nats/` 提供适配层（ack/nack/重试映射），在 `docs/event_fabric/operations.md` 补齐最小部署参数。
- [x] **T069 [Queue]** 在 `internal/service/event_fabric/delivery/` 与 `internal/app/shared/workers/` 将数据库轮询通道下沉为 fallback 模式，仅在主驱动不可用时启用并输出降级告警。
- [x] **T070 [Queue]** 更新 `specs/004-eventbus-message-fabric/quickstart.md`、`spec.md` 与 `checklists/doc-consistency.md`，补充 driver 切换、降级验证与观测检查单。

**Checkpoint**: 默认运行不再依赖数据库高频轮询；Redis 为主通道，多驱动扩展与 fallback 策略可验证。

---

## Dependencies & Execution Order

### Phase Dependencies
- **Phase 1 Setup** → 无前置，可立即执行。
- **Phase 2 Foundational** → 依赖 Phase 1；完成前禁止进入任何用户故事。
- **Phase 3 (US1)** → 依赖 Phase 2 完成，可独立交付。
- **Phase 4 (US2)** → 依赖 Phase 2；可在 US1 完成后并行推进，但若需复用目录事件通知，请先完成 US1。
- **Phase 5 (US3)** → 依赖 US1 & US2，以确保主题/ACL 能力和审计上下文就绪。
- **Phase 6 (US4)** → 依赖 Phase 5 的投递、存储与审计基础。
- **Phase 7 Polish** → 所有目标用户故事完成后执行。
- **Phase 8 Governance** → 依赖 Phase 7；可在后续增量迭代中独立推进（优先 T062/T063/T064）。
- **Phase 9 Queue Driver Strategy** → 依赖 Phase 8；优先完成 T065/T066/T069（先收敛默认驱动与刷库问题），再推进 T067/T068/T070。
- **Phase 10 Admin UI Ops Console** → 依赖 Phase 9；在统一驱动稳定后补齐可观测与运维界面（T071~T076）。

### User Story Dependencies
- US1 独立（MVP 必须）。
- US2 需要 US1 的主题目录作为前置数据源。
- US3 依赖 US1（主题）与 US2（ACL），并复用 Phase 5 中的审计服务。
- US4 依赖 US3（投递、DLQ、审计）来读取历史事件。

### Parallel Opportunities
- Phase 1 的 T001 与 T002/T003 可并行，因为操作不同文件。
- Phase 3 中 T014/T015（REST Handler vs 测试）可与 T013 并行。
- Phase 4 中 T021/T022（Handler 与测试）可与 T020 并行。
- Phase 5 中 T033/T034/T035 可并行（gRPC/REST/测试分离）。
- Phase 6 中 T044/T045/T046 可并行（HTTP/GRPC/测试）。
- Polish 阶段 T048/T050/T051/T052/T053 可按职责并行推进。

---

## Parallel Execution Example (User Story 3)

```bash
# 并行编写 gRPC 服务与 DLQ Handler（不同目录）
Task: T033 [P] [US3] internal/transport/grpc/event_fabric/publisher_server.go
Task: T034 [P] [US3] internal/transport/http/admin/event_fabric/dlq_handler.go
Task: T035 [P] [US3] internal/tests/grpc/event_fabric/delivery_contract_test.go
```

---

## Implementation Strategy
- **MVP**: 完成 US1，提供主题目录 + 审计 (满足基础可见性)。
- **扩展阶段**: 在 US1 基础上实现 US2（安全）→ US3（高可靠投递 + 审计流水）→ US4（回放）。
- 每个用户故事完成后运行对应合同测试与 Quickstart 节点，确保独立验收。
- Polish 阶段统一处理指标、性能基准、安全加固、HA 文档与 CI 验证，为发布做准备。


---

## Phase 10: Admin UI Ops Console（新增）

**Purpose**: 为统一 Event Fabric/TaskDriver 主路径提供可视化运维界面与接口调试手册，避免依赖 SQL 直查。

- [x] **T071 [UI]** 在 `backend/internal/transport/http/admin/event_fabric/overview_handler.go` 扩展 overview 返回体，增加任务队列统计字段（`task_queue.pending/deferred/processing/inflight`）及 challenge timeout subscriber 维度。
- [x] **T072 [UI]** 在 `backend/internal/transport/http/admin/event_fabric/routes.go` 增加任务队列运维接口（只读统计 + 受控操作）并补齐 DTO。
- [x] **T073 [UI]** 在 `web-admin/app/composables/api/services/eventFabricService.ts` 增加 task queue 相关类型与 API 方法，保持与后端契约一致。
- [x] **T074 [UI]（已收敛）** 旧任务 `web-admin/app/pages/settings/event-fabric.vue` 已废弃，统一收敛到 `web-admin/app/pages/settings/monitor.vue`（队列卡片/过滤/刷新/异常高亮由 T082 及后续 monitor 迭代承接）。
- [x] **T075 [UI]（已收敛）** 旧侧边栏入口任务已被新的 Monitor 入口与权限可见性方案替代（不再维护 Event Fabric 独立旧入口）。
- [x] **T076 [Docs]（已替代）** 原文档任务由 T094 覆盖并完成：联调示例统一为语义 topic key + JWT-only，并补充 register 新语义与故障定位。

**Checkpoint**: 不依赖数据库直查即可在 Admin UI 与 API 层完成 Event Fabric 队列状态观测、处置与回归验证。


## Phase 11: Cron Scheduled Tasks（新增）

**Purpose**: 在统一 TaskDriver 主路径上补齐 Cron 定时任务能力，满足智能体定时触发需求。

- [x] **T077 [Cron]** 在 `backend/pkg/corex/db/persistence/model/event_fabric/` 与 repository 层新增 `scheduled_tasks`/`scheduled_task_runs` 数据模型与读写接口。
- [x] **T078 [Cron]** 在 `backend/internal/service/event_fabric/scheduler/` 实现 cron 解析与 `next_run_at` 计算（支持 `timezone` 与 misfire 策略）。
- [x] **T079 [Cron]** 在 `backend/internal/app/shared/workers/` 增加 cron dispatcher worker：到点投递标准 Task Event（不直接执行业务）。
- [x] **T080 [Cron]** 在 `backend/internal/transport/http/admin/event_fabric/` 增加定时任务 API（当前先落地列表/启停/run-now，统一控制内置 worker）。
- [x] **T081 [Cron]** 在 `web-admin/app/composables/api/services/eventFabricService.ts` 增加 Cron 任务类型与 API。
- [x] **T082 [Cron]** 在 `web-admin/app/pages/settings/monitor.vue` 新增 Task/Cron 管理分区（状态、下次触发、run-now、pause、resume）。
- [x] **T083 [Test]** 增加合同与集成测试：覆盖 cron 触发入队、misfire 行为、失败进入 Retry/DLQ。
- [x] **T084 [Docs]** 更新 `docs/guides/event_fabric/operations.md`、`docs/plan/integration/event_bus.md` 与 `specs/004-eventbus-message-fabric/quickstart.md`，补齐 Cron 调试与验收流程。

**Checkpoint**: 智能体定时任务通过统一 Event Fabric 任务链路可观测、可重试、可重放，无独立轮询消费者。

---

## Phase 12: Topic Governance Unified Refactor（新增）

**Purpose**: 统一 Topic 机制到 `event_topics` 注册治理，收敛 WS/Task 双轨，实现 JWT-only + ACL + 缓存的一致运行时。

- [x] **T085 [Registry]** 在 `backend/internal/service/event_fabric/{delivery,replay}` 完成 topic 语义键解析统一（`namespace.name`）并固定解析链路 `tenant -> global -> system`。
- [x] **T086 [Registry]** 在 `backend/internal/transport/http/admin/event_fabric/{delivery_handler.go,replay_handler.go}` 统一错误映射：`topic not found` 返回 4xx 业务错误，移除 500 兜底噪音。
- [x] **T087 [Registry]** 在 `backend/internal/service/event_fabric/acl/` 引入 ACL 结果缓存接口（Redis + 本地缓存），并定义统一 cache key 规范。
- [x] **T088 [Registry]** 在 `backend/internal/service/event_fabric/` 增加 Topic 解析缓存（Redis cache-aside），覆盖 publish/replay 主路径。
- [x] **T089 [WS]** 改造 `backend/internal/transport/websocket/bus/{authorizer.go,publish.go}`，使 WS subscribe/publish 读取统一 `event_topics` + ACL。
- [x] **T090 [WS]** 下线 WS 内存动态注册数据源（`publishDynamicTopics`），保留兼容开关最多一个小版本。
- [x] **T091 [API]** 调整 `backend/internal/transport/http/admin/runtime/ws_bus_handler.go`：`/internal/ws-bus/register` 收敛为注册中心动作或权限绑定动作，不再直接写内存 topic 真相源。
- [x] **T092 [Model]** 在 `specs/004-eventbus-message-fabric/data-model.md` 与后端模型中落实 `event_topics` 作为 Topic 注册治理唯一真相源，并补齐 subscriber 演进模型（`subscriber_registry` / `topic_subscriptions`）。
- [x] **T093 [Migration]** 在 `backend/pkg/corex/db/migration/` 增加 Topic 注册治理迁移（作用域字段、索引与校验），不走 `event_topics` 删表路线。
- [x] **T094 [Docs]** 更新 `docs/guides/event_fabric/operations.md`、`docs/plan/integration/event_bus.md`、`specs/004-eventbus-message-fabric/quickstart.md` 的联调示例，统一为语义 topic key + JWT-only。
- [x] **T095 [Test]** 增加合同/集成测试：
  - publish/replay 使用语义 topic key 成功；
  - WS subscribe 与 task/replay topic 语义一致；
  - 未注册 topic 返回 4xx；
  - 跨租户请求按 JWT 租户拦截。

**Checkpoint**: WS、Task、Replay 三条链路在同一套 `event_topics` + ACL + JWT 机制下运行，旧双轨实现完成移除。

---

## Phase 13: Event ACL Governance UI（新增）

**Purpose**: 将 Topic-Role 授权管理从监控联调页解耦到“系统设置”，形成清晰治理入口。

- [x] **T096 [UI]** 在 `web-admin/app/pages/settings/` 新增 `event-acl.vue`（或同等路由页），挂载到“系统设置 -> 事件权限（Event ACL）”。
- [x] **T097 [UI]** 在 `web-admin/app/pages/settings/monitor.vue` Topic 视图增加“管理权限”跳转，并透传 `topic_key` 查询参数。
- [x] **T098 [API]** 在 `backend/internal/transport/http/admin/event_fabric/acl_handler.go` 调整 ACL 写接口，支持共享 Topic（`global/system`）授权，不强制 topic tenant 与 jwt tenant 相等。
- [x] **T099 [API]** 在 `backend/internal/transport/http/admin/event_fabric/` 增加 ACL 页面聚合查询接口（按 topic 列角色、按角色反查 topic）。
- [x] **T100 [Test]** 增加 `internal/tests/http/admin/event_fabric/` 合同测试：新租户直接对共享 Topic 授权成功；未授权访问返回 4xx；授权后 WS/replay 生效。
- [x] **T101 [Docs]** 更新 `specs/004-eventbus-message-fabric/{spec.md,plan.md,quickstart.md,data-model.md}` 与运维文档，补充“系统设置 ACL 治理”与“全局 Topic + 角色映射”口径。

**Checkpoint**: 新租户无需手工建 Topic，即可在系统设置完成角色授权并在监控中心验证运行效果。
