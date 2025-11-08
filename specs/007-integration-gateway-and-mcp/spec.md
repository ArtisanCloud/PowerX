# Feature Specification: Integration Gateway & MCP Server

**Feature Branch**: `007-integration-gateway-and-mcp`  
**Created**: 2025-10-21  
**Status**: Draft  
**Input**: User description: "Title: Integration Gateway & MCP Server WHAT/WHY: Expose admin/user APIs and MCP server that invoke capabilities via registry/router and publish events. Scope: Admin API, tenant-facing API, MCP handlers, rate-limit, request tracing. Out-of-Scope: Re-implement registry/router/contracts/event fabric; orchestration internals. Dependencies: Contracts; Registry/Router; EventBus; Tool Grants."

## Clarifications

### Session 2025-10-21

- Q: 集成入口在平台内应该采用什么方式确保唯一性（用于配置查询、事件追踪等场景）？ → A: 租户自定义且在租户作用域内唯一的入口别名，平台内部再生成稳定 ID。
- Q: 当管理员未显式设置速率限制参数时，系统应采用怎样的默认时间窗口与突发策略？ → A: 令牌桶，基准速率按 1 分钟窗口发放，并允许 2 倍突发余量。
- Q: 集成入口在生命周期内需要遵循哪种状态机（以便管理 API 与租户调用对齐治理策略）？ → A: Pending → Active → Suspended → Retired，支持临时挂起与最终退役。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 管理端创建与治理统一入口 (Priority: P1)

作为平台管理员，我希望通过管理 API 为租户创建、调整和下线集成入口，配置调用配额与事件通知，从而让 tenant-facing API 与 MCP 服务器能够复用统一的能力路由与事件治理。

**Why this priority**: 没有可运营的管理面，租户无法进入统一通道，所有后续调用场景都会阻断。

**Independent Test**: 仅启动管理 API，调用创建/更新/下线接口，验证配置写入、版本记录与事件通知即可验收。

**Acceptance Scenarios**:

1. **Given** 能力契约、Registry 以及 Tool Grant 均已存在并有效，**When** 管理员通过管理 API 创建一个新的集成入口并配置租户、速率限制、事件路由，**Then** 系统在 5 分钟内生成可查询的入口配置快照，并向事件总线发布“入口已创建”事件。
2. **Given** 已上线的集成入口需调整限流阈值或关闭，**When** 管理员提交更新请求，**Then** 系统生成新版本、保留变更审计，并向所有受影响的调用通道广播“策略更新”事件。

---

### User Story 2 - 租户通过统一 API 触发能力 (Priority: P1)

作为租户侧集成工程师，我希望通过统一的租户 API 触发已授权的能力访问，并获取标准化响应、追踪信息与事件，以便快速在业务流程中调用平台能力。

**Why this priority**: 这是租户实际使用能力的主要入口，可直接衡量业务价值。

**Independent Test**: 仅启用租户 API，配置一个示例入口，模拟合法租户身份调用成功、超额与失败场景，并验证响应结构、限流与追踪字段。

**Acceptance Scenarios**:

1. **Given** 租户已获得对应 Tool Grant，且入口处于启用状态，**When** 租户在授权额度内发起调用，**Then** 系统通过 Registry/Router 解析到正确能力并返回标准化响应，同时在响应和日志中携带一致的追踪 ID。
2. **Given** 租户连续请求超过配置的速率阈值，**When** 后续请求到达，**Then** API 返回限流错误及建议重试时间，并在事件总线中发布“租户超额调用”告警。

---

### User Story 3 - MCP Server 暴露智能体能力 (Priority: P2)

作为内部自动化代理或外部 MCP 客户端，我希望通过 MCP Server 列举已授权的能力清单、获取 schema 并发起调用，以统一方式接入平台能力并延续事件和追踪能力。

**Why this priority**: MCP Server 保障多智能体场景可以复用同一治理体系，但相对租户 HTTP API 稍低优先级。

**Independent Test**: 仅启用 MCP Server，使用示例客户端执行 handshake、能力枚举、调用和错误处理流程，校验授权过滤、事件发布和追踪 ID 传播。

**Acceptance Scenarios**:

1. **Given** MCP 客户端提供有效的代理身份和租户上下文，**When** 请求列举可用能力，**Then** 服务器仅返回当前租户与 Tool Grant 允许的能力，并附带调用所需的输入输出 schema。
2. **Given** 能力调用失败或被治理策略拒绝，**When** MCP 客户端接收到响应，**Then** 结果中包含标准化错误码、追踪 ID，并且平台发布“能力调用失败”事件。

---

### Edge Cases

