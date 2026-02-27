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
- 对外接口规范：HTTP（优先）、gRPC、SDK 草案与鉴权/限流/错误码统一。



### 增量范围（Phase 10）
- Event Fabric Admin 运维可视化页面（`/settings/event-fabric`）属于本特性增量范围。
- UI 仅作为统一 Event Fabric/TaskDriver 机制的观察与处置入口，不引入新的投递或消费机制。
- 页面必须复用现有 Admin API，确保契约口径与 `specs/004-eventbus-message-fabric/contracts/*` 一致。

### 增量范围（Phase 11）
- 新增 Cron 任务能力：仅负责按计划投递 Task Event，不直接执行业务。
- Cron 执行结果必须复用统一 Retry/DLQ/Replay 通道，不允许私有轮询消费器。
- Cron 运维与审计复用 Event Fabric Admin API 与运维页面。

### 增量范围（Phase 13）
- 新增「系统设置 / 事件权限（Event ACL）」治理页，维护 Topic 与角色（或主体）的授权关系。
- Topic 治理采用“全局注册 + JWT 租户鉴别 + ACL 授权”，不再要求每租户复制同名 Topic。
- 监控中心（`/settings/monitor?tab=event-fabric`）保留运行态观测与调试，不承担 ACL 配置主入口。

### Out of Scope
- 业务编排流程（Workflow/Orchestration）具体执行逻辑。
- 能力注册 Registry 的业务逻辑与数据模型。
- 非 Event Fabric 运维台范围内的通用 Gateway/Admin UI 业务界面与交互。

### 依赖
- 安全策略与角色矩阵（Security Policy）需作为 ACL 引用源。
- 消息契约规范（Message Schema Contracts）需提供事件载荷结构的版本对齐。
- 现有基础设施（Redis、Postgres、对象存储）可复用，但规格需留出 Kafka/Loki 等扩展接口。

## 规范治理策略（单一权威 + 防漂移）

### 单一权威文档

- 本文档（`specs/004-eventbus-message-fabric/spec.md`）是 EventBus/TaskBus 的**规范权威源**。
- `specs/023-websocket-notify/*` 仅定义 WS 传输侧实现与运行时约束，不再重复定义 Topic 命名、事件版本兼容、ACL 语义。
- 若 `004` 与其他文档存在冲突，以 `004` 为准，其他文档必须在同一 PR 中同步修订。

### 三类契约冻结（Contract Freeze）

1. **连接契约（Connection Contract）**
   - 范围：WS 入口、心跳、订阅/退订、ack/error/event envelope。
   - 落点：`specs/023-websocket-notify/contracts/http-openapi.yaml`。
2. **发布契约（Publishing Contract）**
   - 范围：register/publish 接口、鉴权、tenant/trace 透传、内部接口边界。
   - 落点：`specs/023-websocket-notify/contracts/http-openapi.yaml` 与 `specs/004-eventbus-message-fabric/contracts/admin_http_openapi.yaml`。
3. **事件契约（Event Contract）**
   - 范围：topic 命名规范、版本策略（v1/v2）、兼容规则、回放标记、DLQ 语义。
   - 权威：`specs/004-eventbus-message-fabric/spec.md` 与 `specs/004-eventbus-message-fabric/contracts/*`。

### 租户与鉴权优先级（必须一致）

#### Tenant 来源优先级

1. 仅接受 JWT claims 中 `tid/tenant_uuid`。
2. 若 claims 缺失 tenant 信息则拒绝请求（`400/401`）。

#### Token 来源优先级

1. `Authorization: Bearer <token>`。
2. WS 握手 query 参数 `authorization`（仅用于 `/api/ws` 兼容场景）。
3. 其他来源一律不作为正式鉴权输入。

#### `proxy=1` 责任边界

- 插件/Framework：负责透传 token、tenant、trace 与 mode。
- PowerX 底座：负责最终鉴权、租户裁决、topic 授权与审计落库。
- 结论：`proxy=1` 不改变底座的最终授权决策权。

## Queue Driver 与降级策略（Phase 9）

### 驱动优先级

1. 默认驱动：`redis`。
2. 可选驱动：`kafka` / `rabbitmq` / `nats`。
3. 数据库通道：仅作为 fallback，不作为常态高频轮询主路径。

### 运行时约束

- 当 `queue.driver=redis`：允许 DB polling fallback 启用。
- 当 `queue.driver=kafka|rabbitmq|nats`：
  - 主任务驱动切换到对应 adapter；
  - DB polling fallback 默认关闭，避免启动后持续刷表；
  - 必须输出统一降级日志字段：`driver`、`tenant`、`reason`。

### 统一降级日志格式

- 禁用 DB polling fallback：
  - `[event_fabric.degrade] driver=<driver> tenant=all reason=db_polling_fallback_disabled`
