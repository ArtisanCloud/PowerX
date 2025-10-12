# Agent Manager and Lifecycle Specification (CoreX / Integration v3 – A2A Unified Edition)

> 本规范定义 **PowerX CoreX/integration 域** 中
> 智能体（Agent）的 **注册、生命周期、通信与调度模型**。
>
> 它是 Workflow/Router/Adaptor/Realtime Gateway 的运行时中枢，
> 实现智能体自治、可发现、可编排、可协作的运行体系。
>
> 本规范 **已正式融合 A2A (Agent-to-Agent)** 通道：
> 每个 Agent 均可作为调用者（Consumer）与被调用者（Provider）参与。

---

## 1️⃣ Agent 定义

### 1.1 基本结构（Manifest）

```yaml
id: com.powerx.agent.sales_copilot
display_name: Sales Copilot
version: 1.2.0
author: PowerX Team
description: "销售线索自动跟进与智能摘要生成"

runtime:
  heartbeat_interval: 10s
  max_concurrency: 4
  tenant_scope: multi
  isolation_level: process       # process|container|thread

skills:
  - crm.lead.fetch
  - ai.text.generate
  - dingding.message.send

endpoints:
  - transport: agent
    uri: agent://session.sales_copilot
    status: healthy
    protocol: ws
  - transport: grpc
    uri: grpc://localhost:8082
```

---

## 2️⃣ 生命周期阶段

| 阶段               | 描述                            | 状态字段           |
| ---------------- | ----------------------------- | -------------- |
| **Registering**  | Agent 启动，向 AgentManager 注册元信息 | `registering`  |
| **Active**       | 正常运行，可调度                      | `active`       |
| **Idle**         | 无任务但心跳正常                      | `idle`         |
| **Busy**         | 任务执行中                         | `busy`         |
| **Degraded**     | 性能下降/负载过高                     | `degraded`     |
| **Disconnected** | 心跳丢失                          | `disconnected` |
| **Retired**      | 主动下线或版本过期                     | `retired`      |

---

## 3️⃣ 生命周期事件流

```
┌────────────────────────────┐
│ Agent Start → Register()   │
│       ↓                    │
│ Heartbeat → Active/Idle    │
│       ↓                    │
│ Assign Task → Busy         │
│       ↓                    │
│ Done/Fail → Idle/Degraded  │
│       ↓                    │
│ Timeout/NoHeartbeat → Disconnected │
└────────────────────────────┘
```

每个状态变化都会以事件形式写入 **EventBus**，并经 **Realtime Gateway** 推送。

---

## 4️⃣ 注册机制

### 4.1 注册接口

```
POST /api/v1/agents/register
```

请求体：

```json
{
  "id": "com.powerx.agent.sales_copilot",
  "display_name": "Sales Copilot",
  "skills": ["crm.lead.fetch","ai.text.generate"],
  "endpoints": [{"transport":"agent","uri":"agent://session.sales_copilot"}],
  "runtime": {"tenant_scope":"multi","heartbeat_interval":"10s"}
}
```

返回：

```json
{
  "agent_token": "agt_abc123",
  "session_id": "sess_884a9",
  "heartbeat_uri": "/api/v1/agents/heartbeat?sess_884a9"
}
```

### 4.2 注册校验逻辑

* 检查 ID 唯一性；
* 验证版本与签名；
* 加载技能（与 Registry 对齐）；
* 创建 AgentSession；
* 返回带签名的 agent_token。

---

## 5️⃣ 心跳与健康检测

| 动作                              | 说明                                                 |
| ------------------------------- | -------------------------------------------------- |
| `POST /api/v1/agents/heartbeat` | 每 `heartbeat_interval` 秒发送一次                       |
| 检查字段                            | `cpu_usage`, `mem_usage`, `active_tasks`, `uptime` |
| 超时策略                            | 3 次丢失 → 状态标记为 `disconnected`                       |
| 恢复策略                            | 自动重连后状态恢复为 `active`                                |

---

## 6️⃣ 调度与分配模型

| 模式              | 描述                                |
| --------------- | --------------------------------- |
| **Workflow 调度** | WorkflowEngine 根据 step.agent 分配任务 |
| **Agent 调度**    | Agent 可主动调用 Router → 其他 Agent     |
| **负载感知**        | 根据 heartbeat 指标 + 注册标签选择目标        |
| **策略**          | 轮询 / 最优负载 / 区域亲和 / 成本加权           |
| **隔离策略**        | 按 tenant/role/skill_scope 分组调度    |

---

## 7️⃣ 调用链与执行语义（Agent 内）

```
Agent A
  │
  ▼
Router(transport=agent)
  │
  ▼
AgentAdaptor → Agent B
  │
  ├─ 调用本地能力
  ├─ 再调用第三方 Plugin (MCP/gRPC)
  └─ 将事件写入 EventBus
```

