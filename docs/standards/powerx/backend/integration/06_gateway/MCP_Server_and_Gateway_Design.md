# MCP Server and Gateway Design

> 本文档定义 **PowerX MCP（Model Context Protocol）Server** 与 **Gateway 模块** 的架构设计、通信模型、连接生命周期、流式事件管理机制，以及与 Adaptor 层的集成方式。
>
> 目标：
>
> * 让 **插件能力** 和 **第三方 MCP 服务** 能够以标准协议注册到 PowerX；
> * 为 PowerX Workflow / Agent 调度提供统一的上下文桥梁；
> * **预留** Agent-to-Agent (A2A) 调用能力，使未来第三方智能体能通过相同 Gateway 体系接入。

---

## 1. 设计目标

| 目标               | 描述                                                |
| ---------------- | ------------------------------------------------- |
| **统一协议层**        | MCP 为跨语言、跨环境的标准通信层                                |
| **双向通信**         | PowerX 既可作为 MCP Server（被调用），也可作为 MCP Client（主动调用） |
| **Context 注入**   | 每次调用均附带 trace_id、tenant_id、actor_id               |
| **Streaming 支持** | 原生支持 Token 流式输出与事件推送                              |
| **Session 管理**   | 支持多租户隔离、TTL、断线重连                                  |
| **A2A 占位**       | 允许未来 AgentAdaptor 利用同一 Gateway 通道复用上下文            |

---

## 2. 架构概览

```
+------------------------------------------------------------------+
|                         PowerX Runtime                           |
|------------------------------------------------------------------|
|  Workflow Engine  |  Agent Manager  |  Router  |  EventBus        |
|------------------------------------------------------------------|
|              ←→   MCP Gateway   ←→   Plugin MCP Clients           |
|                        │                                           |
|                        │                                           |
|              ←→   3rd MCP Platforms (e.g., Coze, VertexAI)        |
|------------------------------------------------------------------|
|            [占位] Agent Bridge Layer (A2A Future Extension)       |
+------------------------------------------------------------------+
```

---

## 3. PowerX 作为 MCP Server（内向型）

### 3.1 职责

* 提供标准化能力调用入口；
* 管理插件注册的能力清单；
* 维持客户端（插件/第三方）的连接状态；
* 向 Workflow Engine 推送流式结果。

### 3.2 端点

```
/mcp/v1/connect        # Session 握手
/mcp/v1/capabilities   # 能力发现
/mcp/v1/invoke         # 调用
/mcp/v1/stream         # 流式数据通道
/mcp/v1/health         # 心跳检测
```

### 3.3 Session 建立流程

```
Plugin SDK → MCP Gateway → Registry
   │
   ├─ handshake: register(plugin_id, capabilities, endpoint, token)
   │
   ├─ validation: verify signature + tenant
   │
   ├─ Registry.update(plugin_id, session_id, status=active)
   │
   └─ start bi-directional stream
```

### 3.4 会话生命周期

| 阶段        | 行为                       |
| --------- | ------------------------ |
| INIT      | 握手、校验、加载 capability list |
| ACTIVE    | 接受调用 / 推送事件              |
| STREAMING | 推送实时输出                   |
| CLOSED    | 会话关闭或超时 TTL              |
| GC        | 定期清理失效 session           |

### 3.5 Session TTL

* 默认 10 分钟；
* Stream keep-alive：每 30 秒 ping；
* 断线超过 3 次视为失联，Registry 标记 `unhealthy`。

---

## 4. PowerX 作为 MCP Client（外向型）

### 4.1 用途

* 主动调用外部 MCP 服务；
* 调度第三方平台的 AI / 工具；
* 与外部智能体交互。

### 4.2 调用路径

```
Workflow / Agent → Router(capability)
        → Adaptor(MCP)
        → Gateway.MCPClient
        → 3rd MCP Provider
```

### 4.3 调用负载结构

