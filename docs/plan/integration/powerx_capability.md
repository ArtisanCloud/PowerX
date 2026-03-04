# PowerX Capability Exposure Plan

## 背景

为支持宿主模式与 Skeleton 模式的插件统一调用 PowerX 核心能力，需要将底座模块的开放接口纳入 "Integration Gateway & MCP" 专题管理。当前 Media、事件总线、定时任务、AI 知识库、Workflow 等能力大多仅通过 Admin API 暴露，导致插件需依赖内部路由。此计划旨在提供统一的 HTTP/OpenAPI 与 gRPC 契约，使任何插件或第三方在拿到授权后即可调用底座能力。

## 建议方案

1. **Registry 扩展**：在 Capability Registry 中新增 `source=corex|plugin` 字段，并预置 Media、Event Fabric、Scheduler、Knowledge、Workflow 等 CoreX 能力记录，统一走 Tool Grant 与限流策略。
2. **对外契约**：每个底座模块维护 `specs/<module>/contracts/http-openapi.yaml` 与 `backend/api/grpc/contracts/<module>/v1/*.proto`，Integration Gateway 以这些契约为源生成 SDK 和文档。
3. **统一调用入口**：第三方通过 `/tenant/capabilities` 与 `/tenant/invocations`（或 gRPC `IntegrationGatewayTenantService`）调用底座能力；宿主模式可继续使用 Admin API 进行配置，但实际能力调用全部归口 Integration Gateway。
4. **观测与治理**：沿用 FR-001~FR-015 的追踪/限流/审计要求，对平台能力与插件能力实施一致的 metrics/audit/event 采集。
5. **媒资公开 API**：PowerX 底座的 **Media Assets Management** 模块已在 `specs/001-docs-media-storage/contracts/http-openapi.yaml` 提供 `{APIPrefix}/media/assets` 路径，包含上传、列表、详情、软删、预签名能力；插件（宿主或 Skeleton）只需携带 Bearer Token（租户由 JWT claims 提供）即可直接调用，对应调用流程记录在本计划与 Quickstart 中。
6. **Agent & 多模态统一开放**：补齐 Agent 运行时与多模态模型调用的对外接口标准（REST/SSE/gRPC/SDK），并将租户隔离、流式协议与幂等错误码纳入统一规范（见下文“Agent 能力开放计划”“多模态模型调用标准”与 `specs/007-integration-gateway-and-mcp/spec.md` 的 FR-019~FR-020）。

## 已内置的平台能力（2025.01）

| Capability ID | 模块 | 访问场景 | 协议/入口 |
| --- | --- | --- | --- |
| `com.corex.media.assets.read` | Media | 媒资列表、详情查询 | REST：`GET {APIPrefix}/media/assets`、`GET {APIPrefix}/media/assets/{uuid}`；gRPC：`powerx.media.v1.MediaAssetAdminService/List|Get` |
| `com.corex.media.assets.manage` | Media | 上传、删除、预签名 | REST：`POST/DELETE {APIPrefix}/media/assets`、`POST {APIPrefix}/media/assets/{uuid}/presign`；gRPC：`Create/Delete/PresignMediaAsset` |
| `com.corex.eventfabric.publish` | Event Fabric | 事件发布 & 订阅 | gRPC：`powerx.event_fabric.v1.EventDeliveryService/PublishEvent`、`EventSubscriberService/Subscribe` |
| `com.corex.scheduler.jobs` | Workflow Scheduler | Workflow/Scheduler 实例触发、暂停、继续 | gRPC：`powerx.workflow.v1.WorkflowService/StartInstance`、`ControlInstance`、`ListInstances` |
| `com.corex.workflow.builder` | Workflow Builder | 模板创建、发布、查询 | gRPC：`powerx.workflow.v1.WorkflowService/CreateDefinition`、`PublishDefinition`、`ListDefinitions` |
| `com.corex.knowledge.space` | Knowledge Space | 空间/策略/增量管理 | gRPC：`powerx.knowledge.v1.KnowledgeSpaceAdminService/Create|Update|TriggerIngestion` |

## Agent 能力开放计划（统一对外标准）

### 目标与范围

