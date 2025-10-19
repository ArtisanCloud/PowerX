# Feature Specification: EventBus & Message Fabric

**Domain Ownership**: CoreX (`corex.event_bus`)

**Feature Branch**: `004-eventbus-message-fabric`  
**Created**: 2025-10-17  
**Status**: Draft  
**Input**: User description: "Title: EventBus & Message Fabric. WHAT/WHY: Provide a shared event backbone with topics, ACL, retry/DLQ semantics for all integration features. Scope: Topic taxonomy, ACL model, publish/subscribe contracts, retry/backoff, DLQ. Out-of-Scope: Orchestration business flows; Registry logic; Gateway admin UI. Dependencies: Security policy; Contracts for message schemas."

## 背景与动机

- 目前平台内存在多条独立事件通道，缺乏统一的主题命名体系与权限边界，导致跨域集成重复造轮子且运维不可控。
- 新上线的能力注册、工作流、插件生态均依赖事件驱动能力，需要一致的发布/订阅协议、重试语义与死信处理，才能满足多租户合规及审计要求。
- 安全集中审计（Security Policy）与契约管理（Message Contracts）已经在其他模块落地，本次规格需与其对齐，形成共享事件骨干。

## 范围声明

### In Scope
- 统一 Topic 分类与命名规则（含租户、域、事件类型等维度），定义事件目录与生命周期管理。
- ACL/权限模型：定义发布者与订阅者授权矩阵、租户隔离策略、Observer/Replay 等能力。
- 发布/订阅合同：标准化事件信封（Envelope）、元数据字段、幂等键、版本策略及 SDK 契约。
- 投递可靠性：重试策略、指数退避/backoff、幂等性窗口、确认机制。
- 死信队列（DLQ）及补偿：进入条件、存储结构、重放流程、告警与可视化能力。

### Out of Scope
- 业务编排流程（Workflow/Orchestration）具体执行逻辑。
- 能力注册 Registry 的业务逻辑与数据模型。
- Gateway/Admin UI 层面的前端界面与交互。

### 依赖
- 安全策略与角色矩阵（Security Policy）需作为 ACL 引用源。
- 消息契约规范（Message Schema Contracts）需提供事件载荷结构的版本对齐。
- 现有基础设施（Redis、Postgres、对象存储）可复用，但规格需留出 Kafka/Loki 等扩展接口。

## Clarifications

### Session 2025-10-17
- Q: 事件总线默认需要提供哪种投递语义保证？ → A: 至少一次（At-Least-Once）投递保证
- Q: 订阅者默认应通过哪种方式接收事件？ → A: 长连接推送（默认 gRPC 流式推送）
- Q: 事件在进入 DLQ 之前，默认允许的最大重试次数是多少？ → A: 默认最多重试 5 次
- Q: 默认的死信队列（DLQ）应落在哪种存储介质？ → A: Postgres 持久化存储
- Q: 事件载荷默认采用哪种序列化格式？ → A: JSON 序列化

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 统一主题治理与发现（Priority: P1）

作为平台集成管理员，我希望通过统一的事件目录管理主题分类、租户范围和生命周期，从而防止重复定义主题、保障跨团队协作一致，并能快速查询某能力的事件发布点。

**Why this priority**: 若没有统一 Topic 治理，后续 ACL、重试策略无法准确落地，属于基础阻断。

**Independent Test**: 在测试环境创建若干主题分类、注册租户级主题并查询目录，验证命名校验、生命周期状态流转和审计日志完整。

**Acceptance Scenarios**:

1. **Given** 管理员提交新的主题定义（含租户限定、事件类型、保留策略），**When** 系统进行命名与冲突校验后接受，**Then** 目录中可查询该主题及其元数据，并记录创建者与变更原因。
2. **Given** 主题计划下线或迁移，**When** 管理员更新主题生命周期状态为 `deprecated` 或 `retired`，**Then** 所有绑定的发布者与订阅者均收到通知，并在到期后禁止新事件发布。

---

### User Story 2 - 精细化事件访问控制（Priority: P1）

作为安全合规负责人，我需要基于租户、域与角色定义哪些服务可以发布或订阅特定主题，并对访问样本进行审计，确保事件流不发生越权。

