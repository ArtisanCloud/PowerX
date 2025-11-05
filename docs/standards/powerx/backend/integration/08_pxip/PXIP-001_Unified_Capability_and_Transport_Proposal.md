# PXIP-001：Unified Capability & Transport Proposal

> PowerX Integration Proposal — 统一能力与传输规范
>
> 本文档为 **PowerX Integration Framework** 的首份 PXIP（PowerX Integration Proposal）草案，
> 旨在确立平台中“能力（Capability）”与“传输层（Transport）”的统一抽象、接口与演化方向。
>
> 它是整个 Integration 架构的设计宪章，为 Router、Registry、Adaptor、Workflow、Agent 提供共同语义。

---

## 1️⃣ 背景与动机

PowerX 作为多协议、多插件、多智能体的运行平台，需要一种统一的通信抽象：

| 历史问题                  | 描述                          |
| --------------------- | --------------------------- |
| **多协议碎片**             | HTTP、gRPC、MCP 各自独立、重复封装     |
| **能力定义分散**            | 插件各自声明能力，缺乏统一注册语义           |
| **Agent 调用困难**        | Workflow/Agent 无法无缝调用插件能力   |
| **缺乏演化标准**            | 无法安全升级或切换传输协议               |
| **Observability 不统一** | trace、metrics、audit 难以跨协议贯通 |

> PXIP-001 提出“统一能力模型 + 传输抽象层”的解决方案，
> 让所有协议、插件、智能体都能在同一运行语义下协作。

---

## 2️⃣ 目标与范围

| 目标         | 说明                              |
| ---------- | ------------------------------- |
| **统一接口层**  | 所有调用均通过 Transport Adapter 实现    |
| **统一能力语义** | 能力通过 Capability Contract 定义     |
| **多协议共存**  | MCP / gRPC / HTTP / Agent (A2A) |
| **运行时感知**  | Registry & Router 自动选择最优通道      |
| **流式支持**   | 全链路 token/log 事件一致化             |
| **未来兼容**   | 可平滑引入 MQ / GraphQL / WASM 调用    |

---

## 3️⃣ 概念图谱（Concept Graph）

```
Capability (定义)
   │
   ▼
Capability Contract (接口描述)
   │
   ▼
Capability Registry (真相源)
   │
   ▼
Router (选路决策)
   │
   ▼
Transport Adapter (执行层)
   │
   ├── gRPC Adapter
   ├── HTTP Adapter
   ├── MCP Adapter
   └── Agent Adapter (A2A)
   │
   ▼
Runtime Endpoint (插件/智能体)
```

---

## 4️⃣ 核心抽象定义

### 4.1 Capability Contract

> 定义能力的输入输出、版本、授权与传输策略。

```yaml
id: crm.lead.create
version: 1.2.0
provider: com.powerx.plugin.crmplus
io:
  input_schema: { type: object, properties: { name: { type: string } } }
  output_schema: { type: object, properties: { id: { type: string } } }
security:
  scope: crm.lead.write
  tenant_mode: isolated
policy:
  timeout_ms: 8000
  retry: 2
  prefer_transport: grpc
```

### 4.2 Transport Interface

```go
type TransportAdapter interface {
    Invoke(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
    Stream(ctx context.Context, req *TransportRequest, sink chan<- *StreamChunk) error
}
```

> Adapter 负责将调用请求映射到对应协议实现（MCP/gRPC/HTTP/Agent）。

---

## 5️⃣ 核心原则（Design Principles）

| 原则                        | 说明                    |
| ------------------------- | --------------------- |
| **Capability = Function** | 一切能力皆函数，输入输出确定、可测、可复放 |
| **Transport = Strategy**  | 传输协议可替换、可动态选择         |
| **Registry = Truth**      | 所有能力与端点来源唯一真相源        |
| **Router = Intelligence** | 动态选路、权重打分、降级容错        |
| **Adaptor = Bridge**      | 协议执行器，不关心调用来源         |
| **EventBus = Continuity** | 统一流式事件通道              |
| **Agent = Orchestrator**  | 上层智能体统一驱动调用流程         |

---

## 6️⃣ 统一调用生命周期

| 阶段           | 行为                     | 说明              |
| ------------ | ---------------------- | --------------- |
| **Plan**     | Workflow / Agent 解析任务  | 选择目标能力          |
| **Route**    | Router 从 Registry 获取端点 | 策略打分与过滤         |
| **Invoke**   | Transport Adapter 调用   | 执行协议请求          |
| **Stream**   | 返回实时事件                 | token/log/state |
| **Observe**  | 记录 trace/metric/audit  | 可观测与回放          |
| **Complete** | 输出汇总结果                 | workflow 返回终态   |

---

## 7️⃣ A2A Agent Transport（新增第四类传输）

> Agent 间调用（Agent→Agent）被正式视为一种 Transport 类型，
> 与 MCP/gRPC/HTTP 并列。

