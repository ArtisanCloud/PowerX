# Agent 直连外部 MCP 的实现步骤

本篇承接总览文档，聚焦“PowerX Agent 如何调用外部 MCP 工具”。文中给出两条可选路径，并指明需要改动的模块。

## 1. 现状复盘

| 模块 | 功能 | 代码位置 |
| --- | --- | --- |
| MCP Server | 启动时加载 Tool Spec，按 `HandlerType` 注册 `native`、`script`、`remote` 工具 | `internal/server/mcp/server.go`。【F:internal/server/mcp/server.go†L16-L104】 |
| Tool 工厂 | 将 `remote` 工具的调用透传到 `metadata.endpoint` 指定的 HTTP/MCP 服务 | `internal/server/mcp/register/factory/remote.go`。【F:internal/server/mcp/register/factory/remote.go†L1-L43】 |
| Agent 调度 | `internal/server/agent/manager_execute.go` 调度任务，调用具体 Agent 的 `Invoke` | `internal/server/agent/manager_execute.go`。【F:internal/server/agent/manager_execute.go†L1-L173】 |
| Eino 驱动 | 执行蓝图节点，目前没有内建 MCP 节点执行器 | `internal/server/agent/drivers/eino/agent.go`。【F:internal/server/agent/drivers/eino/agent.go†L1-L156】 |
| 外部 Agent 接口 | `contract.ExternalAgentClient` 定义外部平台调用方式 | `internal/server/agent/contract/external.go`。【F:internal/server/agent/contract/external.go†L8-L48】 |

## 2. 方案一：经由 PowerX MCP Server 转发

> 适用于希望继续统一管理工具目录的场景。

1. **在 Tool Spec 中声明远程端点**  
   - 在 `PowerX/mcp/tool_specs` 或配置加载的目录新增 `remote` Tool Spec，`metadata.endpoint` 指向外部 MCP/HTTP 服务。  
   - MCP Server 启动时会自动注册该工具。【F:internal/server/mcp/server.go†L16-L104】

2. **在 Agent 中添加 MCP 调用节点**  
   - 复用现有的 MCP HTTP 客户端：`internal/server/mcp/client/client.go` 封装了 `CallTool` 接口，可直接被节点执行器调用。【F:internal/server/mcp/client/client.go†L1-L84】  
   - 在 `internal/server/agent/drivers/eino/agent.go` 注册一个新的节点执行器（例如 `KindMCPCall`），从 Flow 节点参数读取 `tool_id`、`arguments`，再调用 MCP 客户端。

3. **执行链路**  
   Agent Flow 节点 → 新节点执行器 → MCP HTTP 客户端 → PowerX MCP Server → `remote` handler → 外部 MCP/HTTP 服务 → 返回结果。

**优点**：工具目录统一、鉴权一致，无需在 Agent 中处理外部鉴权细节。  
**注意**：需确保 `metadata` 中的认证信息（token、header）在 MCP Server 转发时正确注入，可通过扩展 `remote` handler 读取配置。

## 3. 方案二：Agent 直连外部 MCP

> 当外部 MCP 具备独立会话、流式响应等高级能力时，可在 Agent 内实现 MCP 客户端，绕过 PowerX MCP Server。

1. **实现 MCP 版 `ExternalAgentClient`**  
   - 推荐在 `internal/server/agent/drivers/eino/mcpclient/` 新建包，实现 `contract.ExternalAgentClient` 接口。  
   - `Invoke/InvokeAsync/Stream` 可基于 `internal/server/mcp/client` 或 `github.com/mark3labs/mcp-go/client` 发起 MCP `CallTool` 请求。  
   - `ListFlows/GetFlowInfo` 可返回 MCP 工具列表，或在不支持时返回 `contract.ErrNotSupported`。【F:internal/server/agent/contract/external.go†L8-L48】

2. **注册到 Agent Manager**  
   - 通过 `internal/server/agent/adapters/external_agent_bridge.go` 将 `ExternalAgentClient` 包装为 Agent，并在 `factory.NewAgentClient` 中注入。【F:internal/server/agent/adapters/external_agent_bridge.go†L1-L79】【F:internal/server/agent/factory/agent.go†L1-L36】  
   - 如需配置化，可在 `internal/server/agent/bootstrap/init.go` 中扩展 `AgentConfig`，加载 MCP 客户端的 endpoint、鉴权参数。【F:internal/server/agent/bootstrap/init.go†L1-L74】

3. **Flow 节点对接**  
   - 在蓝图中为 MCP 节点设置类型（如 `use: external_mcp.call`）。  
   - 执行器从节点参数获取 `tool_id`、`arguments`，调用上一步注册的 `ExternalAgentClient.Invoke` 并将结果转成 Flow 的 `Result`。

4. **异步/流式处理**  
   - 若外部 MCP 支持流式输出，可在 `ExternalAgentClient.Stream` 中转成 `aschemas.ExecutionResult` 增量；桥接器已有将外部流转成 Eino 流的逻辑，可复用。【F:internal/server/agent/adapters/external_agent_bridge.go†L25-L79】

**优点**：可利用外部 MCP 的高级能力与协议特性。  
**成本**：需要自行处理鉴权、错误重试与会话管理。

## 4. 方案对比表

| 维度 | 方案一：经 MCP Server 转发 | 方案二：Agent 直连外部 MCP |
| --- | --- | --- |
| 工具目录管理 | 统一在 PowerX MCP Server | 需要独立管理或同步目录 |
| 开发工作量 | 只需新增节点执行器 | 需实现完整的 MCP 客户端适配 |
| 鉴权与审计 | 复用 MCP Server 的机制 | 需自行实现 |
| 流式/异步能力 | 取决于 MCP Server 转发实现 | 可直接透传外部 MCP 能力 |
| 适用场景 | 工具集中治理、以 HTTP 调用为主 | 强依赖第三方 MCP 特性、需要绕过 PowerX MCP Server |

## 5. 推荐落地顺序

1. 先落地方案一，验证工具目录与调用链路是否满足需求；
2. 若需更强的协议能力，再按方案二扩展，并复用已有的桥接器/配置体系。

通过上述分层设计，可确保“主入口文档 + 具体方案”层层递进，便于团队成员根据角色快速定位到需要修改的代码与配置。
