# PowerX Capability Exposure Plan

## 背景

为支持宿主模式与 Skeleton 模式的插件统一调用 PowerX 核心能力，需要将底座模块的开放接口纳入 "Integration Gateway & MCP" 专题管理。当前 Media、事件总线、定时任务、AI 知识库、Workflow 等能力大多仅通过 Admin API 暴露，导致插件需依赖内部路由。此计划旨在提供统一的 HTTP/OpenAPI 与 gRPC 契约，使任何插件或第三方在拿到授权后即可调用底座能力。

## 建议方案

1. **Registry 扩展**：在 Capability Registry 中新增 `source=corex|plugin` 字段，并预置 Media、Event Fabric、Scheduler、Knowledge、Workflow 等 CoreX 能力记录，统一走 Tool Grant 与限流策略。
2. **对外契约**：每个底座模块维护 `specs/<module>/contracts/http-openapi.yaml` 与 `backend/api/grpc/contracts/<module>/v1/*.proto`，Integration Gateway 以这些契约为源生成 SDK 和文档。
3. **统一调用入口**：第三方通过 `/tenant/capabilities` 与 `/tenant/invocations`（或 gRPC `IntegrationGatewayTenantService`）调用底座能力；宿主模式可继续使用 Admin API 进行配置，但实际能力调用全部归口 Integration Gateway。
4. **观测与治理**：沿用 FR-001~FR-015 的追踪/限流/审计要求，对平台能力与插件能力实施一致的 metrics/audit/event 采集。
5. **媒资公开 API**：PowerX 底座的 **Media Assets Management** 模块已在 `specs/001-media-storage/contracts/http-openapi.yaml` 提供 `{APIPrefix}/media/assets` 路径，包含上传、列表、详情、软删、预签名能力；插件（宿主或 Skeleton）只需携带 Bearer Token（租户由 JWT claims 提供）即可直接调用，对应调用流程记录在本计划与 Quickstart 中。
6. **Agent & 多模态统一开放**：补齐 Agent 运行时与多模态模型调用的对外接口标准（REST/SSE/gRPC/SDK），并将租户隔离、流式协议与幂等错误码纳入统一规范（见下文“Agent 能力开放计划”“多模态模型调用标准”与 `specs/007-integration-gateway-and-mcp/spec.md` 的 FR-019~FR-020）。
7. **插件细颗粒度授权统一登记**：插件声明菜单、页面、业务动作、接口权限和数据范围需求；PowerX 在插件安装/同步时登记为统一 Capability/IAM Permission，由 PowerX 角色权限页集中授权。插件设置页不得作为正式角色授权主入口，只能用于 local 调试、业务配置或展示登记状态。

## 插件细颗粒度权限注册与授权目标架构

### 职责边界

| 边界 | 职责 |
| --- | --- |
| 插件 | 声明菜单入口、页面访问、页面内业务动作、接口 binding、数据范围需求、i18n 展示元数据、默认角色建议。 |
| PowerX 底座 | 校验并登记插件声明，生成/同步 CapabilityRecord 与 IAM Permission，在统一角色权限页展示并完成授权。 |
| PowerX 网关 | 对进入插件的用户态请求执行第一层鉴权，按插件声明的权限码和接口 binding 拦截未授权调用。 |
| 插件运行时 | 消费 PowerX 下发或签发的授权结果，控制前端菜单/按钮/页面可见性，并在后端对业务接口做二次校验。 |
| local 模式 | 读取同一份插件权限声明，模拟 PowerX 角色授权结果，用于独立运行、开发调试和回归测试。 |

正式模式下，角色授权的唯一主入口是 PowerX 统一权限中心。插件不得长期维护一套独立正式权限配置系统，也不得把页面内按钮授权、接口授权和菜单授权拆成彼此不一致的来源。

### 声明模型

插件能力声明需要覆盖四类授权对象：

