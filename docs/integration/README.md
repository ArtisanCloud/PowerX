# 📘 PowerX Integration Framework

> PowerX Integration 是 PowerX 核心「能力编排与多协议集成内核」
> 它统一了能力模型（Capability）、传输层（Transport）、
> 注册与选路（Registry / Router）、智能体编排（Agent / Workflow）与安全治理（Security / Governance）。
>
> 本文档集为 PowerX 的 **Integration 蓝图与规范手册全集**。

---

## 📖 文档总览与阅读顺序

| 编号                     | 模块目录             | 内容概要                                |
| ---------------------- | ---------------- | ----------------------------------- |
| **01_architecture**    | 架构与蓝图            | Integration 核心设计与整体生命周期             |
| **02_capability**      | 能力契约与传输规范        | 定义 Capability 与统一 Transport Adapter |
| **03_registry_router** | 注册中心与路由机制        | 能力注册、端点维护与 Router 选优逻辑              |
| **04_orchestration**   | 编排执行与状态流         | Workflow、Flow、Orchestrator、实时流规范    |
| **05_security**        | 安全与治理体系          | 平台治理、安全策略、能力授权与沙箱隔离                 |
| **06_gateway**         | 网关与消息总线          | MCP Gateway、EventBus、Admin API      |
| **07_plugin_sdk**      | 插件 SDK 与运行指南     | 插件开发、运行、调试与注册流程                     |
| **08_pxip**            | Integration 提案文档 | PXIP 架构标准与长期演进方案                    |
| **09_agent**           | Agent 智能体体系      | Agent 管理、编排、适配器与指标体系                |
| **README.md**          | 当前文件             | 文档体系索引与推荐阅读路径                       |

---

## 🧭 推荐阅读顺序（按角色 / 阶段）

| 阶段      | 角色                  | 推荐文档                                |
| ------- | ------------------- | ----------------------------------- |
| 🧱 架构设计 | 架构师 / 平台负责人         | ① Architecture → ⑧ PXIP             |
| ⚙️ 能力开发 | 后端 / Infra 团队       | ② Capability 系列                     |
| 🔁 集成调度 | CoreX Integration 组 | ③ Registry/Router → ④ Orchestration |
| 🧩 插件接入 | 插件与生态团队             | ⑦ Plugin SDK 系列                     |
| 🧠 智能编排 | Agent 研发组           | ④ Workflow → ⑨ Agent 系列             |
| 🔒 安全治理 | 安全 / 平台组            | ⑤ Security 系列                       |
| 🌐 网关运维 | Ops / 外部集成组         | ⑥ Gateway 系列                        |

---

## 🧱 模块概览

### 01. Architecture（架构蓝图）

| 文件                                   | 描述                                     |
| ------------------------------------ | -------------------------------------- |
| `PowerX_Integration_Architecture.md` | 描述 PowerX Integration 的整体拓扑、职责边界与运行流程。 |

---

### 02. Capability（能力模型与传输层）

| 文件                            | 描述                               |
| ----------------------------- | -------------------------------- |
| `Capability_Contract_Spec.md` | 定义统一能力契约（Capability Contract）模型。 |
| `Transport_Adapter_Spec.md`   | 描述多协议（MCP/gRPC/HTTP/Agent）传输适配层。 |
| _更新说明_ | 契约/传输规范新增契约版本策略、传输配置健康治理映射，配合 Admin API (`/api/v1/admin/capabilities/.../transports`) 可维护通道超时、重试、QoS 与健康检查。 |

---

### 03. Registry & Router（注册与路由）

| 文件                                         | 描述                         |
| ------------------------------------------ | -------------------------- |
| `Capability_Registry_and_Router_Design.md` | 能力注册、端点真相源与 Router 策略引擎设计。 |
| `Runtime_Endpoint_Management.md`           | 插件运行时端点与健康管理机制。            |

---

### 04. Orchestration（编排执行与状态流）

| 文件                                         | 描述                                        |
| ------------------------------------------ | ----------------------------------------- |
| `Flow_and_State_Model.md`                  | DAG/Graph 流程状态机模型与执行引擎。                   |
| `Orchestrator_Service_Interface.md`        | 编排服务接口规范（Workflow / Router / Adaptor 协作）。 |
| `Realtime_Streaming_Gateway.md`            | 实时流式推送通道（SSE / WS / EventBus）。            |
| `Workflow_and_Agent_Orchestration_Spec.md` | Workflow + Agent 混合编排规范（A2A 已融合）。         |

---

### 05. Security（安全与治理体系）

