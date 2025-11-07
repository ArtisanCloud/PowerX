# Research Log: Agent Lifecycle & Observability

**Date**: 2025-10-22  
**Branch**: `008-agent-lifecycle-observability`

## Dependency & Pattern Findings

### Agent 容量阈值度量
- **Decision**: 以可同时运行的代理实例/Pod 数量作为默认容量阈值。  
- **Rationale**: 与 Kubernetes/Golang worker 规模控制方式一致，便于映射至现有调度策略，也能直接驱动自动扩缩容与告警阈值。  
- **Alternatives considered**: 以请求吞吐量衡量（难以跨能力对齐且需额外监测模型）；以 CPU/内存配额衡量（与平台配额管理重复，缺少业务语义）。

### 健康告警通知渠道
- **Decision**: 使用企业即时通讯（Slack/钉钉/企业微信）Webhook 群发告警。  
- **Rationale**: CoreX 运维体系已建立 IM 值班群；Webhook 简单稳定，满足 30 秒内送达目标并利于多租户按群隔离。  
- **Alternatives considered**: PagerDuty/OpsGenie（需额外许可与集成工时）；纯邮件通知（延迟不可控且缺少实时互动）。

### 数据保留策略
- **Decision**: 生命周期与健康历史、审计记录统一保留 13 个月。  
- **Rationale**: 与现有审计策略保持一致，满足一年合规要求且便于追溯退役代理指标。  
- **Alternatives considered**: 180 天（不足以覆盖年度审计）；24 个月（额外存储成本，缺少监管驱动）。

### 健康评分管道
- **Decision**: 基于 OpenTelemetry 指标聚合，1 分钟窗口计算吞吐/成功率/延时，再结合日志与追踪异常权重输出健康评分。  
- **Rationale**: 复用现有 OTel 采集链路，减少自研工作量，支持丰富标签维度（代理、租户、实例）。  
- **Alternatives considered**: 自研指标管道（实现成本高）；依赖第三方 APM 专用评分（授权成本与多租户隔离风险）。

### 生命周期事件发布保障
- **Decision**: 所有生命周期变更通过 EventBus 同步发布，失败时写入补偿队列并重试，期间阻断状态提交。  
- **Rationale**: 保证下游系统（调度、计费、审计）一致性，符合核心服务对事件可靠性的要求。  
- **Alternatives considered**: 异步最佳努力发布（可能导致状态与事件不一致）；事务消息（需额外基础设施支持，暂不具备）。