```json
{
  "trace_id": "trc_xxx",
  "tenant_id": "t001",
  "actor_id": "u101",
  "capability": "ai.text.generate",
  "input": {"prompt": "请写一条欢迎语"},
  "stream": true,
  "context": {"workflow_id": "wf_001"}
}
```

### 4.4 响应结构

```json
{
  "status": "ok",
  "output": {"text": "您好，欢迎使用 PowerX！"},
  "events": [
    {"type": "token", "data": "您"},
    {"type": "token", "data": "好"},
    {"type": "done"}
  ]
}
```

---

## 5. Gateway 模块结构

```
MCPGateway
├── Server (PowerX 内部)
│   ├── SessionManager
│   ├── StreamHub (EventBus桥)
│   ├── AuthHandler
│   └── CapabilityRegistryBridge
│
└── Client (外部调用)
    ├── ConnectionPool
    ├── RetryPolicy
    ├── StreamDecoder
    └── MetricsReporter
```

### 5.1 SessionManager

* 管理所有活动 session（Map[plugin_id → session]）；
* 负责 TTL 检查与断线清理；
* 监听 Registry 更新以同步插件状态。

### 5.2 StreamHub

* 将 Plugin 端发送的事件写入 EventBus；
* 反向订阅 EventBus 事件推送至插件；
* 支持多路复用与限流。

### 5.3 AuthHandler

* 验证注册请求中的 token；
* 绑定租户与 plugin_id；
* 检查插件签名与能力声明。

### 5.4 CapabilityRegistryBridge

* 与 Registry 同步更新；
* 插件注册能力后立即生效；
* 插件下线则移除 endpoint。

---

## 6. Streaming 机制

### 6.1 内部事件流

| 类型       | 说明      |
| -------- | ------- |
| `token`  | AI 输出片段 |
| `log`    | 执行日志    |
| `status` | 任务状态变更  |
| `error`  | 错误事件    |
| `done`   | 任务完成标识  |

### 6.2 数据流向

```
Plugin → MCP Gateway → EventBus → Realtime Gateway → 前端
```

### 6.3 断线恢复

* 插件重新连接时，带上 `session_id`；
* Gateway 校验 trace_id 一致后可恢复流；
* 不支持历史重放（需 EventStore）。

---

## 7. Registry 集成

| 操作 | 来源                 | 行为                                           |
| -- | ------------------ | -------------------------------------------- |
| 注册 | Plugin SDK         | 写入 Registry.capabilities 与 runtime_endpoints |
| 下线 | Gateway Session GC | 移除 runtime_endpoints                         |
| 调用 | Router             | 从 Registry 获取 endpoint 并连接 Gateway           |
| 健康 | Gateway ping       | 更新 endpoint.health 状态                        |

---

## 8. Metrics 与监控

| 指标                           | 描述             |
| ---------------------------- | -------------- |
| `mcp_sessions_active`        | 当前活跃 session 数 |
| `mcp_invocations_total`      | 调用次数           |
| `mcp_stream_latency_seconds` | 流式延迟           |
| `mcp_errors_total`           | 调用错误总数         |
| `mcp_bytes_transferred`      | 传输字节量          |
| `mcp_plugin_health{plugin}`  | 插件健康状态         |

所有指标输出到 Prometheus，可用 Grafana 监控。

---

## 9. 安全机制

| 层级 | 策略                              |
| -- | ------------------------------- |
| 认证 | Token / Signature 校验            |
| 授权 | 按租户 / plugin_id / capability 检查 |
| 隔离 | 每个 Session 独立上下文，互不干扰           |
| 限流 | 每 Session 每秒 N 次调用上限            |
| 超时 | 调用默认 10s，流默认 5 分钟               |
| 审计 | 每个调用写入 `audit_log`（trace_id）    |

> 详情见《05_security/Security_and_Governance.md》。

---

## 10. 插件端实现（简述）

* 插件使用 PowerX Plugin SDK 内置 MCP Client；
* 启动时自动注册能力列表；
* 断线自动重连；
* 发送/接收事件使用统一结构；
* SDK 可选封装 stream（token 迭代器）。

