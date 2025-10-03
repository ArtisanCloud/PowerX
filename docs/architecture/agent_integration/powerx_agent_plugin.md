# PowerX Agent 与插件关系

本篇围绕“PowerX Agent 与插件的主从关系”展开，阐明插件在体系中的定位、如何通过 gRPC/HTTP 访问底座能力，以及在何种情况下需要把插件能力转成 MCP 工具。

## 1. 角色定位

| 角色 | 职责 | 关键代码 |
| --- | --- | --- |
| Agent Server | 调度会话、执行蓝图节点，内部再决定调用插件或工具 | `internal/server/agent/manager_execute.go`。【F:internal/server/agent/manager_execute.go†L1-L173】 |
| 插件管理器 | 扫描插件包、启动进程、将插件 API 代理到 PowerX 主体进程 | `internal/infra/plugin/manager/manager.go`、`router/router.go`。【F:internal/infra/plugin/manager/manager.go†L16-L98】【F:internal/infra/plugin/manager/router/router.go†L1-L118】 |
| 插件进程 | 实际承载业务能力，可实现 REST/gRPC/前端页面 | 插件本身；PowerX 提供 `pkg/plugin_mgr/contract.go` 定义生命周期接口。【F:pkg/plugin_mgr/contract.go†L1-L24】 |

**主从关系总结**：Agent Server 始终是“主调度者”，插件是“能力提供者”。Agent 是否调用插件、如何调用，由 Flow 节点或工具配置决定。

## 2. 插件如何复用 PowerX 底座

1. **直接调用底座 gRPC/HTTP**  
   插件在运行时可直接使用 PowerX 暴露的 gRPC 服务或内部 SDK：
   - gRPC 入口由 `internal/server/grpc/server.go` 注册，包括 `AgentStreamService` 等接口。【F:internal/server/grpc/server.go†L44-L112】
   - 如果只需访问领域服务，可直接复用 `internal/server` 下已有的 REST/gRPC handler，或通过 `pkg` 中的客户端封装。  
   这种方式适用于插件本身就是一个完整的子系统，直接消费底座能力，不一定需要对外暴露 MCP 工具。

2. **封装为 MCP 工具供 Agent 调用**  
   当你希望 PowerX 其它 Agent 或外部系统通过统一的工具目录访问插件能力时，可在工具规范中声明 `remote`：
   - 插件对外暴露的 API（例如 `/_p/{plugin}/api/**`）由插件路由自动代理。【F:internal/infra/plugin/manager/router/router.go†L69-L118】
   - 在 `PowerX/mcp/tool_specs` 或业务自定义目录下编写 `remote` 类型 Tool Spec，并将 `metadata.endpoint` 指向上述 API。
   - MCP Server 启动时会将该工具注册到统一目录。【F:internal/server/mcp/server.go†L16-L104】

   这样 Agent 在执行 Flow 时只需调用 MCP 工具，不需要感知插件细节。

## 3. 常见调用形态对照

| 触发者 | 目标 | 是否需 MCP 工具 | 说明 |
| --- | --- | --- | --- |
| Agent Flow 节点 | 插件 API | 可选 | Flow 可直接调用插件 API（需自定义节点执行器），或走 MCP 工具目录。 |
| 插件 → PowerX 底座 | 领域服务/gRPC | 否 | 插件直接调用底座即可，通常通过内部 SDK 或 gRPC 客户端。 |
| 外部系统 → 插件 | 插件对外服务 | 否 | 可由插件自身暴露 HTTP/gRPC，无需经过 Agent。 |
| 外部系统 → PowerX Agent | 插件能力 | 推荐 | 外部系统发起会话给 Agent，由 Agent 再去调用插件对应的 MCP 工具，保持治理一致。 |

## 4. 何时不建议转换为 MCP

- 插件只面向自身前端页面或第三方系统，且调用链稳定、不需要被 PowerX 其它 Agent 使用。  
- 插件调用底座能力仅发生在内部，不需要通过 MCP 暴露给平台其他租户。

在这些场景下，保持插件与底座的直接调用更简单，避免冗余注册流程。

## 5. 何时必须转换为 MCP

- 需要纳入统一的“工具市场”或在 Admin 中集中管理启停、权限。  
- 让 PowerX Agent、外部 Agent 或自动化流程共享同一套调用规范时。

转换步骤详见《Agent 直连外部 MCP 的实现步骤》中“方案一：通过 MCP Server 转发”。
