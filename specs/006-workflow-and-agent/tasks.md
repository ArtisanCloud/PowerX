# Tasks: Workflow & Agent Orchestration

**Input**: Design documents from `/specs/006-workflow-and-agent/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: 按用户故事列出，若无特别说明皆采用 TDD（测试先行）。

**Organization**: 依优先级用户故事拆分，确保每个故事可独立交付与验证。

## Format: `[ID] [P?] [Story] Description`
- **[P]**: 可并行执行（不同文件、无直接依赖）
- **[Story]**: 所属用户故事（Setup/Found/US1/US2/US3/Polish）
- 任务描述中务必包含精确文件路径

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 建立合同与生成流程基础

- [X] T001 [Setup] 更新 buf 配置 (`api/grpc/contracts/buf.yaml`, `api/grpc/contracts/buf.gen.yaml`, `Makefile`) 以注册 `powerx/workflow/v1` 合同并确保 `make proto-*` 覆盖
- [X] T002 [P] [Setup] 编写 gRPC 合同 `api/grpc/contracts/powerx/workflow/v1/workflow.proto`（定义 WorkflowService 与消息结构）
- [X] T003 [P] [Setup] 编写 HTTP OpenAPI 合同 `specs/006-workflow-and-agent/contracts/http-openapi.yaml`（覆盖 definitions/instances/export 端点）
- [X] T004 [Setup] 运行 `make proto-gen` 并提交生成代码 `api/grpc/gen/go/powerx/workflow/v1/`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 建立域模型、仓储、服务骨架 —— ✅ 完成前禁止进入用户故事实现

- [X] T005 [P] [Found] 实现 `WorkflowDefinition` 模型 (`pkg/corex/db/persistence/model/workflow/definition.go`)
- [X] T006 [P] [Found] 实现 `WorkflowInstance` 模型 (`pkg/corex/db/persistence/model/workflow/instance.go`)
- [X] T007 [P] [Found] 实现 `WorkflowStepRecord` 模型 (`pkg/corex/db/persistence/model/workflow/step_record.go`)
- [X] T008 [P] [Found] 实现 `WorkflowStepCompensation` 模型 (`pkg/corex/db/persistence/model/workflow/compensation.go`)
- [X] T009 [P] [Found] 实现 `AgentAssignment` 模型 (`pkg/corex/db/persistence/model/workflow/agent_assignment.go`)
- [X] T010 [P] [Found] 实现 `WorkflowEvent` 投影模型 (`pkg/corex/db/persistence/model/workflow/event.go`)
- [X] T011 [Found] 将上述模型注册到迁移流程 (`pkg/corex/db/database/migration.go`, `cmd/database/migrate.go`)
- [X] T012 [P] [Found] 创建 `WorkflowDefinitionRepository` (`pkg/corex/db/persistence/repository/workflow/definition_repository.go`)
- [X] T013 [P] [Found] 创建 `WorkflowInstanceRepository` (`pkg/corex/db/persistence/repository/workflow/instance_repository.go`)
- [X] T014 [P] [Found] 创建 `WorkflowStepRecordRepository` (`pkg/corex/db/persistence/repository/workflow/step_record_repository.go`)
- [X] T015 [P] [Found] 创建 `AgentAssignmentRepository` (`pkg/corex/db/persistence/repository/workflow/assignment_repository.go`)
- [X] T016 [Found] 建立服务骨架 (`internal/service/workflow/service.go`) 注入仓储、定义接口（Create/Publish/Start/Control 等）
- [X] T017 [Found] 实作调度器与 Redis 定时器骨架 (`internal/service/workflow/scheduler.go`) 处理队列、重试占位逻辑
- [X] T018 [Found] 更新依赖注入 (`internal/app/shared/deps.go`, `internal/app/shared/options.go`) 装配 Workflow 服务/调度器/仓储
- [X] T019 [Found] 在 gRPC Server 注册 WorkflowService (`internal/server/grpc/server.go`)，确保拦截器链完整

**Checkpoint**: 基础设施就绪，可启动用户故事开发

---

## Phase 3: User Story 1 - 设计与启动编排 (Priority: P1) 🎯 MVP

**Goal**: 允许设计者创建/发布工作流并发起实例运行

**Independent Test**: 通过 API 测试创建定义→发布→启动实例全链路

### Tests for User Story 1

- [X] T020 [P] [US1] gRPC 合同测试：覆盖 `CreateDefinition/PublishDefinition/ListDefinitions` (`tests/contract/workflow/workflow_definitions_grpc_test.go`)
- [X] T021 [P] [US1] HTTP 合同测试：覆盖 `POST/GET /definitions` 与 `POST /instances` (`tests/contract/workflow/workflow_definitions_http_test.go`)
- [X] T022 [P] [US1] 集成测试：创建定义并启动实例流程 (`tests/integration/workflow/test_definition_launch_flow.go`)
- [X] T054 [P] [US1] 单元测试：覆盖 StepGraph 执行器路由及并行/决策/人工步骤 (`tests/unit/workflow_executor_test.go`)
- [X] T056 [P] [US1] 安全契约测试：验证创建/发布/启动接口的 JWT/JWKS 校验与 RBAC 拦截 (`tests/contract/workflow/workflow_security_http_test.go`)
- [X] T060 [P] [US1] gRPC 安全契约测试：验证 WorkflowService 拦截器链在未授权/未认证场景下的行为 (`tests/contract/workflow/workflow_security_grpc_test.go`)

### Implementation for User Story 1

- [X] T023 [US1] 完成定义管理逻辑 (`internal/service/workflow/service_definition.go`)：创建、验证 step_graph、发布版本
- [X] T024 [US1] 完成实例启动逻辑 (`internal/service/workflow/service_instance.go`)：写入上下文、排队首个步骤
- [X] T025 [US1] 实现 gRPC Handler (`internal/transport/grpc/workflow/definition_handler.go`) 对接服务层并返回审计 metadata
- [X] T026 [US1] 实现 HTTP Handler (`internal/transport/http/admin/workflow/definitions_handler.go`) 包含请求校验、错误映射
- [X] T027 [P] [US1] 编写 StepGraph 验证/归一化工具 (`internal/service/workflow/validator.go`)，供创建实例与执行前校验使用
- [X] T028 [US1] 将 WorkflowDefinition/Instance 相关事件写入 EventBus (`internal/service/workflow/event_emitter.go`) 并设置审计字段
- [X] T051 [US1] 完成系统/决策/并行/人工审批等步骤类型执行器与路由 (`internal/service/workflow/executor_router.go`, `internal/service/workflow/executor_*.go`)
- [X] T052 [US1] 提供工作流定义模板与校验提示（提升设计效率）(`internal/service/workflow/service_definition.go`, `specs/006-workflow-and-agent/quickstart.md`)
- [X] T057 [US1] 复用并验证 RBAC 配置：为 Workflow API 注册角色/动作映射 (`internal/app/shared/options.go`, `docs/runbooks/event_fabric_authorization.md`)
- [X] T061 [US1] 校验 gRPC 安全拦截器：确保 WorkflowService 注册链路启用 auth/tenant/rbac 拦截器并补充回归测试钩子 (`internal/server/grpc/server.go`, `internal/app/shared/deps.go`)

**Checkpoint**: User Story 1 可独立运行与测试

---

## Phase 4: User Story 2 - 运行态控制与弹性 (Priority: P2)

**Goal**: 运维人员可监控实例、执行重试/补偿、处理 Agent 失败

**Independent Test**: 模拟步骤失败→自动重试→人工补偿全链路

### Tests for User Story 2

- [X] T029 [P] [US2] gRPC 合同测试：覆盖 `ControlInstance` 及扩展 `ListInstances` 场景 (`tests/contract/workflow/workflow_control_grpc_test.go`)
- [X] T030 [P] [US2] HTTP 合同测试：覆盖 `/instances/{id}/actions` 控制接口 (`tests/contract/workflow/workflow_control_http_test.go`)
- [X] T031 [P] [US2] 集成测试：步骤失败 + 自动重试 + 补偿验证 (`tests/integration/workflow/test_retry_compensation_flow.go`)
- [X] T055 [P] [US2] 单元测试：校验 Tool Grant 资格与超时重派逻辑 (`tests/unit/workflow_assignment_test.go`)
- [X] T058 [P] [US2] 多租户隔离集成测试：验证跨租户请求无法访问他人实例/控制接口 (`tests/integration/workflow/test_tenant_isolation.go`)

### Implementation for User Story 2

- [X] T032 [US2] 完成调度器重试与 SLA 解析 (`internal/service/workflow/scheduler.go`)：实现 Redis 延迟队列、心跳检测
- [X] T033 [US2] 实作补偿编排 (`internal/service/workflow/compensation.go`)：逆序执行、人工确认流程
- [X] T034 [US2] 扩展服务层控制接口 (`internal/service/workflow/service_control.go`)：暂停/恢复/取消/重新派单
- [X] T035 [US2] 实现 gRPC 控制 Handler (`internal/transport/grpc/workflow/control_handler.go`)
- [X] T036 [US2] 实现 HTTP 控制 Handler (`internal/transport/http/admin/workflow/instances_handler.go`)
- [X] T037 [US2] 实现 AgentAssignment 状态跟踪与超时处理 (`internal/service/workflow/assignment_tracker.go`)
- [X] T038 [US2] 增加 SLA breach 指标与 OTEL 观测 (`internal/service/workflow/metrics.go`)
- [X] T053 [US2] 在派发阶段校验 Tool Grant 资格并记录版本 (`internal/service/workflow/service_instance.go`, `internal/service/workflow/assignment_tracker.go`)
- [X] T059 [US2] 加固租户过滤：仓储查询统一强制 tenant_uuid (`pkg/corex/db/persistence/repository/workflow`, `internal/service/workflow/service_control.go`)

**Checkpoint**: User Stories 1 + 2 均可独立演示

---

## Phase 5: User Story 3 - 可观测性与审计导出 (Priority: P3)

**Goal**: 合规分析师可检索与导出 workflow 执行历史

**Independent Test**: 指定租户 + 时间范围导出 CSV，包含步骤/Agent/重试信息

### Tests for User Story 3

- [X] T039 [P] [US3] HTTP 合同测试：覆盖 `/instances/export` (`tests/contract/workflow/workflow_export_http_test.go`)
- [X] T040 [P] [US3] 集成测试：执行记录导出并核对字段 (`tests/integration/workflow/test_audit_export_flow.go`)

### Implementation for User Story 3

- [X] T041 [US3] 实现报告服务 (`internal/service/workflow/reporting.go`)：组合定义/实例/步骤数据 + Tool Grant 审计
- [X] T042 [US3] 实现 HTTP 导出 Handler (`internal/transport/http/admin/workflow/export_handler.go`) 支持 CSV/JSON
- [X] T043 [US3] 扩展 gRPC `ListInstances` 返回审计字段 (`internal/transport/grpc/workflow/reporting_handler.go`)
- [X] T044 [US3] 触发审计事件写入 (`internal/service/workflow/event_emitter.go`) 并确保 ClickHouse 投影数据完整

**Checkpoint**: 所有用户故事具备独立验证能力

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 全局优化、文档与交付准备

- [X] T045 [P] [Polish] 增补单元测试：验证 StepGraph 校验与 RetryPolicy (tests/unit/workflow_validator_test.go)
- [X] T046 [P] [Polish] 更新运行手册与 Quickstart (`docs/runbooks/event_fabric_authorization.md`, `specs/006-workflow-and-agent/quickstart.md`) 反映最终接口
- [X] T047 [Polish] 新增性能/健康仪表板与告警模板 (`deploy/observability/workflow_dashboard.json`, `docs/runbooks/observability_workflow.md`)
- [X] T048 [Polish] 执行 `make test-all` + `scripts/demo/event_fabric_quickstart.sh` 验证回归，并记录结果于 `specs/006-workflow-and-agent/tasks.md` Notes 区
- [X] T049 [P] [Polish] 代码巡检与 Dead-letter 清理：确保 Redis 队列有监控 (`internal/service/workflow/scheduler.go`, `docs/runbooks/redis_workflow.md`)
- [X] T050 [Polish] 完成发布说明与变更记录 (`docs/release_notes/workflow_orchestration.md`)

---

## Dependencies & Execution Order

- **Phase 1 → Phase 2** → 用户故事 → Phase 6（遵循计划顺序）
- 若团队规模允许，Phase 3~5 可在完成 Phase 2 后并行推进，但需确保共享文件冲突得到协调

### Parallel Opportunities
- 标记为 **[P]** 的任务可并行，例如模型创建、合同测试、文档更新等
- 同一文件的任务保持顺序执行，避免相互覆盖

### Within Each User Story
- 先实现测试（合同 / 集成），确认失败后再实现功能代码
- 模型/仓储 → 服务逻辑 → 传输层 → 事件/指标
- 每个故事完成后执行 Checkpoint，自给自足地演示功能

---

## Parallel Example: User Story 1

```bash
# 并行编写合同测试
Task: "T020 [P] [US1] gRPC 合同测试：覆盖 CreateDefinition/PublishDefinition/ListDefinitions (tests/contract/workflow/workflow_definitions_grpc_test.go)"
Task: "T021 [P] [US1] HTTP 合同测试：覆盖 POST/GET /definitions 与 POST /instances (tests/contract/workflow/workflow_definitions_http_test.go)"

