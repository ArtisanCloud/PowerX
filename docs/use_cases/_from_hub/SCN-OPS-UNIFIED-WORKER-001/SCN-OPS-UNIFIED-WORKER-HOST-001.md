scn_id: SCN-OPS-UNIFIED-WORKER-HOST-001
title: 宿主分发透传
status: Draft
version: v0.1.0
owners:
  - name: Michael Hu
    role: Product Manager
    contact: matrix-x@artisan-cloud.com
domains: [ops]
layers: [service, integration]
repos:
  - key: powerx
    scope: core-platform
    responsibility: Worker 接口透传、宿主任务分发注册/提交/取消适配、状态/日志同步、审计
  - key: powerx-plugin
    scope: plugin-ecosystem
    responsibility: Handler/SDK 复用、回写格式一致性、宿主模式配置
related_usecases:
  - doc_id: UC-OPS-WORKER-HOST-001
    layer: service
    domain: ops
last_reviewed_at: 2025-10-19

---

# Executive Summary

在宿主模式下，插件通过统一接口将任务透传至底座任务分发，由宿主承担排队、并发与监控。目标是保持与 standalone 一致的提交/取消/查询与回写契约，让 Handler 复用不分叉，并在宿主不可用时具备降级路径。

# Scope & Guardrails

- **In Scope**：宿主任务分发注册/提交/取消；回写透传与格式对齐；超时/重试/幂等策略传递；宿主告警与观测对接；降级策略（回退本地或显式失败）。
- **Out of Scope**：宿主底层资源调度实现；插件业务逻辑；复杂 BPM 流程。
- **Environment & Flags**：`worker-facade-v1`、`host-task-dispatcher`；依赖宿主任务分发、配置中心（策略）、审计与监控链路。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| core-platform | powerx | service | 透传接口、宿主分发适配、回写同步、降级开关与审计 | Michael Hu（matrix-x@artisan-cloud.com） |
| plugin-ecosystem | powerx-plugin | integration | Handler 复用、宿主模式配置、进度/状态回写一致性 | Michael Hu（matrix-x@artisan-cloud.com） |

# End-to-End Flow

1. **注册与提交**：插件启动时注册 Handler；业务提交任务，封装层附带策略透传宿主分发。
2. **宿主执行与回写**：宿主排队/执行并回写进度/状态，封装层同步给业务/前端。
3. **取消/超时**：取消或超时指令透传宿主，确保幂等与审计。
4. **异常与降级**：宿主不可用时按策略回退本地或失败返回，并记录降级事件。

# Key Interactions & Contracts

- **APIs / Events**：`POST /worker/tasks`（宿主透传）、`POST /worker/tasks/{id}/cancel`、宿主分发注册/提交/取消接口、事件 `worker.task.updated`。
- **Configs / Schemas**：超时/重试/幂等/降级策略；回写 schema 与宿主字段映射。
- **Security / Compliance**：宿主/插件 ACL、签名或 token 透传、操作审计、租户隔离。

# Usecase Links

- `UC-OPS-WORKER-HOST-001` — 宿主任务分发透传执行。

# Acceptance Criteria

1. 透传提交/查询/取消成功率 ≥99%，p95 延迟 <250ms；宿主回写在 2s 内同步到封装层。
2. 回写格式与 standalone 保持一致；超时/重试策略按配置执行；幂等键避免重复执行。
3. 宿主不可用时触发降级策略并记录审计；告警与监控可在宿主控制台查看。

# Telemetry & Ops

- 指标：`worker.host.submit_success_total`、`worker.host.delivery_latency_ms`、`worker.host.callback_latency_ms`、`worker.host.degradation_trigger_total`。
- 告警阈值：宿主提交失败率 >3%；回写延迟 >2s；降级触发次数异常。
- 观测来源：宿主监控/告警、封装层日志、Grafana/Datadog `worker.host.*` 面板。

# Open Issues & Follow-ups

| 风险/事项 | 影响范围 | 负责人 | ETA |
|-----------|----------|--------|-----|
| 宿主与封装层回写字段映射差异可能导致显示不一致 | 前端/审计 | Michael Hu | 2025-10-31 |
| 降级回退策略对宿主不可用的探测频率未定 | 可靠性 | Michael Hu | 2025-11-08 |
