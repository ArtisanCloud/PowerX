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

## Phase 6: 用例对齐 - 插件自动注册与沙箱准入 (UC-AGENT-REG-AUTO-001)

**目标**：实现插件 manifest 自动注册、签名/Schema 校验、IAM 策略绑定、沙箱验证与审计闭环。
**Independent Test**：使用沙箱插件包触发 HTTP/gRPC Webhook，验证 5 秒 SLA、回滚与事件输出。

### Tests

 - [x] T043 [P] [AUTO] 在 `tests/contract/agent_lifecycle/autoreg_http_test.go` 编写 HTTP 合同测试，覆盖 manifest 提交、签名异常、重复注册阻断。
 - [x] T044 [P] [AUTO] 在 `tests/contract/agent_lifecycle/autoreg_grpc_test.go` 编写 gRPC 合同测试，验证 RegisterManifest/FinalizeSandbox RPC。
 - [x] T045 [P] [AUTO] 在 `tests/integration/agent_lifecycle/autoreg_sandbox_flow_test.go` 模拟完整注册→沙箱→激活流程，校验 IAM 策略、审计与回滚记录。

### Implementation

 - [x] T046 [AUTO] 在 `internal/transport/http/openapi/agent/autoreg_handlers.go` 与 gRPC 服务中实现 manifest Webhook、重试与速率限制。
 - [x] T047 [AUTO] 在 `internal/service/agent_lifecycle/autoreg_validator.go` 实现 Schema/签名校验、重复检测与错误回滚，复用安全组件。
 - [x] T048 [AUTO] 在 `internal/service/agent_lifecycle/sandbox_runner.go` 编排沙箱执行、指标采集、报告上传，并与生命周期事件/审计集成。

---

## Phase 7: 用例对齐 - 租户自助创建与审批 (UC-AGENT-REG-TENANT-001)

**目标**：为租户管理员提供表单、策略冲突检测、审批编排、沙箱激活与审计记录能力。
**Independent Test**：通过租户 API 驱动提交流程，验证权限冲突、审批驳回与沙箱失败场景。

### Tests

 - [x] T049 [P] [TENANT] 在 `tests/contract/agent_lifecycle/tenant_form_http_test.go` 覆盖表单提交、校验错误与回退。
 - [x] T050 [P] [TENANT] 在 `tests/integration/agent_lifecycle/tenant_approval_flow_test.go` 演练提交→审批→沙箱→激活路径，断言状态同步与通知。

### Implementation

 - [x] T051 [TENANT] 在 `internal/transport/http/admin/agent/tenant_handlers.go` 与 `dto_tenant.go` 实现表单 API、版本化字段及草稿存储。
 - [x] T052 [TENANT] 在 `internal/service/agent_lifecycle/policy_conflict_engine.go` 实现权限/速率冲突检测与提示。
 - [x] T053 [TENANT] 在 `internal/workflow/agent_approval_flow.go` 对接 Workflow/Notification，支持多级审批、驳回、撤销。
 - [x] T054 [TENANT] 在 `internal/service/agent_lifecycle/tenant_activation.go` 对接沙箱 runner、凭证发放与生命周期激活事件。

---

## Phase 8: 用例对齐 - 多租户共享与撤销 (UC-AGENT-REG-SHARE-001)

**目标**：实现共享/撤销 API、配额复制、租户验证、合规审计与告警。
**Independent Test**：模拟共享申请、租户验证失败与撤销流程，验证配额/凭证复制与告警链路。

### Tests

- [x] T055a [P] [SHARE] 在 `tests/contract/agent_lifecycle/share_http_test.go` 编写共享申请成功+重复/白名单冲突场景。
- [x] T055b [P] [SHARE] 在同文件编写撤销/自动回滚/权限拒绝场景。
- [x] T055c [P] [SHARE] 在 `tests/contract/agent_lifecycle/share_grpc_test.go` 覆盖 ShareAgent/RevokeAgentShare RPC。
- [x] T055d [P] [SHARE] 在 `tests/contract/agent_lifecycle/share_events_test.go` 校验 `agent.share.*` 事件载荷。
- [x] T056a [P] [SHARE] 在 `tests/integration/agent_lifecycle/share_validation_flow_test.go` 演练共享→验证→撤销闭环。
- [x] T056b [P] [SHARE] 在 `tests/integration/agent_lifecycle/share_revocation_failure_test.go` 模拟验证失败/撤销重试，确认告警路径。

### Implementation