| 类型 | 用途 | 示例 |
| --- | --- | --- |
| `menu` | 控制插件菜单入口是否展示 | `menu.production.sample_tracks:view` + `menu_path=[production,sample_tracks]` |
| `page` | 控制页面/路由是否可访问 | `production.bulk_order:read` |
| `action` | 控制页面内按钮、节点操作、业务流转动作 | `production.sample_track:factory_schedule`、`production.sample_track:delivery` |
| `api` | 描述 HTTP/gRPC/MCP 等接口与业务权限的绑定关系 | `production.sample_track_api:sample_schedule -> production.sample_track:factory_schedule` |

`menu/page/action` 是管理员在角色权限页可理解、可勾选的业务授权项。`api` 是这些授权项的 protocol binding 和后端 enforcement mapping，不应默认把每个 raw URL 暴露成管理员需要理解的权限项。只有当接口本身代表独立业务授权边界时，才拆成单独 capability。

### 权限层级渲染与强校验策略

PowerX 不是被动渲染插件随意拼接出来的权限字符串。插件负责声明业务语义，PowerX 负责按规范强校验、登记、渲染和授权；不符合规范的声明必须进入登记异常或同步失败，不得出现在正式角色授权树里。

角色权限页固定拆成三类结构：

| 结构 | 来源 | 渲染规则 | 授权语义 |
| --- | --- | --- | --- |
| 菜单树 | `type=menu` | 使用 `menu_path` + i18n 渲染层级，不从 `permission_code` 猜层级。 | 控制导航入口可见。 |
| 业务能力树 | `type=page/action` | 使用 `module/resource/action` + i18n 分组。 | 控制页面访问、按钮、节点流转和业务动作。 |
| API 绑定明细 | `type=api` | 挂到 `business_permission_code` 指向的业务能力下；`independent: true` 才独立展示。 | 为 Gateway 和插件后端提供接口 enforcement mapping。 |

字段语义固定如下：

1. `module` 是业务域，例如 `production`、`settings`、`integration`，不是权限类型，也不是插件 ID；`module=menu` 属于无效声明。
2. `resource` 是业务资源，例如 `sample_track`、`bulk_order`、`template`。
3. `action` 是业务动作，例如 `read`、`create`、`delivery`、`factory_schedule`。
4. 插件 ID 只能作为来源字段，例如 `plugin_id` 或 `iam_permission.source=plugin:<plugin_id>`；不得进入 `permission_code/module/resource/action/menu_path`。
5. 菜单权限码推荐使用 `menu.<business_module>.<menu_key>:view`；业务能力权限码使用 `<business_module>.<resource>:<action>`；API 技术登记码使用 `<business_module>.<resource>_api:<operation>`。
6. 菜单与页面读取权限的联动必须来自显式 `page_permission_codes`，不得按标题、权限码字符串或插件 ID 猜测关联。

登记异常或同步失败规则：

- 缺 `permission_code`、`module`、i18n、风险等级、数据范围。
- `type=menu` 缺 `menu_path`，或把插件 ID、UUID、`/_p/<plugin_id>` 拼入菜单路径。
- `type=page/action` 缺 `resource/action`，或与 `permission_code` 不一致。
- `type=api` 缺 `protocol_bindings`，或缺 `business_permission_code` 且没有 `independent: true`。
- `business_permission_code` 指向不存在、非 active 或已废弃的业务权限。
- 动态路径使用 `{uuid}`、`:id` 或真实 UUID 样本，而不是 `*`。
- 旧 `rbac.resources`、`routes.permissions`、`required_policies` 试图替代正式 `permissions[]`。

建议声明片段：

