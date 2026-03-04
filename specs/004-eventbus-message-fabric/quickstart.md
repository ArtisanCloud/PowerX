# Quickstart: EventBus & Message Fabric（统一口径）

> 目标：基于“Topic 注册制 + JWT 租户鉴别 + 统一 TaskDriver”完成最小联调。

## 0. 前置

```bash
export POWERX_BASE_URL="http://127.0.0.1:8077/api/v1"
export ADMIN_TOKEN="<your-admin-jwt>"
```

- 请求中不要传任何租户 header（租户由 JWT claims 解析）
- topic 使用语义 key：`_topic.<domain>.<name>`

## 1) 查看 Topic（确认已注册）

```bash
curl -sS \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/topics?page=1&page_size=50" | jq
```

## 2) 发布事件（语义 topic key）

```bash
PAYLOAD_B64=$(printf '{"hello":"world"}' | base64)

curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$POWERX_BASE_URL/event-fabric/events:publish" \
  -d '{
    "topic":"_topic.knowledge.space.feedback.reprocess",
    "event_id":"demo-event-001",
    "version":"v1",
    "payload":"'"$PAYLOAD_B64"'",
    "payload_format":"json",
    "trace_id":"demo-trace-001",
    "attributes":{"source":"quickstart"}
  }'
```

## 3) 创建 replay 任务（debug 入口）

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$POWERX_BASE_URL/admin/event-fabric/replay/tasks" \
  -d '{
    "topic":"_topic.knowledge.space.feedback.reprocess",
    "reason":"quickstart replay",
    "operator_id":"quickstart"
  }' | jq
```

记录返回 `id` 为 `TASK_ID`，查询状态：

```bash
curl -sS \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/replay/tasks/$TASK_ID" | jq
```

## 4) 查看 DLQ（可选）

```bash
curl -sS \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/dlq/messages?page=1&page_size=20" | jq
```

## 5) Cron 任务调试（运维接口）

> 不传租户 header，租户由 JWT 自动解析。

### 5.1 查看任务

```bash
curl -sS \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs" | jq
```

### 5.2 立即执行一个任务

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs/event_fabric.retry_dispatch/run-now" | jq
```

### 5.3 观察队列联动

```bash
curl -sS \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/stats" | jq
```

### 5.4 暂停/恢复（可选）

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs/event_fabric.retry_dispatch/pause" | jq

curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs/event_fabric.retry_dispatch/resume" | jq
```

## 6) WS Bus（内部接口）

> WS 内部注册接口同样使用语义 topic key（`_topic.*`），不带 tenant 前缀；租户从 JWT 解析。
> register 主路径会执行 `event_topics` 注册校验 + ACL 绑定，并返回 `mode=registry_acl`。

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "http://127.0.0.1:8077/api/v1/internal/ws-bus/grant" \
  -d '{"topics":["_topic.knowledge.space.feedback.reprocess"],"actions":["publish","subscribe"]}' | jq
```

如果返回 `mode=compat_dynamic`，表示走了兼容降级（非目标态），应检查依赖注入与环境配置。

## 7) 预期结果

1. `topic` 只传语义 key，接口可正常执行。
2. 若 topic 未注册，返回 404 业务错误（非 500）。
3. replay 状态可观察为 `pending/running/completed/failed`。
4. Cron `run-now` 后，`/admin/event-fabric/task-queue/stats` 可观察到队列状态变化。
5. 联调页面可看到 Queue/DLQ/Replay 联动变化。
6. `ws-bus/grant` 返回 `mode=registry_acl`（而不是 `compat_dynamic`）。

## 8) 系统设置中的 ACL 治理（Phase 13）

> 目标口径：Topic 采用“全局注册 + ACL 映射”，新租户无需逐个创建同名 Topic。

### 8.1 页面入口

- 进入：`系统设置 -> 事件权限（Event ACL）`
- 监控中心联动：在 `监控中心 -> Event Fabric -> Topic` 行点击“管理权限”跳转。

### 8.2 治理动作（目标）

- 选择 Topic（优先语义 key：`namespace.name`，可见共享 Topic）。
- 选择角色/主体（如 `role_admin`、`member:*`）。
- 授予动作：`publish` / `subscribe` / `replay`。

### 8.3 验收口径

1. 新租户无需手工创建 `knowledge.space.feedback.reprocess` 也可完成授权。
2. 授权前 replay/subscribe 返回 4xx；授权后成功。
3. 授权变更在审计中可追踪（operator/topic/action/principal）。
