# Transport Adapter Specification

> 本规范定义 **PowerX Integration Framework** 的传输适配层（Transport Adapter）。
> 它描述 PowerX 如何在运行时根据 **能力上下文 (Capability Context)**、**协议策略**、**会话状态** 与 **QoS 指标**，
> 在 **MCP / gRPC / HTTP / Agent(A2A)** 四种协议之间，动态选择最优传输通道并执行调用。

---

## 1️⃣ 设计目标

| 目标        | 描述                                        |
| --------- | ----------------------------------------- |
| **统一抽象**  | 屏蔽协议差异，提供统一的调用接口与上下文模型。                   |
| **多协议共存** | 同一能力可拥有多种传输通道（mcp / grpc / http / agent）。 |
| **动态决策**  | Router 根据延迟、健康、会话状态、成本打分，自动选择最优通道。        |
| **可流式化**  | 所有通道均支持 token/日志/状态流式推送。                  |
| **可扩展性**  | 新增协议无需修改核心逻辑，只需注册新的 Adapter。              |

---

## 2️⃣ 层级关系与职责

```
+----------------------------------------------------------+
|                   CoreX / integration                    |
|----------------------------------------------------------|
|   Router → Orchestrator → Transport → Flow Execution     |
+----------------------------------------------------------+
|                   Transport Adapter Layer                |
|  ┌────────────┬──────────────┬──────────────┬──────────┐ |
|  | MCP        | gRPC         | HTTP         | Agent(A2A)| |
|  | Adapter    | Adapter      | Adapter      | Adapter   | |
|  └────────────┴──────────────┴──────────────┴──────────┘ |
+----------------------------------------------------------+
|              Plugin / External Provider Runtime          |
+----------------------------------------------------------+
```

---

## 3️⃣ 统一接口定义

```go
type TransportAdapter interface {
    Invoke(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
    Stream(ctx context.Context, req *TransportRequest, sink chan<- *StreamChunk) error
    HealthCheck(ctx context.Context) (*HealthStatus, error)
    Close() error
}
```

### 通用请求结构

```go
type TransportRequest struct {
    CapabilityID string
    Input        map[string]any
    Metadata     map[string]string // tenant, trace_id, actor_id, etc.
    Endpoint     *EndpointRecord   // Router 已选定的运行态端点
    Timeout      time.Duration
    Stream       bool
}
```

### 通用响应结构

```go
type TransportResponse struct {
    Output   map[string]any
    Status   string      // success|error|timeout|cancelled
    Duration time.Duration
    TraceID  string
}
```

---

## 4️⃣ 调度流程总览

```
Router 选择最优 Endpoint (transport = mcp/grpc/http/agent)
        │
        ▼
TransportFactory.NewAdapter(endpoint.transport)
        │
        ▼
adapter.Invoke(ctx, request)
        │
        ▼
底层执行协议请求并返回统一响应
```

---

## 5️⃣ 各协议适配规范

### 5.1 MCP Adapter（首选反向通道）

> **核心场景**：插件主动连接 PowerX 的持久化控制通道。
> 支持流式调用、心跳、工具注册、状态上报。

#### 调用模型

* 长连接（WebSocket / MCP-RPC）双向流。
* 插件以 Tool 格式注册，PowerX 以 ToolCall 格式调用。
* 支持 streaming：按 token / step / event 分片返回。

#### 实现要点

* SessionManager 管理会话池；
* 每 15s keepalive；
* 断线重连指数退避；
* 仅认证通过的会话可被 Router 调用；
* 流式结果封装为 `StreamChunk` 传回 Orchestrator。

---

### 5.2 gRPC Adapter（内部高速通道）

> **核心场景**：内部插件或服务通过 gRPC 公开接口。
> 提供高吞吐、低延迟的强类型通信。

#### 调用模型

```yaml
transport: grpc
uri: grpc://127.0.0.1:50051
service: crm.lead.Service
method: Create
```

```go
client := pb.NewCRMLeadServiceClient(conn)
resp, err := client.Create(ctx, &pb.CreateRequest{...})
```

#### 实现要点

* Registry 维护连接池；
* Trace / Tenant / Actor 注入；
* 幂等操作自动重试；
* 流式结果转化为标准 `StreamChunk`；
* 超时由请求上下文控制。

---

### 5.3 HTTP Adapter（外部兼容层）

> **核心场景**：对接第三方 REST API 或无 SDK 的外部 SaaS。

#### 调用模型

```yaml
transport: http
uri: https://api.crmplus.com/api/crm/lead/create
```

#### 实现要点

* 支持 GET / POST；
* JSON body `{input, meta}`；
* Token / OAuth2 / HMAC；
* 自动限速与断路保护；
* Content-Type = `text/event-stream` 时进入流模式；
* 响应解析为统一结构体。

---

### 5.4 Agent (A2A) Adapter（智能体间通信）

