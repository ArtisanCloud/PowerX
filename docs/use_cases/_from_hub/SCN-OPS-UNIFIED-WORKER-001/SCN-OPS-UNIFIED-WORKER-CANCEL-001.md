scn_id: SCN-OPS-UNIFIED-WORKER-CANCEL-001
title: 取消/超时与进程终止
status: Draft
version: v0.1.0
owners:
  - name: Michael Hu
    role: Product Manager
    contact: matrix-x@artisan-cloud.com
domains: [ops]
layers: [service, ops]
repos:
  - key: powerx
    scope: core-platform
    responsibility: 取消/超时接口、状态机、审计、宿主透传取消、SLA 配置
  - key: powerx-plugin
    scope: plugin-ecosystem
    responsibility: Handler 可中断性、子进程终止（下载/转码/ETL）、回写一致性
related_usecases:
  - doc_id: UC-OPS-WORKER-CANCEL-001
    layer: service
    domain: ops
last_reviewed_at: 2025-10-19

---

# Executive Summary

用户或系统需要在配置的 SLA 内取消或超时终止任务，包括停止外部进程，并在 standalone 与宿主模式下保持一致的状态回写与审计。目标是快速止损、避免僵尸任务，并提供可追溯的取消结果。

# Scope & Guardrails

- **In Scope**：取消/超时接口；子进程终止（下载/转码/ETL 等）；状态机更新与进度回写；宿主取消透传；幂等与审计；SLA 配置。
- **Out of Scope**：业务逻辑内的自定义回滚；宿主底层调度实现；超长流程的人工审批。
- **Environment & Flags**：`worker-facade-v1`、`worker-cancel-sla`；依赖进程信号管理、配置中心（SLA/重试）、审计与告警。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| core-platform | powerx | service | 取消/超时 API、状态机、宿主取消透传、审计与告警 | Michael Hu（matrix-x@artisan-cloud.com） |
| plugin-ecosystem | powerx-plugin | ops | Handler 可中断实现、子进程终止、回写一致性、超时处理 | Michael Hu（matrix-x@artisan-cloud.com） |

# End-to-End Flow

1. **触发取消/超时**：用户或系统调用取消接口，校验权限与幂等；超时由调度器触发。
2. **执行终止**：standalone 发送取消信号并终止子进程；宿主透传取消指令并等待执行反馈。
3. **回写与审计**：回写取消/超时状态与原因，记录审计日志；必要时告警。
4. **收敛与清理**：确保无僵尸任务/进程，清理资源，更新指标。

# Key Interactions & Contracts

- **APIs / Events**：`POST /worker/tasks/{id}/cancel`、超时事件触发、`worker.task.updated`（取消/超时状态）。
- **Configs / Schemas**：SLA（取消/超时）、幂等键、回写 schema、进程终止策略。
- **Security / Compliance**：权限校验（租户/插件），操作审计，日志脱敏。

# Usecase Links

- `UC-OPS-WORKER-CANCEL-001` — 取消/超时与进程终止。

# Acceptance Criteria

1. 取消/超时在 SLA 内完成，取消成功率 ≥99%；无残留僵尸进程。
2. 回写状态/原因一致，幂等且可审计；超时触发的自动取消可被前端感知。
3. 宿主模式取消透传成功率 ≥98%，失败触发告警与重试。

# Telemetry & Ops

- 指标：`worker.cancel.success_total`、`worker.cancel.latency_ms`、`worker.timeout.total`、`worker.process.termination_fail_total`。
- 告警阈值：取消失败率 >1%；终止超时；宿主取消透传失败率 >2%。
- 观测来源：审计日志、任务状态存储、Grafana/Datadog `worker.cancel.*`。

# Open Issues & Follow-ups

| 风险/事项 | 影响范围 | 负责人 | ETA |
|-----------|----------|--------|-----|
| 复杂 Handler 的中断点不足，可能导致取消延迟 | 可靠性 | Michael Hu | 2025-11-05 |
| 宿主取消反馈的错误码需要与封装层对齐 | 一致性 | Michael Hu | 2025-11-12 |
