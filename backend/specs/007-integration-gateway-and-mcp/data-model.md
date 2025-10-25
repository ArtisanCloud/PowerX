## Data Model

### IntegrationRoute
- **Identifiers**: `route_id` (UUID, 主键)、`tenant_id` + `route_slug`（租户内唯一复合索引）
- **Attributes**:
  - `route_slug` (string; 小写短划线别名，由租户提供)
  - `capability_id` (string; 对应 Capability Registry 里声明的能力)
  - `tool_grant_ids` (string array; Tool Grant 引用列表)
  - `channels` (enum set: http, mcp; 控制暴露通道)
  - `rate_limit` (JSON; `limit`, `burst`, `window_seconds`)
  - `event_topics` (JSON; success/failure/alert topic & subscriber info)
  - `lifecycle_state` (enum: pending/active/suspended/retired)
  - `status` (enum: enabled/disabled; 与 lifecycle 联动，用于快速开关)
  - `current_version` (int; 最新快照版本)
  - `created_at`, `updated_at`, `created_by`, `updated_by` (审计字段)
- **Relationships**:
  - 1:N ← `IntegrationRouteVersion`（历史版本）
  - 1:N ← `IntegrationInvocationLog`（调用记录）
- **Validation**:
  - `route_slug` 在同一 `tenant_id` 下唯一；存储前小写化并校验长度 3~63。
  - `tool_grant_ids` 必须全部存在且在 active 状态。
  - `rate_limit.limit` ≥ `burst` 且 >0；缺省使用研究结论中的默认值。
  - 非 `active` 状态禁止标记为 `enabled`。

### IntegrationRouteVersion
- **Identifiers**: `route_id` + `version` (int, 递增)
- **Attributes**:
  - `snapshot` (JSONB; 完整配置快照)
  - `change_type` (enum: create/update/suspend/resume/retire)
  - `change_summary` (string; diff 摘要，用于审计)
  - `changed_by` (string; 操作者 ID)
  - `changed_at` (timestamp)
  - `trace_id` (string; 关联操作链路)
- **Relationships**:
  - N:1 → `IntegrationRoute`
- **Validation**:
  - `version` 必须严格递增，写入时需带上上一版本号进行乐观校验。
  - `snapshot.route_slug` 与主表一致，否则拒绝写入。

### IntegrationInvocationLog
- **Identifiers**: `invocation_id` (UUID)
- **Attributes**:
  - `route_id` (UUID)、`tenant_id` (string)
  - `trace_id` (string; 从请求透传)
  - `request_payload` (JSON; 裁剪/脱敏后参数)
  - `response_payload` (JSON; 成功 or 错误摘要)
  - `status` (enum: success/failed/rate_limited/denied)
  - `duration_ms` (int; 从接收到完成的耗时)
  - `routed_capability_id` (string; 实际调用的能力 ID)
  - `routed_adapter` (string; Router 返回的适配器标识)
  - `event_published` (bool; 是否成功投递事件)
  - `created_at` (timestamp)
- **Relationships**:
  - N:1 → `IntegrationRoute`
  - 可选关联：`EventPublication`（通过 `trace_id`/`invocation_id`）
- **Validation**:
  - 仅持久化必要字段，敏感数据需掩码；失败原因需分类（限流/授权/执行错误）。
  - 保留期默认 30 天，归档任务由后续运维脚本执行。

### EventPublication
- **Identifiers**: `event_id` (string; 由事件骨干生成)
- **Attributes**:
  - `route_id`、`tenant_id`
  - `topic` (string; `integration.gateway.*`)
  - `payload` (JSON; 与 EventBus 发送内容一致)
  - `status` (enum: pending/sent/failed/retry_scheduled)
  - `attempts` (int)
  - `last_error` (string; 最近一次失败原因)
  - `trace_id`, `created_at`, `updated_at`
- **Relationships**:
  - N:1 → `IntegrationInvocationLog`（可通过 `trace_id` 对齐）
- **Validation**:
  - 发布失败必须写入 `failed` 并安排补偿；超过阈值触发告警。
  - `topic` 必须在配置的 allowlist 内。

### RateLimitPolicy (嵌入结构)
- **Identifiers**: 不独立建表，作为 JSON 嵌入
- **Attributes**:
  - `limit` (uint64; 每窗口允许的请求数)
  - `burst` (uint64; 允许的额外突发)
  - `window_seconds` (int; 默认 60)
  - `scope` (enum: per_route/per_tenant/per_route_per_tenant)
- **Relationships**:
  - 嵌入 `IntegrationRoute.rate_limit`
- **Validation**:
  - `window_seconds` ∈ [10, 3600]。
  - `limit` >0；`burst` 可为 0，若缺省使用 `limit` 的 100%。
  - `scope` 必须与限流键生成逻辑保持一致（计划在 service 层封装）。

> 所有表默认继承 CoreX 模型字段：`id`（若有）、`tenant_id`、`created_at`、`updated_at`、`created_by`、`updated_by`，并遵循软删除关闭策略；迁移由 `cmd/database/migrate.go` 注册。
