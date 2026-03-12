# Feature Specification: Integration Gateway & MCP Server（多插件能力对齐）

**Domain Ownership**: CoreX (`corex.agent`)

**Feature Branch**: `007-integration-gateway-and-mcp`  
**Created**: 2025-10-21  
**Status**: Draft  
**Input**: 基于《docs/plan/ai_engineering/multi_plugin_capability_guide.md》的对齐要求

## 背景与对齐

- 插件在安装阶段会提交 `capabilities/*.yaml` 与 `contracts/exposure/*`，主站需作为**单一事实来源**消费这些目录，避免 Integration Gateway、Agent Hub、Workflow Builder 出现“各自维护”的调用逻辑。
- 目标是在 `.pxp` 同步后 ≤3 分钟，让 **管理端/租户 API、Agent Hub MCP Server、Workflow Builder Catalog** 同步展示最新能力，并可根据 `capability.policy.prefer` 自动切换 MCP/gRPC/Workflow 协议。
- Selector、Integration Gateway、Workflow Engine、Capability Registry 必须共享相同的 schema、追踪字段、事件命名与限流策略，保证调用链可观测、可回溯、可治理。

## Clarifications

### Session 2025-12-15

- Q: 插件更新 `capabilities_hash` 时已发布 Workflow/Agent 入口是否应自动升级？ → A: 默认锁定旧版本，需管理员显式确认升级。

### Session 2026-02-24

- Q: Integration Gateway 全入口鉴权如何统一？ → A: 单请求单凭证分流（`Authorization: ApiKey` 或 `Authorization: Bearer`），不做失败回退。
- Q: 是否只改 internal/ws-bus？ → A: 否，Gateway 全部 OpenAPI 入口统一改造。
- Q: API Key 是否仅 root 可用？ → A: 否，租户可维护本租户 key；root/admin 租户可配置更高 scope。

### Session 2026-02-26

- Q: 仅凭 JWT 签名有效是否足够？ → A: 不足。必须增加“主体状态校验”（tenant/user/member）防止 DB refresh、租户变更、主体禁用后的旧 token 继续通过第一层鉴权。
- Q: 主体状态校验会不会拖慢请求？ → A: 采用 `cache-first + DB-fallback`；缓存短 TTL + 事件失效，保证性能与一致性平衡。
- Q: 是否要支持强制失效所有旧 token？ → A: 支持。引入 `session_version`（或 `token_epoch`）并纳入 claims，对比不一致即拒绝。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 3 分钟能力目录同步与治理 (Priority: P1)

作为平台能力运营者，我希望插件提交 `.pxp` 或执行 `capabilities submit` 后，Registry 在 3 分钟内完成校验、落库、缓存与事件广播，让 Integration Gateway Admin/Tenant API、Agent Hub MCP Server 与 Workflow Catalog 自动显示并引用该能力。

**Why this priority**: 没有统一能力目录，租户无法知道可用能力，Agent/Workflow 也无从路由，整体价值为零。

**Independent Test**: 仅部署 Capability Sync Worker + Registry + Admin/Tenant API，模拟插件提交后验证 Admin API 查询、Tenant API `GET /capabilities`、MCP `/tools/list` 均出现新能力且协议字段一致。

**Acceptance Scenarios**:

1. **Given** 插件包包含 `capabilities/*.yaml`、`contracts/exposure/mcp-tools.json`、`proto` 与 Workflow 模板，**When** Worker 成功解析并写入 Registry，**Then** Admin/Tenant API 和 MCP `/tools/list` 均展示统一 `capability_id/plugin_id/protocols`，且在 3 分钟内完成。
2. **Given** 插件更新 `tool_scope` 或 `protocol_hash`，**When** Registry 生成新的 `capabilities_hash` 并广播 `capability.catalog.sync_*` 事件，**Then** Agent Hub ToolStore、Workflow Catalog 均刷新其缓存并记录版本。

---

### User Story 2 - Agent Hub 多协议 Selector 自动路由 (Priority: P1)

作为 Agent Hub/Selector 维护者，我希望根据 Registry 输出的 `intents/tool_scope/policy` 自动匹配多插件能力，并在 MCP/gRPC/Workflow 通道之间执行优先级和故障切换，确保多插件并行调用与治理一致。

**Why this priority**: 多插件意图识别与协议选择失败会直接导致 Agent 无法响应租户意图或写入操作失控。

