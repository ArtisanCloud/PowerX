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
- [x] **T002 [Setup]** 更新 `api/grpc/contracts/buf.yaml` 与 `api/grpc/contracts/buf.gen.yaml` 注册 `corex/event_fabric/v1` Proto 包，校验 `managed.go_package_prefix`、`api/grpc/gen` 输出路径，并在 `Makefile` 新增 `proto-event-fabric` 目标触发代码生成。
- [x] **T003 [Setup]** 扩展 `config/config.go`, `config/validator.go`，新增 `EventFabricConfig`（含重试/ACK 超时/Redis 配置），并在 `.env.sample` 说明默认值。
- [x] **T004 [Setup]** 在 `internal/transport/http/admin/routes.go` 注册 `/event-fabric` 路由分组与占位 Handler，便于后续故事挂载。
- [x] **T005 [Setup]** 在 `api/grpc/contracts/corex/event_fabric/v1/event_fabric.proto` 定义基础消息与服务骨架（目录、ACL、Publish/Subscribe、Replay、DLQ），确保契约覆盖 FR-003~FR-009 并能通过 `buf lint`。
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

- [x] **T010 [US1]** 在 `pkg/corex/db/migration/202510170001_create_event_topics.go` 定义 `event_topics` 表（含租户、命名空间、生命周期、payload_format、max_retry 等字段）。
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

- [ ] **T017 [US2]** 在迁移文件 `pkg/corex/db/migration/202510170001_create_event_topics.go` 中补充 `event_acl_bindings` 表结构（或新增 `202510170002_create_event_acl_bindings.go`），包含唯一键 (`tenant_id`,`topic_id`,`principal_id`,`action`) 与过期时间。
- [ ] **T018 [US2]** 增加模型 `pkg/corex/db/persistence/model/event_fabric/acl_binding.go`。
- [ ] **T019 [US2]** 编写仓储 `pkg/corex/db/persistence/repository/event_fabric/acl_repository.go`，支持批量授予/撤销与有效期过滤。
- [ ] **T020 [US2]** 实现 `internal/service/event_fabric/acl/service.go`：整合安全策略（引用 Security Policy 客户端）、审计记录、冲突检测。
- [ ] **T021 [P] [US2]** 完成 REST Handler `internal/transport/http/admin/event_fabric/acl_handler.go`，覆盖 `/acl` 批量接口与鉴权，并更新 OpenAPI 合同的请求/响应定义。
- [ ] **T022 [P] [US2]** 在 `internal/tests/http/admin/event_fabric/acl_contract_test.go` 添加授权/撤销/违规访问合同测试。
- [ ] **T023 [US2]** 在 `internal/service/event_fabric/acl/enforcer.go` 集成运行时权限校验（供 Publish/Subscribe 使用），并在 `internal/app/shared/deps.go` 注入。

**Checkpoint**: ACL 管道生效，可独立验证授权与审计逻辑。

---

## Phase 5: User Story 3 - 高可靠投递与死信补偿 (Priority: P1)

**Goal**: 实现至少一次投递、重试/退避、DLQ 与 gRPC 长连接订阅。
**Independent Test**: 模拟发布/消费失败，验证重试 5 次后写入 DLQ；通过回放/补偿 API 成功恢复。

### Implementation

- [ ] **T024 [US3]** 在新迁移 `pkg/corex/db/migration/202510170003_create_event_delivery_tables.go` 创建 `event_envelopes`, `event_delivery_attempts`, `event_dlq_messages`, `event_subscription_offsets` 表。
- [ ] **T025 [US3]** 增加对应模型文件（`event_envelope.go`, `delivery_attempt.go`, `dlq_message.go`, `subscription_offset.go`）于 `pkg/corex/db/persistence/model/event_fabric/`。
- [ ] **T026 [US3]** 构建仓储 `pkg/corex/db/persistence/repository/event_fabric/{envelope_repository.go,delivery_repository.go,dlq_repository.go}`，封装投递与状态变更。
- [ ] **T027 [US3]** 扩展 `pkg/event_bus`：新增接口支持 Ack/Nack、重试计划、幂等窗口（包含订阅者并发/超时配置），默认 Redis 实现位于 `pkg/event_bus/redis_retry.go`。
- [ ] **T028 [US3]** 编写 `internal/service/event_fabric/delivery/backoff_scheduler.go` 使用 Redis Sorted Set 计算指数退避与抖动。
- [ ] **T029 [US3]** 实现 `internal/service/event_fabric/delivery/service.go`：发布入库、推送订阅者、重试决策、订阅幂等策略、DLQ 写入。
- [ ] **T030 [US3]** 构建 `internal/service/event_fabric/audit/service.go`，封装发布/订阅审计写入（调用审计客户端、覆盖成功/失败场景），并在 `internal/app/shared/deps.go` 注册。
- [ ] **T031 [US3]** 在 `internal/service/event_fabric/delivery/service.go`、`internal/transport/{grpc,http}/event_fabric/*` 集成审计记录；于 `internal/tests/{grpc,http}/event_fabric/audit_contract_test.go` 编写测试验证发布/订阅审计流水与错误回滚。
- [ ] **T032 [US3]** 创建 DLQ 服务 `internal/service/event_fabric/dlq/service.go`，支持查询、重放、告警钩子。
- [ ] **T033 [P] [US3]** 实现 gRPC 服务 `internal/transport/grpc/event_fabric/publisher_server.go` 与 `subscriber_server.go`（使用 `api/grpc/contracts/corex/event_fabric/v1/event_fabric.proto`），接入拦截器并更新 Proto 契约中的 RPC 定义。
- [ ] **T034 [P] [US3]** 编写 Admin REST Handler `internal/transport/http/admin/event_fabric/dlq_handler.go` 与 `/publish` 端点（调用 delivery 服务），同步扩充 OpenAPI 合同。
- [ ] **T035 [P] [US3]** 在 `internal/tests/grpc/event_fabric/delivery_contract_test.go` 覆盖 Publish/Ack/Nack/重试 流程；在 `internal/tests/http/admin/event_fabric/dlq_contract_test.go` 验证 DLQ 操作与审计。
- [ ] **T036 [US3]** 新增后台 worker（可挂载于 `internal/app/shared/workers/event_fabric_retry.go`）周期拉取 Redis 重试队列并调用 delivery 服务。
- [ ] **T037 [US3]** 更新 `internal/server/mcp/templates/usecase.tmpl`（或相关模板）以支持事件发布用例示例。

