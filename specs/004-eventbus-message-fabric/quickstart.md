# Quickstart: EventBus & Message Fabric

> 目标：在本地验证主题目录、ACL 校验、事件发布/订阅、重试与 DLQ、回放流程。示例基于 `make dev` 启动的 CoreX Admin/API 服务与 Redis、本地 Postgres。

## 0. 依赖准备
- 本地 Postgres、Redis（或使用 `docker compose -f deploy/dev/docker-compose.yml up redis postgres`）
- 终端环境变量：
  ```bash
  export DEV_TENANT=tenant-demo
  export DEV_PRINCIPAL=svc-demo-wf
  ```
- 启动应用：
  ```bash
  make dev
  ```

## 1. 创建主题
```bash
curl -X POST http://localhost:8077/admin/event-fabric/topics \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_uuid": "'"$DEV_TENANT"'",
    "namespace": "corex.workflow",
    "name": "approved",
    "payload_format": "json",
    "versioning_mode": "backward",
    "max_retry": 5,
    "retention_policy": "{\"type\":\"time\",\"value\":\"7d\"}"
  }'
```
- 预期：返回 Topic UUID，`lifecycle=active`。

## 2. 授权发布/订阅主体
```bash
curl -X POST http://localhost:8077/admin/event-fabric/acl \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_uuid": "'"$DEV_TENANT"'",
    "topic_full_name": "'"$DEV_TENANT"'.corex.workflow.approved",
    "grants": [
      {"principal_type":"service","principal_id":"'"$DEV_PRINCIPAL"'","action":"publish"},
      {"principal_type":"service","principal_id":"svc-demo-consumer","action":"subscribe"}
    ]
  }'
```
- 预期：返回新增授权条目，审计日志含批准人。

## 3. 订阅端启动（gRPC 流）
使用提供的 CLI（后续实现 `cmd/eventbus/demo_subscriber`）：
```bash
bin/eventbus-subscriber \
  --tenant "$DEV_TENANT" \
  --topic "$DEV_TENANT.corex.workflow.approved" \
  --subscriber svc-demo-consumer \
  --compatibility backward \
  --accepted-version v2
```
- 预期：客户端建立 gRPC 流，声明“向后兼容”策略并等待消息，心跳保持。

## 4. 发布事件
```bash
PAYLOAD=$(printf '{"requestId":"req-123","status":"approved"}' | base64)
curl -X POST http://localhost:8077/admin/event-fabric/events:publish \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_uuid": "'"$DEV_TENANT"'",
    "topic": "'"$DEV_TENANT"'.corex.workflow.approved",
    "event_id": "evt-001",
    "trace_id": "trace-abc",
    "version": "v2",
    "payload": "'"$PAYLOAD"'",
    "payload_format": "json",
    "attributes": {"principal_id": "'"$DEV_PRINCIPAL"'"}
  }'
```
- 预期：订阅 CLI 在 100ms 内收到事件，`headers.version_mode=backward`，并返回 Ack；Admin API 返回 `202 ACCEPTED`。

## 5. 模拟消费失败并观察重试
在订阅 CLI 中触发 `--fail-once` 参数或发送 Nack。检查 Redis 延迟队列和日志：
```bash
redis-cli zrangebyscore event:retry:$DEV_TENANT -inf +inf WITHSCORES
```
- 预期：事件重新排队，按照指数退避（500ms, 1s, 2s ...）重投，最多 5 次。

## 6. 验证 DLQ
在订阅端禁用 Ack，等待重试超出次数。查询 DLQ：
```bash
curl "http://localhost:8077/admin/event-fabric/dlq/messages?tenant_uuid=$DEV_TENANT&status=queued"
```
- 预期：返回 `evt-001` 记录，含失败原因。
 - 可通过 REST 重放：
```bash
curl -X POST http://localhost:8077/admin/event-fabric/dlq/messages:replay \
  -H 'Content-Type: application/json' \
  -d '{"operator":"ops-demo"}'
```

## 7. 回放历史事件
```bash
curl -X POST http://localhost:8077/admin/event-fabric/replay/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_uuid": "'"$DEV_TENANT"'",
    "topic": "'"$DEV_TENANT"'.corex.workflow.approved",
    "trace_id": "trace-abc",
    "shadow": true,
    "reason": "incident backfill",
    "window": {
      "start": "2025-10-17T00:00:00Z",
      "end": "2025-10-17T23:59:59Z"
    }
  }'
```
- 预期：响应返回 `status=completed`，`data.topic` 指向目标主题。
- 订阅端会收到带有 `headers.replay=true`、`headers.shadow=true` 的事件。
- 可通过 `GET /admin/event-fabric/replay/tasks/<task_id>` 查询状态，`POST ...:cancel` 取消未完成任务。

## 8. 观测指标
- 访问 Prometheus/Grafana 仪表板（待实现 `event_fabric_*` 指标）。
- 检查 `event_fabric_delivery_success_ratio`, `event_fabric_retry_latency_ms`, `event_fabric_dlq_size`。

## 清理
```bash
curl -X DELETE http://localhost:8077/admin/event-fabric/topics/$TOPIC_ID?tenant_uuid=$DEV_TENANT
redis-cli del event:retry:$DEV_TENANT
```
- 确保删除测试数据，避免影响后续演示。
