# Realtime Streaming Gateway Specification (CoreX/integration)

> 本文档定义 **PowerX CoreX/integration 域** 的
> **Realtime Streaming Gateway（实时流式网关）**，
> 它是 PowerX 的统一流事件出口层，负责将来自 Agent、Workflow、Adaptor 的实时事件推送至前端、插件页面、或第三方客户端。
>
> 它构成 **「运行时事件可视化与AI响应流」的统一出入口**。

---

## 1️⃣ 设计目标

| 目标         | 描述                                             |
| ---------- | ---------------------------------------------- |
| **统一流式通道** | 汇聚所有运行态事件（AI token、Agent log、Workflow 状态、插件事件） |
| **多协议支持**  | SSE / WebSocket / （预留 GRPC-Stream）             |
| **多源聚合**   | 聚合 MCP / gRPC / Agent Stream / EventBus 消息     |
| **低延迟传递**  | 毫秒级流转 + 优先级调度                                  |
| **安全隔离**   | 多租户 + Scope + Topic ACL 控制                     |
| **全链观测**   | trace_id 跨 Workflow/Agent/Gateway 贯穿           |

---

## 2️⃣ 系统架构总览

```
+---------------------------------------------------------------+
|                         PowerX Runtime                        |
|---------------------------------------------------------------|
|    Orchestrator / Workflow / Agent / Adaptor (producers)      |
|               │                                                |
|               ▼                                                |
|           EventBus (CoreX Integration Bus)                     |
|               │                                                |
|               ▼                                                |
|        Realtime Streaming Gateway (出口聚合层)                |
|          ├─ SSE Endpoint (/realtime/sse)                       |
|          ├─ WebSocket Endpoint (/realtime/ws)                  |
|          └─ Poll Fallback (/realtime/poll)                     |
|               │                                                |
|               ▼                                                |
|         Frontend / Plugin Page / External Client               |
+---------------------------------------------------------------+
```

---

## 3️⃣ 核心组件职责

| 模块                               | 职责                                    |
| -------------------------------- | ------------------------------------- |
| **EventBus (Integration Layer)** | 内部事件总线（NATS/Redis/Kafka）；所有流式事件写入此处。  |
| **Realtime Gateway**             | SSE/WS 推送层；对外暴露统一 `/realtime/...` 通道。 |
| **Stream Transformer**           | 将内部事件封装为标准 SSE/WS 格式。                 |
| **Connection Manager**           | 管理客户端连接、Topic 订阅、心跳与限流。               |
| **Auth Filter**                  | 鉴权与租户隔离，校验 JWT 与 Topic ACL。           |
| **Metrics & Trace**              | 推送延迟、丢包率、订阅数等指标采集。                    |

---

## 4️⃣ 数据流路径示意

```
Adaptor / Agent
    │
    ▼
EventBus.Publish("ai:run_x123", {...})
    │
    ▼
Realtime Gateway (订阅 topic)
    │
    ▼
SSE/WS 推送 → Browser / Plugin 前端
```

### 示例：Agent Stream Token 推送

```
event: token
data: {"seq":42,"text":"生成中...","trace_id":"trc_a7b2f"}
```

---

## 5️⃣ 通信协议与端点

| 协议                | Path                          | 特点           |
| ----------------- | ----------------------------- | ------------ |
| **SSE**           | `/realtime/sse?topic=<topic>` | 单向推送，轻量低延迟   |
| **WebSocket**     | `/realtime/ws`                | 双向通信，可订阅多个主题 |
| **Poll Fallback** | `/realtime/poll`              | 长轮询降级模式      |

Header 认证：

```
Authorization: Bearer <jwt>
X-Tenant-ID: t001
X-Actor-ID: u102
```

---

## 6️⃣ Topic 命名空间

| 类型                | 示例                     | 说明         |
| ----------------- | ---------------------- | ---------- |
| **AI/Agent**      | `agent:run_<uuid>`     | Agent 实时输出 |
| **Workflow**      | `wf:<flow_id>`         | 编排任务进度与状态  |
| **Plugin Event**  | `plugin:<id>:event`    | 插件定义事件     |
| **System**        | `sys:notice`           | 系统广播       |
| **Tenant Custom** | `tenant:<tid>:<topic>` | 租户自定义通道    |

> Topic ACL 与 JWT scope 精确绑定。

---

## 7️⃣ 事件结构规范

### 7.1 通用事件

```json
{
  "seq": 15,
  "type": "token|log|state|error|done",
  "topic": "agent:run_556",
  "data": { "text": "处理中...", "step": "summarize" },
  "trace_id": "trc_ef2b",
  "tenant_id": "t001",
  "timestamp": "2025-10-12T10:21:00Z"
}
```

### 7.2 SSE 封装

```
event: token
data: {"seq":15,"text":"处理中...","trace_id":"trc_ef2b"}
```

### 7.3 WS 帧格式

```json
{
  "action": "publish",
  "topic": "agent:run_556",
  "payload": {...}
}
```

---

## 8️⃣ QoS 与流控制

| 策略               | 内容                                   |
| ---------------- | ------------------------------------ |
| **At-most-once** | 默认传输语义（低延迟优先）。                       |
| **Buffer 模式**    | 可缓存最近 N=50 条事件以支持断线恢复。               |
| **Backpressure** | 当消费过慢时丢弃低优先级消息（log < state < token）。 |
| **Priority 调度**  | token > status > log。                |
| **Timeout**      | SSE/WS 空闲连接超时 15min。                 |

---

## 9️⃣ EventBus 集成模型