- RetryWorker 跳过轮询：
  - `[event_fabric.retry] skip db polling fallback driver=<driver> reason=fallback_disabled`

### 兼容性要求

- 驱动切换不改变 ACK/NACK/Retry 语义。
- 驱动切换不改变 Topic/ACL/审计契约。
- 观测面至少覆盖：`LastTaskDriver`、`TaskDriverInitTotal`、`TaskDriverBlockingTotal`。

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
- **FR-003**: 发布 API 必须要求事件信封包含 `event_id`、`topic`、`version`、`trace_id` 等标准字段；`tenant_uuid` 必须由 JWT 上下文注入，不允许请求体/头覆盖。默认载荷使用 JSON 序列化（Topic 可声明覆盖）。
- **FR-004**: 订阅通道必须提供显式 Ack/Nack 机制，默认采用长连接（如 gRPC 流）推送事件，并允许消费者声明最大并发、超时时间与幂等键策略。
- **FR-005**: 系统必须支持可配置的重试策略（固定/指数退避、最大次数、抖动），默认提供至少一次（At-Least-Once）投递保证且最多重试 5 次，并在每次重试时记录投递状态。
- **FR-006**: 当事件超出重试次数或被标记不可重试时，系统必须将其写入租户隔离的 DLQ（默认落地 Postgres 持久化存储），并提供检索、导出、重放 API。
- **FR-007**: 系统必须向现有审计服务输出发布/订阅活动（含调用者、主题、结果、延迟），支持按租户和主题过滤。
- **FR-008**: 系统必须支持事件版本协商机制，订阅者可以声明兼容策略（仅接受某版本、向后兼容、全部接受）。
- **FR-009**: 系统必须提供事件回放能力，支持按时间窗口、事件类型或追踪 ID 过滤，并标记回放投递。
- **FR-010**: 系统必须暴露指标（投递成功率、重试次数、DLQ 体积、回放耗时）并与现有 Observability 管线对接。
- **FR-011**: 系统必须支持 `Idempotency-Key` 头部用于发布去重，并在订阅端提供去重建议（基于 `event_id`）。
- **FR-012**: 系统必须提供 HTTP/gRPC 的标准错误码与状态映射，并在 OpenAPI/Proto 中固化。
- **FR-013**: 系统必须维护连接契约、发布契约、事件契约三类边界，任一变更必须在权威文档与实现文档中同步。
- **FR-014**: 系统必须冻结 Topic 命名与版本兼容规则；新增/废弃 Topic 必须记录版本变更与兼容窗口。
- **FR-015**: 系统必须提供插件 Framework 的最小迁移清单（adapter、配置、权限、回归用例），确保 task/event 双通道可平滑迁移。
- **FR-016**: 文档治理流程必须要求“改 WS/TaskBus 代码即同步主契约文档”，并通过文档一致性检查阻止漂移。
- **FR-017**: 系统必须提供 Task Queue 可观测接口（pending/deferred/processing/inflight），并可按 tenant/topic/subscriber 过滤。
- **FR-018**: 系统必须提供统一运维处置入口（DLQ 重放、任务刷新、状态巡检），所有动作写入审计字段（`operator_id`,`tenant_uuid`,`trace_id`）。
- **FR-019**: 系统必须支持 Cron 任务定义（cron_expr/timezone/enabled/misfire_policy），并在到点时投递标准 Task Event。
- **FR-020**: Cron 触发后的执行链路必须复用既有 Retry/DLQ/Replay 机制，禁止引入独立队列或数据库轮询消费主路径。

### Non-Functional Requirements

- **NFR-001**: 核心消息路径的 P95 延迟 ≤ 100ms（不含订阅者处理时间），P99 重试延迟 ≤ 1s。
- **NFR-002**: 支持至少 5000 条/秒的并发事件发布，重试与回放不会影响实时流量 SLA。
- **NFR-003**: ACL 判定与审计写入必须是多租户隔离的；任何跨租户数据访问都应被拒绝并记录。
- **NFR-004**: 提供 HA 部署指南，支持水平扩展，重要状态（如偏移、重试计数、DLQ）持久化于可复制存储。
- **NFR-005**: 提供安全加固措施（静态密钥不落地、TLS、签名校验）并覆盖安全策略自检。

### Key Entities

