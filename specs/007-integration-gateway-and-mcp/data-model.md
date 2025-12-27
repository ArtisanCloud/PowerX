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
  - `auth_type`（enum: tenant_jwt/plugin_token/sse）
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

> 所有实体复用 CoreX GORM 基础字段；迁移通过 `pkg/corex/db/database/migration.go` → `MigrateCoreModels` 注册，并提供回滚脚本防止 hash 冲突。
