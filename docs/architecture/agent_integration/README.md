# PowerX Agent × MCP × 插件：分层总览

为避免将所有调用链路塞进单一文档，本目录作为 Agent × MCP × 插件的主入口，先按照“编排层 → 工具注册层 → 能力托管层”逐级介绍 PowerX Agent、MCP Server 与插件的协作方式，并指向后续的细化文档与实现路径。

## 1. 分层一览

| 层次 | 关键模块 | 主要职责 | 入口/文件 |
| --- | --- | --- | --- |
| 编排层（Agent Server） | `internal/server/agent`、`internal/server/grpc` | 承载会话、蓝图执行，决定调用哪些工具或外部 Agent | gRPC 服务在 `internal/server/grpc/server.go` 注册；调度逻辑在 `internal/server/agent/manager_execute.go` |
| 工具注册层（MCP Server） | `internal/server/mcp` | 解析 `ToolSpec`，把 PowerX 内建、插件扩展及第三方能力统一暴露为 MCP 工具 | 入口在 `internal/server/mcp/server.go`；`register/factory` 负责按 `HandlerType` 路由 |
| 能力托管层（插件体系） | `internal/infra/plugin/manager`、`pkg/plugin_mgr` | 扫描插件、启动进程、将插件 API 通过 `/_p/{id}/**` 等路径代理出来 | 管理器位于 `internal/infra/plugin/manager/manager.go`，路由在 `internal/infra/plugin/manager/router/router.go` |

## 2. 调度关系速查表

| 调用方 | 目标能力 | 推荐路径 | 是否经过 MCP Server | 说明 |
| --- | --- | --- | --- | --- |
| PowerX 内部 Agent (Eino) | PowerX 内建工具 | 直接调用 MCP Server 注册的 `native/script` 工具 | ✅ | 统一的工具目录与鉴权；详见《MCP 工具编目与命名》文档 |
| PowerX 内部 Agent (Eino) | 插件封装能力（HTTP/gRPC） | 插件通过 `remote` Tool Spec 暴露为 MCP 工具，再由 Agent 调用 | ✅ | 插件仍由插件管理器托管，但实际执行透传到插件服务 |
| PowerX 内部 Agent (Eino) | 外部第三方 MCP/HTTP 服务 | 在 Tool Spec 中声明 `remote`，或实现直连 MCP 客户端 | ✅ / ⛔️ | 默认经 MCP Server；需要直连时可参考《Agent 直连外部 MCP》实现桥接 |
| 外部系统 / 第三方 Agent | 复用 PowerX Agent 蓝图 | 通过 gRPC `AgentStreamService` | ⛔️ | 外部 Agent 与 PowerX 会话编排解耦，MCP 调用仍由 PowerX Agent 内部决定 |
| 插件自身 | 调用 PowerX 基座能力 | 直接访问提供的 gRPC/HTTP SDK，或将调用封装成 MCP 工具供其它 Agent 使用 | 可选 | 详见《PowerX Agent 与插件关系》文档 |

## 3. 分层阅读指引

1. **[PowerX Agent 与插件关系](powerx_agent_plugin.md)**：阐述 Agent 与插件的主从关系、gRPC/HTTP 交互方式，以及如何在不经过 MCP 的情况下复用底座能力。
2. **[MCP 工具编目与命名规范](mcp_catalog.md)**：说明如何把官方、插件、自研与外部 MCP 工具放在同一目录下，避免混淆。
3. **[Agent 直连外部 MCP 的实现步骤](agent_external_mcp.md)**：提供两种落地方案（通过 MCP Server 转发 / 直接实现 MCP 客户端），对照代码模块给出操作步骤。
4. **[Agent 通信与协议蓝图](communicate.md)**：描述客户端与 Agent Server 之间的 WebSocket 与 SSE 契约，以及运行期状态机、重连与背压策略。

建议按顺序阅读，自上而下先理解角色划分，再进入具体实现。

## 4. 现有子方案

- [多 Agent 技术架构方案](multi_agent_architecture.md)：基于现有 Engine 拓展单任务多 Agent 协同能力，涵盖编排模型、数据结构、调度策略与演进计划。
- [多 Agent 产品体验方案](multi_agent_product_experience.md)：展示多 Agent 协作在前端的时间线呈现、关键交互及跨端响应式设计。

如后续有新的子方案，可继续在本目录下补充。

## 5. 相关模块速览

| 模块 | 关键路径 | 说明 |
| --- | --- | --- |
| Agent Runtime | `internal/server/agent/runtime` | 负责会话驱动、事件流输出，是多 Agent 调度的基础执行器 |
| Arena & PlanRunner | `internal/server/agent/arena` | 承载多 Agent 扩展后的会话上下文、Plan 调度、并行控制 |
| MCP / 插件集成 | `internal/server/agent/adapters`、`docs/plugins/` | 提供外部工具、MCP 插件接入方式，可作为 Agent 节点被编排 |
| 协议与通信 | `docs/architecture/agent_integration/communicate.md` | 定义客户端与 Agent Server 的 WebSocket / SSE 协议，扩展事件可参照多 Agent 方案 |

结合本目录与 `docs/plugins`、`docs/iam` 等文档，可系统了解 Agent 运行时、插件扩展与安全治理的整体架构。
