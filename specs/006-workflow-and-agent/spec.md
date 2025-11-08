# Feature Specification: Workflow & Agent Orchestration

**Feature Branch**: `006-title-workflow-agent`  
**Created**: 2025-10-20  
**Status**: Draft  
**Input**: User description: "Title: Workflow & Agent Orchestration\nWHAT/WHY: Execute multi-step workflows and multi-agent collaboration using registered capabilities.\nScope: State model, step types, compensation/retry strategy, agent participation rules.\nOut-of-Scope: Defining capability contracts, registry/router, event backbone, auth policy.\nDependencies: Contracts & Transport; Registry & Router; EventBus; Tool Grants."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Design & launch orchestrations (Priority: P1)

Operations designers define a workflow that stitches together multiple registered capabilities and launch an execution that coordinates the right Agents without custom coding.

**Why this priority**: Enables the core promise of orchestration—turning existing capabilities into end-to-end automation for high-value use cases.

**Independent Test**: Create a workflow with at least three steps (including an Agent action) and execute it end-to-end using only the orchestration tools.

**Acceptance Scenarios**:

1. **Given** a designer has permissions, **When** they create and publish a workflow definition with ordered steps, **Then** the system stores the definition with versioning and validates step dependencies.
2. **Given** a published workflow, **When** the designer starts a new instance with input parameters, **Then** the system transitions the workflow to `Running` state and schedules the first step against an eligible Agent.

---

### User Story 2 - Runtime control & resilience (Priority: P2)

Runtime operators monitor active workflows, manage retries/compensation, and intervene when Agents fail or require human input.

**Why this priority**: Ensures reliability and compliance by giving operators the ability to contain failures and keep service level agreements.

**Independent Test**: Force a step failure, trigger automated retry, perform manual compensation, and observe accurate state transitions and audit logs.

**Acceptance Scenarios**:

1. **Given** a step fails with a retriable error, **When** retry policy criteria are met, **Then** the system schedules a retry with backoff and records attempts.
2. **Given** a workflow enters compensation mode, **When** operators approve manual rollback, **Then** compensation steps execute in reverse order with audit entries for each reversal.

---

### User Story 3 - Insight & auditability (Priority: P3)

Compliance analysts export workflow execution history, identify bottlenecks, and verify who performed each action across Agents.

**Why this priority**: Supports governance and continuous improvement by exposing traceable evidence of orchestrated activity.

**Independent Test**: Run queries and exports that include workflow timelines, Agent participation, and decision outcomes for a single tenant over a defined window.

**Acceptance Scenarios**:

1. **Given** analysts filter by tenant, time window, and workflow, **When** they export execution logs, **Then** the system returns a structured dataset with timestamps, step outcomes, assigned Agents, retry counts, and manual interventions.

---

### Edge Cases

- Agents assigned to a step lose connectivity after accepting a task—workflow should detect missed heartbeats and reassign or escalate per policy.
- Parallel blocks where one branch succeeds and another hangs indefinitely—system must honor branch completion rules and enforce timeouts.
- Compensation step throws an unrecoverable error—workflow should pause in `CompensationFailed`, notify operators, and prevent further automated progression.
- Cyclic dependency detected in designer input—definition validation must reject cycles before publication.
- Workflow inputs missing required contextual data (e.g., tenant binding)—instance creation should fail fast with actionable feedback.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST allow authorized users to create, version, and publish workflow definitions containing ordered and/or parallel steps, with validation that prevents cyclic dependencies and undefined references.
- **FR-001a**: 工作流定义发布后不可直接编辑；若需调整，系统必须创建新的定义版本并保留旧版本供现有实例继续使用。
- **FR-002**: System MUST support the following step types with declarative configuration: Agent action, system task, decision/gateway, parallel block, human approval, and compensation step.
- **FR-003**: Each workflow instance MUST track lifecycle states (`Draft`, `Running`, `Waiting`, `Suspended`, `Succeeded`, `Failed`, `Compensating`, `Compensated`, `Canceled`) with atomic transitions and persisted timestamps.
- **FR-004**: System MUST enforce step-level retry policies (max attempts, backoff strategy) and log each attempt, including success, failure reason, and next action.
- **FR-005**: For Agent steps, system MUST verify Tool Grant eligibility at dispatch time and record the Grant version used.
- **FR-006**: System MUST support automatic compensation in LIFO order for steps marked compensatable, including the ability to request manual approval before executing each compensation step.
- **FR-007**: Operators MUST be able to manually pause, resume, cancel, retry, or reassign workflow instances or individual steps, with all actions captured in audit records.
- **FR-008**: System MUST emit structured events to EventBus for workflow and step state changes, enabling downstream monitoring and alerting.
- **FR-009**: Users MUST be able to query and export workflow execution history by tenant, workflow definition, time range, status, and Agent, receiving results in JSON and CSV formats.
- **FR-010**: The orchestration runtime MUST enforce SLA monitors (e.g., maximum step wait time) and trigger alerts when thresholds are breached.

