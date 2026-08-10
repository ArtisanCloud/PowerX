# Feature Specification: Capability Registry & Router

**Domain Ownership**: CoreX (`corex.capability_registry`)

**Feature Branch**: `003-capability-registry-router`  
**Created**: 2025-10-15  
**Status**: Draft  
**Input**: User description: "Title: Capability Registry & Router. WHAT/WHY: Provide a registry of capabilities and a router that resolves endpoint/adapter at runtime with health, weights and policies. Scope: Registry API, router policies, health-check, discovery cache, fallback. Out-of-Scope: Defining capability contracts (use Contracts feature); Orchestration logic; Gateway UI. Dependencies: Contracts & Transport; Tool Grants; EventBus for async probe events."

## Clarifications

### Session 2025-10-15
- Q: 能力注册的管理维度应该是什么？ → A: 每个能力 ID × 租户生成独立注册快照，环境再作为策略字段
- Q: 客户端缓存能力快照的默认 TTL 应该是多少？ → A: 2 分钟，兼顾变更传播与缓存命中
- Q: Router 将适配器标记不可用后的默认冷却时间多久？ → A: 60 秒，在稳定性与恢复速度之间折中
- Q: 能力注册更新时并发提交如何控制？ → A: 乐观并发控制，使用快照版本号

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 可观测且一致的能力注册源（Priority: P1）

作为平台运维负责人，我希望所有能力提供者通过统一接口注册能力元数据、可用适配器与运行策略，这样调用方始终可以从可信真相源获取一致的能力配置，而不是依赖各自维护的配置文件。

**Why this priority**: 没有稳定的注册中心，就无法开展后续路由或治理能力，属于基础阻断事项。

**Independent Test**: 仅部署 Registry 服务，提交新的能力注册、查询能力详情并校验与契约、Tool Grant 的引用即可完成验收。

**Acceptance Scenarios**:

1. **Given** 能力提供者提交包含契约引用、适配器列表、健康策略的注册请求，**When** Registry 校验成功写入存储，**Then** 5 分钟内调用方通过查询 API 能获取完整的能力元数据和运行策略。
2. **Given** 已注册能力需要更新权重或启停某个适配器，**When** 运维人员通过管理 API 提交变更，**Then** Registry 生成新版本快照、保留历史记录并将更新事件发布到 EventBus。

---

### User Story 2 - 基于健康与权重的实时路由（Priority: P1）

作为平台路由服务的开发者，我需要在每次调用时根据能力的健康状态、权重、租户策略动态选择最合适的适配器，这样调用请求可以自动绕过失效节点并按既定 SLA 提供响应。

**Why this priority**: 路由决策直接影响服务稳定性与成本，是对外 SLA 的核心。

**Independent Test**: 启动 Router 与虚拟适配器，模拟健康/权重变化，验证路由选择、降级和回退是否符合策略，同时记录指标。

**Acceptance Scenarios**:

1. **Given** 某能力配置了三个适配器及不同权重，**When** 路由器接收调用请求，**Then** 它按权重抽样选出健康状态为良好的适配器，并在观察窗口内维持均衡分布。
2. **Given** 主适配器连续三次健康检查失败，**When** Router 下一次接收调用，**Then** 它应在 500 毫秒内降级至下一个符合策略的适配器，并记录降级事件及原因。

---

### User Story 3 - 缓存与失败切换体验（Priority: P2）

作为集成产品经理，我希望调用方能够在本地缓存能力快照，并在能力不可用时获取明确的降级响应或 fallback 能力，从而减轻对实时控制面的依赖并保障业务连续性。

**Why this priority**: 缓存与 fallback 可以显著减少延迟与全局故障的影响，但优先级低于注册与路由核心功能。

**Independent Test**: 构建轻量级 Discovery 客户端，验证快照缓存 TTL、生效刷新、fallback 能力选择以及错误提示流程。

**Acceptance Scenarios**:

1. **Given** 客户端持有最近的能力快照，**When** Registry 无法访问时，**Then** 客户端仍能在 TTL 内完成调用，并在日志中标记使用缓存。
2. **Given** 某能力的所有适配器均不可用，且配置了 fallback 能力 ID，**When** 调用继续发生，**Then** Router 返回 fallback 能力的响应或统一的失败说明，并触发预警事件。

### Edge Cases

