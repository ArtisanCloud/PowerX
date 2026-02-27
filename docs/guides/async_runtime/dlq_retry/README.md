# DLQ / Retry 管理（async_runtime）

> 状态：已实现（可联调）  
> 平台入口：`docs/guides/async_runtime/README.md`

## 1. 范围

1. 失败任务重试（Retry）
2. 死信队列（DLQ）查询/重放
3. Retry 与 Cron 的联动验证

## 2. 已实现入口

1. `GET /admin/event-fabric/dlq/messages`
2. `POST /admin/event-fabric/dlq/messages:replay`
3. `GET /admin/event-fabric/cron/jobs`
4. `POST /admin/event-fabric/cron/jobs/event_fabric.retry_dispatch/run-now`

## 3. 验证建议

1. 先查询 DLQ 基线
2. 触发 `run-now`
3. 再查询 `task-queue/stats` 与 `messages.history`
4. 必要时执行 DLQ replay 并再次确认历史变化

## 4. 参考文档

1. `docs/guides/async_runtime/event_fabric/integration_playbook.md`
2. `docs/guides/async_runtime/task/mechanism.md`

