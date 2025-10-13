# Phase 0 Research — Unified Capability Contracts & Transport Adapters

## Storage Strategy for Capability Truth Source

- Decision: 将 Capability Contract 与版本策略持久化在现有 CoreX Postgres 数据库中，复用 `gorm.io/gorm` 与多租户表前缀策略。  
- Rationale: 仓库默认驱动为 Postgres，`config/defaults.go` 及 `pkg/corex/db/database/connection.go` 已提供连接与租户隔离；沿用现有数据库可与 Registry/Router 共享事务一致性，并满足审计需求。  
- Alternatives considered: 独立文档存储（例如 MongoDB/Elastic）在本阶段会引入新的运维与一致性问题；使用文件或对象存储不利于事务校验与多租户隔离。

## Scale & Tenant Scope

- Decision: 设计契约模型以支撑多租户场景下至少 100+ 能力、每个能力 2-3 个主版本并发存在，以及高峰期 ~1k RPS 的调用查询。  
- Rationale: Integration 架构文档（`docs/integration/01_architecture/PowerX_Integration_Architecture.md`）定位为统一编排内核，需服务插件、Agent 与第三方；成功指标（SC-001~SC-004）要求高可用与快速上线，因此以百级能力、千级调用作为容量基线。  
- Alternatives considered: 仅面向单租户或几十个能力的规模无法满足平台化目标；过早按照万级能力优化目前缺乏需求，留待后续容量提升。

## IAM Scope & Tool Grant 校验

- Decision: 契约发布与调用流程必须对接 IAM Scope 与 Tool Grant 三层验证模型，发布时校验 Scope 存在性，并在 Transport Adapter 执行阶段校验代理调用的 Tool Grant。  
- Rationale: `docs/integration/05_security/Capability_and_Tool_Grants_Spec.md` 明确三层安全（Scope Engine / Tool Grant Evaluator / Policy Evaluator）；契约若引用不存在的 Scope/Grant 会导致运行时拒绝，应在发布阶段前置校验。  
- Alternatives considered: 将校验留到运行时虽然减少发布耦合，但会导致能力上线失败率上升且违背零信任要求。

## EventBus & Audit Integration

- Decision: 契约发布、版本升级与废弃需向 EventBus 广播事件并写入审计日志，事件命名遵循 `integration.capability.<action>`，内容含版本、替代关系与影响范围。  
- Rationale: PXIP-001 及成功指标（SC-005）要求事件在 1 分钟内可被订阅模块消费；当前 EventBus 是统一连续性通道，沿用可与现有 Router/Workflow 监听机制兼容。  
- Alternatives considered: 仅使用数据库触发器或内部回调无法满足跨模块通知需求，也不利于外部系统观测。

## Transport Adapter Interface Patterns

- Decision: 采用 `TransportAdapter` 接口定义同步 `Invoke`、流式 `Stream`、`HealthCheck`、`Close` 方法，并统一 `TransportRequest/Response/StreamChunk` 数据结构，覆盖 trace、tenant、重试策略与错误码映射。  
- Rationale: PXIP-001 与 `docs/integration/09_agent/AgentAdaptor_and_Transport_Spec.md` 均强调多协议共用抽象；统一接口可让 Router 在协议间平滑切换，并确保错误映射至契约的 Error Taxonomy。  
- Alternatives considered: 为每个协议保留独立接口将继续造成碎片化，实现成本高且难以保证一致的观测与重试语义。

## Error Taxonomy Baseline

- Decision: 定义分级错误模型（`capability.<category>.<code>`），包含严重级别（info/warn/error/fatal）、调用阶段（validate/invoke/stream/observe）与补救建议字段，统一落地在契约定义中，并在适配器中强制映射。  
- Rationale: 规格 FR-004 与 PXIP-001 要求跨协议一致的错误语义；统一 taxonomy 有利于指标聚合与调用方自恢复。  
- Alternatives considered: 复用现有 HTTP 状态码或 gRPC status 会丢失跨协议一致性，且无法表达多阶段调用细节。