**Checkpoint**: 完成至少一次投递、重试、DLQ 与审计流水接入，gRPC/REST 通路均可独立测试。

---

## Phase 6: User Story 4 - 事件契约与回放能力 (Priority: P2)

**Goal**: 提供事件回放 API，支持版本协商与影子通路。
**Independent Test**: 针对指定时间窗口与 Trace ID 发起回放，确认订阅者收到 `replay=true` 标记且不影响实时流量；订阅者可声明可接受的事件版本并正确协商。

### Implementation

- [ ] **T038 [US4]** 扩展 `internal/service/event_fabric/delivery/version_negotiator.go` 与 `delivery/service.go`，实现事件版本协商策略（strict/backward/any）并记录兼容结果。
- [ ] **T039 [US4]** 更新订阅注册入口：在 `internal/transport/grpc/event_fabric/subscriber_server.go` 与 Admin `/topics` Handler 中允许消费者声明兼容策略与支持版本列表，并同步更新 Proto/OpenAPI 契约。
- [ ] **T040 [P] [US4]** 编写 `internal/tests/grpc/event_fabric/version_negotiation_test.go` 与 `internal/tests/http/admin/event_fabric/topic_version_contract_test.go` 验证版本协商行为。
- [ ] **T041 [US4]** 在迁移 `pkg/corex/db/migration/202510170004_create_event_replay_tables.go` 定义 `event_replay_requests` 表。
- [ ] **T042 [US4]** 添加模型与仓储 `pkg/corex/db/persistence/model/event_fabric/replay_request.go`、`repository/event_fabric/replay_repository.go`。
- [ ] **T043 [US4]** 实现 `internal/service/event_fabric/replay/service.go`：支持回放任务调度、状态机、影子通路（确保不触发业务副作用）。
- [ ] **T044 [P] [US4]** 在 `internal/transport/http/admin/event_fabric/replay_handler.go` 提供回放 API；更新 `routes.go` 挂载并扩充 OpenAPI 合同。
- [ ] **T045 [P] [US4]** 实现 gRPC `internal/transport/grpc/event_fabric/replay_server.go` 对应 `EventReplayService`，同步维护 Proto 定义。
- [ ] **T046 [P] [US4]** 编写测试 `internal/tests/http/admin/event_fabric/replay_contract_test.go` 与 `internal/tests/grpc/event_fabric/replay_contract_test.go` 验证回放行为。
- [ ] **T047 [US4]** 更新 `quickstart.md` 增补回放与版本协商示例，确保影子通路操作说明。

**Checkpoint**: 回放与版本协商能力上线，P1 + P2 用户故事均可独立演示。

---

## Phase 7: Polish & Hardening

**Purpose**: 收尾性能、可观测性、安全与发布准备工作。

- [ ] **T048 [P] [Polish]** 在 `internal/service/event_fabric/metrics/metrics.go` 输出投递成功率、重试延迟、DLQ 积压、回放耗时指标；更新 `internal/app/shared/deps.go` 注册。
- [ ] **T049 [Polish]** 扩展 `pkg/dto/error.go`、`pkg/dto/base.go` 增加事件 Fabric 错误码与统一响应。
- [ ] **T050 [P] [Polish]** 在 `docs/event_fabric/operations.md` 编写运维与高可用指南（多副本部署、复制存储、故障演练），满足 NFR-004。
- [ ] **T051 [P] [Polish]** 在 `internal/tests/perf/event_fabric/latency_benchmark_test.go` 与 `internal/tests/perf/event_fabric/throughput_benchmark_test.go` 编写性能基准，校验 NFR-001/002。
- [ ] **T052 [P] [Polish]** 新增 `tools/event_fabric_loadtest/` 或脚本（Go/Make target）以运行压力测试并生成报告，记录在 `docs/event_fabric/operations.md`。
- [ ] **T053 [P] [Polish]** 在 `internal/service/event_fabric/security` 新增 TLS/签名校验中间件，更新 `config/config.go` 支持密钥配置，并编写自检脚本覆盖 NFR-005。
- [ ] **T054 [P] [Polish]** 编写并执行 `scripts/demo/event_fabric_quickstart.sh`，依照 `quickstart.md` 完成端到端验证并保存日志。
- [ ] **T055 [Polish]** 运行 `make format vet unit-test proto-event-fabric` 确认 CI 通过，整理提交说明。

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