### Agent 执行上下文

```json
{
  "trace_id": "trc_99a",
  "goal": "生成销售摘要",
  "inputs": {...},
  "grants": ["crm.lead.fetch","dingding.message.send"],
  "tenant_id": "t001",
  "actor_id": "u102"
}
```

---

## 8️⃣ 通信信道（A2A 传输）

| 通道类型            | 协议             | 用途                |
| --------------- | -------------- | ----------------- |
| **Agent WS**    | WebSocket (双向) | 默认通道，支持 streaming |
| **Agent MCP**   | MCP session    | 安全、结构化 RPC        |
| **Agent Local** | 内部调用 (同进程)     | 高性能短路通道           |

> AgentAdaptor 管理这些通道，与 EventBus/Gateway 同步。

---

## 9️⃣ 事件与监控

所有 Agent 事件都会写入 EventBus：

* `agent.registered`
* `agent.heartbeat`
* `agent.assigned`
* `agent.completed`
* `agent.failed`
* `agent.disconnected`
* `agent.reconnected`

Gateway 订阅以下 Topic：

* `agent:<id>:state`
* `agent:<id>:log`
* `agent:<id>:token`

---

## 🔟 安全与授权体系

| 层级         | 策略                           |
| ---------- | ---------------------------- |
| **身份认证**   | Agent 使用注册时签发的 `agent_token` |
| **权限边界**   | 仅可调用 tool_grants 授权的能力       |
| **租户隔离**   | `tenant_id` 注入执行上下文          |
| **调用深度限制** | 防止循环 A2A（max_depth=3）        |
| **签名校验**   | 所有 Agent 间消息签名验证             |
| **配额限制**   | Agent 级与租户级调用频率控制            |

---

## 11️⃣ Agent Context Store

| 内容   | 说明                                 |
| ---- | ---------------------------------- |
| 当前会话 | Agent 的执行上下文（goal、inputs、trace_id） |
| 输出缓存 | 中间 token/log 结果                    |
| 运行统计 | 执行计数、平均延迟、错误率                      |
| 状态恢复 | 支持断线恢复（从 store 重建 session）         |

---

## 12️⃣ Metrics 与 Tracing

| 指标                           | 含义          |
| ---------------------------- | ----------- |
| `agent_registered_total`     | 已注册智能体数     |
| `agent_active_total`         | 当前活跃数       |
| `agent_heartbeat_latency_ms` | 心跳延迟        |
| `agent_invocations_total`    | 调用总数（含 A2A） |
| `agent_to_agent_latency_ms`  | A2A 调用平均延迟  |
| `agent_failures_total`       | 执行失败次数      |

全链追踪：

```
workflow → agent(caller) → agent(callee) → adaptor → provider
(trace_id 贯穿)
```

---

## 13️⃣ 控制与管理接口

| Method   | Path                             | 功能          |
| -------- | -------------------------------- | ----------- |
| `GET`    | `/api/v1/agents`                 | 查询已注册 Agent |
| `GET`    | `/api/v1/agents/{id}`            | 查看详情        |
| `POST`   | `/api/v1/agents/register`        | 注册新 Agent   |
| `POST`   | `/api/v1/agents/heartbeat`       | 心跳上报        |
| `POST`   | `/api/v1/agents/{id}/assign`     | 指定任务分配      |
| `POST`   | `/api/v1/agents/{id}/disconnect` | 下线          |
| `DELETE` | `/api/v1/agents/{id}`            | 注销          |

---

## 14️⃣ 故障恢复与高可用

| 场景     | 策略                              |
| ------ | ------------------------------- |
| 心跳丢失   | 标记为 disconnected；若重连则恢复 session |
| 注册中心重启 | Agent 自动重新注册（带 token）           |
| 节点故障   | AgentManager 重新分配任务至其他节点        |
| 通道异常   | 备用通道降级（MCP → WS → HTTP）         |

---

## 15️⃣ 与其他模块关系

| 模块                  | 交互                |
| ------------------- | ----------------- |
| **Workflow Engine** | 调用与调度 Agent 任务    |
| **Router**          | 选路到 agent 端点      |
| **Adaptor**         | 执行 agent 传输层协议    |
| **EventBus**        | 发布/订阅 agent 状态事件  |
| **Gateway**         | 推送流式输出            |
| **Registry**        | 存储 agent 能力与端点    |
| **Security Layer**  | 验证签名与 tool_grants |

---

## ✅ 一句话总结

> **Agent Manager 是 PowerX 智能体生态的控制平面与运行心跳。**
> 它让每个 Agent 既能“提供”能力，又能“调用”他人，
> 实现注册、心跳、调度、通信、监控、追踪、容错的一体化管理，
> 支撑完整的 **A2A 智能体协作与编排运行时**。
