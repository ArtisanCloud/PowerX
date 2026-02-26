## Data Model

### CapabilityRecord
- **Identifiers**: `capability_id` (string, 全局唯一)、`plugin_id` + `version` (hash) 组合追踪插件版本。
- **Attributes**:
  - `plugin_id`、`plugin_version`
  - `title`, `description`, `categories`（数组）
  - `intents` / `tool_scope`（数组，用于 Agent Hub 匹配）
  - `protocols`（JSONB，参照 ProtocolBinding 嵌套结构）
  - `workflow_template_refs`（JSONB 数组，引用插件提供的模板元信息）
  - `composite_graphs`（JSONB 数组，复合任务 DAG）
  - `policy`（JSONB；`prefer`, `fallback`, `rollback_capability_id` 等）
  - `capabilities_hash`（Worker 计算的整体 hash）
  - `protocol_hash`（协议资产 hash）
  - `status`（enum: active/disabled/deprecated）
  - `created_at`, `updated_at`, `created_by`, `updated_by`
- **Relationships**:
  - 1:N ← `CapabilitySyncJob`（每次同步记录 job）
  - 多个 Registry 消费者（Agent Hub、Workflow Builder、Integration Gateway）通过 Redis cache 引用
- **Validation**:
  - `capability_id` 必须匹配 `com.{vendor}.{product}.{action}` 规范。
  - `policy.prefer` ∈ {mcp, grpc, rest, workflow, composite}；若缺省，读= mcp, 写= grpc。
  - `workflow_template_refs` 中引用的 `template_id` 必须唯一。

### ProtocolBinding（嵌入结构）
- **Attributes**:
  - `channel` (enum: rest/grpc/mcp/workflow/composite/agent_stream)
  - `endpoint` / `tool_ref`（字符串，REST URL、gRPC service、MCP tool 名等）
  - `schema_ref`（指向 OpenAPI/Proto/MCP schema 文件）
  - `auth_type`（enum: api_key/tenant_jwt/plugin_token/sse）
  - `health_state`（enum: healthy/degraded/offline）
  - `latency_p95_ms`, `error_rate`
  - `last_checked_at`
- **Relationships**:
  - 嵌入 `CapabilityRecord.protocols[*]`
- **Validation**:
  - REST 通道需声明 `method`，gRPC 通道需声明 `rpc` 名称，MCP 通道需包含 `tool_scope`。
  - `health_state=offline` 时必须附加 `reason`。

### WorkflowTemplateRef
- **Identifiers**: `template_id`（插件内唯一）、`capability_id`
- **Attributes**:
  - `name`, `description`, `steps`（JSON DAG）
  - `params_schema`（JSON Schema）
  - `protocol_requirements`（每节点的通道声明）
  - `capabilities_hash_snapshot`
  - `requires_manual_upgrade`（bool，默认 true）
- **Relationships**:
  - N:1 → `CapabilityRecord`
  - 被 Workflow Builder/Engine TPL Catalog 引用
- **Validation**:
  - 每个 `steps[*].capability_id` 必须存在于当前 Registry 快照。
  - `requires_manual_upgrade` 为 true 时，Workflow Builder 保存编排时写入 `template_version` 以供升级提示。

### CapabilitySyncJob
- **Identifiers**: `job_id` (UUID)
- **Attributes**:
  - `plugin_id`, `plugin_version`
  - `status` (enum: pending/running/succeeded/failed)
  - `started_at`, `finished_at`
  - `hash_before`, `hash_after`
  - `error_summary`（失败时必填）
- **Relationships**:
  - N:1 → `CapabilityRecord`（一对多，对应 job 产出）
- **Validation**:
  - 同一插件若存在 `status=running` 的 job，需要幂等去重。
  - 失败 job 需触发 `capability.catalog.sync_failed` 并记录错误快照。

### SelectorPolicySnapshot
- **Identifiers**: `hash`（Registry 版本） + `tenant_id`
- **Attributes**:
  - `intent_mappings`（JSON：intent → tool_scope → capability_id）
  - `prefer_matrix`（JSON：capability_id → prefer/fallback）
  - `rate_limit_overrides`
  - `generated_at`
- **Relationships**:
  - 缓存在 Redis，Agent Hub/Workflow Engine 启动时拉取。
- **Validation**:
  - `hash` 必须与 Registry 提供的 `capabilities_hash` 一致，否则 Selector 拒绝服务。

### InvocationTrace
- **Identifiers**: `trace_id`（W3C trace context）
- **Attributes**:
  - `tenant_uuid`, `route_id`（可选）、`capability_id`, `plugin_id`
  - `protocol_used`, `fallback_used`（bool）
  - `request_metadata`（JSON，脱敏）
  - `response_metadata`（JSON，脱敏）
  - `status`（success/fallback_success/failure/denied/rate_limited）
  - `latency_ms`
  - `event_published`（bool）
  - `created_at`
- **Relationships**:
  - 关联 `EventPublication`（通过 `trace_id`）
  - 供 Audit/监控查询
- **Validation**:
  - 所有写入必须包含 `tenant_uuid` 与 `protocol_used`。
  - `fallback_used=true` 时必须记录 `fallback_reason`。

### AuthSubjectSnapshot（缓存快照，Redis）
- **Identifiers**:
  - `auth:user:{user_id}`
  - `auth:member:{member_id}`
  - `auth:tenant:{tenant_uuid}`
