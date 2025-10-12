# Orchestrator Service Interface Specification

> 本文档定义 **CoreX/integration/orchestrator** 模块的接口设计与运行规范。
> Orchestrator 是 PowerX 编排内核的 **统一服务门面（Service Facade）**，
> 它负责接收上层（Agent / Workflow）的执行请求，协调 Router / Transport / Flow / EventBus 等子系统，
> 并输出完整的可追踪执行链（TraceID）。

---

## 1️⃣ 定位与职责

| 职责          | 说明                                                       |
| ----------- | -------------------------------------------------------- |
| **统一入口**    | 负责接收一切 “能力调用” 或 “Flow 执行” 请求                             |
| **调用协调**    | 调用 Router 选路、通过 Transport 执行实际 Provider 能力               |
| **上下文封装**   | 统一构造 `ExecutionContext`，注入租户、用户、追踪信息                     |
| **事件分发**    | 将状态、日志、token 等事件写入 EventBus                              |
| **回调网关**    | 对接 Realtime Gateway，为前端推送流式执行结果                          |
| **A2A 调度桥** | 若 `transport=agent`，通过 AgentAdaptor 实现 Agent-to-Agent 通信 |

---

## 2️⃣ 模块架构

```
+-----------------------------------------------------------+
|                   Orchestrator Service                    |
|-----------------------------------------------------------|
|  API Layer  →  Controller →  Executor →  Flow Engine      |
|                    ↓              ↓             ↓         |
|               Router (capability) → Transport → Provider  |
|                    │                                    |
|                    └── EventBus → Realtime Gateway      |
+-----------------------------------------------------------+
```

**模块职责划分**

| 子模块           | 功能               | 路径                         |
| ------------- | ---------------- | -------------------------- |
| `controller/` | 对外接口层（HTTP/gRPC） | `/api/v1/orchestrator/...` |
| `executor/`   | 调度器，负责执行节点或能力调用  | `ExecuteStep()`            |
| `context/`    | 执行上下文构建器         | 租户 / 追踪 / 鉴权               |
| `flow/`       | Flow Engine 集成接口 | 连接 DAG 状态机                 |
| `event/`      | 事件上报与聚合          | 写入 EventBus                |
| `metrics/`    | 指标收集与统计          | Prometheus Exporter        |

---

## 3️⃣ 接口定义总览

| 类型   | 协议              | 路径 / 方法                             | 说明                 |
| ---- | --------------- | ----------------------------------- | ------------------ |
| REST | `POST`          | `/api/v1/orchestrator/execute`      | 执行单个能力（capability） |
| REST | `POST`          | `/api/v1/orchestrator/workflow/run` | 启动工作流执行            |
| REST | `POST`          | `/api/v1/orchestrator/flow/run`     | 直接运行 Flow 图        |
| REST | `GET`           | `/api/v1/orchestrator/runs/{id}`    | 查询运行状态             |
| gRPC | `Execute`       | `powerx.orchestrator.v1`            | 等价 RPC 调用          |
| gRPC | `StreamExecute` | 双向流式接口（token 逐步输出）                  |                    |

---

## 4️⃣ 数据结构定义

### 4.1 ExecutionContext（核心）

```json
{
  "trace_id": "trc_39d8a",
  "tenant_id": "t001",
  "actor_id": "u108",
  "scopes": ["crm.*", "ai.*"],
  "source": "agent:sales_copilot",
  "workflow_run_id": "wf_998",
  "options": {
    "timeout_ms": 15000,
    "retry": 2,
    "stream": true
  }
}
```

### 4.2 ExecuteRequest（能力调用）

```json
{
  "capability": "crm.lead.fetch",
  "inputs": {"id": "L102"},
  "prefer": "grpc",
  "context": { "tenant_id": "t001", "actor_id": "u108" }
}
```

### 4.3 ExecuteResponse（流式）

```json
{
  "trace_id": "trc_39d8a",
  "status": "running",
  "type": "token|log|state|done",
  "data": {"text": "处理中..."},
  "timestamp": "2025-10-12T15:40:21Z"
}
```

---

## 5️⃣ 编排执行主流程

```
Client/Agent
   │
   ├──▶ POST /orchestrator/execute
   │
   ├──▶ [1] 构建 ExecutionContext
   ├──▶ [2] Router.Select(capability)
   ├──▶ [3] Transport.Invoke(endpoint)
   ├──▶ [4] 收集结果/流式输出 → EventBus
   ├──▶ [5] 更新 StateStore（Flow/Workflow）
   └──▶ [6] Gateway 推送结果 → 前端 / Agent 回调
```

---

## 6️⃣ gRPC Service 定义（原型）

```proto
service Orchestrator {
  rpc Execute (ExecuteRequest) returns (ExecuteResponse);
  rpc StreamExecute (ExecuteRequest) returns (stream ExecuteResponse);
}
```

### ExecuteRequest

```proto
message ExecuteRequest {
  string capability = 1;
  map<string, google.protobuf.Value> inputs = 2;
  string prefer = 3; // mcp|grpc|http|agent
  ExecutionContext context = 4;
}
```

---