```yaml
permissions:
  - capability_id: com.powerx.plugin.production.menu.sample_tracks
    permission_code: menu.production.sample_tracks:view
    type: menu
    module: production
    menu_path:
      - production
      - sample_tracks
    title_i18n: production.permissions.menu_sample_tracks
    description_i18n: production.permissions.menu_sample_tracks_desc
    page_permission_codes:
      - production.sample_track:read
    risk_level: low
    data_scope: tenant
    default_role_grants: []

  - capability_id: com.powerx.plugin.production.sample_track.read
    permission_code: production.sample_track:read
    type: page
    module: production
    resource: sample_track
    action: read
    title_i18n: production.permissions.sample_track_read
    description_i18n: production.permissions.sample_track_read_desc
    risk_level: low
    default_role_grants: []
    protocol_bindings:
      - channel: rest
        method: GET
        path: /admin/operations/ai-craft/production/sample-tracks
        actor_context: admin_user
        resource_scope: tenant
      - channel: rest
        method: GET
        path: /admin/operations/ai-craft/production/sample-tracks/*
        actor_context: admin_user
        resource_scope: tenant

  - capability_id: com.powerx.plugin.production.sample_track.factory_schedule
    permission_code: production.sample_track:factory_schedule
    type: action
    module: production
    resource: sample_track
    action: factory_schedule
    title_i18n: production.permissions.sample_track_factory_schedule
    description_i18n: production.permissions.sample_track_factory_schedule_desc
    risk_level: medium
    data_scope: tenant

  - capability_id: com.powerx.plugin.production.sample_track_api.sample_schedule
    permission_code: production.sample_track_api:sample_schedule
    business_permission_code: production.sample_track:factory_schedule
    type: api
    module: production
    resource: sample_track_api
    action: sample_schedule
    title_i18n: production.permissions.sample_track_sample_schedule_api
    description_i18n: production.permissions.sample_track_sample_schedule_api_desc
    risk_level: medium
    data_scope: tenant
    protocol_bindings:
      - channel: rest
        method: POST
        path: /sample-tracks/*/nodes/sample-schedule
        actor_context: admin_user
        resource_scope: tenant
```

声明约束：

1. `capability_id` 必须全局稳定，插件能力使用 `com.powerx.plugin.<domain>.<resource>.<action>` 命名。
2. `permission_code` 是 IAM/RBAC 的唯一结构化权限字段，不得从标题、描述、路径或历史字段推导；菜单、业务能力和 API 技术登记码分别遵守上文命名格式。
3. 用户可见标题、说明、分组名称必须通过 i18n key 声明，不能把 `capability_id`、UUID、raw route 作为主要展示文案。
4. `type=page` 是插件后台页面访问授权项。每个用户可访问的 SPA 逻辑页面和详情页必须声明 page 权限，并提供 GET `protocol_bindings`；静态资产、`/_nuxt/**`、图片、CSS、JS、health、debug bridge 等非业务路由不声明 page 权限。
5. page binding 的 `path` 使用插件内部稳定业务路由，例如 `/admin/operations/ai-craft/production/sample-tracks`；插件权限声明里的动态路径统一使用 `*`，不使用 `{uuid}`、`:id` 或真实 UUID 样本。
6. 插件打包发布必须检查合并 catalog 后的 effective manifest，并用 route dump / 后端 RBAC route 表与 `permissions[].protocol_bindings` 做差异审计，确保所有真实业务接口都有 `type=api` binding。
7. 所有 REST binding 必须显式声明 `method/path/actor_context/resource_scope`，不得用 path-only 或 method wildcard 隐式放开写操作。
8. 新增插件普通成员默认可用能力时，必须显式声明 `default_role_grants: [role_user]`；否则 Core 只默认授予租户 owner/admin。
9. 缺少 `permission_code`、i18n key、page/api binding 元数据或真实 transport 的声明必须同步失败，不得静默忽略或降级为粗权限。迁移窗口内对历史插件页面的 warn/allow 只属于运维保护，不改变新插件声明准入规则。

### 授权与运行时链路