内部 EventBus 支持 **NATS JetStream / Redis Stream / Kafka** 等实现。

```go
sub := eventbus.Subscribe(topic)
for msg := range sub.Chan() {
    gateway.Publish(topic, msg)
}
```

* 所有事件带 `seq` → 保序；
* 同 Topic 多源写入时由 `trace_id` 聚合；
* `status` 事件按周期聚合（减少噪音）。

---

## 🔟 多协议源事件映射

| 来源                   | 转换为内部事件类型            | 说明                |
| -------------------- | -------------------- | ----------------- |
| **MCP Stream**       | `token/log/state`    | 直接映射。             |
| **gRPC Stream**      | 拆分 Recv() → EventBus | 消息分片。             |
| **Agent A2A Stream** | `agent.output`       | Agent Channel 映射。 |
| **HTTP SSE**         | 转换后写入 EventBus。      |                   |
| **Local Function**   | Channel→EventBus。    |                   |

Realtime Gateway 是**聚合出口层**，不关心来源，只做统一封装。

---

## 11️⃣ 安全与隔离策略

| 层级       | 策略                                   |
| -------- | ------------------------------------ |
| **认证**   | JWT 验证（tenant_id, actor_id, scopes）。 |
| **授权**   | Topic ACL（scope-topic 映射）。           |
| **租户隔离** | 每租户独立命名空间 `tenant:<tid>:`。           |
| **限流**   | 每用户最大连接数 & 每 topic 每秒事件速率。           |
| **加密**   | TLS + WSS 强制。                        |
| **签名校验** | 可选：系统级事件签名验证。                        |

---

## 12️⃣ Metrics 与 Observability

| 指标                                 | 说明          |
| ---------------------------------- | ----------- |
| `realtime_connections_active`      | 当前活跃连接数     |
| `realtime_publish_rate`            | 每秒事件发布数     |
| `realtime_delivery_latency_ms`     | 推送延迟        |
| `realtime_dropped_messages_total`  | 丢包数         |
| `realtime_topic_subscribers`       | 每 topic 订阅数 |
| `realtime_buffer_recoveries_total` | 断线重连恢复次数    |

**Trace 链路：**

```
Workflow/Agent → EventBus → Gateway → Client
(trace_id 贯穿全链)
```

---

## 13️⃣ 故障恢复与降级

| 场景          | 行为                             |
| ----------- | ------------------------------ |
| 客户端断线       | 自动重连，恢复最近 N 条缓存。               |
| Gateway 重启  | 从 EventBus 恢复状态。               |
| EventBus 异常 | 内存缓存重试 3 次。                    |
| 网络不稳定       | 自动降级 `/poll` 模式。               |
| 多节点部署       | Gateway 通过 Redis/NATS 分布式广播同步。 |

---

## 14️⃣ 插件与 Gateway 交互模式

### 插件作为事件源

```go
eventbus.Publish("plugin:crm:event", data)
```

### 插件前端作为消费者

```
GET /_p/com.powerx.crmplus/realtime/sse?topic=plugin:crm:event
```

* PluginManager 反代 Gateway；
* JWT 自动注入；
* 可展示 AI 输出、日志、工作流进度等。

> 流程：**Plugin Runtime → EventBus → Gateway → Plugin Frontend**

---

## 15️⃣ 性能目标

| 指标       | 目标值            |
| -------- | -------------- |
| 平均推送延迟   | ≤ 100 ms       |
| 并发连接数/节点 | ≥ 10,000       |
| 吞吐量      | ≥ 50,000 msg/s |
| 丢包率      | ≤ 0.01%        |
| 恢复率      | 100%（断线自动恢复）   |

---

## 16️⃣ 管理与监控 API

| 方法       | 路径                                              | 说明         |
| -------- | ----------------------------------------------- | ---------- |
| `GET`    | `/api/v1/integration/realtime/topics`           | 查询活跃 topic |
| `GET`    | `/api/v1/integration/realtime/connections`      | 当前连接       |
| `POST`   | `/api/v1/integration/realtime/broadcast`        | 系统广播       |
| `DELETE` | `/api/v1/integration/realtime/connections/{id}` | 断开连接       |

---

## 17️⃣ 技术选型建议（Go 实现）

| 模块         | 推荐实现                           |
| ---------- | ------------------------------ |
| EventBus   | NATS JetStream（或 Redis Stream） |
| SSE        | `net/http` + FlushWriter       |
| WS         | `gorilla/websocket`            |
| JWT        | `github.com/golang-jwt/jwt/v5` |
| Metrics    | Prometheus + OTEL              |
| State Sync | Redis pub/sub 或 NATS 分布式事件     |

---

## 18️⃣ 与其他模块关系

| 模块                      | 交互                     |
| ----------------------- | ---------------------- |
| **Transport Adapter**   | 写入流式事件至 EventBus       |
| **Workflow / Agent**    | 产生 AI/任务事件             |
| **RuntimeResolver**     | 提供租户上下文与 trace_id      |
| **Router**              | 提供调用上下文（trace/tenant）  |
| **Observability Layer** | 采集推送与丢包指标              |
| **PluginManager**       | 提供反代入口 `/realtime/...` |

---

## ✅ 一句话总结

> **Realtime Streaming Gateway = PowerX 的运行时事件出口与前端实时桥。**
> 它聚合所有流式事件、统一封装、鉴权推送，
> 支撑 AI Token、Workflow 状态、Agent Log 的毫秒级实时可视化，
> 是 PowerX Integration Framework 的「流式中枢与可观测出口」。