- [x] T057a [SHARE] 定义共享数据模型/仓储：`AgentShareRecord` + `AgentShareRepository`（含唯一索引、状态字段）。
- [x] T057b [SHARE] 在 `internal/service/agent_lifecycle/share_service.go` 实现 ShareAgent/RevokeAgentShare 业务逻辑（审计事件、事件总线、冲突校验）。
- [x] T057c [SHARE] 在 `internal/transport/http/admin/agent/share_handlers.go` + `grpc/agentlifecycle/server.go` 暴露共享/撤销 API。
- [x] T057d [SHARE] 在 `internal/transport/http/admin/routes.go` & OpenAPI/Proto 中挂载新端点。
- [x] T058a [SHARE] 在 `internal/service/agent_lifecycle/quota_provisioner.go` 实现配额/凭证复制、冲突检测、回滚补偿。
- [x] T058b [SHARE] 接入 IAM/Secret/RateLimit 模块（可用 mock）并在失败时回滚共享记录。
- [x] T058c [SHARE] 在 `internal/service/agent_lifecycle/share_models.go` 定义配额/共享响应结构。
- [x] T059a [SHARE] 在 `internal/service/agent_lifecycle/tenant_validator.go` 编排租户验证（沙箱调用、日志分区/上下文隔离校验）。
- [x] T059b [SHARE] 实现验证失败回滚逻辑（调用 RevokeAgentShare 并写审计、告警）。
- [x] T060a [SHARE] 在 `internal/service/agent_lifecycle/share_compliance.go` 实现周期复核（到期撤销、验证失败告警、指标写入）。
- [x] T060b [SHARE] 在 `internal/notifications/im/` & 事件模块中扩展 `agent.share.issued/revoked/validation_failed` 告警。
- [x] T060c [SHARE] 更新 `contracts/http-openapi.yaml` 与 `agent_lifecycle.proto`、Quickstart，补充共享 API 说明。

---

## Phase 9: 场景桥接 - ReAct & 任务执行可观测 (SCN-AGENT-REACT-ORCH-001 & SCN-AGENT-TASK-EXEC-001)

**目标**：为 ReAct Thought/Action/Memory/Audit 以及 Task Execution Plan/Coord/Recovery/Closure 提供实时事件、Trace、查询与控制接口。
**Independent Test**：通过 StateBus 与 API 模拟 ReAct/任务执行链路，验证事件延迟、Trace 完整性与控制操作。

### Tests

- [x] T061 [P] [BRIDGE] 在 `tests/contract/agent_lifecycle/statebus_event_schema_test.go` 校验 `agent.lifecycle.*` 事件 Schema 与必填字段。
- [x] T062 [P] [BRIDGE] 在 `tests/integration/agent_lifecycle/react_task_bridge_test.go` 模拟 ReAct/任务执行订阅事件、查询 API、触发冻结/回滚。

### Implementation

- [x] T063 [BRIDGE] 在 `internal/service/agent_lifecycle/event_streamer.go` 统一 StateBus 推送、Trace 透传与回放引用。
- [x] T064 [BRIDGE] 在 `internal/transport/http/openapi/agent/bridge_handlers.go` 提供 Planner/Coordinator/Recovery/Closure 所需的查询与控制接口。
- [x] T065 [BRIDGE] 在 `internal/service/agent_lifecycle/audit_bridge.go` 整合生命周期审计、健康摘要与回放接口，供 ReAct Audit/Task Closure 使用。
- [x] T066 [BRIDGE] 更新 `backend/config/statebus/topics.yaml`、`docs/quickstart`，同步事件 Schema、订阅示例与 Copilot Handoff 集成指南。

---

## Phase 10: Polish & Cross-Cutting

- [x] T039 [P] 在 `tests/unit/agent_lifecycle/` 补充仓储、事件发布、告警与订阅逻辑单元测试。
- [x] T040 在 `specs/008-agent-lifecycle-observability/quickstart.md`、`README.md` 更新运行指南，覆盖订阅配置与保留策略说明。
- [x] T041 执行 Quickstart 流程（注册→扩容→订阅→告警），收集指标/日志截图并附于文档。
- [x] T042 校验 `make proto-gen`, `make unit-test`, `make test-all` 全量通过，补充依赖或脚本更新。

---

## Dependencies & Execution Order

- Phase 1 → Phase 2 → Phase 3~5（US1~US3）→ Phase 6（AutoReg）→ Phase 7（Tenant）→ Phase 8（Share）→ Phase 9（Bridge）→ Phase 10（Polish）；任何新 Phase 必须在前序阶段完成后再启动。
- T016/T024/T035/T038/T046/T057/T063 依赖同一 Proto/事件 Schema，按时间顺序更新以避免冲突；Schema 变更需同步 buf 生成。
- Redis/EventBus 集成（T008, T022, T025, T063）需按序执行，确保缓存与 StateBus 主题在事件桥接前就绪。
- 数据保留（T026）依赖仓储实现（T004、T006）与配置加载（T007），共享/桥接任务引用相同查询接口。
- 告警实现（T036、T060）依赖通知发送器（T010）与健康/共享策略（T033、T058）。
- AutoReg 与 Tenant/Share 阶段均依赖 Sandbox Runner（T048、T054、T059），需在 Phase 6 完成核心 Runner 后复用。

### 可并行任务示例

```bash
# 并行启动 US1 的合同测试开发
task run --id T011 &
task run --id T012 &

# 并行实现仓储模型
task run --id T004 &
task run --id T006 &

# 并行推进共享/桥接测试
task run --id T055 &
task run --id T061 &
```

完成每个阶段后建议执行相关测试与 lint，确保增量可演示与可回滚。任务完成后，更新任务清单与提交记录以保持可追溯性。
