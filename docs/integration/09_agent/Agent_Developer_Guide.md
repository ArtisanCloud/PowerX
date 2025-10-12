# Agent Developer Guide

> 本文档定义 **AgentAdaptor（智能体传输层适配器）** 的设计规范、职责边界与接口标准。
> 它是 PowerX **Integration Framework** 的重要组成部分，用于实现 Agent-to-Agent（A2A）通信、
> 并与 Router / Orchestrator / Security / EventBus 模块协同工作。

---

## 1️⃣ 设计目标

| 目标              | 说明                                |
| --------------- | --------------------------------- |
| **统一 A2A 调用通道** | 在 PowerX 内部实现 Agent 之间的调用与事件交换    |
| **多协议互通**       | 兼容 MCP / gRPC / HTTP / Local 调用   |
| **安全与可控**       | 所有 A2A 调用都基于 ToolGrant + Scope 验证 |
| **可观测与可追踪**     | 全链路 Trace 与 Metrics 支持            |
| **插件无侵入**       | 插件侧仅需实现标准 Agent SDK，即可注册能力端点      |

---

## 2️⃣ 模块架构

```
+-------------------------------------------------------------+
|                    PowerX Integration Layer                 |
|-------------------------------------------------------------|
|  Router ───▶ AgentAdaptor ───▶ Transport (grpc/mcp/http)    |
|      │                   │                                 |
|      │                   ├── ToolGrantValidator             |
|      │                   ├── ContextPropagator              |
|      │                   ├── StreamHandler (EventBus Bridge)|
|      │                   └── FallbackHandler (降级策略)     |
|      ▼                                                       |
|  Registry / Runtime / EventBus / Gateway                    |
+-------------------------------------------------------------+
```

---

## 3️⃣ AgentAdaptor 的职责

| 职责        | 说明                                      |
| --------- | --------------------------------------- |
| **抽象调用**  | 屏蔽底层协议（agent, grpc, mcp, http）差异        |
| **上下文传递** | 在调用链中传播 trace_id / tenant / tool_grants |
| **安全校验**  | 验证调用方权限与 ToolGrant                      |
| **事件流转**  | 将 token/log/state 转为统一事件写入 EventBus     |
| **错误隔离**  | 处理断线、超时、限流与降级逻辑                         |
| **嵌套调用**  | 支持 Agent → Agent → Provider 多层链路        |

---

## 4️⃣ 核心数据结构

### 4.1 AgentInvokeRequest

```json
{
  "trace_id": "trc_1f7c2",
  "tenant_id": "t001",
  "caller_agent": "agent:sales_copilot",
  "target_agent": "agent:crm_helper",
  "goal": "补全客户档案",
  "inputs": {
    "lead_id": "L10023"
  },
  "tool_grants": ["crm.lead.fetch", "dingding.message.send"],
  "constraints": {
    "timeout_ms": 8000,
    "stream": true
  }
}
```

### 4.2 AgentInvokeResponse

```json
{
  "trace_id": "trc_1f7c2",
  "type": "token|log|state|error|done",
  "data": { "text": "已补全客户档案" },
  "timestamp": "2025-10-12T15:45:10Z"
}
```

---

## 5️⃣ 调用生命周期

```
Agent-A 发起任务
   │
   ├──▶ Router.select(transport=agent)
   ├──▶ AgentAdaptor.Invoke(target_agent, request)
   │       ├── Validate ToolGrant
   │       ├── Establish Session (via MCP/gRPC)
   │       ├── Propagate Context (trace, tenant)
   │       ├── StreamHandler -> EventBus
   │       └── Return Response
   │
   └──▶ Orchestrator 收集事件 / Gateway 推送前端
```

---

## 6️⃣ 传输模式

| 模式         | 协议                | 场景                     |
| ---------- | ----------------- | ---------------------- |
| **MCP**    | WebSocket Session | Agent 间异步通信，默认通道       |
| **gRPC**   | 内部集群高速通道          | 同宿主或同租户 Agent          |
| **HTTP**   | 兼容外部插件            | 不支持实时流式                |
| **Local**  | 进程内通信             | 嵌入式 Agent 执行           |
| **Hybrid** | 动态选择协议            | 根据 Router 打分（延迟、健康、成本） |

---

## 7️⃣ SDK 实现要求