> **核心场景**：PowerX Agent 调用其他 Agent / 子 Agent / 外部智能体。
> A2A 是 PowerX 的第四类传输协议，核心于智能体协作。

#### 调用模型

```yaml
transport: agent
channel: agent://com.powerx.plugin.analytics/session-4fa2
```

#### 特点

* 通过 Agent Manager 的 Channel Broker 进行消息分发；
* 支持同步 (sync) 与异步 (async) 模式；
* 兼容多租户上下文、trace_id 与 intent_id；
* 对接时可桥接远程 MCP Agent；
* 统一在 Router 的选路结果中视为一种 Endpoint。

#### 实现要点

* AgentAdapter 内部使用 ChannelRegistry；
* 建立 A2A Session；
* 复用 Message Bus (NATS/Kafka) 或内存通道；
* 流式响应可用于协同生成、多轮计划执行。

---

## 6️⃣ Transport Factory 注册机制

```go
var TransportFactories = map[string]TransportFactory{
    "mcp":   NewMCPAdapter,
    "grpc":  NewGRPCAdapter,
    "http":  NewHTTPAdapter,
    "agent": NewAgentAdapter,
}

func RegisterAdapter(name string, factory TransportFactory)
```

> ⚙️ 新协议只需实现 `TransportAdapter` 接口并注册即可。
> 不影响 Router 或 Registry 层逻辑。

---

## 7️⃣ 调用生命周期

| 阶段          | 描述                              |
| ----------- | ------------------------------- |
| **Prepare** | 构造请求上下文（trace / tenant / actor） |
| **Resolve** | Router 选择最优 Endpoint            |
| **Connect** | Adapter 建立会话或连接                 |
| **Invoke**  | 执行协议请求（同步或流式）                   |
| **Stream**  | 持续推送结果或事件                       |
| **Observe** | 记录 Metrics / Trace / Audit      |
| **Close**   | 回收资源或关闭通道                       |

---

## 8️⃣ 流式传输规范

### 通用事件结构

```json
{
  "seq": 42,
  "type": "token|log|state|error|done",
  "data": { "text": "生成中..." },
  "trace_id": "abc123"
}
```

### 生命周期

1. Adapter 建立流；
2. 每个事件转为 `StreamChunk`；
3. 发送至 Orchestrator EventBus；
4. 转发至 Workflow 或前端 SSE；
5. 发送 `done` 事件收尾。

---

## 9️⃣ 超时与重试策略

| 场景                 | 行为                     |
| ------------------ | ---------------------- |
| 网络抖动               | 线性重试 3 次               |
| gRPC `Unavailable` | 指数退避（200ms→3s）         |
| MCP Session 中断     | 等待重连 ≤10s              |
| Agent 通道超时         | 返回 “agent_unavailable” |
| Timeout            | 中断并上报 metrics/audit    |

> 非幂等请求不得自动重试（例如 `create`、`delete`）。

---

## 🔟 安全与上下文注入

| 字段          | 描述                   |
| ----------- | -------------------- |
| `tenant_id` | 当前租户                 |
| `actor_id`  | 当前操作用户               |
| `trace_id`  | 调用链追踪 ID             |
| `intent_id` | 当前 Agent 意图 ID       |
| `token`     | 调用凭证（HTTP/MCP/Agent） |
| `scope`     | 能力访问范围               |

所有 Adapter 必须在调用头中附带这些上下文信息。
A2A 与 MCP 共享安全验证机制。

---

## 11️⃣ Observability（可观测性）

* **Metrics**

  * `transport_latency_seconds{transport,provider}`
  * `transport_errors_total{transport}`
  * `active_sessions{transport}`
* **Tracing**

  * Span 名称：`PowerX.Transport.{transport}`
* **Audit**

  * 记录调用摘要、结果、trace_id、租户、actor。

---

## 12️⃣ 未来方向

| 方向                    | 描述                     |
| --------------------- | ---------------------- |
| **MQ Adapter**        | 支持异步消息总线（Kafka / NATS） |
| **GraphQL Adapter**   | 对接 GraphQL 服务          |
| **WebSocket Adapter** | 实时双向流通道                |
| **Function Adapter**  | Serverless 函数调用        |
| **Hybrid A2A**        | 多智能体协同编排（内核与插件混合执行）    |

---

## 13️⃣ 一图总结

```
Router
  │
  ▼
TransportFactory
  │
  ├── MCPAdapter   → Plugin Session
  ├── GRPCAdapter  → Internal Service
  ├── HTTPAdapter  → External REST
  └── AgentAdapter → Agent Channel (A2A)
```

---

## ✅ 总结

> **Transport Adapter = 能力调用执行引擎 + 智能协议中枢。**
> 它让 PowerX 能在同一调用链下：
>
> * 统一调度插件 / 服务 / 智能体；
> * 自动切换最优传输通道；
> * 实现全链路追踪与安全治理；
> * 支持同步调用与流式生成共存。