1. **对外能力范围**：Agent 对话与执行（非流式 + SSE/WS 流式）、Session 管理、消息查询；统一支持 REST + SSE + gRPC + SDK。
2. **租户隔离**：Agent 只能访问本租户 Agent 资源；`agent_id` 必须属于 JWT claims 指定租户，跨租户访问直接拒绝（403）。
3. **统一授权**：全部走 Tool Grant / Tenant JWT；Skeleton/Plugin 通过 `PX_GATEWAY_BASE_URL` 与 tenant token 调用 `/tenant/invocations` 或开放 REST。

### 能力映射（计划新增到 Registry）

| Capability ID | 模块 | 场景 | 协议/入口 |
| --- | --- | --- | --- |
| `com.corex.agent.stream` | Agent | SSE/WS 流式聊天 | REST：`GET {APIPrefix}/agents/stream/sse`，WS：`GET {APIPrefix}/agents/stream/ws`；gRPC：`powerx.agent.v1.AgentStreamService/Stream` |
| `com.corex.agent.invoke` | Agent | 非流式调用 | REST：`POST {APIPrefix}/agents/invoke`；gRPC：`powerx.agent.v1.AgentInvokeService/Invoke` |
| `com.corex.agent.session.manage` | Agent | Session CRUD/消息读取 | REST：`/agents/sessions*`；gRPC：`powerx.agent.v1.AgentSessionService/*` |

> 注：当前 Agent API 已存在于受保护路由（`/agents/stream/sse`、`/agents/invoke` 等）。本计划要求补齐 **公开契约** 与 **Integration Gateway 路由**，并补强租户隔离校验与审计链路。

### 接口标准（建议）

1. **REST（非流式）**：
   - `POST /agents/invoke`
   - 入参（JSON）：
     ```json
     {
       "agent_id": "uuid",
       "session_id": "uuid|optional",
       "message": "用户输入文本",
       "attachments": [{"type":"image_url","url":"https://..."}],
       "meta": {"trace_id":"", "user_id":""}
     }
     ```
   - 返回（JSON）：`session_id`、`reply`、`usage`、`agent_id`
2. **SSE（流式）**：
   - `GET /agents/stream/sse?q=...&agent_id=...&session_id=...`
   - 事件规范：`start/meta/token/final/end/error`（对齐现有 SSE 事件）
3. **WS（控制通道）**：
   - `GET /agents/stream/ws?authorization=Bearer xxx`
   - 控制：cancel/heartbeat/subscribe（SSE 仅负责输出）
4. **gRPC**：
   - `AgentStreamService/Stream`：服务端单向流
   - `AgentInvokeService/Invoke`：普通请求-响应

### Session 语义（Agent）

1. **会话创建**：
   - `POST /agents/sessions`（明确创建）
   - `POST /agents/invoke` 或 `GET /agents/stream/sse` 若不传 `session_id`，自动创建并返回 `session_id`。
2. **消息管理**：
   - `GET /agents/sessions/:id/messages` 获取消息列表
   - 消息写入：由 invoke/stream 自动写入
3. **会话隔离**：
   - 校验 `session_id` 必须属于当前租户且关联 `agent_id`；不满足返回 403。
4. **持久化策略**：
   - SSE/WS 流式输出过程中，`final` 时写入消息，`token` 不落库（避免写放大）。

### 具体接口与错误码（Agent）

1. **`POST /agents/invoke`**
   - 请求：
     ```json
     {
       "agent_id": "uuid",
       "session_id": "uuid|optional",
       "message": "你好，帮我总结这份文档",
       "attachments": [{"type":"image_url","url":"https://.../a.png"}],
       "meta": {"trace_id":"", "user_id":""}
     }
     ```
   - 响应：
     ```json
     {
       "session_id": "uuid",
       "agent_id": "uuid",
       "reply": "总结内容...",
       "usage": {"prompt_tokens":123,"completion_tokens":456,"total_tokens":579}
     }
     ```
2. **`GET /agents/stream/sse`**
   - Query：`q`、`agent_id`、`session_id`（可选）
   - SSE 事件序列（建议）：
     ```
     event: start
     data: {"session_id":"...","agent_id":"...","request_id":"..."}

     event: meta
     data: {"model_key":"...","trace_id":"..."}

     event: token
     data: {"text":"分段输出..."}

     event: final
     data: {"message":"完整输出","usage":{...}}

     event: end
     data: {"ok":true}
     ```