| 文件                                       | 描述                                |
| ---------------------------------------- | --------------------------------- |
| `Security_and_Governance.md`             | 平台总体安全治理架构、RBAC、审计、租户隔离。          |
| `Capability_and_Tool_Grants_Spec.md`     | 能力授权与 ToolGrant 授权模型（Agent 调用控制）。 |
| `Agent_Security_and_Isolation_Policy.md` | 智能体运行时沙箱与资源隔离策略（Agent 级执行安全）。     |

---

### 06. Gateway（消息与接口）

| 文件                                       | 描述                             |
| ---------------------------------------- | ------------------------------ |
| `MCP_Server_and_Gateway_Design.md`       | MCP Server / Gateway 结构与多租户通道。 |
| `EventBus_and_Message_Fabric.md`         | 内部事件总线与消息织体模型。                 |
| `Integration_API_and_Admin_Interface.md` | /api/v1/admin 控制接口规范与可视化管理入口。  |

---

### 07. Plugin SDK（插件开发与运行）

| 文件                               | 描述                    |
| -------------------------------- | --------------------- |
| `PowerX_Plugin_SDK_Guide.md`     | 插件开发规范、注册能力、通信协议与样例。  |
| `Plugin_Runtime_Guide.md`        | 插件运行态端口注入、生命周期与上下文说明。 |
| `Plugin_Test_and_Debug_Guide.md` | 本地调试、反连注册与端点诊断指南。     |

---

### 08. PXIP（Integration 提案与标准）

| 文件                                                      | 描述                    |
| ------------------------------------------------------- | --------------------- |
| `PXIP-001_Unified_Capability_and_Transport_Proposal.md` | 统一能力与传输提案（PXIP 宪章文件）。 |

---

### 09. Agent（智能体运行体系）

| 文件                                    | 描述                     |
| ------------------------------------- | ---------------------- |
| `Agent_Manager_and_Lifecycle_Spec.md` | 智能体注册、生命周期与调度机制。       |
| `AgentAdaptor_and_Transport_Spec.md`  | A2A 调用协议与传输适配设计。       |
| `Agent_Metrics_and_Observability.md`  | 智能体指标、追踪与可观测性。         |
| `Agent_Developer_Guide.md`            | Agent SDK / 调用规范与最佳实践。 |

---

## 🧩 Integration 内核结构（源码映射）

```
pkg/corex/integration/
├── registry/        # 能力真相源与运行态端点
├── router/          # 智能选路与打分策略
├── orchestrator/    # 编排入口：Workflow/Agent 调度
├── flow/            # DAG / Graph Runtime
├── transport/       # 多协议调用适配层 (MCP/gRPC/HTTP/Agent)
└── security/        # 安全上下文与治理策略
internal/
├── server/          # 对外暴露服务层 (grpc / agent / mcp)
└── infra/
    └── plugin/manager/  # 插件运行时管理与端口分配
```

---

## 🧠 关键术语索引

| 概念               | 描述                                 |
| ---------------- | ---------------------------------- |
| **Capability**   | 抽象业务能力（如 `crm.lead.create`）        |
| **Transport**    | 协议适配层（MCP/gRPC/HTTP/Agent）         |
| **Registry**     | 能力与端点真相源                           |
| **Router**       | 智能选路引擎                             |
| **Orchestrator** | 调度执行门面（Agent/Workflow）             |
| **Flow**         | DAG / Graph 执行引擎                   |
| **Agent**        | 智能体（可调用 / 可被调用）                    |
| **ToolGrant**    | Agent 调用授权清单                       |
| **EventBus**     | 内部事件总线与消息织体                        |
| **Gateway**      | 对外推送与通信网关                          |
| **Security**     | 全域安全治理策略与隔离体系                      |
| **PXIP**         | PowerX Integration Proposal（架构标准集） |

---

## 🧩 演进路线（Roadmap）

| 阶段       | 目标                                                                             |
| -------- | ------------------------------------------------------------------------------ |
| 🟡 当前进行中 | 统一 CoreX/integration 模块结构（Registry / Router / Orchestrator / Flow / Transport） |
| 🟢 下一步   | 完成 AgentAdaptor(A2A) 全栈实现 + 多维打分路由（延迟 / 成本 / 健康 / 会话）                          |
| 🔵 后续    | 集成异步总线（Kafka / NATS）与分布式 Workflow 运行时                                          |
| ⚫ 长期     | 构建 Agent-Native 生态（Plugin + Agent + Workflow 完全融合）                             |

---

## 总结

> **PowerX Integration Framework = 能力 × 协议 × 智能编排 × 安全治理。**
> 所有插件、智能体、第三方平台与核心服务都在这一统一语义下协作，
> 构建可路由、可追踪、可治理的 Agent-Native 集成生态。
