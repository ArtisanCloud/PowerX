# EventBus and Message Fabric

> 本文档定义 PowerX Integration Framework 的**事件总线与消息织网（EventBus & Message Fabric）**规范。
> 它是连接 **Router / Orchestrator / AgentAdaptor / Gateway / Security / Metrics** 的底层通信层。
>
> 核心目标：
> **让所有事件、日志、token、状态、告警、审计、指标都在一个统一的 Fabric 上流动。**

---

## 1️⃣ 设计目标

| 目标         | 说明                                             |
| ---------- | ---------------------------------------------- |
| **统一事件骨架** | 所有模块事件使用统一 Envelope 结构                         |
| **多源聚合**   | 支持来自 Agent、Workflow、Transport、Security、Metrics |
| **多协议实现**  | 可基于 NATS / Redis Stream / Kafka 自适配            |
| **可订阅可回放** | 支持持久化、历史回放与主题订阅                                |
| **低延迟高并发** | 毫秒级事件分发、十万级吞吐能力                                |
| **可观测与安全** | 每条事件可追踪、带 Trace、租户、Scope、签名                    |

---

## 2️⃣ 架构总览

```
+-------------------------------------------------------------------+
|                        PowerX Message Fabric                      |
|-------------------------------------------------------------------|
|  Publisher Modules                                                |
|   ├── Router / Orchestrator (state events)                        |
|   ├── AgentAdaptor / Transport (token/log events)                 |
|   ├── Security / Audit (grant, auth)                              |
|   ├── Metrics / Runtime Monitor                                   |
|   └── System (alerts, ops)                                        |
|-------------------------------------------------------------------|
|  EventBus Core                                                    |
|   ├── TopicManager   (租户隔离 + 权限)                            |
|   ├── SubscriptionManager (Fan-out / QoS)                         |
|   ├── PersistenceLayer (Stream Storage)                           |
|   ├── Relay / Bridge (to Realtime Gateway)                        |
|   └── MetricsExporter (Prom / OTLP)                               |
|-------------------------------------------------------------------|
|  Consumers                                                        |
|   ├── Realtime Gateway (SSE/WS 推送)                              |
|   ├── Admin Console (Audit / Alert UI)                            |
|   ├── Plugin SDK (事件订阅)                                        |
|   ├── Analytics Pipeline (指标分析)                                |
|   └── External Webhook                                            |
+-------------------------------------------------------------------+
```

---

## 3️⃣ 事件 Envelope 规范

```json
{
  "event_id": "evt_94ac",
  "seq": 128,
  "topic": "workflow:run_8f2d",
  "type": "token|state|log|error|audit|metric|alert",
  "tenant_id": "t001",
  "actor_id": "u102",
  "trace_id": "trc_9e8a2f",
  "data": { "text": "生成中..." },
  "meta": {
    "capability": "crm.lead.fetch",
    "agent": "sales_copilot",
    "provider": "crmplus"
  },
  "timestamp": "2025-10-12T16:52:21Z",
  "signature": "sha256:8aef..."
}
```

### 字段说明

| 字段          | 描述                      |
| ----------- | ----------------------- |
| `event_id`  | 唯一事件 ID（全局有序）           |
| `seq`       | 序号，用于乱序重排               |
| `topic`     | 事件主题（见 Topic 命名规范）      |
| `type`      | 事件类别                    |
| `trace_id`  | 全链路追踪 ID                |
| `tenant_id` | 租户隔离标识                  |
| `actor_id`  | 操作人或 Agent ID           |
| `meta`      | 附加元信息（能力、插件、Provider 等） |
| `signature` | 签名校验字段                  |
| `timestamp` | ISO8601 UTC 时间戳         |

---

## 4️⃣ Topic 命名规范

