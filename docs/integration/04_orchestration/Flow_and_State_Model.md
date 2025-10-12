# Flow and State Model Specification

> 本文档定义 PowerX Orchestration 子系统中的 **Flow（工作流图）与 State（状态机）模型**，
> 用于支撑 Workflow Engine 与 Agent 执行调度。
> 目标是：在 CoreX/integration 层内，形成一个统一的 DAG 执行内核，可被任意 Agent 或 Workflow Blueprint 调用。

---

## 1️⃣ 设计目标

| 目标             | 说明                                              |
| -------------- | ----------------------------------------------- |
| **通用化 DAG 引擎** | 每个 Workflow / AgentPlan 都被编译为 Flow（节点 + 边）      |
| **显式状态机模型**    | 所有节点遵循同一状态流转（pending → running → success/error） |
| **事件驱动调度**     | 执行完全由事件驱动，支持暂停、恢复、并行、补偿                         |
| **可追踪 / 可回放**  | 每个节点执行均具备 TraceID，可被回放、审计、追踪                    |
| **A2A 原生支持**   | Flow 节点可为 “Agent Node”，允许嵌套子 Agent 调度           |

---

## 2️⃣ 概念模型

```
Flow（DAG图）
 ├── Nodes（Step/Agent/Task）
 │     ├── Inputs / Outputs
 │     ├── State（pending/running/success/error）
 │     └── Policy（timeout/retry/condition）
 ├── Edges（依赖关系）
 │     └── Type：direct / conditional / event-driven
 └── Context（运行上下文）
        ├── tenant_id / actor_id
        ├── trace_id / run_id
        └── data_store（中间产物）
```

> Flow 是抽象执行图，State Machine 管理每个 Node 的生命周期。

---

## 3️⃣ Flow Schema（YAML）

```yaml
id: flow_crm_summary
version: 1.0.0
entry: fetch_lead
nodes:
  fetch_lead:
    type: capability
    capability: crm.lead.fetch
    next: [summarize]
  summarize:
    type: capability
    capability: ai.text.generate
    next: [notify]
  notify:
    type: capability
    capability: dingding.message.send
edges:
  - from: fetch_lead
    to: summarize
  - from: summarize
    to: notify
options:
  retry_policy:
    max_retries: 2
    backoff: exponential
  concurrency: 3
  stream: true
```

> `Workflow Engine` 会在运行时将 `workflow.yaml` 编译为该标准 `Flow` 结构。

---

## 4️⃣ 状态机（State Machine）设计

### 4.1 Node 状态枚举

| 状态             | 说明          |
| -------------- | ----------- |
| `pending`      | 等待执行（依赖未满足） |
| `running`      | 正在执行        |
| `success`      | 正常完成        |
| `error`        | 执行失败（可重试）   |
| `skipped`      | 被条件跳过       |
| `canceled`     | 被中断         |
| `compensating` | 执行补偿中       |
| `compensated`  | 已补偿完成       |

### 4.2 状态流转图

```
pending ──▶ running ──▶ success
   │            │
   │            ├──▶ error ──▶ compensating ──▶ compensated
   │            │
   └──▶ skipped │
        │       ▼
        └──▶ canceled
```

### 4.3 状态机控制事件

| 事件                         | 触发方           | 说明              |
| -------------------------- | ------------- | --------------- |
| `StartNode`                | Scheduler     | 满足依赖后进入 running |
| `NodeSuccess`              | Executor      | 节点执行完成          |
| `NodeError`                | Executor      | 执行异常            |
| `RetryNode`                | Policy Engine | 超时或错误重试         |
| `CompensateNode`           | Policy Engine | 启动补偿逻辑          |
| `PauseFlow` / `ResumeFlow` | Control API   | 手动暂停/恢复         |

---

## 5️⃣ 执行模型（Flow Runtime）

### 5.1 流程总览

```
FlowEngine.Run(flow_id)
     │
     ├─▶ Load DAG → 初始化 StateStore
     ├─▶ Detect entry nodes
     ├─▶ Push StartNode Events
     │
     └─▶ Event Loop:
            ├─ Handle StartNode
            ├─ Execute Node (via Orchestrator)
            ├─ Collect Result (success/error)
            ├─ Update StateStore
            └─ Fire Next Edges
```

### 5.2 数据结构（Go 伪代码）