| 特性       | 说明                                            |
| -------- | --------------------------------------------- |
| **协议层**  | 通过 AgentAdaptor 实现                            |
| **认证层**  | ToolGrant + Token                             |
| **调用层**  | AgentRuntime → Router → Target Agent Endpoint |
| **安全限制** | depth ≤ 3，scope 限制                            |
| **返回模式** | 流式或同步                                         |

### 7.1 调用流程

```
AgentA (调用方)
   │
   ▼
Router(capability=crm.lead.fetch, prefer=agent)
   │
   ▼
AgentAdaptor.Invoke()
   │
   ▼
AgentB (被调用 Agent)
```

### 7.2 AgentAdaptor 调用语义

```go
resp, err := agentAdaptor.Invoke(ctx, &TransportRequest{
    CapabilityID: "crm.lead.fetch",
    Metadata: map[string]string{"caller": "agent:sales_copilot"},
})
```

---

## 8️⃣ Registry 中的端点表示（统一格式）

```yaml
resolved_endpoints:
  - transport: grpc
    uri: grpc://127.0.0.1:51043
  - transport: mcp
    session: mcp://sess_99a2
  - transport: http
    uri: http://127.0.0.1:51044/api
  - transport: agent
    agent_id: thirdparty.sales.copilot
    health: healthy
```

---

## 9️⃣ 调用优先级与路由策略

默认优先级：
`Agent > MCP > gRPC > HTTP`

可通过 `prefer` 或 `only` 覆盖。

> Router 根据延迟、健康、成本、scope、region 打分。
> 当目标端点不可用时，自动降级切换到下一级协议。

---

## 🔟 事件流统一模型

所有协议（包括 A2A）返回事件统一封装为：

```json
{
  "type": "token|log|state|error|done",
  "seq": 32,
  "topic": "workflow:run_832",
  "trace_id": "trc_7d9a",
  "data": { "text": "执行中..." }
}
```

事件流通过 EventBus → Realtime Gateway → 前端。
详见《04_orchestration/Realtime_Streaming_Gateway.md》。

---

## 11️⃣ Metrics & Observability

| 模块           | 指标                                            | 示例   |
| ------------ | --------------------------------------------- | ---- |
| **Router**   | `router_selection_latency_ms`                 | 20   |
| **Adaptor**  | `transport_latency_seconds{transport="grpc"}` | 0.02 |
| **Agent**    | `agent_invocation_total`                      | 300  |
| **EventBus** | `stream_event_throughput`                     | 1k/s |
| **Registry** | `registry_endpoints_total`                    | 42   |

全链路 Trace：

```
Workflow → Router → Adaptor → Endpoint → EventBus → Gateway
```

---

## 12️⃣ 安全与治理

| 安全层          | 策略                      |
| ------------ | ----------------------- |
| **租户隔离**     | tenant_id 全链路传递         |
| **作用域控制**    | scope 校验                |
| **签名校验**     | Token + HMAC            |
| **Agent 授权** | ToolGrant 验证            |
| **数据脱敏**     | masking_policy 应用于事件与日志 |
| **审计日志**     | trace_id 贯穿能力调用与代理链     |

---

## 13️⃣ 兼容性与演进路径

| 阶段     | 内容                                       |
| ------ | ---------------------------------------- |
| ✅ 当前   | Registry + Router + MCP/gRPC/HTTP 统一     |
| 🟡 下一步 | A2A AgentAdaptor 实现与治理启用                 |
| 🟢 后续  | 引入 MQ / GraphQL / Function 调用            |
| 🔵 长期  | Agent-native 生态，支持插件自定义传输                |
| ⚫ 最终   | PowerX = Unified Agent-Capability Fabric |

---

## 14️⃣ 与各子系统映射

| 模块                                         | 职责         |
| ------------------------------------------ | ---------- |
| `Capability_Contract_Spec.md`              | 定义能力契约语义   |
| `Transport_Adapter_Spec.md`                | 定义统一传输接口   |
| `Capability_Registry_and_Router_Design.md` | 运行时注册与选路   |
| `Workflow_and_Agent_Orchestration_Spec.md` | 编排与执行引擎    |
| `AgentAdaptor_and_Transport_Spec.md`       | A2A 实现层    |
| `Agent_Security_and_Isolation_Policy.md`   | 代理安全边界     |
| `Realtime_Streaming_Gateway.md`            | 事件流出口      |
| **PXIP-001**                               | 顶层标准与一致性宪章 |

---

## ✅ 一句话总结

> **PXIP-001 = PowerX 统一能力与传输规范。**
> 它确立了「一切能力可路由、一切传输可替换」的设计哲学，
> 将插件、智能体、协议与事件流整合为一致的调用平面（Unified Invocation Plane），
> 成为 PowerX Integration Framework 的核心宪章文件。
