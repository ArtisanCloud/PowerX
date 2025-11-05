# Workflow and Agent Orchestration Specification (CoreX / Integration v3 – A2A Unified Edition)

---

## 1️⃣ 总览与定位

> 本规范定义 **PowerX CoreX/integration 域** 的 **Workflow + Agent Orchestration Framework（工作流与智能体编排框架）**。
>
> 它实现完整的 **多协议智能编排体系**：
>
> * Workflow Engine（DAG 执行器）
> * Agent Manager（智能体注册、技能与调度）
> * Router / Adaptor（多通道执行器）
> * EventBus + Realtime Gateway（事件与流式反馈）
>
> 并正式融合 **A2A (Agent-to-Agent)** 通道，
> 将每个 Agent 视为一个能力节点（Capability Provider + Consumer 双角色），
> 构建「Agent-Native Integration Graph」。

---

## 2️⃣ 设计目标

| 目标         | 说明                                     |
| ---------- | -------------------------------------- |
| **统一编排**   | Workflow 与 Agent 共用一个执行内核（DAG Runtime） |
| **多通道支持**  | MCP / gRPC / HTTP / **Agent (A2A)** 并列 |
| **多智能体协作** | 任意 Agent 可调用其他 Agent 的能力               |
| **全链流式反馈** | EventBus → Realtime Gateway 实时推送       |
| **弹性容错**   | 支持重试、补偿、人工介入                           |
| **语义自治**   | Agent 具备计划、感知、行动的状态机逻辑                 |
| **安全可控**   | Tool Grants、租户隔离、Scope 校验              |

---

## 3️⃣ 核心运行结构

```
┌──────────────────────────────────────────────────────┐
│                   PowerX Runtime                     │
│------------------------------------------------------│
│ Workflow Engine  │  Agent Manager │  Router / Adaptor│
│------------------------------------------------------│
│          Integration Bus (EventBus + Trace)          │
│------------------------------------------------------│
│          Realtime Gateway (SSE / WS)                 │
│------------------------------------------------------│
│ Plugins / Agents / External Providers (双向参与者)     │
└──────────────────────────────────────────────────────┘
```

---

## 4️⃣ Workflow DSL（A2A 融合语法）

### 基本结构

```yaml
id: wf_lead_summary
name: 智能线索摘要与回访
version: 2.0.0

trigger:
  type: webhook
  input_schema:
    properties:
      lead_id: { type: string }

steps:
  - id: fetch_lead
    capability: crm.lead.fetch
    prefer: grpc
    next: [summarize]

  - id: summarize
    agent: com.powerx.agent.writer        # 指定 agent
    goal: "为线索生成摘要"
    inputs:
      text: "{{ fetch_lead.output }}"
    tool_grants:
      - ai.text.generate
    stream: true
    next: [followup]

  - id: followup
    agent: com.powerx.agent.sales_copilot
    goal: "制定销售回访策略并发送提醒"
    tool_grants:
      - crm.lead.fetch
      - dingding.message.send
    constraints:
      time_budget_ms: 8000
```

### 说明

| 字段            | 含义                            |
| ------------- | ----------------------------- |
| `agent`       | 调用目标智能体 ID（注册于 Agent Manager） |
| `goal`        | Agent 接收到的任务目标描述              |
| `tool_grants` | 被授权可代为调用的能力集                  |
| `constraints` | 限制执行时间/预算/优先级                 |
| `stream`      | 启用实时 token 流输出                |

---

## 5️⃣ 执行模型

```
WorkflowEngine
   ├─ Step → Router(capability / agent)
   │           ├─ transport=mcp/grpc/http/agent
   │           ▼
   │        Adaptor(协议执行)
   │           ▼
   │        Provider / Agent
   │           ▼
   └─ EventBus → Realtime Gateway → 前端
```

**A2A 调用：**

```
Agent A (Caller)
   │
   ▼
Router → Endpoint(transport=agent)
   │
   ▼
AgentAdaptor → Agent B (Callee)
   │
   ├─ 执行自身能力或再调用插件
   └─ 推送 token/log 至 EventBus → Gateway
```

---

## 6️⃣ AgentAdaptor（正式通道）

| 功能        | 描述                                        |
| --------- | ----------------------------------------- |
| **协议类型**  | `transport=agent`                         |
| **调用方式**  | 双向长连接（WS / MCP 会话）                        |
| **上下文传递** | trace_id / tenant / actor / goal / grants |
| **执行模型**  | request-response + streaming              |
| **调用权限**  | 通过 Tool Grants 控制可调用能力                    |

### 结构示例

```json
{
  "type": "agent.call",
  "caller": "com.powerx.agent.writer",
  "callee": "com.powerx.agent.sales_copilot",
  "goal": "整理客户摘要",
  "input": {...},
  "grants": ["crm.lead.fetch","ai.text.generate"],
  "trace_id": "trc_9xx",
  "stream": true
}
```

