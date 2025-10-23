# Tasks: Agent Lifecycle & Observability

**Input**: Design documents from `/specs/008-agent-lifecycle-observability/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

## Phase 1: Setup（共享基础）

- [x] T001 在 `api/grpc/contract/buf.yaml` 与 `buf.gen.yaml` 注册 `powerx/agent/v1` proto 包路径，并创建 `api/grpc/contracts/powerx/agent/v1/` 目录骨架，确保生成目标指向 `api/grpc/gen`.
- [x] T002 初始化目录结构：`internal/service/agent_lifecycle/`, `internal/transport/http/{admin,openapi}/agent/`, `internal/transport/grpc/agentlifecycle/`, `internal/notifications/im/`，并添加 README 说明用途，保持 Go 包非空。
- [x] T003 校准 Makefile：在 `Makefile` 的 `proto-gen`/`proto-lint`/`proto-clean` 目标中覆盖新的 agent proto 路径。

---

## Phase 2: Foundational（阻断性前置）

- [x] T004 [P] 在 `pkg/corex/db/persistence/model/agent/` 中新增 `profile.go`, `lifecycle_event.go`, `health_snapshot.go` 定义 GORM 模型，满足数据模型字段与约束。
- [x] T005 在 `pkg/corex/db/database/migration.go` 与 `cmd/database/migrate.go` 注册 Agent 模型 AutoMigrate 顺序，保持与现有模块一致。
- [x] T006 [P] 在 `pkg/corex/db/persistence/repository/agent/` 下实现 `profile_repository.go`, `lifecycle_repository.go`, `health_repository.go`，嵌入 `BaseRepository` 并提供查询/写入接口。
- [x] T007 更新 `config/defaults.go`, `etc/config.yaml`, `config/config.go`，新增 `agent_lifecycle` 配置段（事件前缀、默认容量、通知 webhook 等）并完成校验。
- [x] T008 在 `internal/app/shared/deps.go` 注入 AgentLifecycle 所需依赖（Redis、EventBus、Audit、Telemetry），并注册 Redis key 前缀。
- [x] T009 在 `internal/service/agent_lifecycle/instrumentation/` 建立指标与追踪封装，提供健康评分计算与事件 trace 透传接口。
- [x] T010 在 `internal/notifications/im/` 实现企业 IM Webhook 发送器与重试策略，供健康告警调用。

**Checkpoint**：完成以上任务后，生命周期服务具备持久化、依赖注入与观测基线，可开展用户故事开发。

---

## Phase 3: 用户故事 1 - 统一注册与启用代理 (Priority: P1)

**目标**：管理员可注册代理、校验依赖并激活，发布生命周期事件与审计记录。
**Independent Test**：仅部署生命周期接口，通过注册→激活流程验证状态变更与事件。

### Tests（先于实现）

- [x] T011 [P] [US1] 在 `tests/contract/agent_lifecycle/admin_http_test.go` 编写 HTTP 合同测试（注册/激活），基于 `contracts/http-openapi.yaml`.
- [x] T012 [P] [US1] 在 `tests/contract/agent_lifecycle/admin_grpc_test.go` 编写 gRPC 合同测试（Register/Activate），基于 `contracts/agent_lifecycle.proto`.
- [x] T013 [P] [US1] 在 `tests/integration/agent_lifecycle/registration_activation_flow_test.go` 覆盖注册→激活端到端场景，校验事件与审计写入。

### Implementation

- [x] T014 [US1] 在 `internal/service/agent_lifecycle/registry.go` 实现注册/激活逻辑：依赖校验、状态机、事件发布、审计写入。
- [x] T015 [US1] 在 `internal/transport/http/admin/agent/handlers.go` & `dto.go` 实现注册/激活 Handler、请求校验与响应包装。
- [x] T016 [US1] 在 `internal/transport/grpc/agentlifecycle/service.go` 实现 RegisterAgent/ActivateAgent RPC，并在 `internal/server/grpc/server.go` 注册服务。
- [x] T017 [US1] 在 `internal/service/agent_lifecycle/events.go` 抽象生命周期事件发布与补偿逻辑，并联动 EventBus 主题。

**Checkpoint**：管理员可完成代理注册与激活，相关测试全部通过。

---

## Phase 4: 用户故事 2 - 按需调度代理容量 (Priority: P2)

**目标**：运维可暂停/恢复/退役/扩缩容代理，系统维护容量与事件一致性，并满足退役后 13 个月保留要求。  
**Independent Test**：启动生命周期控制接口，执行 pause/resume/scale/retire 指令验证状态、事件与数据保留。

### Tests（先于实现）

- [x] T018 [P] [US2] 在 `tests/contract/agent_lifecycle/admin_http_test.go` 扩展 HTTP 合同测试，覆盖暂停/恢复/退役/扩缩容端点。
- [x] T019 [P] [US2] 在 `tests/contract/agent_lifecycle/admin_grpc_test.go` 扩展 gRPC 合同测试，验证 Pause/Resume/Retire/Scale RPC。
- [x] T020 [P] [US2] 在 `tests/integration/agent_lifecycle/registration_activation_flow_test.go` 覆盖注册→激活→调度流程，断言事件输出。
- [x] T021 [P] [US2] 在 `tests/integration/agent_lifecycle/retirement_retention_test.go` 验证退役代理 13 个月保留与可查询能力。

### Implementation

- [x] T022 [US2] 在 `internal/service/agent_lifecycle/service.go` 扩展状态机，处理暂停/恢复/退役/扩缩容逻辑并发布事件。
- [x] T023 [US2] 在 `internal/transport/http/admin/agentlifecycle/handlers.go` 增补生命周期控制 Handler，涵盖错误处理与审计。
- [x] T024 [US2] 在 `internal/transport/grpc/agentlifecycle/server.go` 实现 Pause/Resume/Retire/Scale RPC 并返回生命周期事件。
- [x] T025 [US2] 在 `internal/service/agent_lifecycle/instrumentation/instrumentation.go` 记录生命周期与健康观测指标。
- [x] T026 [US2] 在 `internal/service/agent_lifecycle/health.go` 编排健康快照与保留策略，支撑 ≥13 个月查询能力。

**Checkpoint**：运维可独立调度代理容量，事件、缓存与数据保留策略保持一致。

---

## Phase 5: 用户故事 3 - 端到端可观测与告警 (Priority: P3)

**目标**：SRE 可查看实时健康评分与历史趋势，退化自动告警至企业 IM。
**Independent Test**：模拟指标/日志/追踪异常，验证健康视图与告警通知。

### Tests（先于实现）

- [x] T027 [P] [US3] 在 `tests/contract/agent_lifecycle/health_http_test.go` 编写 HTTP 合同测试，覆盖健康摘要/历史端点。
- [x] T028 [P] [US3] 在 `tests/contract/agent_lifecycle/health_grpc_test.go` 编写 gRPC 合同测试，覆盖 GetHealthSummary/ListHealthSnapshots RPC。
- [x] T029 [P] [US3] 在 `tests/integration/agent_lifecycle/health_alert_flow_test.go` 模拟健康退化→告警流程，校验 IM 通知与自动事件。
- [x] T030 [P] [US3] 在 `tests/contract/agent_lifecycle/subscription_http_test.go` 编写 HTTP 合同测试，验证订阅创建/更新即时生效与回滚。
- [x] T031 [P] [US3] 在 `tests/contract/agent_lifecycle/subscription_grpc_test.go` 编写 gRPC 合同测试，覆盖订阅配置 RPC。
- [x] T032 [P] [US3] 在 `tests/integration/agent_lifecycle/subscription_effect_test.go` 模拟订阅变更后指标/告警过滤即时生效场景。

### Implementation

- [x] T033 [US3] 在 `internal/service/agent_lifecycle/health.go` 实现健康评分聚合、阈值判定与历史查询（依赖仓储）。
- [x] T034 [US3] 在 `internal/transport/http/openapi/agent/health_handlers.go` 实现健康摘要/历史 Handler，封装推荐动作返回。
- [x] T035 [US3] 在 `internal/transport/grpc/agentlifecycle/health_service.go` 实现健康相关 RPC，并复用 instrumentation 输出。
- [x] T036 [US3] 在 `internal/service/agent_lifecycle/instrumentation/alerts.go` & `internal/notifications/im/sender.go` 实现退化告警触发、节流与重试。
- [x] T037 [US3] 在 `internal/service/agent_lifecycle/subscription.go` 实现订阅保存、校验、缓存刷新与回滚逻辑。
- [x] T038 [US3] 在 `internal/transport/http/admin/agent/subscription_handlers.go` 与 `internal/transport/grpc/agentlifecycle/subscription_service.go` 实现订阅接口映射，并在 `internal/server/grpc/server.go` 注册。

**Checkpoint**：健康评分与告警可用，SRE 能独立定位异常。

---

## Phase 6: Polish & Cross-Cutting

- [ ] T039 [P] 在 `tests/unit/agent_lifecycle/` 补充仓储、事件发布、告警与订阅逻辑单元测试。
- [ ] T040 在 `specs/008-agent-lifecycle-observability/quickstart.md`、`README.md` 更新运行指南，覆盖订阅配置与保留策略说明。
- [ ] T041 执行 Quickstart 流程（注册→扩容→订阅→告警），收集指标/日志截图并附于文档。
- [ ] T042 校验 `make proto-gen`, `make unit-test`, `make test-all` 全量通过，补充依赖或脚本更新。

---

## Dependencies & Execution Order

- Phase 1 → Phase 2 → User Stories → Phase 6；任何用户故事开始前必须完成 Phase 2。
- T016/T024/T035/T038 依赖同一 gRPC 文件，严格按 US1 → US2 → US3 顺序编辑，避免冲突。
- Redis/EventBus 集成（T008, T022, T025）顺序执行，确保缓存结构先定义再在服务层使用。
- 数据保留（T026）依赖仓储实现（T004、T006）与配置加载（T007）。
- 告警实现（T036）依赖通知发送器（T010）与健康聚合（T033）。

### 可并行任务示例

```bash
# 并行启动 US1 的合同测试开发
task run --id T011 &
task run --id T012 &

# 并行实现仓储模型
task run --id T004 &
task run --id T006 &
```

完成每个阶段后建议执行相关测试与 lint，确保增量可演示与可回滚。任务完成后，更新任务清单与提交记录以保持可追溯性。