**Independent Test**: 启动 Agent Hub、Selector 与两个示例插件；通过 `intent_router` 发起读写请求，验证 MCP 优先、写请求强制 gRPC、Workflow 节点调度与 fallback 逻辑都被执行并产生一致事件。

**Acceptance Scenarios**:

1. **Given** Registry 中某能力 `policy.prefer=mcp` 且 `type=read`，**When** Agent Hub 接到对应意图，**Then** Selector 首先使用 MCP Adapter，若在超时时间内失败则自动切换至 gRPC Adapter 并记录 fallback 事件。
2. **Given** 另一能力 `policy.prefer=grpc` 或被识别为写操作，**When** Agent Hub 触发调用，**Then** Selector 直接走 gRPC，并把 `idempotency_key/tenant_uuid/trace_id` 注入请求及审计日志。

---

### User Story 3 - Workflow Builder 引入插件 Workflow/Composite 模板 (Priority: P2)

作为 Workflow Builder 用户，我希望从 Registry 导入插件提供的 `workflow_template_ref` 与 `composite.graph`，在 UI 中拖拽节点并在执行时沿用 Selector 规则，使复合任务能够复用插件定义的步骤与协议。

**Why this priority**: 没有模板导入，租户无法复用插件提供的复杂流程，复合任务价值无法兑现。

**Independent Test**: 仅启用 Workflow Catalog/Builder/Engine，加载插件 Workflow，编排并执行一次，确认节点协议与 Agent Hub 使用的能力一致、可观测字段完整，失败时可触发补偿。

**Acceptance Scenarios**:

1. **Given** 插件导出 `contracts/exposure/workflow/*.json`，**When** Workflow Builder 同步 Registry 后渲染节点，**Then** 用户可选择节点、查看 schema、配置参数并持久化编排。
2. **Given** Workflow Engine 执行该编排，**When** 某节点声明 `params.protocol=mcp|grpc`，**Then** Engine 调用相同 Selector，并对 MCP/REST 并发读、gRPC 写入、Workflow fallback 等策略进行可观测打点。

---

### Edge Cases

- 插件提交的 `capability_id` 与 Registry 已存在的 ID 冲突 → Worker 必须拒绝并在事件中返回冲突详情。
- 插件缺失声明的协议资产（如 `mcp-tools.json` 引用缺失）→ Sync 失败并触发 `capability.catalog.sync_failed`，既有能力不受影响。
- 多插件共享同一 `tool_scope` → Selector 需要依据 `intent + tenant policy` 选择最优能力并记录决策来源，避免随机漂移。
- MCP Session 中断或心跳超时 → Selector 自动切换至 gRPC 并标记 Session 不健康，直至 `capabilities_hash` 与插件恢复一致。
- Workflow 节点依赖的插件版本回滚 → Builder/Engine 需检测 hash 变化并提示租户重新发布或选择 fallback 节点。
- 事件总线不可用 → 调用结果仍需返回，但事件需写入补偿队列，在恢复后补发并对失败次数打点。

### User Story 4 - Admin 开放能力总览（Priority: P2）

作为平台超级管理员（`IsRoot`），我希望通过 Web Admin “设置 > 开放能力” 页面实时查看 PowerX 底座开放的所有模块能力，包括协议类型、调用入口与调试示例，以便将宿主/Skeleton 插件引导到正确的统一接口。

**Why this priority**：Base Capability Exposure 已在 Registry 中管理 `source=corex` 能力，但没有一个面向管理员的可视化总览，难以及时告知插件开发者有哪些官方能力、对应协议与调试方法。

**Independent Test**：仅启动 Web Admin + Capability Registry，手动或通过 Mock 数据写入 `source=corex` 能力。使用 `IsRoot` 账号访问“设置 > 开放能力”，验证页面按模块聚合能力数量，并能展开查看协议标签、OpenAPI/gRPC/MCP 调试入口；使用非 Root 账号则看不到菜单。

**Acceptance Scenarios**：