3. **`GET /agents/stream/ws`**
   - 控制消息示例：
     ```json
     {"type":"cancel","request_id":"..."}
     ```
4. **`POST /agents/sessions`**
   - 请求：`{"agent_id":"uuid","title":"optional"}`
   - 响应：`{"session_id":"uuid"}`
5. **`GET /agents/sessions/:id/messages`**
   - 返回：消息列表（role + content）
6. **统一错误码（HTTP）**：
   - `agent_not_found`（404）
   - `agent_not_allowed`（403）
   - `session_not_allowed`（403）
   - `stream_not_supported`（400）
   - `rate_limited`（429）

> 对应契约文件：`specs/007-integration-gateway-and-mcp/contracts/agent.http-openapi.yaml`；gRPC：`backend/api/grpc/contracts/powerx/agent/v1/agent_api.proto`。

### 实施任务（Agent）

1. **契约补齐**：新增/更新 `specs/agent/contracts/http-openapi.yaml` 与 `backend/api/grpc/contracts/powerx/agent/v1/*.proto`。
2. **Gateway 路由**：在 Integration Gateway 增加 `source=corex` 的 Agent 能力路由，支持 SSE/WS 透传或代理。
3. **租户校验**：在 Agent handler/service 层校验 `agent_id` 所属租户，并写入审计事件。
4. **统一错误码**：`agent_not_found`、`agent_not_allowed`、`session_not_allowed`、`stream_not_supported`。

## 多模态模型调用标准（按模态拆分）

### 目标与范围

1. **统一 URI 规则**：按模态拆分为 `/ai/{modality}/invoke`，LLM 额外提供 session + stream。
2. **租户模型隔离**：仅允许调用**本租户**已配置或已测试通过的模型/Provider（AI Settings active profile 或同租户已测试通过且凭据已保存的 provider）。
3. **开放调用入口**：REST + SSE + gRPC + SDK 同步，所有调用进入 Invocation Trace 与审计链路。

### 能力映射（计划新增到 Registry）

| Capability ID | 模块 | 场景 | 协议/入口 |
| --- | --- | --- | --- |
| `com.corex.ai.llm.invoke` | AI | LLM 无状态调用 | REST：`POST {APIPrefix}/ai/llm/invoke`；gRPC：`powerx.ai.v1.MultimodalService/Invoke` |
| `com.corex.ai.llm.session.create` | AI | LLM 会话创建 | REST：`POST {APIPrefix}/ai/llm/sessions`；gRPC：`powerx.ai.v1.MultimodalSessionService/CreateSession` |
| `com.corex.ai.llm.session.append` | AI | LLM 会话追加 | REST：`POST {APIPrefix}/ai/llm/sessions/{id}/messages`；gRPC：`powerx.ai.v1.MultimodalSessionService/AppendMessage` |
| `com.corex.ai.llm.stream` | AI | LLM 会话流式 | SSE：`GET {APIPrefix}/ai/llm/sessions/{id}/stream`；gRPC：`powerx.ai.v1.MultimodalService/Stream` |
| `com.corex.ai.image.invoke` | AI | 图像调用 | REST：`POST {APIPrefix}/ai/image/invoke`；gRPC：`powerx.ai.v1.MultimodalService/Invoke` |
| `com.corex.ai.video.invoke` | AI | 视频调用 | REST：`POST {APIPrefix}/ai/video/invoke`；gRPC：`powerx.ai.v1.MultimodalService/Invoke` |
| `com.corex.ai.tts.invoke` | AI | 语音合成 | REST：`POST {APIPrefix}/ai/tts/invoke`；gRPC：`powerx.ai.v1.MultimodalService/Invoke` |
| `com.corex.ai.embedding.invoke` | AI | 向量生成 | REST：`POST {APIPrefix}/ai/embedding/invoke`；gRPC：`powerx.ai.v1.EmbeddingService/Embed` |

### 请求规范（建议）

1. **统一请求体**：
   - `model_key`: 可显式传入；优先使用传入值，若省略则回退到租户 Active Profile
   - `inputs`: 结构化输入（LLM/Image/Video/TTS 使用 `ContentItem`，Embedding 使用 `string[]`）
   - `params`: 采样/温度/最大 token 等（可选）
