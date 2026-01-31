# Event Bus（统一事件总线）

## 目标
- 让 PowerX 内部模块、插件、插件之间用同一套事件机制进行发布/订阅。
- 统一投递语义、重试、延迟、审计、观测与权限控制。

## 与 Scheduler 的关系
- Scheduler 负责“到点触发”，Event Bus 负责“分发投递”。
- Scheduler 产出事件 → Event Bus 投递给订阅者。
- Event Bus 的延迟重试依赖 Scheduler 的底层调度能力（延迟队列/定时扫描）。

## 统一概念
- **Event**：业务事件（topic + payload）。
- **Envelope**：事件包裹（event_id、tenant、trace、created_at）。
- **Subscriber**：订阅者（服务/插件），按 topic 消费。
- **Delivery**：投递记录，带状态（ack/nack/timeout/scheduled）。

## 事件协议（草案）
```
topic: string
payload: object
meta:
  event_id: string
  tenant_uuid: string
  trace_id: string
  created_at: timestamp
  source: string        # 服务名/插件名
  version: string       # 事件版本
```

## API 草案（HTTP）
```
POST /api/v1/admin/event-bus/publish
  body:
    topic: string
    payload: object
    meta:
      source: string
      version: string

POST /api/v1/admin/event-bus/subscribe
  body:
    subscriber_id: string
    topics: string[]
    endpoint: string         # http(s) 回调 or plugin handler
    scopes: string[]

POST /api/v1/admin/event-bus/ack
  body:
    event_id: string
    subscriber_id: string

POST /api/v1/admin/event-bus/nack
  body:
    event_id: string
    subscriber_id: string
    reason: string

GET /api/v1/admin/event-bus/subscriptions
  query:
    tenant_uuid: string
    topic: string (optional)
    page: number
    page_size: number
```

## OpenAPI Schema（草案）
```yaml
paths:
  /api/v1/admin/event-bus/publish:
    post:
      summary: Publish event
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                topic: { type: string }
                payload: { type: object }
                meta:
                  type: object
                  properties:
                    source: { type: string }
                    version: { type: string }
              required: [topic, payload]
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/OkResponse"
        "401":
          description: unauthorized
        "403":
          description: forbidden
        "500":
          description: internal_error
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
  schemas:
    ErrorResponse:
      type: object
      properties:
        code: { type: string }
        message: { type: string }
        detail: { type: string }
      required: [code, message]
    OkResponse:
      type: object
      properties:
        ok: { type: boolean }
        data: { type: object }
      required: [ok]
```

## API 草案（gRPC）
```
service EventBusService {
  rpc Publish(EventPublishRequest) returns (EventPublishResponse);
  rpc Subscribe(EventSubscribeRequest) returns (EventSubscribeResponse);
  rpc Ack(EventAckRequest) returns (EventAckResponse);
  rpc Nack(EventNackRequest) returns (EventNackResponse);
}
```

## Proto Message（草案）
```
message EventPublishRequest {
  string topic = 1;
  bytes payload_json = 2;
  string source = 3;
  string version = 4;
  string tenant_uuid = 5;
}
message EventPublishResponse {
  string event_id = 1;
}
message EventSubscribeRequest {
  string subscriber_id = 1;
  repeated string topics = 2;
  string endpoint = 3;
  repeated string scopes = 4;
}
message EventSubscribeResponse {
  bool ok = 1;
}
message EventAckRequest {
  string event_id = 1;
  string subscriber_id = 2;
}
message EventAckResponse {
  bool ok = 1;
}
message EventNackRequest {
  string event_id = 1;
  string subscriber_id = 2;
  string reason = 3;
}
message EventNackResponse {
  bool ok = 1;
}
```

## SDK 示例（伪代码）
```
bus := eventbus.NewClient(baseURL, token)
bus.Publish(ctx, "scheduler.job.triggered", payload, Meta{
  Source: "plugin.my-plugin",
  Version: "v1",
})
```

## 字段校验（建议）
- `topic`：必须符合 `^[a-z0-9_.-]+$`，建议带版本（`.v1`）。
- `payload`：必须是 JSON object，禁止超大字段（默认上限 256 KB）。
- `meta.source`：必填，格式 `plugin.<id>` 或 `core.<service>`。
- `tenant_uuid`：必须存在或可从 token 推断。

