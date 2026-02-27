# Observability（Logs / Metrics / Trace）

> 状态：部分实现（日志与链路已可用，告警规范待补）  
> 平台入口：`docs/guides/async_runtime/README.md`

## 1. 范围

1. HTTP/WS 请求日志
2. 任务链路字段（task_id/trace_id/topic/subscriber）
3. 指标与追踪（Prometheus / OpenTelemetry）

## 2. 当前可用能力

1. 联调可通过后端日志观察任务与 WS 链路
2. Event Fabric 脚本和 runbook 已定义最小验证路径
3. 运行态统计可通过 Queue API 直接观测

## 3. 建议最小字段

1. `trace_id`
2. `task_id`
3. `tenant_key`
4. `subscriber_id`
5. `topic`
6. `status`

## 4. 参考文档

1. `docs/guides/async_runtime/event_fabric/integration_playbook.md`
2. `docs/guides/async_runtime/task/mechanism.md`
3. `docs/guides/async_runtime/log_trace/README.md`（兼容入口）

## 5. 待补齐项（占位）

1. Prometheus 指标字典
2. Trace span 命名规范
3. 告警阈值与SLA模板