- 当租户在多个入口之间共享相同配额时，系统需要按租户维度合并速率限制，避免重复计数。
- 当 Registry 中能力暂时不可用或 Router 返回降级策略时，租户接口与 MCP Server 必须返回清晰的降级说明并附带后续动作指引。
- 当管理端配置引用不存在的能力 ID、契约或 Tool Grant 时，系统必须拒绝配置并提供详细的修复提示。
- 当事件总线暂时不可用时，调用不得被阻塞，应记录待补偿的事件投递任务并在恢复后补发。
- 当追踪 ID 缺失或格式异常时，系统需在入口层生成新的追踪 ID 并记录异常来源。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 系统必须提供管理 API，以租户为作用域创建、更新、停用集成入口，配置关联能力 ID、事件策略与可用通道（HTTP/MCP）；创建入口时要求租户提供在租户作用域内唯一的入口别名（slug），并返回平台生成的稳定入口 ID；入口状态机必须支持 Pending → Active → Suspended → Retired 的顺序转换，并允许在 Active 与 Suspended 之间往返。
- **FR-002**: 管理 API 必须支持为每个入口配置请求速率、并发阈值与配额重置窗口，并提供变更历史查询；若未显式配置，则默认采用 1 分钟窗口的令牌桶基准速率并允许 2 倍突发余量。
- **FR-003**: 管理 API 必须允许为入口定义事件主题、成功/失败事件类型与订阅目标，以驱动后续事件发布。
- **FR-004**: 租户 API 必须在每次调用时校验租户身份及 Tool Grant 范围，仅允许访问已授权的集成入口。
- **FR-005**: 租户 API 必须执行入口级与租户级速率限制，并在超限时返回统一错误结构及下一步指引。
- **FR-006**: 租户 API 必须将请求上下文标准化为能力路由请求，调用 Registry/Router 获取目标能力并返回标准化响应封装。
- **FR-007**: 租户 API 必须在成功或失败时生成事件负载，附带追踪 ID、租户信息、入口 ID 与执行结果，并异步发布到 EventBus。
- **FR-008**: 系统必须在所有入口（管理 API、租户 API、MCP Server）生成或传播统一追踪 ID，并记录关键阶段（鉴权、路由、事件发布）的追踪节点。
- **FR-009**: MCP Server 必须支持基于租户上下文列举已授权的能力列表，并提供能力元数据与输入输出 schema。
- **FR-010**: MCP Server 必须允许客户端发起能力调用、订阅流式事件，并在策略拒绝或执行失败时返回标准化错误响应和可重试指引。
- **FR-011**: 系统必须提供治理指标导出（调用量、成功率、限流命中、事件发布状态），以便运维在统一监控面观测。
- **FR-012**: 系统必须在配置或调用失败时保留审计记录，包含操作者、时间、入口 ID 和错误摘要，便于事后追踪。

### Key Entities *(include if feature involves data)*

- **IntegrationRoute**: 表示租户可用的集成入口，包含平台生成的稳定 route ID、租户提供且租户内唯一的入口别名（slug）、能力 ID、租户 ID、生命周期状态（Pending/Active/Suspended/Retired）、启停状态、速率限制策略、事件配置与版本号。
- **IntegrationInvocationLog**: 表示单次调用的追踪与审计信息，记录追踪 ID、入口 ID、租户、关键阶段耗时、执行结果及错误摘要，用于关联日志、事件与审计。
- **EventPublication**: 表示将要或已发布的事件记录，包含事件类型、目标主题、重试状态与补偿标记。
- **RateLimitPolicy**: 表示入口和租户组合的速率与配额规则，包含窗口大小、请求阈值、并发上限与恢复策略，默认策略为 1 分钟窗口的令牌桶基准速率并允许 2 倍突发余量。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 90% 的新集成入口在管理员提交后 5 分钟内对租户 API 与 MCP Server 生效，并可通过查询接口验证。
- **SC-002**: 在基准负载下，租户调用的首次成功率达到 99%，且标准响应中都包含追踪 ID。
- **SC-003**: 至少 95% 的成功与失败事件在 1 分钟内发布到 EventBus，并可在监控面上看到对应指标更新。
- **SC-004**: 超限调用产生的告警事件在 2 分钟内发送到指定订阅方，帮助租户在 1 天内将重复超限次数降低 30%。

## Assumptions

- 平台已有统一的身份体系与 Tool Grant 机制，本特性只复用校验逻辑，不重新实现鉴权。
- Registry、Router、EventBus 等依赖服务已具备高可用部署，本特性默认它们按既有 SLA 运行。
- 事件消费者与租户客户端可以处理带追踪 ID 的标准化响应结构，无需新增协议适配。