1. 插件安装或升级时，PowerX Plugin Manager 读取插件权限声明并触发 Capability Sync Worker。
2. Sync Worker 校验声明、协议 binding、i18n 元数据和默认角色声明，写入 Capability Registry，并同步 IAM Permission。
3. PowerX 角色权限页按“插件 / 模块 / 菜单 / 页面 / 动作”展示可授权项，管理员给租户角色授权。
4. 插件前端通过 PowerX 提供的当前用户有效权限集控制菜单、页面和按钮，不从插件设置页读取正式授权配置。
5. PowerX 网关根据请求路径、HTTP method、插件 ID、当前 member 和权限码做第一层拦截。
6. 插件后端从 PowerX token claims 或 authz/introspection 获取授权结果，对接口对应的 `permission_code` 做二次校验。
7. 所有拒绝必须返回明确错误码并写入审计日志，至少包含 `tenant_uuid/member_uuid/plugin_id/capability_id/permission_code/route/method/trace_id`。

delegated 模式的最终授权传递方案采用以下组合之一：

| 方案 | 用途 | 约束 |
| --- | --- | --- |
| A. token claims | 高频 UI/API 校验，减少实时查询 | claims 必须包含权限版本或 hash，权限变更后可失效。 |
| B. authz/introspection | 权限实时性要求高、claims 过大或需要数据范围裁决 | 插件后端必须显式请求 PowerX，不得从自由文本或请求头猜测权限。 |
| C. gateway pre-check | 所有进入插件的用户态请求第一层拦截 | 插件后端仍需做二次校验，不能只信任网关。 |

目标实现优先采用 `A + C`；当权限集过大、数据范围裁决复杂或需要强实时撤权时，采用 `B + C`。

### local 模式要求

local 模式是 delegated 模式的本地模拟，不是独立正式授权体系：

1. local 权限目录必须来自同一份插件权限声明。
2. local 角色/用户配置只模拟 PowerX 统一授权结果，用于开发、调试和测试。
3. local 模式生成的权限结果字段必须与 delegated 模式消费字段一致。
4. local 模式不得维护另一份正式授权定义；只能输出 `permission_codes/policy_version/perms_hash/source=local_mock` 结构来模拟 PowerX delegated 授权快照。
5. local 页面可以展示“本插件声明了哪些权限”和“当前模拟角色获得哪些权限”，但不得引入与 PowerX 角色权限页不同的正式配置语义。

### 生产单示例

插件声明：

| 授权项 | 类型 | 权限码 | 接口 binding |
| --- | --- | --- | --- |
| 生产单 / 小样跟踪单 | `menu` | `menu.production.sample_tracks:view` | `menu_path=[production,sample_tracks]` |
| 小样跟踪单读取 | `page` | `production.sample_track:read` | `GET /sample-tracks`、`GET /sample-tracks/*` |
| 小样打样排产 | `action` | `production.sample_track:factory_schedule` | `POST /sample-tracks/*/nodes/sample-schedule` |
| 小样交付资料 | `action` | `production.sample_track:delivery` | `POST /sample-tracks/*/nodes/delivery` |
| 企划审核 | `action` | `production.sample_track:planner_review` | `POST /sample-tracks/*/nodes/planner-review/*` |
| 设计师验收 | `action` | `production.sample_track:designer_acceptance` | `POST /sample-tracks/*/nodes/designer-acceptance/*` |
| 生产单 / 大货跟踪单 | `menu` | `menu.production.bulk_orders:view` | `menu_path=[production,bulk_orders]` |
| 大货单全部操作 | `action` | `production.bulk_order:manage` | 大货单写接口集合 |

PowerX 管理员在统一角色权限页给“工厂用户角色”勾选业务项。插件前端据此显示排产、交付等按钮；插件后端在对应接口上校验同一个 `permission_code`，避免菜单、按钮和接口各管各的。

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
4. **插件业务页授权**：Host 模式下，PowerX 网关对 `/_p/<plugin_id>/admin/**` 与插件 API 入口按插件声明的 `permission_code` 做用户态 RBAC 预检；Skeleton/local 模式使用同一声明生成本地模拟授权，不允许另设一套正式角色授权模型。
5. **Insomnia/cURL 模板**：
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

