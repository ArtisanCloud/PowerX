# Runtime Scheduler Quickstart

> 当前为目标契约。实现完成前，以下命令用于验收新 SchedulerService；现有 `/admin/event-fabric/cron/jobs` 仍只用于底座内部 Cron 运维。

## 1. API Key local+proxy 创建插件 Job

API Key 只证明租户和 Scheduler REST 权限，`owner_id` 是该租户内声明的 plugin namespace，不证明插件身份。顶层不要传 `tenant_uuid`，PowerX 从 API Key 解析租户；业务 payload 里可以保留 `tenant_uuid` 供插件 handler 定位业务数据。

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/scheduler/jobs" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "owner_type": "plugin",
    "owner_id": "com.powerx.plugins.ai-craft",
    "name": "delivery_reminder_15m",
    "schedule_type": "once",
    "schedule_expr": "2026-05-22T10:30:00+08:00",
    "timezone": "Asia/Shanghai",
    "payload": {
      "tenant_uuid": "'$TENANT_UUID'",
      "business_action": "delivery_reminder_15m",
      "order_id": "order_123"
    },
    "idempotency_key": "tenant:ai-craft:delivery_reminder_15m:order_123"
  }' | jq
```

## 2. STS/Bearer 创建插件 Job

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/scheduler/jobs" \
  -H "Authorization: Bearer $PLUGIN_STS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "owner_type": "plugin",
    "owner_id": "com.powerx.plugins.ai-craft",
    "name": "delivery_reminder_15m",
    "schedule_type": "once",
    "schedule_expr": "2026-05-20T10:30:00+08:00",
    "timezone": "Asia/Shanghai",
    "payload": {
      "business_action": "delivery_reminder_15m",
      "order_id": "order_123"
    },
    "idempotency_key": "tenant:ai-craft:delivery_reminder_15m:order_123"
  }' | jq
```

Bearer/STS 模式要求 token audience 包含 `plugin:com.powerx.plugins.ai-craft`，否则 `owner_id` 校验失败。

预期：

1. 返回 `job_id`。
2. `status=active`。
3. `next_run_at` 与 `schedule_expr` 对齐。

## 3. 查询 Job

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/scheduler/jobs?owner_type=plugin&owner_id=com.powerx.plugins.ai-craft" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY" | jq
```

## 4. 手动触发

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/scheduler/jobs/$JOB_ID/trigger" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY" | jq
```

预期：

1. 创建 `trigger_source=manual` 的 run。
2. 发布 `powerx.runtime.scheduler.triggered.v1`。

## 5. 暂停与恢复

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/scheduler/jobs/$JOB_ID/pause" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY" | jq

curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/scheduler/jobs/$JOB_ID/resume" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY" | jq
```

## 6. 查看运行记录

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/scheduler/jobs/$JOB_ID/runs" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY" | jq
```

预期：

1. 能看到 `run_id/status/trigger_source/event_id/trace_id`。
2. 可通过 `trace_id` 关联 EventBus 投递记录和日志。

## 7. 不要使用 Event Fabric Cron 注册插件 Job

以下接口仅用于底座内部 worker 运维：

```text
GET  /admin/event-fabric/cron/jobs
POST /admin/event-fabric/cron/jobs/{job_id}/run-now
POST /admin/event-fabric/cron/jobs/{job_id}/pause
POST /admin/event-fabric/cron/jobs/{job_id}/resume
```

它们不接受插件业务 job 创建，也不等同于 Runtime Scheduler。