```go
type FlowRun struct {
    ID          string
    FlowID      string
    TenantID    string
    TraceID     string
    StateStore  map[string]*NodeState
    EventChan   chan FlowEvent
}

type NodeState struct {
    ID        string
    Status    string
    Retries   int
    StartedAt time.Time
    EndedAt   time.Time
    Output    map[string]any
}
```

### 5.3 EventBus 集成

所有节点状态变化都会写入 EventBus：

```json
{
  "type": "state",
  "topic": "workflow:run_873.state",
  "data": {
    "node": "summarize",
    "status": "running"
  },
  "trace_id": "trc_xxx"
}
```

---

## 6️⃣ 并发与依赖解析

| 逻辑          | 说明                                      |
| ----------- | --------------------------------------- |
| **Fan-out** | 单节点可同时触发多个下游（并发）                        |
| **Fan-in**  | 节点等待所有上游成功（或条件满足）                       |
| **条件依赖**    | 支持表达式，如 `{{ prev.output.score > 0.7 }}` |
| **循环检测**    | 构建时进行 DAG 拓扑排序验证，禁止环路                   |
| **动态分支**    | 节点可在运行时创建子节点（由 Agent Planner）           |

---

## 7️⃣ 补偿与回滚策略

| 策略       | 行为                     |
| -------- | ---------------------- |
| **自动补偿** | 定义 `compensate` 字段触发回滚 |
| **人工补偿** | 流程暂停后由操作员手动触发          |
| **条件补偿** | 仅当 error.type 匹配规则时执行  |

示例：

```yaml
nodes:
  create_order:
    capability: ecommerce.order.create
    on_error: [rollback_order]
  rollback_order:
    capability: ecommerce.order.cancel
    trigger: manual
```

---

## 8️⃣ 状态存储（StateStore）

### 8.1 持久化结构

存储于 PostgreSQL（或 Redis cache + DB 持久化）：

| 表                  | 字段                                              |
| ------------------ | ----------------------------------------------- |
| `flow_runs`        | id, flow_id, status, started_at, ended_at       |
| `flow_node_states` | run_id, node_id, status, retries, output, error |
| `flow_events`      | run_id, seq, type, data, trace_id               |

### 8.2 查询接口

| API                                     | 用途     |
| --------------------------------------- | ------ |
| `GET /api/v1/workflow-runs/{id}`        | 查看运行详情 |
| `GET /api/v1/workflow-runs/{id}/nodes`  | 节点状态列表 |
| `GET /api/v1/workflow-runs/{id}/events` | 事件流回放  |

---

## 9️⃣ Metrics & Tracing（继承自 Orchestration 层）

| 指标                             | 说明                                  |
| ------------------------------ | ----------------------------------- |
| `flow_node_latency_seconds`    | 单节点执行耗时                             |
| `flow_active_runs`             | 当前活跃 Flow 数                         |
| `flow_error_total`             | Flow 错误总数                           |
| `flow_state_transitions_total` | 状态转换次数                              |
| TraceID                        | 贯穿 Agent / Orchestrator / Transport |

---

## 🔟 与 Orchestrator / Agent 的关系

| 模块                     | 职责                            |
| ---------------------- | ----------------------------- |
| **Workflow Engine**    | 调用 FlowEngine 解析并运行 DAG       |
| **Orchestrator**       | 调度 Node → Router → Adaptor 执行 |
| **Agent Manager**      | 生成 AgentPlan → Flow（子图）       |
| **EventBus / Gateway** | 分发状态、token、log 事件             |
| **Security Layer**     | 校验执行权限与租户上下文                  |

---

## 🧩 扩展方向

| 功能                         | 说明                           |
| -------------------------- | ---------------------------- |
| **Dynamic Flow Injection** | Agent 可在运行中插入新节点（基于 Planner） |
| **Graph Runtime**          | 多租户分布式执行（Eino Graph Runtime） |
| **Checkpoint Resume**      | Flow 中断可从快照恢复                |
| **Partial Replay**         | 支持事件级回放与差分重放                 |
| **AI-Driven Optimization** | 根据执行数据自动调整拓扑结构               |

---

## ✅ 一句话总结

> **Flow Engine = PowerX 的有向图执行内核**
> 它将 Workflow / Agent 计划转化为可追踪、可回放的状态机模型，
> 通过事件驱动调度与统一状态管理，
> 实现多协议、多智能体、多租户的异步协同执行。