## 管理端“插件权限中心”页面

- **入口**：Web Admin “设置 > 角色与权限”中的插件权限分组，面向租户 owner/admin；Root 只在平台支持模式或全局治理视角查看登记状态，不默认代表租户授权。
- **数据来源**：只读取 Capability Registry 与 IAM Permission 中 `source=plugin` 的正式声明，不从插件设置页读取授权项。
- **展示结构**：每个插件下固定显示三类结构：菜单树、业务能力树、API 绑定明细。菜单树按 `menu_path` 渲染；业务能力树按 `module/resource/action` 渲染；API 绑定明细挂到 `business_permission_code` 指向的业务能力下。
- **授权对象**：角色绑定到 `effective_permission_code`，而不是 URL、数字 ID、插件 ID、raw API permission 或临时 action key。普通 API binding 用于网关和插件后端 enforcement，不作为默认主勾选项；只有 `independent: true` 的 API 才可作为独立授权项。
- **状态提示**：若插件声明缺少 i18n、接口 binding、`permission_code`、风险等级或真实 transport，页面显示“登记失败/待修复”状态，并给出同步错误，不允许管理员授权半登记能力。
- **审计**：角色授权、撤销、插件权限声明同步、声明变更导致的权限废弃都写入审计，记录 `tenant_uuid/operator_member_uuid/plugin_id/capability_id/permission_code/change_set/trace_id`。

## 里程碑

- **M1 (Media)**：将 Media Assets Management 能力纳入 Registry，发布对外 OpenAPI/gRPC 契约，支持 `/tenant/invocations` 把文件上传/预签名请求路由到 Media Service。
- **M2 (Event Fabric & Scheduler)**：为事件广播与定时任务输出能力记录，公开订阅/触发接口以及安全策略。
- **M3 (Knowledge & Workflow)**：Knowledge Space、Workflow Builder/Engine 对外提供模板查询与执行能力，确保 Skeleton 插件能直接调用。
- **M4 (统一 SDK/Docs)**：Integration Gateway 汇总上述契约生成 SDK 与统一说明书，覆盖宿主与 Skeleton 场景。
- **M5 (插件细颗粒度权限)**：插件安装/同步时登记 `menu/page/action/api` 权限声明，PowerX 角色权限页统一授权，网关与插件后端按同一 `permission_code` 执行拒绝。

## 风险与缓解

- **契约漂移**：通过 Buf/OpenAPI 验证与 CI contracts-test 防止接口不一致。
- **鉴权复杂度**：复用现有 Tool Grant / STS / Tenant Auth 体系，避免重复造轮子。
- **插件权限声明不完整**：同步阶段 fail-fast，缺 `permission_code`、i18n、binding 或真实 transport 时拒绝登记，并在插件权限中心显示错误。
- **菜单、按钮、接口权限漂移**：以 Capability Registry + IAM Permission 为唯一事实来源；前端显隐、网关预检、插件后端二次校验必须使用同一 `permission_code`。
- **旧粗权限迁移**：旧 `operations.order:read/manage` 等粗权限不得长期兼容并存；升级时提供迁移说明和一次性 backfill/映射报告，缺失细权限时明确失败。

## Next Steps

1. 更新 `specs/007-integration-gateway-and-mcp/spec.md`，纳入 Base Capability Exposure Roadmap（已完成）。
2. 为 Media/Event/Workflow 等模块补充对外 OpenAPI/gRPC 契约，并在 Registry 中登记 `source=corex` 能力。
3. 扩展 Integration Gateway Handler/Service，支持 1P 能力的调用路由与治理策略。
4. 定义插件权限声明 schema，覆盖 `menu/page/action/api`、i18n、默认角色、risk level、REST binding 与数据范围字段。
5. 扩展插件安装/同步流程，把插件权限声明写入 Capability Registry 并同步 IAM Permission。
6. 改造 PowerX 角色权限页，按插件权限声明展示和授权；local 模式只模拟同一授权结果。