# 并行构建模型
Task: "T005 [P] [Found] 实现 WorkflowDefinition 模型 (pkg/corex/db/persistence/model/workflow/definition.go)"
Task: "T006 [P] [Found] 实现 WorkflowInstance 模型 (pkg/corex/db/persistence/model/workflow/instance.go)"
```

---

## Implementation Strategy

1. **完成 Setup + Foundational**，确保合同、模型、仓储与调度骨架就绪
2. **MVP：交付 User Story 1**，验证设计者可创建并启动工作流
3. **扩展**：实现 User Story 2 以增强运行态可靠性
4. **合规**：实现 User Story 3 提供导出与审计能力
5. **Polish**：补齐测试、文档、观测；按需迭代

每个阶段结束前执行 Checkpoint，保证可独立交付与回归测试。

---

## Phase 7: Runtime Completion for Native Agent (Required)

**Purpose**: 现有 `[X]` 任务代表 006 早期骨架完成，不代表 native-agent 所需 Workflow Runtime 完成。本阶段是启用专家知识库迭代、营销智能体 Workflow Pack、Human Review 和真实 Workflow Builder 的硬前置。

### Backend Runtime

- [X] T062 [P0] 在 `specs/006-workflow-and-agent/spec.md`、`data-model.md`、`contracts/http-openapi.yaml`、`contracts/workflow.proto` 对齐 `node_kind`、Node Catalog、Human Review、Workflow Pack seed、UUID 参数命名。
- [X] T063 [P0] 为 `pkg/corex/db/persistence/model/workflow/*` 增加 `node_kind`、`node_ref`、`input_mapping`、`output_mapping`、结构化错误字段。
- [X] T064 [P0] 新增 `HumanReviewTask` 和 `WorkflowPackSeedRecord` 模型、迁移和 repository。
- [X] T065 [P0] 在 `pkg/corex/db/persistence/repository/workflow/step_record_repository.go` 增加 queued step lease 和 attempt-safe update。
- [X] T066 [P0] 实现 `backend/internal/service/workflow/node_adapter.go`，定义 NodeAdapter、NodeResult、NodeAdapterRegistry。
- [X] T067 [P0] 实现 `backend/internal/service/workflow/runner.go`，支持 step lease、adapter 执行、next step 计算、waiting/succeeded/failed/compensating 状态收敛。
- [X] T068 [P0] 实现 adapters：`adapter_skill.go`、`adapter_capability.go`、`adapter_metadata.go`、`adapter_knowledge.go`、`adapter_event.go`。
- [X] T069 [P0] 实现 `backend/internal/service/workflow/human_review.go`，支持 approve/reject/request_changes/cancel 和 Runner wake-up。
- [X] T070 [P0] 实现 `backend/internal/service/workflow/node_catalog.go`，从 adapter registry、Skill Registry、Capability Registry、Knowledge、Metadata 汇总节点目录。
- [X] T071 [P0] 实现 `backend/internal/service/workflow/workflow_pack_seed.go` 和 `backend/config/workflow_packs`，包含 `expert_knowledge_capture`、`marketing_knowledge_capture`、`campaign_review_to_methodology`。

### HTTP / gRPC / Capability

- [X] T072 [P0] 更新 `backend/internal/transport/http/admin/workflow/routes.go`，路径参数统一为 `definition_uuid`、`instance_uuid`，新增 node-catalog、review-tasks、packs 路由。
- [X] T073 [P0] 更新 `backend/internal/transport/grpc/workflow/server.go` 和 proto 生成代码，补齐 Node Catalog、Human Review、Workflow Pack RPC。
- [X] T074 [P0] 更新 `backend/config/platform_capabilities/workflow.yaml`，覆盖新增 Admin API 和 gRPC capability，并运行 `make capability-check`。
- [X] T075 [P0] 把 workflow seed 纳入 `make seed` 的顺序执行链路；不得作为隐藏手工步骤。

### Web Admin

- [X] T076 [P0] 更新 `web-admin/app/composables/api/services/workflowService.ts`，baseUrl 改为 `/admin/workflows`，移除 `getKinds()`、`getPalette()` mock。
- [X] T077 [P0] 更新 `web-admin/app/pages/workflow/index.vue`，移除内置 workflow list，接真实 definitions API、分页、搜索和状态筛选。
- [X] T078 [P0] 更新 `web-admin/app/components/workflow/WorkflowEditor.vue`，左侧节点目录来自 Node Catalog API，属性面板按 schema 渲染。
- [X] T079 [P0] 新增实例监控与审核页面：`web-admin/app/pages/workflow/instances/*`、`web-admin/app/pages/workflow/review-tasks/*`。
- [X] T080 [P0] 补齐 workflow 前端 i18n；节点名称、按钮、错误、状态不得硬编码用户可见文案。

### Tests

- [X] T081 [P0] 单测：NodeAdapterRegistry、Runner route、HumanReview、Knowledge publish review gate。
- [X] T082 [P0] 集成测试：`expert_knowledge_capture` 完整链路。
- [X] T083 [P0] 集成测试：`marketing_knowledge_capture` 完整链路。
- [X] T084 [P0] HTTP 合同测试：definitions/instances/node-catalog/review-tasks/packs。
- [X] T085 [P0] gRPC 合同测试：WorkflowService 与 HTTP 能力对齐。
- [X] T086 [P0] 前端测试：Workflow Builder 无 mock 数据，节点目录来自 API。

**Completion Definition**:

1. `marketing_knowledge_capture` 可以真实启动并推进到 Human Review。
2. Human Review approve 后可以发布到 Knowledge Space。
3. reject 后不会发布正式知识。
4. 所有节点都有 StepRecord、WorkflowEvent 和 trace_id。
5. Web Admin 不再使用 mock workflow 数据。
6. native-agent 启用时能校验 Workflow Pack 是否可执行。

- 2025-10-21 13:20 UTC：`make test-all` 执行完成，全部 API 套件通过；Redis/Grafana 监控方案通过新增运行手册与仪表板模板覆盖；单元回归 `GOCACHE=$(pwd)/.gocache go test ./tests/unit/...`、导出合同 `./tests/contract/workflow -run Export` 已验证。
- 2025-10-21 13:35 UTC：尝试 `scripts/demo/event_fabric_quickstart.sh`，后端返回 HTTP 404（能力创建失败）；需待管理端服务上线后重试确认。
- 2025-10-21 18:15 UTC：完成 `scripts/demo/event_fabric_quickstart.sh`（使用 system/root 凭证 + 实际租户/成员 UUID），能力与 Grant 创建成功，JSON/CSV 审计导出正常，日志见 `reports/event_fabric_authorization_quickstart.log`。
