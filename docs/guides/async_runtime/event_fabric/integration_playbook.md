# Event Fabric Integration Playbook（联调与验收）

> 统一入口：`docs/guides/async_runtime/event_fabric/README.md`

## 1. 前置条件

```bash
export POWERX_BASE_URL="http://127.0.0.1:8077/api/v1"
export ADMIN_TOKEN="<root-admin-jwt>"
```

并确保：

1. 已执行 `make db-refresh`
2. backend / web-admin 已启动
3. 已用 Root 登录管理台

## 2. Step 1：Topic 与 ACL 基线

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/topics?page=1&page_size=100" | jq
```

至少应有：

1. `global._topic.knowledge.space.feedback.reprocess`
2. `global._topic.system.notification`

## 3. Step 2：Replay 联调

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$POWERX_BASE_URL/admin/event-fabric/replay/tasks" \
  -d '{"topic":"_topic.knowledge.space.feedback.reprocess","reason":"playbook-replay"}' | jq
```

验收：

1. 返回 `task_id`
2. 前端出现 `queued/running/completed|failed`
3. Queue 历史看到 `<tenant_uuid> + _subscriber.event_fabric.replay`

## 4. Step 3：Pipeline 联调

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$POWERX_BASE_URL/admin/event-fabric/pipeline/tasks" \
  -d '{"title":"Pipeline联调","content":"from playbook","type":"system","category":"system"}' | jq
```

验收：

1. 返回 `task_id`
2. 铃铛收到通知
3. Queue 历史看到 `global + _subscriber.system.notification_dispatch`

## 5. Step 4：Queue 校验

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/stats" | jq
```

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/messages?tenant_key=global&subscriber_id=_subscriber.system.notification_dispatch&limit=30" | jq
```

说明：`stats` 可能瞬时归零，历史以 `messages.history` 为准。

## 6. Step 5：Cron / Retry（可选）

先制造一条 Retry 样本（否则 run-now 可能无数据）：

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$POWERX_BASE_URL/admin/event-fabric/retry/tasks" \
  -d '{"topic":"_topic.system.notification","reason":"playbook-retry-seed"}' | jq
```

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs" | jq
```

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs/event_fabric.retry_dispatch/run-now" | jq
```

分支结果：

1. 有到期重试任务：队列/历史有变化
2. 无到期重试任务：接口成功但队列可能无变化（正常）

UI 对照（监控中心 -> Task / Cron）：

1. `kind=interval` 作业显示 `interval=Ns · batch=M`
2. `kind=queue` 作业显示 `trigger=queue · batch=M`
3. `立即执行` 是立刻执行当前行作业，不是随机新建任务
4. `event_fabric.retry_dispatch`：立即执行=扫描到期重试并重投
5. `event_fabric.authorization_challenge_timeout`：立即执行=扫描授权超时并处理

## 7. 脚本回归

Event Fabric（只读）：

```bash
scripts/event_fabric/integration_playbook.sh
```

Event Fabric（读写）：

```bash
scripts/event_fabric/integration_playbook.sh --with-write
```

WebSocket（读写）：

```bash
scripts/websocket/integration_playbook.sh --with-write
```

Cron（读写）：

```bash
scripts/cron/integration_playbook.sh --with-write
```

## 8. 核心接口总表（运维速查）

1. 总览：`GET /admin/event-fabric/overview`
2. Topic：`GET /admin/event-fabric/topics`
3. ACL：`GET /admin/event-fabric/acl/topic-matrix`
4. Queue 统计：`GET /admin/event-fabric/task-queue/stats`
5. Queue 历史：`GET /admin/event-fabric/task-queue/messages`
6. Replay 调试：`POST /admin/event-fabric/replay/tasks`
7. Pipeline 调试：`POST /admin/event-fabric/pipeline/tasks`
8. DLQ 查询：`GET /admin/event-fabric/dlq/messages`
9. DLQ 重放：`POST /admin/event-fabric/dlq/messages:replay`
10. Cron 列表：`GET /admin/event-fabric/cron/jobs`
11. Cron run-now：`POST /admin/event-fabric/cron/jobs/{job_id}/run-now`
12. Retry 样本：`POST /admin/event-fabric/retry/tasks`

## 9. WebSocket 内部调试

说明：

- `POST /api/v1/admin/event-fabric/topics` 用于创建 topic；
- `POST /api/v1/internal/ws-bus/grant` 仅做 topic 授权绑定（ACL），不创建 topic。
- 插件 topic 需先在 `plugin.yaml` 声明，并在安装/启用流程幂等同步到 `event_topics`。
- 底座启用插件时会在插件安装目录查找 `event_fabric.yaml`（推荐：`config/event_fabric.yaml`）并执行 topic/ACL 播种。

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "http://127.0.0.1:8077/api/v1/internal/ws-bus/grant" \
  -d '{"topics":["_topic.system.notification"],"actions":["subscribe"]}' | jq
```

判定：

1. `mode=registry_acl`：主路径
2. `mode=compat_dynamic`：兼容降级路径

## 10. 常见故障最短定位

1. `403 unauthorized`：先查 ACL，再查 JWT 主体上下文
2. `topic not found`：先查 `event_topics` 注册
3. Queue 全 0：先确认分片（`tenant_key + subscriber_id`）是否看对
4. 页面无更新：先查 WS 连接与订阅，再看队列历史

## 11. 运维观察建议

1. 先看 `stats`（运行态）
2. 再看 `messages.history`（追溯态）
3. 重试链路必须配合 `cron/jobs` 一起看