1. **Given** Registry 中存在多条 `source=corex` 能力记录，`module` 字段分别为 `Media`、`EventFabric`、`Workflow`，**When** `IsRoot` 管理员进入“设置 > 开放能力”，**Then** 列表按模块归类展示“能力数量”“支持协议（REST/gRPC/MCP/Workflow）”“最新 `capabilities_hash`”，并提供复制 cURL/Insomnia/MCP Tool 名称的调试按钮。
2. **Given** 普通租户管理员（非 Root）访问“设置”菜单，**When** 渲染侧边栏，**Then** “开放能力”入口不会出现，避免无授权用户访问底座能力说明。
3. **Given** Registry 新增/下架某平台能力，**When** Admin 页面刷新或触发“立即同步”，**Then** 列表 1 秒内更新数据，并通过 Badge 标记“新”或“已下线”状态；点击“查看契约”可打开对应 OpenAPI/gRPC 文档链接。
4. **Given** 管理员点击“复制 `tenant/invocations` Snippet”，**When** 页面调用 Selector 提供的模板，**Then** 复制的内容包含 `capability_id`、`preferred_protocol`、租户来源说明（JWT claims）等必要字段，方便宿主/Skeleton 插件直接调用。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Capability Sync Worker 必须解析 `.pxp/capabilities/catalog.json`、`capabilities/*.yaml` 与 `contracts/exposure/*`，校验引用完整性、生成 `capability_id/plugin_id/protocols/tool_scope/workflow_template_ref/composite.graph/protocol_hash`，并将记录写入 Postgres 与 Redis 缓存，确保从插件提交到 Registry 可被查询的时间 ≤3 分钟。
- **FR-002**: Registry 必须提供统一的查询 API（Admin/Tenant/Agent/Workflow 均使用），支持按 `tenant/tool_grant/protocol/channel` 过滤，返回一致的 schema 与 `capabilities_hash` 版本号，任何手工维护的能力条目一律禁止写入。
- **FR-003**: Integration Gateway Admin API 在展示或配置能力时必须直接引用 Registry 输出的 `capability_id` 及协议矩阵，禁止手工填写协议/Schema；当管理员需要给租户暴露能力时，系统必须自动填充协议参数、速率策略与事件策略，确保 UI/CLI 不存在“脱离 Registry 的自由输入”。
- **FR-004**: Tenant API 与 MCP Server 在列举和调用能力时必须直接读取 Registry 缓存，套用租户授权（Tool Grant、Feature Flag、Safe Mode）后返回结果，禁止插件自定义 schema 漂移；MCP `/tools/list` 响应需与插件 `mcp-tools.json` 完全一致。
- **FR-005**: Agent Hub ToolStore 必须订阅 `capability.catalog.sync_*` 事件以刷新缓存，并在与插件建立 MCP Session 时校验 `capabilities_hash`，若不一致则拒绝调用并请求重新同步。
- **FR-006**: Selector 必须读取 `capability.policy.prefer/fallback/rollback_capability_id` 与 `type=read|write|workflow/composite`，执行协议优先级（读默认 MCP 并发 REST、写默认 gRPC、Workflow 节点按模板声明），并在协议切换时打点 `integration.gateway.invocation.fallback` 事件。
- **FR-007**: MCP、gRPC、Workflow Adapter 均需复用统一的 `CapabilityInvokeRequest` 结构（含 `capability_id/plugin_id/tenant_uuid/idempotency_key/trace_id/preferred_protocol/tool_scope`），并在响应中写入相同的追踪字段与审计信息。
- **FR-008**: Workflow Builder & Engine 必须能够导入插件提供的 `workflow_template_ref` 与 `composite.graph`，渲染节点 schema，并在执行期沿用 Selector 与限流/追踪/事件策略，确保 Workflow 路径与 Agent Hub/Tenant API 一致。
- **FR-009**: 系统必须保证多插件隔离：每次调用都绑定 `plugin_id + capability_id + tenant_uuid`，Selector、Integration Gateway 与 Workflow Engine 不得跨插件共享状态（缓存、配额、限流指标）。
- **FR-010**: 所有入口在处理请求时必须生成或传播 W3C Trace Context，并输出 Metrics（`powerx_capability_invoke_total/latency_ms/error_total` 按能力×协议）、Audit（操作者、tenant、capability、tool_scope、trace_id）。
- **FR-011**: 当任何协议失败或 Registry/MCP 不可用时，系统需执行 fallback（MCP→gRPC、Workflow 节点补偿等）并在 EventBus 中发布 `integration.gateway.invocation.failed` 与 `capability.policy.degraded` 事件，同时保留重试与补偿计划。
- **FR-012**: Integration Gateway、Agent Hub、Workflow Engine 必须在响应中暴露一致的错误结构与修复指引（包含错误码、降级说明、下一步行动），并在降级/封禁情况下提示租户联系管理员更新策略。
- **FR-013**: Registry 与下游组件必须实现健康探测与缓存刷新：若 Redis 缓存失效或版本漂移，组件需回源 Postgres 并在恢复后重新构建缓存，过程中不得返回陈旧的协议映射。
- **FR-014**: CLI 或后台批任务在发现插件缺失 `contracts/exposure` 或 schema 无法解析时，需要向插件开发者与平台运营者发送告警，并阻止该能力出现在任何对外目录中。
- **FR-015**: 当插件发布新的 `capabilities_hash` 时，已上线的 Workflow 与 Agent 配置默认继续绑定旧版本，需由管理员或 Workflow Builder 在 Admin API/Builder 界面显式确认升级后才切换至新模板，以避免未验证的 schema 变更自动生效。
- **FR-016**: 管理端必须在“设置”侧边栏下提供“开放能力”页面，仅 `IsRoot` 管理员可见。页面需按模块（Media、Event、Scheduler、Knowledge、Workflow 等）分组展示：每个模块的能力数量、支持的协议类型（REST/gRPC/MCP/Workflow）、调试入口（例如 `/tenant/invocations` cURL、MCP Tool 名称、公开 OpenAPI 链接），并实时读取 Capability Registry（`source=corex`）数据，确保宿主与 Skeleton 插件都能在文档内找到对外调用方式。
- **FR-017**: “设置 > AI > 能力注册表” 页面同样仅向 `IsRoot` 展示，但该页面默认视图必须聚焦 `source=plugin` 的 Registry 记录（即插件/租户自定义的能力），并通过 `source` 筛选在 “插件 / 平台 / 全部” 之间切换；后端 `/admin/capabilities` 需要暴露 `source` 查询参数以区分 corex 底座与插件能力，避免两个页面出现相同数据。
- **FR-018**: `/api/v1/tenant/invocations` 作为统一调度入口时，必须根据 `preferred_protocol` 将传入 payload 转换为 REST/gRPC/MCP 调用并代理真实响应，返回结构包含两部分：①原始业务响应体（REST JSON、gRPC JSON 映射、MCP payload 等）原样输出；②在响应 JSON 包装层附带 `trace_id/protocol_used/fallback_used` 等元信息，便于审计。换言之，无论调用何种协议，插件在 HTTP 层都能拿到一致的“业务结果 + trace”结果，避免只能看到 trace 的空壳响应。
- **FR-019**: 平台需将 Agent 能力纳入 `source=corex` 的能力目录，公开 REST/SSE/gRPC 契约，并在 Integration Gateway 中支持流式代理或直连（SSE/WS）；租户只能访问本租户 Agent 与 Session（`agent_id` 与 `session_id` 必须归属当前租户）。
- **FR-020**: 平台需将多模态模型调用纳入 `source=corex` 能力目录，区分无状态调用与有状态会话（Sessioned）；所有请求必须校验 `model_key` **仅在当前租户范围内可用**（租户已配置 Profile 或已测试通过且凭据已保存），跨租户访问一律拒绝并审计。
- **FR-021**: Integration Gateway 全部入口必须采用统一鉴权分流：`Authorization: ApiKey <key>` 仅走 API Key 鉴权链路，`Authorization: Bearer <token>` 仅走 JWT 鉴权链路；禁止失败回退与混合授权决策。
- **FR-022**: 平台必须提供租户级 API Key 生命周期管理（创建、查询、轮换、吊销、审计），并通过 `api_key_profile` 管理用途主体与权限模板；明文 key 只允许创建时返回一次，持久层仅保存 hash/prefix。
- **FR-023**: API Key 与 JWT 必须统一收敛到 `AuthContext`（`tenant_uuid/principal_id/scopes/actions/plugin_id`），所有 Gateway Handler（含 `/internal/ws-bus/grant|publish`）复用同一授权器，禁止按入口散落鉴权逻辑。
- **FR-024**: 租户管理员可为本租户 API Key 配置 scope/action/resource 级权限；root/admin 租户可配置更高权限 scope，但必须显式授权并记录审计。
- **FR-025**: JWT 鉴权在签名/过期校验通过后，必须执行主体状态校验：`tenant/user/member` 均需存在且可用；若任一主体失效，返回 401/403，并清晰标识失败原因（如 `tenant_disabled`, `member_not_found`）。
- **FR-026**: 主体状态校验必须采用 `cache-first + DB-fallback`：先读取 Redis 快照，未命中或快照不完整时回源 DB 并回填缓存；缓存 TTL 默认 60 秒（可配置），且支持事件驱动失效。
- **FR-027**: 系统必须支持会话版本强制失效机制：JWT claims 增加 `session_version`（或等效字段），请求时与服务端当前版本对比；版本不一致时拒绝请求，确保密码重置、租户迁移、db-refresh 后可快速收敛旧 token。
- **FR-028**: `/admin/user/auth/me/context` 在 token 通过签名但主体关系已漂移（如 token tenant 不在当前 memberships）时必须返回非 200（推荐 401/403），禁止“自动兜底切租户”掩盖失效态，避免前端误判为登录仍有效。
- **FR-029**: 插件事件主题必须在 `plugin.yaml` 中显式声明（建议 `events.topics[]`），并在安装/启用流程中幂等同步到 `event_topics`；`/internal/ws-bus/grant` 仅执行授权绑定（ACL grant），禁止承担 topic 创建职责。运行时若 topic 未注册，必须返回明确错误（推荐 404/403），不允许隐式自动创建。

