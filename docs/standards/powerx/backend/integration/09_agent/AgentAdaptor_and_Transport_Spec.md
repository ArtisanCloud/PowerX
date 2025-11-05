# AgentAdaptor and Transport Specification (CoreX / Integration v3 – A2A Unified Edition)

> 本规范定义 PowerX 平台中 **AgentAdaptor**（智能体传输适配层）的通信协议、消息语义、会话管理与安全规则。
>
> 它是继 `transport=grpc`、`transport=http`、`transport=mcp` 之后的 **第四个一等传输通道**，
> 用于在 PowerX 内部或跨插件之间实现智能体的直接交互（Agent ↔ Agent）。

---

## 1️⃣ 核心定位

| 层级                | 模块               | 职责                  |
| ----------------- | ---------------- | ------------------- |
| Integration Layer | Router           | 选路至 agent:// 端点     |
| Integration Layer | **AgentAdaptor** | 建立/管理 Agent Channel |
| Runtime Layer     | Agent Runtime    | 接收指令、执行任务、流式反馈      |
| Control Plane     | Agent Manager    | 维护注册、心跳、会话状态        |

**目标：**

> 在多智能体环境中，提供低延迟、可追踪、流式、安全的通信层，
> 统一 Agent-to-Agent、Agent-to-Workflow、Agent-to-Plugin 的调用方式。

---

## 2️⃣ 通信模型概览

```
┌───────────────┐          ┌────────────────┐
│  Agent A      │  ─────▶  │  AgentAdaptor  │
│ (Caller)      │           │  (PowerX Core) │
└───────────────┘          └────────────────┘
         │                           │
         │  transport=agent          │
         ▼                           ▼
┌────────────────┐          ┌───────────────┐
│  AgentAdaptor  │  ◀─────  │  Agent B      │
│  (PowerX Core) │           │  (Callee)    │
└────────────────┘          └───────────────┘
```

---

## 3️⃣ 通道类型

| 通道类型               | 描述                   | 典型场景          |
| ------------------ | -------------------- | ------------- |
| **Local Channel**  | 同进程调用（Go 内部 channel） | 同节点多 Agent 协作 |
| **WS Channel**     | WebSocket 双向流        | 默认通道          |
| **MCP Channel**    | 使用 MCP 协议反向连接        | 高安全远程调用       |
| **Bridge Channel** | 跨节点/租户中继             | 多租户集群协作       |

---

## 4️⃣ 协议定义（消息层）

### 4.1 基础消息格式

```json
{
  "type": "agent.call",
  "trace_id": "trc_9aa",
  "from": "com.powerx.agent.writer",
  "to": "com.powerx.agent.sales_copilot",
  "goal": "生成销售摘要",
  "inputs": {...},
  "grants": ["crm.lead.fetch","dingding.message.send"],
  "session": "agt_sess_812b",
  "timestamp": "2025-10-12T11:22:00Z"
}
```

### 4.2 事件类型表

| 类型                | 方向              | 描述              |
| ----------------- | --------------- | --------------- |
| `agent.call`      | Caller → Callee | 发起任务调用          |
| `agent.token`     | Callee → Caller | 流式输出（token/log） |
| `agent.done`      | Callee → Caller | 任务完成            |
| `agent.error`     | Callee → Caller | 错误响应            |
| `agent.ping/pong` | 双向              | 心跳保持            |
| `agent.cancel`    | Caller → Callee | 中断任务            |
| `agent.reply`     | 双向              | 标准 RPC 回复（非流式）  |

---

## 5️⃣ 调用与响应语义

### 调用过程

```
Agent A → AgentAdaptor → Agent B
    │          │               │
    │ call()   │               │
    │────────▶ │ openChannel() │
    │          │──────────────▶│ receive()
    │          │◀──────────────│ token/done/error
    │ receive()│               │
```

### 调用选项

```go
type AgentCallOptions struct {
  Stream       bool
  Timeout      time.Duration
  RetryPolicy  RetryPolicy
  Grants       []string
  TraceID      string
}
```

### 流式回传

```json
{
  "type": "agent.token",
  "seq": 18,
  "data": {"text": "生成中..."},
  "trace_id": "trc_9aa",
  "usage": {"prompt":45,"completion":190}
}
```

---

## 6️⃣ Session 与 Channel 管理

### 6.1 Session 状态机

| 状态          | 描述      |
| ----------- | ------- |
| `opening`   | 通道初始化中  |
| `active`    | 双向通信建立  |
| `streaming` | 正在传输数据  |
| `closing`   | 任务完成或终止 |
| `closed`    | 已断开     |
| `error`     | 异常终止    |

### 6.2 Session Registry（内存态）

```go
type AgentSession struct {
  SessionID   string
  CallerID    string
  CalleeID    string
  ChannelType string  // local|ws|mcp
  CreatedAt   time.Time
  LastPing    time.Time
  Status      string
}
```

* Session 生命周期由 AgentAdaptor 管理；
* 支持自动心跳、断线重连、流控；
* 会话结束后写入 Registry 快照用于审计。

