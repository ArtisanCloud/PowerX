# Feature Specification: Workflow & Agent Orchestration

**Feature Branch**: `006-title-workflow-agent`  
**Created**: 2025-10-20  
**Status**: Draft + Runtime Completion Amendment  
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

### User Story 4 - Semantic node runtime for native agents (Priority: P0)

Native Agent、知识库策展和插件复合能力需要通过 Workflow Runtime 执行真实业务节点，而不是只创建实例和记录初始步骤。

**Why this priority**: native-agent 的知识库增量迭代、Human Review、Knowledge publish、Skill/Capability 调用都依赖完整 Runner。没有该能力，Workflow 只能算骨架，不能支撑 PowerX 原生智能体。

**Independent Test**: 发布 `marketing_knowledge_capture` Workflow Pack，启动实例，执行 `input.capture -> skill.invoke -> metadata.classify -> knowledge.stage -> human.review -> knowledge.publish -> event.emit`，审核通过后写入 Knowledge Space，审核拒绝时不发布正式知识。

**Acceptance Scenarios**:

1. **Given** a published workflow definition with registered semantic node kinds, **When** an instance starts, **Then** WorkflowRunner leases queued steps, invokes the matching NodeAdapter, writes StepRecord/WorkflowEvent, and advances the DAG until waiting, succeeded, failed, or compensating.
2. **Given** a workflow contains `human.review`, **When** the node executes, **Then** the system creates a first-class review task, moves the instance to `waiting`, and resumes only after an authorized reviewer acts.
3. **Given** a workflow contains `knowledge.publish`, **When** the previous review was not approved, **Then** publication is blocked with a structured error and no formal knowledge version is created.
4. **Given** a Workflow Builder loads nodes, **When** the page renders the palette, **Then** nodes come from Node Catalog API and not from frontend mock data.

---

### Edge Cases

- Agents assigned to a step lose connectivity after accepting a task—workflow should detect missed heartbeats and reassign or escalate per policy.
- Parallel blocks where one branch succeeds and another hangs indefinitely—system must honor branch completion rules and enforce timeouts.
- Compensation step throws an unrecoverable error—workflow should pause in `CompensationFailed`, notify operators, and prevent further automated progression.
- Cyclic dependency detected in designer input—definition validation must reject cycles before publication.
- Workflow inputs missing required contextual data (e.g., tenant binding)—instance creation should fail fast with actionable feedback.
- NodeAdapter missing for a `node_kind`—definition publication must fail with `workflow.node_adapter_unavailable`.
- Skill, Capability, Metadata namespace, or Knowledge Space dependency missing—definition publication must fail with structured dependency details.
- Human Review approved but Runner wake-up fails—review task must not be reported as knowledge published.

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
- **FR-011**: System MUST support semantic `node_kind` on each step, mapped to a lower-level step type. Initial node kinds MUST include `input.capture`, `skill.invoke`, `capability.invoke`, `metadata.classify`, `knowledge.stage`, `decision.gateway`, `parallel.fanout`, `parallel.join`, `human.review`, `knowledge.publish`, `event.emit`, and `compensation.rollback`.
- **FR-012**: System MUST provide a WorkflowRunner that leases queued steps, builds input context, invokes NodeAdapter, persists outputs, calculates next steps, and transitions instances atomically.
- **FR-013**: System MUST provide a NodeAdapterRegistry. Workflow publication MUST fail if any `node_kind` is unknown, unavailable, or has invalid config.
- **FR-014**: System MUST expose Node Catalog API for Workflow Builder. Frontend MUST NOT hardcode or mock executable node kinds.
- **FR-015**: System MUST model Human Review as a first-class runtime task with reviewer policy, approve/reject/request_changes actions, audit records, and Runner wake-up.
- **FR-016**: System MUST support Workflow Pack Catalog validation and tenant-explicit installation for native-agent flows, including `expert_knowledge_capture`, `marketing_knowledge_capture`, and `campaign_review_to_methodology`; regular database seed MUST NOT materialize built-in packs for every tenant.
- **FR-017**: Knowledge publishing MUST go through `knowledge.stage -> human.review -> knowledge.publish`; direct formal Knowledge Space writes from unreviewed workflow output are forbidden.
- **FR-018**: Workflow Admin APIs, gRPC contracts, events, audit records, and frontend routes MUST use UUID-based business references. Numeric IDs are internal only and MUST NOT be API identifiers.
- **FR-019**: Workflow Runtime MUST validate Skill, Capability, Knowledge Space, Metadata namespace, and RBAC dependencies before definition publication and before step execution.

### Key Entities *(include if feature involves data)*

- **WorkflowDefinition**: Immutable versioned blueprint containing metadata (name, tenant scope), step graph, retry/compensation policies, and published status.
- **WorkflowInstance**: Runtime record holding definition reference, state, input context, schedule timestamps, SLA metrics, and current step pointer.
- **WorkflowStepRecord**: Per-instance step execution log with step id, assigned Agent, state, retries, output payload, failure reason, and compensation status.
- **AgentAssignment**: Links Agent identity and Grant version to a specific step execution, storing dispatch time, acknowledgment, and completion timestamps.
- **WorkflowEvent**: Auditable message capturing state changes, manual actions, and summary metrics for analytics exports.
- **WorkflowNodeCatalogItem**: Runtime and Builder visible declaration for a semantic node kind, including input/output schema, required permissions, required capabilities, idempotency, and compensation support.
- **HumanReviewTask**: First-class approval task linked to a workflow instance and step, storing approver policy, review payload, decision, reviewer UUID, and completion timestamp.
- **WorkflowPackSeedRecord**: Version/checksum record for seeded Workflow Pack definitions used by native-agent and plugin-provided agent flows.

### 非功能需求（Non-Functional Requirements）

- **NFR-001 可观测性**：所有 Workflow 服务与调度器必须通过现有 OTEL 管道输出核心指标（等待时间、重试次数、SLA 违约），并按 `.specify/memory/constitution.md` 要求在 `internal/service/workflow/metrics.go` 中统一注册。
- **NFR-002 安全**：所有 HTTP/gRPC 接口必须复用 CoreX 既有 JWT/JWKS 验证与 RBAC 权限模型，Agent 调度必须记录 Tool Grant 版本并拒绝过期授权。
- **NFR-003 性能**：Workflow API 与调度操作在常规负载下需保持 API p95 < 200ms，触发 SLA 警报时须在 5 分钟内完成自动或人工恢复。
- **NFR-004 多租户隔离**：所有存储查询必须以 `tenant_uuid` 过滤；导出与监控数据亦需保证租户隔离与审计轨迹完整。
- **NFR-005 弹性与容错**：调度队列需要支持至少 100 并发实例的重试/补偿场景，Redis 不可用时需退化为缓冲告警并阻止新增实例，防止数据丢失。
- **NFR-006 前端真实性**：Web Admin Workflow Builder 和实例监控页面必须调用真实 Admin API；不得使用 mock workflow list、mock node palette 或硬编码节点目录作为产品能力。

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
- Skill Registry for `skill.invoke`.
- Capability Registry and Tenant Invocation for `capability.invoke`.
- Metadata Governance for `metadata.classify`.
- Knowledge Space services for `knowledge.stage` and `knowledge.publish`.
- Native Agent planning docs: `docs/plan/ai_engineering/native-agent/`.
- Workflow Runtime planning docs: `docs/plan/ai_engineering/workflow/`.

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