- **TopicDefinition**: 描述 Topic 注册表中的语义定义（`namespace.name`），包含生命周期、版本兼容、重试与 Ack 策略。
- **AclBinding**: 关联主体（服务账号/角色）与权限（Publish/Subscribe/Replay），绑定 `topic_id`，并记录审批来源与过期时间。
- **EventEnvelope**: 统一事件信封，包含事件 ID、主题、版本、追踪信息、幂等键、发布时间以及 Payload 摘要；租户由 JWT 注入，并记录 `payload_format`（默认 JSON，可覆盖）。
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
- **SC-007**: 运维台页面刷新周期 5 秒内可稳定反映 Task Queue 状态变化，且与 API 返回口径一致。
- **SC-008**: Cron 任务触发时间偏差（计划时间 vs 实际入队时间）P95 ≤ 3 秒，且失败任务 100% 可进入统一重试或 DLQ。

## 风险与缓解

- **多租户隔离复杂度**：若租户识别来源不唯一会造成越权 → 统一 JWT-only，并以 ACL + 审计覆盖跨租户用例。
- **基础设施差异**：Redis/Kafka 等实现特性差异大 → 定义抽象接口与兼容层，首期实现 Local/Redis，留出 Kafka 扩展点。
- **Schema 演进冲突**：缺乏契约管理将导致消费失败 → 与 Contracts 服务集成，发布前进行 Schema 校验。
- **DLQ 堆积**：若补偿流程不畅会导致死信膨胀 → 设定阈值告警与自动化补偿脚本，并在规格中明确运维 SOP。

## 发布与迁移策略

### 版本变更记录要求

- 每次发布必须记录：新增字段、废弃字段、删除字段、默认值变化、兼容窗口截止时间。
- 事件兼容策略统一为：`backward` 默认，`strict` 与 `any` 需显式声明。
- 任何破坏性变更必须提供 migration note 与双写/灰度策略。

### 外部插件迁移最小改动清单

1. 适配层：接入 Framework 的 task/event 发布封装，不直接拼接底座内部接口。
2. 配置项：补齐 `mode=taskbus|dual|fallback`、`proxy`、`tenant`、`trace` 透传开关。
3. 权限：确认 `api_key_profile`（或其下发的 API Key 主体）对目标 topic 的 publish/subscribe/replay ACL。
4. 回归用例：覆盖 local + proxy=0/1 + mode 三种组合。
5. 观测：日志必须输出 `tenant_uuid/topic/trace_id/mode/proxy`。

## 文档防漂移执行要求

- PR 模板必须包含检查项：修改 WS/TaskBus 代码时是否同步 `specs/004-eventbus-message-fabric/spec.md`。
- CI 必须执行文档一致性检查，至少覆盖：
  1. topic 语义键一致（统一为 `namespace.name`，不拼 tenant 前缀）；
  2. internal 接口路径一致（`/internal/ws-bus/grant`、`/internal/ws-bus/publish`）；
  3. envelope 必填字段一致（`topic/type/payload/ts/trace_id`，tenant 来自 JWT）。
- 检查规则与执行入口见：`specs/004-eventbus-message-fabric/checklists/doc-consistency.md`。

## 验收前置清单

- [ ] Topic 目录 API、ACL 校验、投递/重试、DLQ、回放的合同文档与示例已准备就绪。
- [ ] 与安全策略、契约服务的集成测试通过（含负面用例）。
- [ ] Observability 指标与告警规则在测试环境验证。
- [ ] 运维手册涵盖主题生命周期、权限审批、DLQ 补偿、回放 SOP。

## Phase 13: Event ACL Governance UI（新增）

**Purpose**: 建立“配置态治理”与“运行态联调”分层，落地系统设置中的 Topic-Role 权限矩阵，避免每租户手工建 Topic。

### Scope & Order

1. **系统设置入口**：在 `系统设置` 下新增 `事件权限（Event ACL）` 页面入口。
2. **Topic 视图**：按 `topic_key` 展示可治理 Topic（含共享 Topic：`global/system`）。
3. **角色授权矩阵**：支持按角色或主体批量授予 `publish/subscribe/replay`。
4. **监控联动**：监控中心 Topic 行提供“管理权限”跳转（带 topic 参数），不直接编辑 ACL。
5. **审计闭环**：所有授权变更进入审计事件，记录 operator、topic、action、principal。

### Design Constraints

- 严禁“每新增租户手工创建同名 Topic”作为默认路径；默认使用全局 Topic + ACL 映射。
- ACL 管理接口必须支持共享 Topic 授权，不得强制要求 topic tenant 与 jwt tenant 完全一致。
- 前端主展示优先语义字段（`namespace.name` / 可读名称），UUID 仅用于内部引用与调试。
- 监控页与系统设置页职责分离：监控页负责观测排障，系统设置负责治理配置。

### Validation Targets

- 新租户无需额外创建 Topic，即可在 ACL 页看到共享 Topic 并完成角色授权。
- 授权后 WS subscribe/publish 与 replay 按 ACL 生效；无授权时返回一致 4xx 业务错误。
- 从监控页可一键跳转 ACL 页并保留 topic 上下文。