| SDK 功能                 | 说明                              |
| ---------------------- | ------------------------------- |
| `RegisterAgent()`      | 注册自身信息、提供可调用能力列表                |
| `HandleInvoke()`       | 接收调用请求并返回结果或流式输出                |
| `StreamEmit()`         | 输出 token/log/state 事件到 EventBus |
| `ValidateGrant()`      | 内置 ToolGrant 验证逻辑               |
| `ContextPropagation()` | 自动传递 trace_id、tenant_id、scope   |

---

## 8️⃣ 调用序列图

```
Agent-A        AgentAdaptor        Orchestrator        Agent-B
   │                 │                    │                │
   │---Invoke(req)--▶│                    │                │
   │                 │---ValidateGrant--▶ │                │
   │                 │---Connect(MCP)--------------------▶ │
   │                 │                                    │
   │                 │◀──Stream(token/log/state)──────────│
   │                 │                                    │
   │◀──EventBus──Router──Realtime Gateway──UI/Console────│
```

---

## 9️⃣ 错误与降级策略

| 场景             | 策略                           |
| -------------- | ---------------------------- |
| 连接超时           | 自动重试 2 次，指数退避                |
| Agent 不在线      | fallback → 同租户同能力的备用 Agent   |
| ToolGrant 无效   | 拒绝执行，返回 `grant_denied`       |
| 调用链过深          | 终止并记录 `proxy_depth_exceeded` |
| 输出异常           | 缓冲事件并重放                      |
| MCP Session 断开 | 自动重连 + 恢复上下文                 |

---

## 🔟 安全验证流程

1️⃣ 校验 JWT → 验证 `tenant_id`、`actor_id`
2️⃣ 校验 Scope → 调用者必须具备该能力
3️⃣ 校验 ToolGrant → 若代理调用则验证授权列表
4️⃣ 限流与计数 → 每个 Agent 调用次数 / 时延监控
5️⃣ 审计 → 写入安全事件 `security.agent_invocation`

---

## 11️⃣ Metrics 指标

| 指标                                  | 说明         |
| ----------------------------------- | ---------- |
| `agent_adaptor_invocations_total`   | 调用总数       |
| `agent_adaptor_latency_ms`          | 平均调用耗时     |
| `agent_adaptor_stream_events_total` | 流式事件数      |
| `agent_adaptor_errors_total`        | 调用错误数      |
| `agent_adaptor_retries_total`       | 重试次数       |
| `agent_adaptor_active_sessions`     | 活跃 MCP 会话数 |

---

## 12️⃣ Trace 贯穿链

```
Agent-A → AgentAdaptor → Router → Orchestrator → Transport → Agent-B
                     ↑                 ↓
             Security Layer     EventBus / Gateway
```

每个调用均带全局 `trace_id`，
事件流（token/log/state/error）全部写入链路追踪（OpenTelemetry）。

---

## 13️⃣ 配置与部署建议

| 配置项                     | 默认值                 | 说明               |
| ----------------------- | ------------------- | ---------------- |
| `A2A_Enabled`           | true                | 启用 AgentAdaptor  |
| `A2A_TransportPriority` | `[mcp, grpc, http]` | 协议优先级            |
| `MCP_ReconnectInterval` | 5s                  | 重连间隔             |
| `Grant_TTL`             | 600s                | 默认 ToolGrant 有效期 |
| `MaxProxyDepth`         | 3                   | 最大代理深度           |

---

## 14️⃣ 与其他模块关系

| 模块                 | 交互说明                                  |
| ------------------ | ------------------------------------- |
| **Router**         | 选中 `transport=agent` 时调用 AgentAdaptor |
| **Orchestrator**   | 调用入口，注入上下文与追踪信息                       |
| **Agent Manager**  | 注册与发现 Agent Endpoint                  |
| **Security Layer** | 验证 ToolGrant / Scope                  |
| **EventBus**       | 转发 token/log/state 事件                 |
| **Metrics Layer**  | 收集调用指标                                |
| **Registry**       | 存储 Agent Endpoint 信息                  |

---

## ✅ 一句话总结

> **AgentAdaptor = PowerX 智能体间通信桥。**
> 它统一封装 A2A 调用与安全机制，使得任意 Agent 可像调用内部能力一样调用其他 Agent，
> 同时具备可观测、可追踪、可授权、可回放的传输层能力。
