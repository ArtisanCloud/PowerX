# Task 子系统说明（async_runtime）

> 状态：已实现（基础能力）  
> 平台级入口：`docs/guides/async_runtime/README.md`

## 1. 这份文档解决什么问题

1. 怎么确认 Task 机制已打通
2. 怎么手动验证 Replay / Pipeline 两条链路
3. 怎么看队列运行态与历史态
4. 出错时先查哪几个接口

## 2. 前置条件

```bash
export POWERX_BASE_URL="http://127.0.0.1:8077/api/v1"
export ADMIN_TOKEN="<root-admin-jwt>"
```

要求：

1. backend 已启动
2. 已完成 `make db-refresh`
3. 当前账号有 Root 管理权限

## 3. 手动验证（最短路径）

### Step 1：确认分片与基础状态

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/stats" | jq
```

预期：

1. 返回 `code=200`
2. `data.task_queue.by_subscriber` 有分片行
3. 常见分片包含：
   - `<tenant_uuid> + _subscriber.event_fabric.replay`
   - `global + _subscriber.system.notification_dispatch`

### Step 2：触发 Replay Task

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$POWERX_BASE_URL/admin/event-fabric/debug/tasks/replay" \
  -d '{"topic":"_topic.knowledge.space.feedback.reprocess","reason":"task-readme-replay"}' | jq
```

预期：

1. 返回 `code=200`
2. 响应中有 `data.id`（task_id）

### Step 3：按 replay 分片查历史

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/messages?tenant_key=<tenant_uuid>&subscriber_id=_subscriber.event_fabric.replay&limit=30" | jq
```

预期：

1. 返回 `code=200`
2. `data.history` 能查到刚才的 replay `task_id`
3. 运行态可能很快归零，归零不代表失败

### Step 4：触发 Pipeline Task

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$POWERX_BASE_URL/admin/event-fabric/debug/tasks/pipeline" \
  -d '{"title":"Task调试","content":"from task readme","type":"system","category":"system"}' | jq
```

预期：

1. 返回 `code=200`
2. 响应中有 `data.task_id`

### Step 5：按 notification 分片查历史

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/messages?tenant_key=global&subscriber_id=_subscriber.system.notification_dispatch&limit=30" | jq
```

预期：

1. 返回 `code=200`
2. `data.history` 命中 Step 4 的 `task_id`

## 4. 状态解释（你在页面上看到的含义）

1. `queued`：任务已入队，等待消费
2. `running`：消费者正在执行
3. `completed`：执行成功
4. `failed`：执行失败，通常进入重试或人工处理
5. `cancelled`：人工取消（常见于 Replay）

## 5. 常见故障最短定位

1. `403 unauthorized`：先查 Topic ACL，再查 token 里的角色/租户
2. `topic not found`：先查 `event_topics` 是否注册
3. `stats` 全 0：先核对 `tenant_key + subscriber_id` 是否看对
4. 页面看不到但接口有：优先看 WS 是否订阅了目标 topic

## 6. 自动化脚本

1. 只读：`scripts/event_fabric/integration_playbook.sh`
2. 读写：`scripts/event_fabric/integration_playbook.sh --with-write`

## 7. 深入机制

1. `docs/guides/async_runtime/task/mechanism.md`
2. `docs/guides/async_runtime/event_fabric/naming_convention.md`