---

## 11. 与 Adaptor 的集成关系

```
Router → Adaptor(MCP)
        │
        ├─ Gateway.Client.Invoke()
        │
        ├─ StreamDecoder → EventBus
        │
        └─ Metrics.Report()
```

Adaptor 层仅处理“调用语义”，具体连接与流式管理由 Gateway 完成。

---

## 12. A2A 通道集成

> CoreX/integration 内核已经启用 Agent-to-Agent 传输；Gateway 模块为兼容旧版部署，默认仅对外部插件关闭桥接能力，可通过 `enable_a2a` 开关显式放行。

### 12.1 概念

* AgentAdaptor 利用同一 Gateway 通道；
* PowerX Agent ↔ 外部 Agent 的通信以 MCP 格式封装；
* 内核模块共享 Session 与事件流机制；
* 默认关闭外部插件桥接，平台管理员启用 `enable_a2a` 后即可对外暴露。

### 12.2 协议对齐

| 项                 | MCP 对应字段 | A2A 映射                |
| ----------------- | -------- | --------------------- |
| `session_id`      | 对话标识     | conversation_id       |
| `capability`      | 工具名      | goal / intent         |
| `input`           | 调用参数     | agent message payload |
| `event:token`     | 模型输出     | agent response chunk  |
| `event:tool_call` | 工具请求     | tool 调用代理             |
| `event:done`      | 结束       | 会话完成                  |

### 12.3 复用机制

* A2A 调用时仍通过 Gateway 注册与 SessionManager；
* 可视作特殊 Provider（`transport=agent`）；
* 复用同一 EventBus / StreamHub；
* 当 `enable_a2a` 开启后，外部 Agent/插件即可复用该链路。

---

## 13. 审计与合规（摘要）

| 内容    | 记录字段                            |
| ----- | ------------------------------- |
| 调用主体  | actor_id, tenant_id             |
| 调用对象  | plugin_id, capability, endpoint |
| trace | trace_id, session_id            |
| 结果    | status, latency_ms, bytes       |
| 异常    | code, message, retry_count      |
| 安全    | token 验证、签名哈希                   |

日志写入数据库及审计队列，周期性导出。

---

## 14. 容错与恢复

| 故障             | 恢复策略                    |
| -------------- | ----------------------- |
| 插件断线           | 自动重连 + TTL 重建           |
| Session 过期     | Registry 清理 + 重新握手      |
| 事件阻塞           | StreamHub 断路保护，自动丢弃低优先级 |
| 外部 Provider 错误 | 重试策略 + 降级（HTTP 调用）      |
| MCP Server 异常  | 自动拉起 + 日志补偿重播           |

---

## 15. 性能与优化建议

| 方向  | 措施                       |
| --- | ------------------------ |
| 连接数 | 长连接复用（HTTP/2）            |
| 延迟  | Pipeline 异步调用            |
| 流式  | Chunk 压缩与批量写入            |
| 内存  | Stream buffer 限定（默认 4MB） |
| 并发  | Goroutine Pool + 限流器     |
| 容量  | Session GC 与指标驱动扩容       |

---

## 16. 未来演进计划

| 阶段     | 内容                                              |
| ------ | ----------------------------------------------- |
| ✅ 当前   | MCP 双向通道（Server + Client）                       |
| 🟡 下一步 | 插件注册能力自动同步到 Registry                            |
| 🟢 后续  | AgentAdaptor（A2A Bridge）启用，与 Gateway 复用 Session |
| 🔵 远期  | 支持 WebSocket-MCP（统一通道）与 GraphQL 调用网关            |

---

## ✅ 一句话总结

> PowerX 的 **MCP Server 与 Gateway** 是插件与第三方平台能力的统一中枢。
> 它为所有协议提供统一连接、Session 管理与流式事件管道。
>
> 未来，**AgentAdaptor（A2A）** 可在不改变此结构的前提下复用同一机制，
> 实现 **“Agent-to-Agent over MCP Gateway”** 的多智能体协作。