- **Attributes**:
  - `status`（active/disabled/deleted）
  - `tenant_uuid`（member 快照必填）
  - `user_id`（member 快照必填）
  - `session_version`（可选，强制失效场景使用）
  - `updated_at`
- **Relationships**:
  - 被 JWT/API Key 统一授权器读取，构建 `AuthContext`
  - 未命中时由 DB 回填
- **Validation**:
  - 缓存 TTL 默认 60 秒（配置项可覆盖）
  - 主体状态变更事件必须触发对应 key 失效

### AuthContext（请求内聚合上下文）
- **Identifiers**: `trace_id` + `principal_type` + `principal_id`
- **Attributes**:
  - `credential_type`（jwt/api_key）
  - `tenant_uuid`
  - `user_id` / `member_id` / `api_key_id`
  - `scopes[]`, `actions[]`
  - `session_version`
  - `validated_at`, `validation_source`（cache/db）
- **Relationships**:
  - 所有 Gateway Handler 与 ws-bus 授权复用同一结构
  - 审计日志与 InvocationTrace 直接引用
- **Validation**:
  - `tenant_uuid` 必须与 credential 内声明一致
  - JWT 路径下 `session_version` 不一致必须拒绝

### SessionVersionState
- **Identifiers**: `(tenant_uuid, principal_id, principal_type)`
- **Attributes**:
  - `session_version`（uint64, 单调递增）
  - `updated_by`, `updated_at`
  - `reason`（password_reset/tenant_rebuild/manual_revoke 等）
- **Relationships**:
  - 登录签发 token 时写入 claims
  - 请求鉴权时对比当前版本并决定是否失效
- **Validation**:
  - 任意会话失效操作必须保证版本递增
  - 不允许回退版本号

### APIKeyProfile（`iam_api_key_profile`）
- **Identifiers**: `id` (uint64), `tenant_uuid`, `key`
- **Attributes**:
  - `name`, `status`（0=disabled, 1=active）
- **Relationships**:
  - 1:N → `GatewayAPIKey`（一个 Profile 可签发多个 key）
  - M:N → `Permission`（通过 `APIKeyProfilePermission`）
- **Validation**:
  - Profile 停用后不可新建/轮换 key。
  - Profile 严格租户隔离，不允许跨租户复用。

### APIKeyProfilePermission（`iam_api_key_profile_permission`）
- **Identifiers**: `(profile_id, permission_id)` 复合主键
- **Attributes**:
  - `created_at`
- **Relationships**:
  - N:1 → `APIKeyProfile`
  - N:1 → `Permission`（`iam_permission`）
- **Validation**:
  - 仅允许绑定 `permissions/catalog` 返回的可用 permission（`iam_permission.allow_api_key=true` 且含 `meta.api_key` 映射）。
  - 保存模式为覆盖式（`PUT .../permissions`）。

### GatewayAPIKey（`integration_gateway_api_keys`）
- **Identifiers**: `id` (UUID), `tenant_uuid`, `key_prefix`
- **Attributes**:
  - `profile_id`, `name`, `description`
  - `key_hash`（仅 hash）
  - `status`（active/revoked/expired）
  - `expires_at`, `last_used_at`
  - `created_by`, `created_at`, `updated_at`
- **Relationships**:
  - N:1 → `APIKeyProfile`
  - 1:N → `GatewayAPIKeyPermission`（派生快照）
  - 1:N → `GatewayAPIKeyAuditLog`
- **Validation**:
  - 创建/轮换时从 Profile 当前 `permission_ids` 派生权限快照。
  - 明文 key 仅创建或轮换响应返回一次。

### GatewayAPIKeyPermission（`integration_gateway_api_key_permissions`）
- **Role**: key 级别权限快照（非配置源）。
- **Identifiers**: `id` (UUID), `api_key_uuid`
- **Attributes**:
  - `scope`, `action`, `resource_type`, `resource_pattern`
  - `plugin_id`（可选）, `effect`
- **Relationships**:
  - N:1 → `GatewayAPIKey`
- **Validation**:
  - 快照由后端从 Profile 绑定权限派生；不提供直接手工编辑接口。
  - Profile 权限变更不会回写历史 key 快照，需要轮换/重建生效。

### GatewayAPIKeyAuditLog
- **Identifiers**: `id` (UUID)
- **Attributes**:
  - `api_key_id`, `tenant_uuid`
  - `path`, `method`, `status_code`, `result`（allow/deny/error）
  - `reason`, `trace_id`
  - `requested_at`, `latency_ms`
- **Relationships**:
  - N:1 → `GatewayAPIKey`
- **Validation**:
  - 每次 API Key 请求都必须写入审计。
  - `trace_id` 为空时必须补写系统生成值。

> 所有实体复用 CoreX GORM 基础字段；迁移通过 `pkg/corex/db/database/migration.go` → `MigrateCoreModels` 注册，并提供回滚脚本防止 hash 冲突。

## Auth Validation Flow（模型应用约束）

1. 解析 `Authorization`：`ApiKey` 或 `Bearer`，单请求仅允许一种凭证语义。
2. JWT 路径先做签名/`exp`/`aud` 校验，再做主体状态校验（`tenant/user/member`）。
3. 主体状态校验先读取 `AuthSubjectSnapshot`，未命中回源 DB 并回填缓存。
4. 若启用 `session_version`，需对比 `SessionVersionState`；不一致立即拒绝。
5. 构建 `AuthContext` 注入请求上下文，后续 Handler/授权器/审计统一消费。
