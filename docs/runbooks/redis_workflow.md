# Workflow Redis 队列运行手册

## 1. 队列结构
- 重试队列：按租户写入 Sorted Set，Key 结构 `event:retry:<tenantKey>`。
- 死信队列：调度失败时写入 `event:dlq:<tenantKey>`（由 ReliableQueue 落地）。
- 关联指标：`workflow_retry_scheduled_total`、`workflow_retry_timeout_total`（由服务侧上报）。

## 2. 队列巡检
```bash
# 查看待执行任务数量
redis-cli -u $REDIS_URL ZCARD "event:retry:<TENANT_KEY>"

# 查看最近 5 条任务明细
redis-cli -u $REDIS_URL ZRANGE "event:retry:<TENANT_KEY>" 0 4 WITHSCORES

# 监控 DLQ 是否堆积
redis-cli -u $REDIS_URL LLEN "event:dlq:<TENANT_KEY>"
```
- 当 `ZCARD` 持续增长且 `PopDue` 线程正常时，应检查执行器是否处理失败、或 `AcquireLease` 未释放。
- DLQ 长度大于 0 时，需追踪 `workflow.step.retry_failed` 审计事件并人工处理。

## 3. 定期清理
- 每日巡检 `event:dlq:*`，导出消息并根据 `metadata.step_id` 与 `attempt` 制定补救方案。
- 可通过 `ZREMRANGEBYSCORE` 清理过期的重试任务（通常不需要，谨慎操作）：
  ```bash
  redis-cli -u $REDIS_URL ZREMRANGEBYSCORE "event:retry:<TENANT_KEY>" -inf <UNIX_MS_THRESHOLD>
  ```
- 建议结合 `workflow.scheduler.retry_queue_depth` 指标设置告警阈值（参考监控仪表板）。

## 4. 常见故障
| 现象 | 排查步骤 | 处理建议 |
|------|----------|----------|
| 重试不生效 | 检查队列 `ZCARD` 与实例状态是否滞留 `waiting` | 确认 Worker 是否消费、查看日志中 `AcquireLease` 是否失败 |
| DLQ 堆积 | 查询最近 `workflow.step.retry_failed` 事件 | 回放任务、必要时人工补偿 |
| 队列键缺失 | 验证 Scheduler `RetryQueueKey` 输出 | 若租户 Key 配置错误，更新调度器启动参数 |

> 提示：`Scheduler.RetryQueueKey()` 可直接生成监控所需的 Redis Key，推荐在内部仪表板中引用该辅助函数。
