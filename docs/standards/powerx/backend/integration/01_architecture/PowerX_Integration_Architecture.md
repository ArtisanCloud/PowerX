# 📘 PowerX Integration Architecture

## 🧭 架构定位与蓝图

PowerX Integration Framework 是 PowerX 的统一编排内核，
负责整合 **插件能力 (Plugin Capabilities)**、**外部第三方平台 (MCP/GRPC/HTTP)** 与 **智能体调度 (Agent Orchestration)**。

核心思想：

> “一切能力皆可注册，一切调用皆经编排。”

PowerX 通过 **CoreX/integration 域** 提供统一的能力接入、注册、路由与编排执行机制，
实现「Agent 调度 → Orchestrator → Router → Transport → Flow」的完整闭环。

---

## 🧱 架构分层与职责

### 🧩 1. CoreX Integration（编排内核）

> 位于 `pkg/corex/integration`
> 所有能力的注册、选路、执行与编排都在此域完成。

| 模块                | 职责                                                  |
| ----------------- | --------------------------------------------------- |
| **registry/**     | 能力与运行态端点真相源。由 PluginManager 上报运行信息。                 |
| **router/**       | 负责多协议选路与打分决策（延迟 / 成本 / 健康 / 会话状态）。                  |
| **orchestrator/** | 编排门面，统一封装 Router / Transport / Flow / Event。        |
| **flow/**         | Workflow / DAG / Graph 执行引擎（基于 Eino Graph Runtime）。 |
| **transport/**    | 通信层抽象：`mcp / grpc / http / agent(A2A)` 四类协议适配器。     |

---

### 🧩 2. Server 层（智能服务入口）

> 位于 `internal/server/{agent|grpc|mcp}`
> 仅负责暴露接口、鉴权和调用 Orchestrator。

| 模块         | 职责                                     |
| ---------- | -------------------------------------- |
| **agent/** | 管理 Agent 生命周期与意图规划；执行委托给 Orchestrator。 |
| **grpc/**  | 提供 gRPC 服务端入口，供外部调用。                   |
| **mcp/**   | 提供 MCP Server / Client 能力，支持插件与外部生态注册。 |

---

### 🧩 3. Infra 层（运行时与宿主环境）

> 位于 `internal/infra/plugin/manager`
> 负责插件运行、安装、健康检查、反代与运行态端点上报。

| 模块                 | 职责                             |
| ------------------ | ------------------------------ |
| **plugin/manager** | 插件生命周期管理；端口分配；运行态上报至 Registry。 |

---

## ⚙️ 数据与控制流（简化流程）

```
Plugin / Third-Party
        │
        ▼
  PluginManager ──▶ Registry (runtime endpoints)
        │               │
        │               ▼
        │          Router (selects optimal path)
        │               │
        ▼               ▼
      Agent ───▶ Orchestrator ───▶ Transport (mcp/grpc/http/agent)
                                        │
                                        ▼
                                     Flow Engine
```

---

## 🔄 当前开发状态

PowerX 正在将分散于各模块的逻辑（Agent、Flow、Router、Registry、Transport）
全面整合入 **CoreX/integration 域**，以实现：

* 能力注册与执行的单一真相源；
* 统一编排调用链；
* 对多协议通信的无缝抽象。

> 当前阶段目标：完成 CoreX/integration 的模块聚合与接口统一。
> 下一阶段目标：完善 `transport/agent (A2A)` 协议层，实现跨 Agent 调度与多维路由打分。

---

## 🧩 演进路线（Roadmap）

| 阶段       | 目标                                                                                                |
| -------- | ------------------------------------------------------------------------------------------------- |
| 🟡 当前进行中 | 将 **Registry / Router / Orchestrator / Flow / Transport** 整合至 **CoreX/integration 域**，形成统一编排内核结构。 |
| 🟢 下一步   | 实现 **Transport/agent (A2A)**，Router 引入多维打分（延迟 / 成本 / 健康 / 会话状态）。                                  |
| 🔵 后续    | 集成异步任务总线（Kafka / NATS）与事件溯源，支持跨 Agent Workflow 并行执行。                                              |
| ⚫ 长期     | 优化多租户 Graph Runtime（分布式执行 + 弹性扩展）；完善统一观测体系（Tracing / Audit / Metrics）。                            |
| 🟣 最终愿景  | 构建 **Agent-Native 生态**：Agent 既可作为 Provider 也可作为 Consumer，通过 Transport 抽象互联。                       |

---

## 🔐 安全与合规概览

* 所有调用上下文包含：`tenant_id`、`actor_id`、`trace_id`
* 通信协议均支持安全通道（TLS / Token / IAM）
* 日志与调用全程可追踪（Trace）可审计（Audit）可观测（Metrics）
* 与 CoreX IAM / RBAC 深度集成，实现多租户安全隔离

---

## 🧩 关键设计理念

| 原则           | 说明                                            |
| ------------ | --------------------------------------------- |
| **一切能力皆可注册** | 每个插件或第三方服务的功能都以 Capability 形式注册。              |
| **一切调用皆经编排** | 所有调用都统一通过 Orchestrator 进入 Router / Transport。 |
| **协议可插拔**    | 支持 mcp / grpc / http / agent 多协议动态切换。         |
| **智能调度优先**   | Router 按健康、延迟、成本、负载智能打分选路。                    |
| **安全优先级高**   | 全链路租户隔离、IAM、审计、限流与加密通道。                       |

---

## 💡 总结

> PowerX Integration = 「统一能力模型 × 多协议抽象 × 智能编排 × 安全治理」

PowerX 不仅是插件运行的宿主，更是一个
**多协议智能编排平台**，
通过 CoreX/integration 的统一内核，将 **插件、智能体与第三方能力** 连接成一个完整的协作生态。