> ⚙️ 提示：CoreX 内核已启用 Agent-to-Agent 调度；若需让外部插件或第三方 Agent 通过 Gateway 复用该通道，需在平台侧开启 `enable_a2a` 开关。

---

## 7️⃣ Agent 结构与注册

```yaml
id: com.powerx.agent.writer
display_name: Writer Agent
skills:
  - ai.text.generate
  - ai.text.rewrite
endpoints:
  - transport: agent
    uri: agent://writer.session.93ba
    status: healthy
runtime:
  heartbeat_interval: 10s
  tenant_scope: global
```

AgentManager 负责：

* 注册与心跳；
* 加载技能集；
* 维护 AgentChannel（与 Router/Adaptor 协调）。

---

## 8️⃣ Workflow 与 Agent 的互操作

| 模式                      | 描述                            |
| ----------------------- | ----------------------------- |
| **Workflow 调用 Agent**   | step 中使用 `agent:` 定义          |
| **Agent 调用 Capability** | Agent 内通过 Router 执行能力         |
| **Agent 调用 Agent**      | transport=agent 通道调用另一智能体     |
| **Hybrid**              | Workflow 混合 Agent + Plugin 调用 |

> 所有流式结果通过 EventBus 统一推送。

---

## 9️⃣ 事件与流式反馈

| 类型      | 内容     |
| ------- | ------ |
| `state` | 节点状态变化 |
| `token` | 实时增量输出 |
| `log`   | 执行日志   |
| `error` | 异常信息   |
| `done`  | 结束标志   |

统一事件格式：

```json
{
  "topic": "agent:run_123",
  "type": "token",
  "seq": 23,
  "data": {"text": "生成中..."},
  "trace_id": "trc_9xx",
  "tenant_id": "t001"
}
```

---

## 🔟 调用策略与容错

| 策略   | 描述                                |
| ---- | --------------------------------- |
| 幂等检测 | trace_id + step_id 唯一             |
| 超时   | 由 `constraints.time_budget_ms` 控制 |
| 重试   | 支持 agent/gRPC/MCP 统一重试策略          |
| 降级   | Router 自动切换可替代 Agent              |
| 补偿   | 定义 `on_error` 节点                  |

---

## 11️⃣ 安全与授权

| 层级         | 策略                     |
| ---------- | ---------------------- |
| **认证**     | Agent/Plugin 均需注册并签名验证 |
| **授权**     | Tool Grants 指定可代理调用能力  |
| **租户隔离**   | tenant_id 全链传递         |
| **数据脱敏**   | 按 masking_policy 生效    |
| **调用深度限制** | 避免递归代理（max depth = 3）  |

---

## 12️⃣ 并行与依赖（含子Agent）

* Fan-out / Fan-in 支持子 Agent 并发；
* 循环检测；
* 任意 Agent 可生成子任务分支；
* DAG 节点支持跨 Agent 执行。

---

## 13️⃣ Metrics 与 Tracing

| 分类       | 指标                                                    |
| -------- | ----------------------------------------------------- |
| Workflow | `workflow_runs_total`、`step_latency_seconds`          |
| Agent    | `agent_invocations_total`、`agent_to_agent_latency_ms` |
| Router   | `router_selection_time_ms`                            |
| Stream   | `stream_delivery_latency_ms`、`gateway_dropped_total`  |
| Trace    | `workflow → agent → adaptor → provider` 全链可观测         |

---

## 14️⃣ 控制与调度接口

| 方法     | 路径                                      | 功能            |
| ------ | --------------------------------------- | ------------- |
| `GET`  | `/api/v1/agents`                        | 查询注册 Agent    |
| `GET`  | `/api/v1/workflows`                     | 查询工作流定义       |
| `POST` | `/api/v1/workflow-runs`                 | 启动工作流         |
| `POST` | `/api/v1/agent-calls`                   | 主动触发 Agent 调用 |
| `GET`  | `/api/v1/workflow-runs/{run_id}/events` | 查看事件流         |

---

## 15️⃣ 执行链示意

```
Workflow Engine
   │
   ├─ Step(capability) → Router → (grpc/http/mcp)
   │
   ├─ Step(agent) → Router(transport=agent)
   │                     │
   │                     ▼
   │                 AgentAdaptor
   │                     │
   │                     ▼
   │              Agent Runtime（执行或再调用其他能力）
   │                     │
   └──────→ EventBus → Realtime Gateway → 前端
```

---

## ✅ 一句话总结

> **A2A（Agent-to-Agent）已成为 PowerX 的正式传输层与编排单元。**
> 每个 Agent 同时是 Provider 与 Consumer，
> Workflow Engine、Router、Adaptor、EventBus 全面支持 agent 调用链，
> 实现 **多智能体协作、跨插件编排、实时反馈、全链追踪** 的 PowerX 智能操作系统级运行时。