#### Gateway Proxy Envelope（请求/响应）

为落实 FR-018，`CapabilityInvokeRequest` 与响应 Envelope 需遵循以下约定：

- **请求字段**
  - `capability_id`：Registry 中的唯一 ID，用于 Selector 查表与授权。
  - `preferred_protocol`：可选 `rest`/`grpc`/`mcp`/`workflow`，若为空则按 `policy.prefer`。
  - `payload`：
    - REST：`{ "method": "GET", "endpoint": "/api/v1/media/assets", "headers": { ... }, "query": { ... }, "body": { ... } }`
    - gRPC：`{ "endpoint": "powerx.media.v1.MediaAssetAdminService", "rpc": "ListMediaAssets", "body": { "page": 1 } }`
    - MCP：沿用 `mcp/toolcall` JSON。
  - 允许缺省 `endpoint`/`headers` 等字段，由 Selector 从 Registry 协议定义补齐后再发起真实调用。
- **响应字段**
  - HTTP 200：
    ```json
    {
      "code": 200,
      "message": "success",
      "data": {
        "payload": { /* 与底层 REST/gRPC 相同的业务结果 */ },
        "trace_id": "xxx",
        "protocol_used": "http",
        "fallback_used": false,
        "latency_ms": 12
      },
      "timestamp": 1766566902
    }
    ```
  - HTTP 错误：Envelope 结构一致，`code` 与 HTTP status 对齐，`data.payload` 写入底层错误详情（如 gRPC Status、REST 错误 JSON）。
  - 所有协议都需写入 `traceparent` header，并将同一个 `trace_id` 记录到 `InvocationTrace` 表、事件 `integration.gateway.invocation.*` 及响应 Envelope。

