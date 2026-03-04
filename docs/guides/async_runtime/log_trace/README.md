# Logs / Trace 观测说明（async_runtime）

> 状态：兼容入口（主文档已迁移）  
> 平台级入口：`docs/guides/async_runtime/README.md`
> 主入口：`docs/guides/async_runtime/observability/README.md`

## 1. 范围

1. HTTP/WS 请求日志字段约定
2. 任务链路日志与事件审计（含 `task_id/trace_id`）
3. 指标与追踪（Prometheus / OpenTelemetry）
4. 联调时最小观测清单

## 2. 当前可用观测入口

1. Event Fabric 运维与故障：`docs/guides/async_runtime/event_fabric/integration_playbook.md`
2. Task 机制与链路字段：`docs/guides/async_runtime/task/mechanism.md`
3. 联调验收步骤：`docs/guides/async_runtime/event_fabric/integration_playbook.md`

## 3. 最小字段要求（联调建议）

1. `trace_id`
2. `task_id`
3. `tenant_key`
4. `subscriber_id`
5. `topic`
6. `status`