### Key Entities *(include if feature involves data)*

- **WorkflowDefinition**: Immutable versioned blueprint containing metadata (name, tenant scope), step graph, retry/compensation policies, and published status.
- **WorkflowInstance**: Runtime record holding definition reference, state, input context, schedule timestamps, SLA metrics, and current step pointer.
- **WorkflowStepRecord**: Per-instance step execution log with step id, assigned Agent, state, retries, output payload, failure reason, and compensation status.
- **AgentAssignment**: Links Agent identity and Grant version to a specific step execution, storing dispatch time, acknowledgment, and completion timestamps.
- **WorkflowEvent**: Auditable message capturing state changes, manual actions, and summary metrics for analytics exports.

### 非功能需求（Non-Functional Requirements）

- **NFR-001 可观测性**：所有 Workflow 服务与调度器必须通过现有 OTEL 管道输出核心指标（等待时间、重试次数、SLA 违约），并按 `.specify/memory/constitution.md` 要求在 `internal/service/workflow/metrics.go` 中统一注册。
- **NFR-002 安全**：所有 HTTP/gRPC 接口必须复用 CoreX 既有 JWT/JWKS 验证与 RBAC 权限模型，Agent 调度必须记录 Tool Grant 版本并拒绝过期授权。
- **NFR-003 性能**：Workflow API 与调度操作在常规负载下需保持 API p95 < 200ms，触发 SLA 警报时须在 5 分钟内完成自动或人工恢复。
- **NFR-004 多租户隔离**：所有存储查询必须以 `tenant_id` 过滤；导出与监控数据亦需保证租户隔离与审计轨迹完整。
- **NFR-005 弹性与容错**：调度队列需要支持至少 100 并发实例的重试/补偿场景，Redis 不可用时需退化为缓冲告警并阻止新增实例，防止数据丢失。

## Assumptions

- Tool Grants, capability registry, and EventBus provide reliable responses; orchestration focuses on coordination logic.
- Human approvals integrate with existing enterprise review channels; operators have SLA to respond within business hours.
- Default retry policy is exponential backoff (initial 30s, factor 2, max 5 attempts) unless overridden per step.
- SLA breaches trigger alerts routed through existing observability stack.
- Workflow definitions are tenant-isolated; each tenant只能访问和管理自身定义。
- 已发布定义不可直接修改；任何更新需创建新版本，仅影响新实例，运行中的实例继续使用发布时的版本。

## Dependencies

- Contracts & Transport for exposing orchestration APIs and schemas.
- Registry & Router for discovering eligible Agents per capability.
- EventBus for publishing workflow lifecycle events.
- Tool Grants for permission checks and audit integration.

## Clarifications

### Session 2025-10-20

- Q: 工作流定义的租户边界应该如何管理？ → A: 每个租户拥有独立的工作流定义库
- Q: 已发布的工作流定义是否允许被编辑，并且编辑后的内容是否影响正在运行的实例？ → A: 已发布定义不可直接编辑，修改需创建新版本，仅影响新实例

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 80% of new workflows can be created and published by designers without engineering support in under 30 minutes.
- **SC-002**: 95% of workflow instances complete within their configured SLA windows under nominal load (≤ 100 concurrent instances per tenant).
- **SC-003**: Mean time to recover from an Agent failure (automatic retries or compensation) is ≤ 5 minutes for 90% of occurrences.
- **SC-004**: Compliance analysts can export workflow execution evidence for the prior 90 days in ≤ 2 minutes, with datasets covering 100% of required audit fields.