上述 Envelope 必须写进文档（docs/guides/*、quickstart）与契约（OpenAPI/Protobuf 注释），并在测试中验证。

### Base Capability Exposure Roadmap

为实现“宿主模式与 Skeleton 模式使用同一接口”的目标，底座内建模块需要逐步纳入本特性管辖范围。自 2026Q1 起新增以下交付项：

1. **Media Assets Management 能力开放**：基于 `specs/001-docs-media-storage/contracts/http-admin.yaml` 的实体再输出一份对外 OpenAPI（对外开放路径定义在 `specs/001-docs-media-storage/contracts/http-openapi.yaml`），并由 Integration Gateway 提供 `media.assets.*` 预置能力记录，使插件以 `/tenant/invocations` 即可消费媒资存储、预签名等能力。
2. **事件/任务接口统一**：事件总线（Event Fabric）、定时任务、AI 知识库与 Workflow Builder 的底座功能都需要在 Registry 中登记“平台级能力”，通过 Admin API 暴露配置入口，通过 Tenant API/gRPC 暴露调用入口，确保宿主与 Skeleton 走同一认证/限流/审计链路。
3. **Agent & 多模态能力开放**：统一对外 REST/SSE/gRPC 契约（HTTP：`specs/007-integration-gateway-and-mcp/contracts/agent.http-openapi.yaml` 与 `ai-multimodal.http-openapi.yaml`；gRPC：`backend/api/grpc/contracts/powerx/agent/v1/agent_api.proto` 与 `backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto`），并通过 Registry 标记 `source=corex`，允许插件统一经 `/tenant/invocations` 或直接 OpenAPI 访问。
4. **对外契约集中维护**：所有底座能力的 HTTP 契约统一放在 `specs/<module>/contracts/http-openapi.yaml`，gRPC 契约统一放在 `backend/api/grpc/contracts/...`，并在本 spec 的 `FR` 条目下跟踪落地。Integration Gateway 负责汇总这些契约并生成供插件调用的 SDK/文档。
5. **Registry 标签**：为区分“插件能力”与“平台能力”，Registry 数据模型新增 `source=corex|plugin` 字段；Platform 能力默认内置 Tool Grant，可按租户启用/限流，插件调用时无需关心来源差异。

该路线确保未来添加任意核心模块（Media、事件广播、定时任务、AI 知识库、Workflow 等）时，都能通过 Integration Gateway 暴露统一接口，避免重复设计私有 Admin API。

### Key Entities *(include if feature involves data)*

- **CapabilityRecord**: Registry 中的主记录，字段包含 `capability_id`, `plugin_id`, `protocols.*`, `intents/tool_scope`, `workflow_template_ref`, `composite.graph`, `policy.prefer/fallback`, `protocol_hash`, `capabilities_hash`, 状态与审计信息。
- **ProtocolBinding**: 描述一个能力在 REST/gRPC/MCP/Workflow/Composite 通道中的协议引用、schema 位置、认证方式与健康度，用于 Integration Gateway、Selector 与 Workflow Engine。
- **CapabilitySyncJob**: Worker 运行时的任务记录，追踪插件版本、校验状态、失败原因、补偿状态与触发的 `capability.catalog.sync_*` 事件。
- **SelectorPolicySnapshot**: Agent Hub/Workflow Engine 缓存的策略视图，包含 `intent → tool_scope → capability_id` 映射、优先级、限流参数与版本号。
- **WorkflowTemplateRef**: 插件提供的 Workflow/Composite 模板元数据，包含节点列表、参数 schema、每个节点的协议与补偿策略。
- **InvocationTrace**: 统一的调用追踪与审计实体，记录 `trace_id`, `tenant_uuid`, `plugin_id`, `capability_id`, `protocol`, `latency`, `result`, `fallback` 与事件投递状态。
- **GatewayAPIKey**: 租户 API Key 主实体，记录 `tenant_uuid/name/key_prefix/key_hash/status/expires_at/last_used_at`。
- **GatewayAPIKeyPermission**: API Key 权限映射实体，记录 `scope/action/resource_pattern/plugin_id/effect`。
- **GatewayAPIKeyAuditLog**: API Key 调用审计实体，记录 `api_key_id/path/method/result/reason/trace_id/requested_at`。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 95% 的插件能力更新在提交后 3 分钟内在 Admin/Tenant API、Agent Hub MCP `/tools/list` 与 Workflow Catalog 中可查询并含有一致的 schema 与版本号。
- **SC-002**: 在基准负载下，Agent Hub/Integration Gateway 的读调用 90% 以上通过 MCP 或 REST 并发完成，写调用 100% 走 gRPC，且 Selector 触发的协议 fallback 成功率 ≥ 98%。
- **SC-003**: 至少 95% 的调用在成功或失败后 1 分钟内生成完整的 Trace、Metrics 与 Audit 记录，并能在统一观测面查询到 `capability_id/plugin_id/tenant_uuid` 维度的数据。
- **SC-004**: Workflow Builder 导入插件模板后，80% 的新建 Workflow 能直接复用插件节点并在一次执行内完成编排；对于复合任务，95% 的节点在策略调整或插件更新后无需重新配置即可继续运行。
- **SC-005**: 100% Gateway 入口支持 API Key + JWT 双鉴权，并在 API Key 命中时记录 `auth_source=api_key`。
- **SC-006**: 95% API Key 请求在 1 分钟内可按 `api_key_id`/`tenant_uuid`/`trace_id` 检索到审计记录。

## Assumptions

- 插件与 CLI 已按《006/多插件指南》完成能力声明、协议资产与 lint；本特性仅消费其产物。
- Registry、EventBus、Redis、Postgres 基础设施具备既定 SLA，本特性默认它们可用并只在失败时做补偿。
- 平台已有 Tool Grant、安全审计与追踪基础能力，本特性复用其接口，不新增独立的身份或计费系统。
