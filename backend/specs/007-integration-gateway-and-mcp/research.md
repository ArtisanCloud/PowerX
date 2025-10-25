## Research Findings

### Decision: 集成入口配置持久化在 CoreX GORM 模型并维护版本表
- **Rationale**: 复用既有 CoreX 模型/迁移体系，便于多租户约束、审计字段和 AutoMigrate；单独的 `integration_routes` 主表 + `integration_route_versions` 版本表可满足“可查询快照”“追踪历史”两类需求，与 Registry/Tool Grants 结构一致。
- **Alternatives considered**:
  - 仅存储在 JSON 配置中心：缺乏事务与审计能力，不符合 Constitution 对持久化的要求。
  - 复用 Capability Registry 表：语义不同（入口聚合多能力策略），会导致写路径耦合、影响 Registry 边界。

### Decision: 限流实现复用事件骨干 Redis 令牌桶限流器，新前缀 `integration_gateway:rl`
- **Rationale**: `authorization.NewRateLimiter` 已实现令牌桶 + 冷却逻辑，扩展前缀即可满足租户/入口粒度限流；兼容 Redis 配置与现有监控，不需重复维护限流存储。
- **Alternatives considered**:
  - 全新实现：增加维护成本且难以复用监控指标。
  - 仅依赖 API Gateway 外部限流：无法对 MCP/内部调用统一治理，与 Success Criteria 的告警要求冲突。

### Decision: 事件发布复用 EventBus，新增主题前缀 `integration.gateway.*`
- **Rationale**: Spec 要求创建/调用等事件在 1 分钟内可订阅，与现有 `event_bus.EventBus` 异步发布模型匹配；统一命名空间便于租户、审计与下游订阅管理。
- **Alternatives considered**:
  - 直接写审计表：缺乏实时通知能力，无法驱动自动化响应。
  - 新建 Kafka Topic 管理层：与现有 EventBus 体系重复建设。

### Decision: TraceID 传播沿用 HTTP/Gin 中间件 + gRPC 拦截器 + MCP Tool 上下文注入
- **Rationale**: 现有 `internal/http/middleware.TraceInjectionMiddleware`、gRPC `middleware2` 拦截器已经在上下文写入 `trace_id`，新增 Handler 只需确保取用；MCP Server 提供的 `server.Context` 可写入相同 key 以覆盖事件发布与日志，保持一致链路观测。
- **Alternatives considered**:
  - 自定义 Trace 实现：重复造轮子且可能与当前日志体系不兼容。
  - 仅依赖外部调用方传递：在缺省情况下无法满足 “标准响应包含 trace id” 的要求。

### Decision: MCP 能力通过现有 register.ToolRegistry 注册两种工具 (`integration.route.list`/`integration.route.invoke`)
- **Rationale**: register 工厂与模板已支持工具规范、权限与指标；拆分“列举可用能力”和“执行调用”便于复用授权逻辑以及与 Tool Grants 对齐；同时满足 spec 对 schema 暴露和调用触发的诉求。
- **Alternatives considered**:
  - 单工具多模式：调用参数复杂且难以复用缓存。
  - 独立 MCP Server：破坏当前统一部署模式，违背 Constitution CoreX 模块约束。