2. **鉴权与隔离**：
   - Header：`Authorization: Bearer <tenant token>`（必需）；租户仅从 JWT claims 解析，不支持租户 header fallback。
   - 校验规则（**仅限当前租户**）：
     - 若 `model_key` 对应的 **AI Model Profile** 已存在，则允许调用
     - 若未建立 Profile，但该 provider 在当前租户下 **测试连接成功（health=healthy）且凭据已保存**，也允许调用
     - 以上规则均不得跨租户复用；其他租户 token 无法访问本租户配置
3. **流式输出**：
   - SSE：`data:` 事件输出 token/chunk，`final` 结束
   - gRPC：server-streaming chunk

### 多模态调用模式（必须区分）

1. **无状态调用（Stateless）**：
   - 适用：Embedding、OCR、单轮图像理解、短文本问答、TTS
   - API：`POST /ai/{modality}/invoke`（`llm|image|video|tts|embedding`）
   - 请求需包含完整上下文，不依赖 session
2. **LLM 有状态对话（Sessioned）**：
   - 适用：多轮对话、长上下文
   - API：`POST /ai/llm/sessions`（创建）
   - `POST /ai/llm/sessions/:id/messages`（追加消息）
   - `GET /ai/llm/sessions/:id/stream`（SSE 流式）
3. **会话隔离**：
   - `session_id` 必须属于当前租户；`model_key` 必须满足“租户模型隔离”规则
4. **消息结构（建议，LLM）**：
   - `role`: `system|user|assistant|tool`
   - `content`: `[{type:"text", text:"..."}, {type:"image_url", url:"..."}]`
   - `tool_calls` 与 `tool_results` 统一记录在消息内

### 具体接口与错误码（多模态）

1. **LLM 无状态调用**
   - `POST /ai/llm/invoke`
   - 请求：
     ```json
     {
       "model_key": "ollama/llama3",
       "inputs": [{"type":"text","text":"帮我总结一下"}],
       "params": {"temperature":0.2,"max_tokens":512}
     }
     ```
   - 响应：
     ```json
     {
       "output": {"type":"text","text":"总结内容..."},
       "usage": {"prompt_tokens":123,"completion_tokens":456,"total_tokens":579}
     }
     ```
2. **LLM 会话创建**
   - `POST /ai/llm/sessions`
   - 请求：`{"model_key":"...", "title":"optional"}`
   - 响应：`{"session_id":"uuid"}`
3. **追加消息**
   - `POST /ai/llm/sessions/:id/messages`
   - 请求：
     ```json
     {
       "role": "user",
       "content": [
         {"type":"text","text":"这张图是什么"},
         {"type":"image_url","url":"https://.../a.png"}
       ]
     }
     ```
4. **流式输出**
   - `GET /ai/llm/sessions/:id/stream`
   - SSE 事件序列（建议）：
     ```
     event: start
     data: {"session_id":"...","request_id":"..."}

     event: token
     data: {"text":"分段输出..."}

     event: final
     data: {"message":"完整输出","usage":{...}}

     event: end
     data: {"ok":true}
     ```
5. **嵌入向量**
   - `POST /ai/embedding/invoke`
   - 请求：`{"model_key":"...","inputs":["a","b","c"]}`
   - 响应：`{"vectors":[[...],[...],[...]],"usage":{...}}`
6. **统一错误码（HTTP）**：
   - `model_not_configured`（400）
   - `model_not_allowed`（403）
   - `provider_unreachable`（502）
   - `modality_not_supported`（400）
   - `rate_limited`（429）

> 对应契约文件：`specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml`；gRPC：`backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto`。

### 实施任务（多模态）

1. **契约补齐**：更新 `specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml`（按模态拆分）与 `backend/api/grpc/contracts/powerx/ai/v1/*.proto`。
2. **Provider 适配层**：在 `agent/intent` 或独立 `ai` 服务中抽象 provider 调用，复用租户 profile 的模型配置。
3. **Invocation 归口**：全部走 Integration Gateway 计费/限流/审计。
4. **错误码**：`model_not_configured`、`provider_unreachable`、`modality_not_supported`、`rate_limited`。

