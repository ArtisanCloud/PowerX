scn_id: SCN-OPS-UNIFIED-WORKER-ADMIN-BOARD-001
title: PowerX Admin 任务看板
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
    responsibility: 看板查询接口、指标与日志聚合、受控取消/重试、ACL 与审计
  - key: powerx-plugin
    scope: plugin-ecosystem
    responsibility: 回写上下文提供、日志片段对接、模式标识支持
related_usecases:
  - doc_id: UC-OPS-WORKER-ADMIN-BOARD-001
    layer: ops
    domain: ops
last_reviewed_at: 2025-10-19

---

# Executive Summary

PowerX Admin 任务看板为运维/管理员提供统一视图，按租户/插件/运行模式查看异步任务的队列、并发、成功/失败/重试/取消占比、耗时分布、告警与日志片段，并在 ACL 与审计保护下发起取消/重试。目标是提升跨模式的可见性与操作效率。

# Scope & Guardrails

- **In Scope**：任务列表与筛选（租户/插件/模式/状态）、队列与并发指标、进度/耗时可视化、日志片段查询、受控取消/重试、审计、数据延迟 SLA。
- **Out of Scope**：插件业务日志深度检索、宿主控制台 UI；计费或额度管理。
- **Environment & Flags**：`worker-admin-board`、`worker-facade-v1`; 依赖任务状态/日志存储、观测指标、权限/审计服务。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| core-platform | powerx | ops | 看板 API、数据聚合、权限/审计、受控操作（取消/重试） | Michael Hu（matrix-x@artisan-cloud.com） |
| plugin-ecosystem | powerx-plugin | service | 提供回写上下文、日志片段、模式标识，保证字段一致 | Michael Hu（matrix-x@artisan-cloud.com） |

# End-to-End Flow

1. **查询与筛选**：看板按租户/插件/模式/状态查询任务，展示队列/并发/成功率/重试/取消/耗时分布。
2. **详情与日志**：任务详情页显示进度、回写上下文、日志片段、执行节点/模式。
3. **受控操作**：具备权限的用户可发起取消/重试，写入审计并反馈结果。
4. **告警与延迟**：看板标识告警与数据延迟状态，缺失数据需提示采集健康。

# Key Interactions & Contracts

- **APIs / Events**：看板查询接口、任务详情/日志接口、受控取消/重试 API、`worker.task.updated`。
- **Configs / Schemas**：筛选字段（租户/插件/模式/状态）、指标定义、日志片段字段、权限模型。
- **Security / Compliance**：ACL/MFA、审计日志、日志脱敏、租户隔离。

# Usecase Links

- `UC-OPS-WORKER-ADMIN-BOARD-001` — Admin 任务看板与受控操作。

# Acceptance Criteria

1. 看板查询 p95 <1s，数据延迟 <1 分钟；筛选/分页正常。
2. 受控取消/重试有权限校验与审计，操作成功率 ≥99%，结果及时回显。
3. 日志片段与模式标识可见，缺失数据有健康提示；告警状态与任务状态一致。

# Telemetry & Ops

- 指标：`worker.admin.query_latency_ms`、`worker.admin.query_success_total`、`worker.admin.cancel_total`、`worker.admin.retry_total`、`worker.admin.audit_write_failure_total`。
- 告警阈值：查询失败率 >1%；数据延迟超阈；受控操作失败率 >2%；审计写入失败。
- 观测来源：看板 API 日志、任务状态/日志存储、Grafana/Datadog `worker.admin.*`。

# Open Issues & Follow-ups

| 风险/事项 | 影响范围 | 负责人 | ETA |
|-----------|----------|--------|-----|
| 日志脱敏策略未与安全组评审，可能阻断日志片段展示 | 合规/可观测性 | Michael Hu | 2025-11-12 |
| 宿主与本地模式字段对齐需验证，防止筛选/统计偏差 | 数据一致性 | Michael Hu | 2025-11-08 |
