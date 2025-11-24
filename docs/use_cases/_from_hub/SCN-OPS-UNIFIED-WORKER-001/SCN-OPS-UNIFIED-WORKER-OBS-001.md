scn_id: SCN-OPS-UNIFIED-WORKER-OBS-001
title: 观测、告警与降级
status: Draft
version: v0.1.0
owners:
  - name: Michael Hu
    role: Product Manager
    contact: matrix-x@artisan-cloud.com
domains: [ops]
layers: [ops, service]
repos:
  - key: powerx
    scope: core-platform
    responsibility: 指标/日志采集、告警策略、降级开关与审计、宿主观测对接
  - key: powerx-plugin
    scope: plugin-ecosystem
    responsibility: Handler 回写上下文、插件侧日志/指标补充、降级兼容
related_usecases:
  - doc_id: UC-OPS-WORKER-OBS-001
    layer: ops
    domain: ops
last_reviewed_at: 2025-10-19

---

# Executive Summary

为双模式任务执行提供一致的可观测性与告警能力，包括进度/状态、队列与并发指标、重试与取消统计，并支持宿主不可用时的降级（回退本地或显式失败）。目标是让运维在统一视图下追踪执行链路、及时发现异常并审计降级决策。
- PowerXPlugin 脚手架默认在 Handler 模板与 standalone 启动入口中输出运行模式标识、进度/日志字段，便于无宿主环境也能对齐观测与告警。

# Scope & Guardrails

- **In Scope**：指标/日志采集与聚合；告警规则（队列、回写、宿主投递、降级）；降级策略配置与审计；模式标识与回写一致性。
- **Out of Scope**：业务指标/日志定制；宿主底层监控实现；计费相关观测。
- **Environment & Flags**：`worker-facade-v1`、`worker-observability`、`worker-degradation`; 依赖日志/指标管道、告警通道、审计服务。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| core-platform | powerx | ops | 指标与日志采集、告警策略、降级控制与审计、宿主观测对接 | Michael Hu（matrix-x@artisan-cloud.com） |
| plugin-ecosystem | powerx-plugin | service | 回写上下文补充、插件日志/指标接入、降级兼容处理、脚手架输出模式标识/日志字段模板 | Michael Hu（matrix-x@artisan-cloud.com） |

# End-to-End Flow

1. **数据采集**：收集队列/并发、进度回写、成功/失败/重试/取消、宿主投递/回写延迟等指标与日志。
2. **告警与抑制**：按策略触发告警，支持抑制/合并，异常时落审计。
3. **降级决策**：宿主不可用时评估策略（回退本地或失败），记录降级事件与上下文。
4. **呈现与追溯**：观测数据供 Admin 看板与运维查询，确保模式与降级状态可视。

# Key Interactions & Contracts

- **APIs / Events**：观测上报接口、`worker.task.updated`、降级事件；告警通知（Webhook/IM）。
- **Configs / Schemas**：指标/告警阈值、降级策略、日志/回写字段（含运行模式标识）。
- **Security / Compliance**：日志脱敏、租户隔离、告警/降级操作审计。

# Usecase Links

- `UC-OPS-WORKER-OBS-001` — 观测、告警与降级。

# Acceptance Criteria

1. 指标/日志覆盖核心链路（提交、队列、执行、回写、取消、重试、降级），宿主与 standalone 字段一致。
2. 告警送达率 ≥99%；降级事件 100% 审计；观测数据延迟 <1 分钟。
3. 降级策略可配置，触发后能被看板与运维查询到完整上下文。

# Telemetry & Ops

- 指标：`worker.queue.depth`、`worker.progress.write_failure_total`、`worker.host.delivery_failure_total`、`worker.degradation.trigger_total`、`worker.degradation.success_total`。
- 告警阈值：回写失败率 >1%；宿主投递失败率 >5%；降级触发率异常；观测数据延迟超阈。
- 观测来源：日志/指标聚合、宿主监控、Admin 看板。

# Open Issues & Follow-ups

| 风险/事项 | 影响范围 | 负责人 | ETA |
|-----------|----------|--------|-----|
| 降级回退对单实例资源的占用需要限流策略 | standalone 资源 | Michael Hu | 2025-11-05 |
| 宿主与本地日志字段对齐待验证，影响告警抑制与合并 | 可观测性 | Michael Hu | 2025-11-12 |