**Why this priority**: 多租户与合规要求需要强制的 ACL 控制，否则事件骨干无法对外开放。

**Independent Test**: 配置两个租户、三个服务账号，授予/撤销发布或订阅权限，验证未经授权的调用会被拒绝且产生审计记录。

**Acceptance Scenarios**:

1. **Given** 某服务账户被授权发布 `tenantA.corex.workflow.approved` 主题，**When** 该账户调用 Publish API，**Then** 系统接受事件、生成审计记录并将事件写入投递流水。
2. **Given** 未被授权的订阅者尝试订阅 `tenantA.corex.workflow.approved`，**When** 订阅请求发送，**Then** 系统拒绝并返回明确错误码，同时在安全日志中保留违规记录。

---

### User Story 3 - 高可靠投递与死信补偿（Priority: P1）

作为平台事件消费方，我希望系统提供可配置的重试策略、幂等性保障以及死信队列以便在消费失败时自动补偿，确保事件不会因为短暂故障丢失。

**Why this priority**: 没有可靠投递协议就无法满足核心业务（如能力注册事件、账务事件）的一致性要求。

**Independent Test**: 模拟消费失败与超时场景，验证系统执行指数退避重试；超出重试次数后进入 DLQ，并通过补偿 API 成功重放。

**Acceptance Scenarios**:

1. **Given** 订阅者在处理事件时返回可重试错误，**When** 达到配置的最大尝试次数前，**Then** 系统按指数退避间隔重新投递，并在幂等窗口内确保不会重复交付给其他消费者。
2. **Given** 事件超过最大重试次数或被标记为不可恢复错误，**When** 进入 DLQ，**Then** 管理员可通过 DLQ API 检索、批量重放或转储事件，并触发告警。

---

### User Story 4 - 事件契约与回放能力（Priority: P2）

作为集成开发者，我希望事件在不同版本之间保持兼容，并能通过回放功能重建状态或调试问题，减少 Schema 演进和事故恢复成本。

**Why this priority**: 契约治理与回放能力是持续演进事件生态的保障，但优先级稍次于核心安全与可靠性。

**Independent Test**: 创建同一主题的两个架构版本，验证发布方携带版本字段；订阅方声明兼容策略并成功解析；同时在时间窗口内执行回放，确保排序与幂等性。

**Acceptance Scenarios**:

1. **Given** 发布者升级事件 Schema，**When** 发布 `v2` 版本事件，**Then** 订阅者根据自身兼容策略（向后兼容或拒绝）进行处理，并记录版本指标。
2. **Given** 管理员在审计窗口内发起回放请求，**When** 系统根据时间范围与过滤条件回放事件，**Then** 所有回放事件标记 `replay` 标志且不会触发原业务副作用（支持影子渠道）。

### Edge Cases

- 主题命名冲突或保留字（如 `system.*`）需要阻止创建并提示合法前缀。
- 跨租户订阅：若主题声明为 `global`，仍需验证订阅者具备全局权限，否则拒绝。
- 长时间未确认（Ack）事件必须进入超时处理队列并计入重试次数。
- 重放与实时消费并发执行时，需确保订阅者可以区分回放来源并避免重复记账。
- DLQ 持续堆积超过阈值时应触发阻断策略（例如临时暂停该订阅者或切换到只读模式）。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 系统必须提供 Topic 目录 API（创建、查询、更新、状态管理）并强制执行命名规范与唯一性。
- **FR-002**: 系统必须支持基于租户、域、角色的发布/订阅 ACL 校验，支持批量授权与撤销。
- **FR-003**: 发布 API 必须要求事件信封包含 `event_id`、`topic`、`tenant_id`、`version`、`trace_id` 等标准字段，并校验消息契约版本；默认载荷使用 JSON 序列化（Topic 可声明覆盖）。
- **FR-004**: 订阅通道必须提供显式 Ack/Nack 机制，默认采用长连接（如 gRPC 流）推送事件，并允许消费者声明最大并发、超时时间与幂等键策略。
- **FR-005**: 系统必须支持可配置的重试策略（固定/指数退避、最大次数、抖动），默认提供至少一次（At-Least-Once）投递保证且最多重试 5 次，并在每次重试时记录投递状态。
- **FR-006**: 当事件超出重试次数或被标记不可重试时，系统必须将其写入租户隔离的 DLQ（默认落地 Postgres 持久化存储），并提供检索、导出、重放 API。
- **FR-007**: 系统必须向现有审计服务输出发布/订阅活动（含调用者、主题、结果、延迟），支持按租户和主题过滤。
- **FR-008**: 系统必须支持事件版本协商机制，订阅者可以声明兼容策略（仅接受某版本、向后兼容、全部接受）。
- **FR-009**: 系统必须提供事件回放能力，支持按时间窗口、事件类型或追踪 ID 过滤，并标记回放投递。
- **FR-010**: 系统必须暴露指标（投递成功率、重试次数、DLQ 体积、回放耗时）并与现有 Observability 管线对接。