## 宿主 vs Skeleton 调用流程

1. **统一发现入口**：无论插件运行模式如何，均需调用 `/tenant/capabilities?source=corex`（或 gRPC `ListTenantCapabilities`）列出平台能力。Host 模式可直接复用宿主 Web Admin 已注入的 TENANT Token；Skeleton 模式通常通过 STS/Service Account 获取租户级 JWT，两者都通过 JWT claims 提供租户信息。
2. **Invocation 请求**：插件向 `/tenant/invocations`（或 gRPC `InvokeCapability`）发送 `CapabilityInvokeRequest`。当 `capability_id` 属于 `source=corex` 时，Selector 会将调用路由到 Media/Event/Workflow 等底座模块，同时写入 `InvocationTrace` 与 `integration.gateway.invocation.*` 事件。
3. **OpenAPI 直连**：部分平台能力（例如 Media）还暴露 `{APIPrefix}/media/assets` 等公开路径，允许插件在确认 Tool Grant 后直接调用。`APIPrefix` 默认 `/api/v1`，可在 `cfg.Server.APIPrefix` 中改为 `/api/admin/v1` 或 `/api/v2`，调用时只需要租户 Token。
4. **Insomnia/cURL 模板**：
   - cURL 上传 Media：
     ```bash
     curl -X POST "$POWERX_BASE_URL/media/assets" \
          -H "Authorization: Bearer $TENANT_TOKEN" \
          -F "file=@samples/logo.png"
     ```
   以上流程在宿主模式（插件嵌入式）与 Skeleton 模式（独立服务）保持一致，只是 Token 获取来源不同。

## 管理端“开放能力”页面（T057）

- **入口**：Web Admin “设置 > 开放能力”，仅 `IsRoot` 管理员可见。页面进入后自动拉取 Registry 中 `source=corex` 能力。
- **模块分组**：基于能力的 `module` 字段生成卡片（Media/Event/Scheduler/Knowledge/Workflow 等），显示能力数量、最新 `capabilities_hash`、支持协议徽章。
- **能力明细**：展开后可查看 `capability_id`、描述、协议列表与最新状态（active/disabled），同时提供复制 cURL/Insomnia/MCP Tool snippet 的按钮，以及跳转到 OpenAPI/gRPC 文档或 `/media/assets` 等公开接口的链接。
- **刷新机制**：提供“立即同步”按钮触发再拉取，如需实时也可订阅 `capability.catalog.sync_*` 事件刷新；所有读取动作写入审计日志。
- **用途**：该页面成为宿主/Skeleton 插件开发者的官方入口，可直接复制调试示例并了解各模块提供的底座能力，减少依赖内部路由的情况。

## 里程碑

- **M1 (Media)**：将 Media Assets Management 能力纳入 Registry，发布对外 OpenAPI/gRPC 契约，支持 `/tenant/invocations` 把文件上传/预签名请求路由到 Media Service。
- **M2 (Event Fabric & Scheduler)**：为事件广播与定时任务输出能力记录，公开订阅/触发接口以及安全策略。
- **M3 (Knowledge & Workflow)**：Knowledge Space、Workflow Builder/Engine 对外提供模板查询与执行能力，确保 Skeleton 插件能直接调用。
- **M4 (统一 SDK/Docs)**：Integration Gateway 汇总上述契约生成 SDK 与统一说明书，覆盖宿主与 Skeleton 场景。

## 风险与缓解

- **契约漂移**：通过 Buf/OpenAPI 验证与 CI contracts-test 防止接口不一致。
- **鉴权复杂度**：复用现有 Tool Grant / STS / Tenant Auth 体系，避免重复造轮子。
- **兼容性**：在 Admin API 保留原有路径，逐步引导插件迁移到 Integration Gateway 调用链。

## Next Steps

1. 更新 `specs/007-integration-gateway-and-mcp/spec.md`，纳入 Base Capability Exposure Roadmap（已完成）。
2. 为 Media/Event/Workflow 等模块补充对外 OpenAPI/gRPC 契约，并在 Registry 中登记 `source=corex` 能力。
3. 扩展 Integration Gateway Handler/Service，支持 1P 能力的调用路由与治理策略。