## 7️⃣ 执行模式

| 模式            | 描述              | 示例场景        |
| ------------- | --------------- | ----------- |
| **Sync**      | 单能力调用，等待返回      | CRM 创建客户    |
| **Async**     | 流式事件推送          | AI Token 生成 |
| **Workflow**  | 执行 DAG          | 工作流定义的多步骤调用 |
| **AgentPlan** | Agent 自动生成计划并调用 | A2A 智能体协作   |

---

## 8️⃣ A2A 执行模式（AgentAdaptor 集成）

> Orchestrator 内置对 Agent-to-Agent 调度的原生支持。

| 步骤                                               | 行为 |
| ------------------------------------------------ | -- |
| 1️⃣ Router 选到 `transport=agent` 端点               |    |
| 2️⃣ Orchestrator 调用 `AgentAdaptor.Invoke()`      |    |
| 3️⃣ 对端 Agent 执行计划（Plan）并产生 Tool 调用               |    |
| 4️⃣ Tool 调用回流至本 Orchestrator 再执行                 |    |
| 5️⃣ 所有 token/log/status 事件统一走 EventBus → Gateway |    |

> 即：Orchestrator 是 **A2A 会话控制中心**。
> 每个 Agent 调用链仍共享同一 `trace_id`，可跨 Agent 追踪。

---

## 9️⃣ 内部模块接口（Golang 伪代码）

```go
type OrchestratorService interface {
    Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error)
    StreamExecute(req *ExecuteRequest, stream Orchestrator_StreamExecuteServer) error
    RunFlow(ctx context.Context, flow *FlowRunRequest) (*FlowRunResponse, error)
}

type Executor interface {
    RunStep(ctx ExecutionContext, step StepSpec) (StepResult, error)
    StreamStep(ctx ExecutionContext, step StepSpec, writer EventWriter) error
}
```

---

## 🔟 Metrics & Logging

| 指标                             | 说明                  |
| ------------------------------ | ------------------- |
| `orchestrator_requests_total`  | 执行请求总数              |
| `orchestrator_latency_seconds` | 平均执行耗时              |
| `orchestrator_active_flows`    | 当前活跃流程数             |
| `orchestrator_errors_total`    | 异常数（按 transport 分类） |
| `a2a_invocations_total`        | Agent-to-Agent 调用次数 |

日志示例：

```json
{
  "ts": "2025-10-12T15:55:43Z",
  "trace_id": "trc_94a7e",
  "component": "Orchestrator",
  "capability": "crm.lead.fetch",
  "transport": "grpc",
  "duration_ms": 321,
  "status": "success"
}
```

---

## 11️⃣ 控制接口（Admin API）

| Method | Path                                    | 功能       |
| ------ | --------------------------------------- | -------- |
| `GET`  | `/api/v1/orchestrator/runs`             | 查询所有运行实例 |
| `GET`  | `/api/v1/orchestrator/runs/{id}`        | 查看执行详情   |
| `POST` | `/api/v1/orchestrator/runs/{id}/pause`  | 暂停执行     |
| `POST` | `/api/v1/orchestrator/runs/{id}/resume` | 恢复执行     |
| `POST` | `/api/v1/orchestrator/runs/{id}/cancel` | 取消执行     |
| `GET`  | `/api/v1/orchestrator/stats`            | 返回实时指标摘要 |

---

## 12️⃣ 安全与隔离

| 层级         | 策略                            |
| ---------- | ----------------------------- |
| **认证**     | JWT 或服务令牌验证                   |
| **授权**     | 按 scope 验证 capability 调用权限    |
| **租户隔离**   | ExecutionContext 限定 tenant_id |
| **调用深度限制** | 防止递归调用（A2A）                   |
| **敏感日志脱敏** | Mask input/output 字段          |

---

## 13️⃣ 故障与恢复

| 场景              | 处理                      |
| --------------- | ----------------------- |
| 连接中断            | 断点重试（基于 trace_id 快照）    |
| Orchestrator 重启 | 从 StateStore 恢复 Flow 状态 |
| EventBus 阻塞     | 缓存队列写入本地 WAL            |
| Router 错误       | 重选 Endpoint             |
| AgentAdaptor 超时 | 降级到 HTTP fallback       |

---

## 14️⃣ 与其他模块关系

| 模块                     | 交互说明          |
| ---------------------- | ------------- |
| **Flow Engine**        | 提供节点依赖解析与状态更新 |
| **Router / Registry**  | 提供能力发现与端点选择   |
| **Transport Adapter**  | 执行实际协议调用      |
| **Agent Manager**      | 提供上层决策计划输入    |
| **EventBus / Gateway** | 推送执行流事件       |
| **Security Layer**     | 执行权限校验与审计     |
| **Metrics Layer**      | 输出运行指标与日志     |

---

## ✅ 一句话总结

> **Orchestrator 是 PowerX 编排系统的“统一执行门面”**，
> 负责调度所有能力调用、Flow 执行与 Agent 协作。
> 它封装 Router / Transport / EventBus / FlowEngine 的内部细节，
> 并以可观测、可追踪、可审计的方式提供统一执行入口。