| 模式                   | 示例                                 | 说明        |
| -------------------- | ---------------------------------- | --------- |
| `workflow:<run_id>`  | `workflow:run_88fa`                | 工作流运行事件   |
| `agent:<agent_id>`   | `agent:sales_copilot`              | Agent 事件流 |
| `plugin:<plugin_id>` | `plugin:com.powerx.plugin.crmplus` | 插件自定义事件   |
| `security:*`         | `security.grant_issued`            | 授权与审计事件   |
| `metrics:*`          | `metrics.agent`                    | 指标上报      |
| `system:*`           | `system.alert`                     | 系统广播/警报   |
| `tenant:<tid>:*`     | `tenant:t001:alert`                | 租户自定义主题   |

> 每个租户拥有独立命名空间与 ACL。

---

## 5️⃣ Channel 类型

| 类型                  | 特点                      | 典型实现                 |
| ------------------- | ----------------------- | -------------------- |
| **Transient (瞬态)**  | 仅实时消费，不持久化              | NATS / PubSub        |
| **Persistent (持久)** | 顺序流存储，可回放               | Redis Stream / Kafka |
| **Bridge (桥接)**     | 从 EventBus → Gateway 推送 | SSE / WS             |
| **DeadLetter (死信)** | 存放投递失败消息                | 内置 DLQ               |

---

## 6️⃣ 生产者接口（Publisher API）

```go
type EventBus interface {
    Publish(ctx context.Context, event *Event) error
    PublishBatch(ctx context.Context, events []*Event) error
    Flush(ctx context.Context) error
}
```

### 发送示例

```go
bus.Publish(ctx, &Event{
    Topic: "agent:sales_copilot",
    Type:  "token",
    Data:  map[string]any{"text": "处理中..."},
    TraceID: traceID,
})
```

* SDK 自动填充 `tenant_id`、`actor_id`、`timestamp`、`signature`。

---

## 7️⃣ 订阅者接口（Subscriber API）

```go
type Subscription struct {
    Topic     string
    Group     string
    Cursor    string
    AutoAck   bool
    Callback  func(*Event) error
}
```

### 订阅示例

```go
bus.Subscribe(Subscription{
    Topic: "workflow:run_8f2d",
    Group: "orchestrator",
    Callback: func(evt *Event) error {
        logger.InfoF(context.Background(), "%s %v", evt.Type, evt.Data)
        return nil
    },
})
```

> `Group` 表示消费者组，用于水平扩展。

---

## 8️⃣ QoS 与投递策略

| 策略                | 说明                                 |
| ----------------- | ---------------------------------- |
| **At-most-once**  | 默认；快速无重发                           |
| **At-least-once** | 需 Ack；确保送达                         |
| **Exactly-once**  | Kafka 模式；幂等 + 事务提交                 |
| **Backpressure**  | 慢消费自动降采样或丢弃低优事件                    |
| **Batch Flush**   | 聚合发送以提升吞吐                          |
| **PriorityQueue** | 按 `type` 优先级：`token > state > log` |

---

## 9️⃣ 权限与安全机制

| 安全层      | 策略                              |
| -------- | ------------------------------- |
| **认证**   | JWT 校验（tenant, actor, scope）    |
| **授权**   | 仅允许访问本租户 topic；跨租户禁止            |
| **签名验证** | 对关键事件（audit, security）做 HMAC 校验 |
| **加密传输** | 使用 TLS（NATS+TLS / HTTPS）        |
| **防篡改**  | 签名 + hash 链机制（可选）               |

---

## 🔟 事件生命周期

```
[Producer]
   │  publish()
   ▼
[EventBus Core]
   │  validate + sign
   ▼
[Broker]  (NATS / Kafka)
   │
   ├─► Persistent Store
   │
   └─► Realtime Gateway (Bridge)
          │
          ▼
       [Consumers]
        ├─ Agent Console
        ├─ Frontend SSE
        └─ Admin Dashboard
```

---

## 11️⃣ Metrics 指标

