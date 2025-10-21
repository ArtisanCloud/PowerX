# Workflow Observability Runbook

## 1. 主要指标
| 指标 | 类型 | 说明 | 建议阈值 |
|------|------|------|---------|
| `workflow_retry_scheduled_total` | Counter | 步骤失败后触发自动重试次数 | 速率突增（> 50/min）报警 |
| `workflow_compensation_running` | Gauge | 当前处于补偿状态的实例数 | > 5 持续 10 分钟需人工介入 |
| `workflow_retry_queue_depth` | Gauge | Redis 重试队列长度 | 单租户 > 200 时提示扩容或排查长尾 |
| `workflow_control_events_total` | Counter | 人工控制操作次数（暂停/恢复/取消/重试） | 监控异常频率 |
| `workflow_export_rows_total` | Counter | 审计导出生成的记录数 | 用于合规统计 |

> 指标由 `MetricsRecorder` 实现（OTEL Exporter），默认命名空间 `workflow.runtime.*`。

## 2. 日志与追踪
- 日志字段 `workflow_event` 搭配 event_type，可快速定位补偿、控制操作。
- 对异步执行链（Agent 调用）采样率建议 ≥ 10%，并在 Span Attribute 中包含 `workflow.instance_uuid`、`step_id`。

## 3. 仪表板布局
1. **运行态**：实例状态分布（饼图）、重试速率（折线）、补偿中的实例（单值）。
2. **队列健康度**：重试队列长度、DLQ 长度、`AcquireLease` 失败率。
3. **人工操作审计**：`workflow_control_events_total` 分动作堆积柱状图。
4. **导出统计**：导出行数、最近成功/失败事件。

对应的 Grafana 模板位于 `deploy/observability/workflow_dashboard.json`。

## 4. 告警策略
| 告警 | 触发条件 | 响应 |
|------|----------|------|
| `WorkflowRetryBurst` | `workflow_retry_scheduled_total` 5 分钟速率 > 基线 3 倍 | 检查最近上线、目标 Agent 健康度 |
| `WorkflowCompensationBacklog` | `workflow_compensation_running` > 5 持续 10 分钟 | 确认补偿流程是否卡住，必要时人工介入 |
| `WorkflowRetryQueueHigh` | `workflow_retry_queue_depth` > 200 | 排查调度 worker、Redis 延迟 |

## 5. 操作指引
- 遇到高重试速率：先查看仪表板重试原因，结合 `workflow_events` 审计日志定位故障步骤。
- 补偿 backlog：按 `workflow.step.compensation_*` 事件查询补偿详情，并在控制台人工确认。
- 导出失败：检查 `workflow.reporting.export.generated` 事件 Payload 中的 `row_count` 与错误详情。

> 建议与 Redis 运行手册配合使用，确保队列与 DLQ 状态实时受控。
