scn_id: SCN-OPS-UNIFIED-WORKER-STANDALONE-001
title: standalone 队列+池执行
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
    responsibility: Worker 接口、本地队列与 goroutine 池调度、进度/日志存储、审计
  - key: powerx-plugin
    scope: plugin-ecosystem
    responsibility: Handler/SDK 实现、进度回写、子进程管理
related_usecases:
  - doc_id: UC-OPS-WORKER-STANDALONE-001
    layer: service
    domain: ops
last_reviewed_at: 2025-10-19

---

# Executive Summary

在独立运行模式下，插件需要通过统一 Worker 接口，将长耗时任务提交到本地队列并由 goroutine 池执行。目标是以一致的并发/队列/超时/重试策略保障任务可靠完成，进度与状态可查、日志可追溯，并防止队列溢出或资源耗尽。

# Scope & Guardrails

- **In Scope**：任务提交/查询/取消接口；本地队列与并发池调度；超时、重试/退避、幂等键；进度/状态回写与日志；队列溢出保护。
- **Out of Scope**：宿主任务分发、复杂 BPM 编排、宿主侧监控告警。
- **Environment & Flags**：`worker-facade-v1`、`standalone-queue`；依赖配置中心（并发/队列/超时/重试）、任务状态存储、日志管道。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| core-platform | powerx | service | Worker 接口、排队与并发控制、超时/重试、状态/日志回写、审计 | Michael Hu（matrix-x@artisan-cloud.com） |
| plugin-ecosystem | powerx-plugin | ops | Handler 复用、进度回写、子进程与信号管理 | Michael Hu（matrix-x@artisan-cloud.com） |

# End-to-End Flow

1. **提交与入队**：业务调用统一接口提交任务，校验并发/队列/幂等策略后入队。
2. **执行与回写**：goroutine 池按容量取任务执行 Handler（可调用外部工具），周期性回写进度/状态与日志。
3. **超时与重试**：执行超时或失败按退避策略重试，达上限后告警并标记失败。
4. **完成与审计**：成功或终止后写入最终状态、耗时与输出，记录审计。

# Key Interactions & Contracts

- **APIs / Events**：`POST /worker/tasks`、`GET /worker/tasks/{id}`、`PATCH /worker/tasks/{id}/progress`、事件 `worker.task.updated`。
- **Configs / Schemas**：并发、队列长度、超时、重试/退避、幂等键；任务状态/进度回写 schema。
- **Security / Compliance**：租户/插件 ACL、操作审计、日志脱敏、幂等令牌。

# Usecase Links

- `UC-OPS-WORKER-STANDALONE-001` — standalone 队列与池执行。

# Acceptance Criteria

1. 任务提交/查询接口成功率 ≥99%，p95 延迟 <200ms；入队后 2s 内可查询状态。
2. 队列深度与并发遵循配置，溢出返回可诊断错误且记录告警；重试遵守退避与幂等键。
3. 进度/状态回写可靠，日志可检索；超时/失败有清晰的终态与审计记录。

# Telemetry & Ops

- 指标：`worker.queue.depth`、`worker.queue.wait_ms`、`worker.pool.concurrency_inuse`、`worker.task.success_total`、`worker.task.retry_total`、`worker.task.timeout_total`。
- 告警阈值：队列深度或等待时长超阈；重试失败率 >2%；进度回写失败率 >1%。
- 观测来源：日志聚合、任务状态存储、Grafana/Datadog `worker.*` 面板。

# Open Issues & Follow-ups

| 风险/事项 | 影响范围 | 负责人 | ETA |
|-----------|----------|--------|-----|
| 队列溢出与资源竞争的压测基线未建立 | standalone 资源 | Michael Hu | 2025-10-31 |
| 外部工具日志格式未统一，影响回写与检索 | 可观测性 | Michael Hu | 2025-11-10 |