### Non-Functional Requirements

- **NFR-001**: 核心消息路径的 P95 延迟 ≤ 100ms（不含订阅者处理时间），P99 重试延迟 ≤ 1s。
- **NFR-002**: 支持至少 5000 条/秒的并发事件发布，重试与回放不会影响实时流量 SLA。
- **NFR-003**: ACL 判定与审计写入必须是多租户隔离的；任何跨租户数据访问都应被拒绝并记录。
- **NFR-004**: 提供 HA 部署指南，支持水平扩展，重要状态（如偏移、重试计数、DLQ）持久化于可复制存储。
- **NFR-005**: 提供安全加固措施（静态密钥不落地、TLS、签名校验）并覆盖安全策略自检。

### Key Entities

- **TopicDefinition**: 描述主题的唯一标识、租户范围、命名空间、数据保留策略与生命周期状态。
- **AclBinding**: 关联主体（服务账号/角色）与权限（Publish/Subscribe/Replay），记录审批来源与过期时间。
- **EventEnvelope**: 统一事件信封，包含事件 ID、主题、版本、租户、追踪信息、幂等键、发布时间以及 Payload 摘要，并记录 `payload_format`（默认 JSON，可覆盖）。
- **DeliveryAttempt**: 投递尝试记录，追踪订阅者、重试次数、Ack/Nack 状态、耗时与错误。
- **DlqMessage**: 死信消息实体，持久化于 Postgres，记录失败原因、最后错误码、重放状态、审核日志。
- **ReplayRequest**: 回放任务，定义时间窗口、过滤条件、发起人以及执行进度。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% 新增主题必须通过命名校验，目录 API 支持在 500ms 内返回 100 条主题记录。
- **SC-002**: 未授权发布/订阅请求拦截率 100%，所有拒绝请求均在 1 分钟内进入审计日志。
- **SC-003**: 事件投递成功率 ≥ 99.9%，DLQ 转化率（DLQ → 成功重放）≥ 95%。
- **SC-004**: 重试机制在预设负载（5000 msg/s）下平均附加延迟 ≤ 50ms，P99 ≤ 200ms。
- **SC-005**: 回放功能支持 24 小时窗口内按 Trace ID 精确回放，回放事件与实时事件绝不混淆（重放消费方可检测 `replay` 标记）。
- **SC-006**: 指标与告警在 Prometheus/Grafana 中可视化，包含主题级 QPS、错误率、DLQ 积压趋势，并在 5 分钟内可用于故障分析。

## 风险与缓解

- **多租户隔离复杂度**：若 Topic 目录设计不合理可能造成越权 → 采纳租户分区 + ACL 多维校验，并在测试中覆盖跨租户用例。
- **基础设施差异**：Redis/Kafka 等实现特性差异大 → 定义抽象接口与兼容层，首期实现 Local/Redis，留出 Kafka 扩展点。
- **Schema 演进冲突**：缺乏契约管理将导致消费失败 → 与 Contracts 服务集成，发布前进行 Schema 校验。
- **DLQ 堆积**：若补偿流程不畅会导致死信膨胀 → 设定阈值告警与自动化补偿脚本，并在规格中明确运维 SOP。

## 验收前置清单

- [ ] Topic 目录 API、ACL 校验、投递/重试、DLQ、回放的合同文档与示例已准备就绪。
- [ ] 与安全策略、契约服务的集成测试通过（含负面用例）。
- [ ] Observability 指标与告警规则在测试环境验证。
- [ ] 运维手册涵盖主题生命周期、权限审批、DLQ 补偿、回放 SOP。