- 所有适配器权重之和为 0 或均被禁用时，路由器必须拒绝调用并返回可操作的错误提示。
- 健康检查误报导致频繁上下线时，需要支持抖动保护（例如最小生效时间或冷却时间）。
- 注册请求引用的契约或 Tool Grant 不存在时必须拒绝并提供修复指引。
- 客户端缓存过期但 Registry 仍不可达时，应立即触发强制刷新并返回降级响应，避免使用过期策略。
- 跨租户/环境策略冲突（例如 sandbox 禁用的适配器在 prod 仍启用）时，应优先应用更严格策略并记录审计。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Registry 必须提供创建、更新、查询、禁用能力的 API/SDK，并对每次变更生成版本化快照（含生效时间、操作者、原因）；快照以能力 ID 与租户为联合维度，环境通过策略字段区分，写操作采用 ETag/版本号的乐观并发控制。
- **FR-002**: 注册能力时必须引用已发布的 Capability Contract、Transport Adapter 类型与 Tool Grant，Registry 需校验关联引用合法性。
- **FR-003**: 每个能力可配置多个适配器目标，需描述协议类型、终端地址、权重、最大并发、超时、租户/环境可见性。
- **FR-004**: Registry 需支持策略化健康检查（主动探测、被动熔断、外部探活事件），并将结果持久化及推送至 Router。
- **FR-005**: Router 必须按照权重、健康状态、租户/场景策略实时选择适配器，并支持优先级、静态绑定（sticky）和按能力层面的限流。
- **FR-006**: 当主适配器不可用时，Router 需在可配置时间窗口内执行多级 fallback，包括备用适配器、备用能力、以及统一失败响应。
- **FR-007**: Router 必须支持透明的调用观测数据上报，包括选路决策、耗时、错误码、降级次数，并向 Observability 规范输出指标和 tracing span。
- **FR-008**: Registry 与 Router 需提供 Discovery/Sync API，允许客户端订阅增量事件或获取完整快照，并支持 ETag/版本号校验。
- **FR-009**: 客户端缓存策略需支持 TTL、最大闲置时间与强制刷新机制，并在本地持久化安全上下文（避免暴露敏感配置）；默认 TTL 为 2 分钟，可按能力覆盖。
- **FR-010**: 系统必须提供权限与审计控制，记录能力注册、策略变更、手动下线/上线等操作，并允许按用户、能力、时间过滤。
- **FR-011**: 当健康状态或权重发生变化时，Registry 需向 EventBus 发布事件，Router 与下游订阅者在 1 秒内收到并更新。
- **FR-012**: Router 执行 fallback 后需同步回 Registry，确保后续健康评估包含失败次数与恢复时间，避免误判。
- **FR-013**: 必须提供自助测试接口（sandbox mode），允许在不影响生产的前提下模拟策略与路由结果。
- **FR-014**: 支持跨区域/多集群部署，Router 在同城内使用本地缓存优先，跨区域失败时可回源 Registry 或请求远端 Router。
- **FR-015**: Registry 必须保存插件细颗粒度权限元数据，包括 `source=plugin`、`permission_code`、`type=menu|page|action|api`、i18n key、默认角色建议、风险等级和 protocol binding，并向 IAM 角色权限中心、Gateway 预检和插件运行时授权快照提供一致查询。

### Key Entities

- **CapabilityRegistration**: Registry 中的能力主记录，以能力 ID 与租户为唯一键，包含契约引用、状态、适配器列表、版本信息与策略元数据。
- **PluginPermissionRegistration**: 插件权限登记快照，记录 `permission_code`、授权类型、i18n、协议 binding 与同步状态，是 IAM Permission 同步和 Gateway enforcement 的来源。
- **AdapterEndpoint**: 代表单个可路由目标，描述协议、地址、健康窗口、权重、并发限制和标签。
- **RoutingPolicy**: 定义权重策略、租户/环境过滤、fallback 顺序、熔断规则和限流阈值。
- **HealthProbeResult**: 保存主动/被动探测结果、故障原因、置信度与过期时间，供 Router 判定，并记录默认 60 秒的恢复冷却窗口及最近写入版本号。
- **DiscoveryCacheEntry**: 客户端缓存的能力快照，含版本号、生效范围、TTL 及安全校验信息。
- **FallbackPlan**: 当主能力不可用时的回退描述，包含备用能力 ID、静态响应模板及触发条件。

## Assumptions & Dependencies

- Capability Contract、Transport Adapter 接口已在 `002-title-unified-capability` 范围内交付，本功能依赖其提供的 schema 与 SDK。
- EventBus 可提供至少一次投递保证，并允许 Registry/Router 订阅健康事件与策略变更。
- 权限与审计体系（Tool Grants/IAM）已支持 fine-grained scope，Registry 管理接口会重用这些能力。
- 底层存储（例如 Postgres + Redis）已可用，支持版本化快照存储及低延迟缓存。
- 运维团队可提供外部健康探测结果（如 Prometheus、Agent heartbeat）并推送至 EventBus。

## Success Criteria *(mandatory)*

- **SC-001**: 能力注册或策略更新在 3 分钟内对所有 Router 实例生效，95% 的变更在 30 秒内完成推送。
- **SC-002**: 主适配器故障后，Router 平均在 500 毫秒内完成自动降级，成功率 ≥ 99%。
- **SC-003**: Registry 提供的查询 API 99% 分位响应时间 ≤ 150 毫秒，月度可用性 ≥ 99.9%。
- **SC-004**: 缓存模式下客户端命中率 ≥ 80%，且使用过期快照导致的失败事件每月不超过 3 起。
- **SC-005**: 全链路观测覆盖率达到 100%，可按能力维度输出选路决策、降级次数和失败原因，支持回溯最近 30 天。

## Review & Acceptance Checklist

### Content Quality

- [x] 无实现细节（语言、框架、具体 API）
- [x] 聚焦用户价值与业务需求
- [x] 面向非技术干系人易于理解
- [x] 必填章节已完成

### Requirement Completeness

- [x] 无 [NEEDS CLARIFICATION] 标记
- [x] 需求具备可验证性与明确度量
- [x] 成功标准可衡量
- [x] 范围清晰、边界明确
- [x] 依赖关系已识别

## Execution Status

- [x] 解析用户描述
- [x] 提炼核心概念
- [x] 明确范围与边界
- [x] 梳理用户场景
- [x] 生成需求列表
- [x] 定义关键实体
- [x] 制定成功标准
- [x] 完成审阅检查