| 指标                             | 标签             | 含义      |
| ------------------------------ | -------------- | ------- |
| `eventbus_publish_total`       | `topic,type`   | 发布次数    |
| `eventbus_delivery_latency_ms` | `topic`        | 投递延迟    |
| `eventbus_dropped_total`       | `topic,reason` | 丢弃消息数   |
| `eventbus_backpressure_active` | `tenant`       | 背压激活标志  |
| `eventbus_storage_size_bytes`  | `topic`        | 持久化存储大小 |
| `eventbus_subscribers_active`  | `topic`        | 活跃订阅者数  |

---

## 12️⃣ 可观测性与追踪

* 每条事件自动注入：

  * `trace_id`
  * `span_id`
  * `tenant_id`
  * `origin_module`
* 可通过 `/api/v1/admin/trace/{trace_id}` 回溯事件路径。

**事件流示意：**

```
AgentAdaptor → EventBus(topic=agent:a1)
       │
       ▼
 Orchestrator → topic=workflow:run_8f2d
       │
       ▼
 RealtimeGateway → topic=tenant:t001:ui
```

---

## 13️⃣ 故障与恢复机制

| 场景        | 处理策略                |
| --------- | ------------------- |
| Broker 掉线 | 自动重连 + 本地缓存重放       |
| 消费者组失联    | 转交给备用组              |
| 消息重复      | 幂等判定 (event_id)     |
| 超时未 Ack   | 重投递，计数器限次           |
| 存储积压      | 触发 backpressure 降采样 |
| 无订阅者      | 延迟删除或写入 DLQ         |

---

## 14️⃣ 推荐实现（默认配置）

| 功能          | 实现方案                             |
| ----------- | -------------------------------- |
| Broker      | NATS JetStream（轻量快速）             |
| Persistence | Redis Stream（内嵌）                 |
| Bridge      | 内置 Gateway Bridge（SSE/WS）        |
| DLQ         | Redis List                       |
| Monitoring  | Prometheus + Tempo               |
| SDK         | `pkg/corex/integration/eventbus` |

---

## 15️⃣ 开发与调试接口

| Method   | Path                                         | 功能           |
| -------- | -------------------------------------------- | ------------ |
| `GET`    | `/api/v1/admin/eventbus/topics`              | 查询所有活跃 Topic |
| `GET`    | `/api/v1/admin/eventbus/stats`               | 查看事件流量统计     |
| `POST`   | `/api/v1/admin/eventbus/publish`             | 手动发布事件（测试）   |
| `GET`    | `/api/v1/admin/eventbus/subscribe?topic=xxx` | SSE 模拟订阅     |
| `DELETE` | `/api/v1/admin/eventbus/topics/{id}`         | 删除持久化 Topic  |

---

## 16️⃣ 与其他模块关系

| 模块                          | 交互说明                        |
| --------------------------- | --------------------------- |
| **Workflow / Orchestrator** | 发布 step state、token、done 事件 |
| **AgentAdaptor**            | 输出 token/log/state          |
| **Security Layer**          | 发布授权 / 审计 / 告警事件            |
| **Metrics Collector**       | 周期性发布指标快照                   |
| **Realtime Gateway**        | 从 EventBus 订阅后向前端推送         |
| **Admin API**               | 通过 `/eventbus/*` 查询状态       |
| **Plugin SDK**              | 提供轻量级发布与订阅接口                |

---

## 17️⃣ 性能指标目标

| 指标    | 目标值             |
| ----- | --------------- |
| 单节点吞吐 | ≥ 100,000 msg/s |
| 平均延迟  | ≤ 50 ms         |
| 最大订阅者 | ≥ 10,000        |
| 可靠性   | ≥ 99.99% 投递成功率  |
| 恢复时间  | ≤ 5s 重连恢复       |

---

## ✅ 一句话总结

> **EventBus & Message Fabric = PowerX 的实时神经网络。**
> 所有智能体、插件、工作流、治理与安全事件都通过统一总线流动，
> 形成一张低延迟、可追踪、可治理、可扩展的“事件织网”，
> 支撑 PowerX 的多智能体协作与企业级可观测体系。