## 幂等与去重
- 发布端可带 `Idempotency-Key` 头，避免重复发布。
- 订阅端必须幂等处理（建议使用 `event_id` 去重）。

## 限流与负载
- 默认限流：按租户 + topic 维度限速。
- 超限错误：`event_bus.rate_limited`（HTTP 429）。

## 错误码与 HTTP 状态（建议）
- 400：`event_bus.invalid_topic` / `event_bus.invalid_payload`
- 401：`event_bus.not_authorized`
- 403：`event_bus.subscriber_denied`
- 429：`event_bus.rate_limited`
- 500：`event_bus.publish_failed`

## 示例（HTTP）
```
POST /api/v1/admin/event-bus/publish
Authorization: Bearer <TOKEN>
x-tenant-uuid: <TENANT_UUID>
Idempotency-Key: 4d2a...

{
  "topic": "scheduler.job.triggered",
  "payload": { "job_id": "..." },
  "meta": { "source": "core.scheduler", "version": "v1" }
}
```

## 认证与租户头部
- `Authorization: Bearer <TOKEN>`
- `x-tenant-uuid: <TENANT_UUID>`（可选，优先于 token 中租户）

## 错误码（建议）
- `event_bus.not_authorized`
- `event_bus.invalid_topic`
- `event_bus.subscriber_denied`
- `event_bus.publish_failed`

## Plugin Manifest 示例
```
event_bus:
  subscriptions:
    - topic: scheduler.job.triggered
      handler: /plugin/events/handle
  scopes:
    - event.subscribe
```

## PowerXPlugin 对接
- 插件侧通过统一 `EventBridge` 出口接入 PowerX Event Bus。
- 模式建议：
  - `local`：仅本地实现（不依赖底座）
  - `taskbus`：调用 PowerX Event Bus（HTTP/gRPC/SDK）
  - `dual`：双写/双读，便于灰度与回滚
- 兜底策略：当底座不可用时自动降级到本地实现。
- 权限：插件 Manifest 声明 publish/subscribe，底座按 scope 校验。

## 投递语义
- 至少一次（At-least-once）。
- 订阅者需实现幂等（建议提供 idempotency_key）。
- 失败可重试（指数回退 + 抖动），超过阈值进入 DLQ。

## 订阅模型
- **核心模块**：在启动时注册订阅（配置/代码）。
- **插件**：通过插件 Manifest 或注册 API 声明订阅的 topic。
- **权限**：订阅需校验租户 + tool scope + capability。

## 事件生命周期
1) Producer 发布事件（HTTP/gRPC/内部调用）。
2) Event Bus 存储 envelope + delivery 记录。
3) Scheduler/Worker 拉取待投递事件并执行分发。
4) Subscriber 处理并 ack / nack。
5) 失败 → 重试/延迟 → DLQ。

## 可观测
- 指标：event.publish_total、delivery.success_total、delivery.retry_total、delivery.dlq_total、delivery.latency_p95。
- 日志：event_id、tenant_uuid、topic、subscriber_id、trace_id。
- 追踪：通过 trace_id 串联 producer → bus → subscriber。

## 插件接入（建议）
- 插件声明：
  - subscriptions: ["tenant.xxx.topic", "core.xxx.topic"]
  - scopes: ["event.subscribe"]
- 插件消费回调：`/plugin/events/handle` 或内部 gRPC handler。

## 最小落地实现建议
- 存储：`event_envelopes` + `event_delivery_attempts`（现有表复用）。
- 调度：复用 event_fabric 的 backoff scheduler（Redis ZSET）。
- 锁：基于 Redis lease（tenant + subscriber 维度），避免重复投递。
- 清理：DLQ + 过期策略，支持手动重放。

## 接口与队列策略
- 接口优先级：HTTP 为第一优先；gRPC 与 SDK 作为后续扩展。
- 队列默认：Redis（延迟/重试、租户隔离、轻量落地）。
- 可替换：通过 Provider 接口切换 Kafka/NATS/SQS 等。

## 下一步
- 明确统一的 Event Bus API（publish/subscribe/ack/nack）。
- 定义 DLQ 的管理界面与重放流程。