---

## 7️⃣ Router 与 Registry 集成

| 模块               | 责任                                 |
| ---------------- | ---------------------------------- |
| **Router**       | 从 Registry 获取 `transport=agent` 端点 |
| **Registry**     | 存储所有活跃 Agent 的 endpoints           |
| **AgentAdaptor** | 负责真实通信与复用                          |

Registry 中示例：

```yaml
resolved_endpoints:
  - transport: agent
    uri: agent://session.sales_copilot
    health: healthy
    tenant_id: t001
```

Router 会基于健康状态、延迟和 Tool Grants 选出目标 Agent。

---

## 8️⃣ Tool Grants 与安全边界

### 8.1 授权机制

每个 A2A 调用需附带 `grants` 字段，指定被调用 Agent 可代为执行的能力集。

示例：

```yaml
tool_grants:
  - crm.lead.fetch
  - ai.text.generate
```

### 8.2 安全策略

| 安全层      | 策略                           |
| -------- | ---------------------------- |
| **认证**   | 所有 Agent 调用需携带 `agent_token` |
| **授权**   | 校验 `grants` 与 Agent 的声明技能    |
| **租户隔离** | 不同租户的 Agent 不能互相访问           |
| **签名验证** | 消息体签名防篡改                     |
| **深度限制** | 防止无限代理链（默认 max_depth = 3）    |

---

## 9️⃣ 流式传输与事件总线

A2A 调用的流式事件可选两种路径：

| 模式       | 路径                 | 用途             |
| -------- | ------------------ | -------------- |
| **直连流式** | WS/MCP 内部流         | 低延迟双向 token 传递 |
| **总线转发** | EventBus → Gateway | UI 或第三方订阅      |

所有事件均统一 TraceID，可被前端实时展示或追踪。

---

## 🔟 容错与重连策略

| 场景         | 策略                       |
| ---------- | ------------------------ |
| Channel 中断 | 自动重连 + 状态恢复              |
| Callee 崩溃  | Router 重新选路并重试           |
| Caller 取消  | `agent.cancel` 信号广播      |
| 超时         | 触发 RetryPolicy 或标记失败     |
| 网络异常       | 降级为 HTTP fallback（仅状态通知） |

---

## 11️⃣ 性能与QoS

| 指标      | 目标             |
| ------- | -------------- |
| 平均 RTT  | ≤ 50ms         |
| 并发连接数   | ≥ 10,000       |
| 吞吐量     | ≥ 30,000 msg/s |
| 流式事件丢包率 | ≤ 0.01%        |
| 会话恢复时间  | < 3s           |

支持：

* 带宽自适应；
* Backpressure；
* Priority Channel（token > status > log）。

---

## 12️⃣ 指标与可观测性

| 指标                               | 说明       |
| -------------------------------- | -------- |
| `agent_channels_active`          | 当前活跃通道数  |
| `agent_calls_total`              | 调用总数     |
| `agent_channel_reconnects_total` | 重连次数     |
| `agent_latency_ms`               | A2A 平均延迟 |
| `agent_stream_rate`              | 流式速率     |
| `agent_failures_total`           | 调用失败次数   |

Tracing：

```
Agent A → AgentAdaptor → Agent B → EventBus → Gateway
(trace_id 全链贯穿)
```

---

## 13️⃣ 开发接口（Golang 示例）

```go
// 发起调用
resp, err := agentAdaptor.Call(ctx, AgentCall{
    To: "com.powerx.agent.sales_copilot",
    Goal: "客户摘要生成",
    Input: map[string]interface{}{"lead_id":"L001"},
    Grants: []string{"crm.lead.fetch","ai.text.generate"},
    Stream: true,
})
```

```go
// 接收调用
agentAdaptor.OnCall(func(ctx context.Context, req AgentCall) (AgentStream, error) {
    // 执行逻辑...
    return stream, nil
})
```

---

## 14️⃣ 故障处理与降级流程

```
Agent A → Call(AgentAdaptor)
     │
     ├── timeout → Retry / on_error
     ├── conn_lost → Reconnect / Fallback
     ├── callee_down → Router 选路到备用 Agent
     └── cancel → graceful close
```

---

## 15️⃣ 与其他模块关系

| 模块                   | 关系             |
| -------------------- | -------------- |
| **AgentManager**     | 注册/心跳/断线检测     |
| **WorkflowEngine**   | 调用与调度入口        |
| **Router**           | 选路到 agent://   |
| **Registry**         | 存储活跃会话端点       |
| **EventBus**         | 事件聚合与分发        |
| **Realtime Gateway** | 推送流式 token/log |
| **Security Layer**   | 鉴权与签名验证        |

---

## ✅ 一句话总结

> **AgentAdaptor = PowerX 的 A2A 通信内核。**
> 它让每个 Agent 都能成为“节点”，
> 通过安全、低延迟、可观测的通道互相调用、流式交流与协作执行，
> 构建出真正的 **Agent-Native 多智能体运行时网络**。
