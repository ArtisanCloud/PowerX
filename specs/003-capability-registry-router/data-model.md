## Data Model

### CapabilityRegistration
- **Identifiers**: `capability_id` (string), `tenant_id` (string)
- **Attributes**:
  - `contract_ref` (string; references CapabilityContract ID + version)
  - `status` (enum: draft/published/deprecated/disabled)
  - `environment_policies` (map; prod/sandbox overrides)
  - `routing_policy_id` (UUID)
  - `adapter_endpoints` (array of AdapterEndpoint IDs)
  - `fallback_plan_id` (UUID, nullable)
  - `tool_grant_ids` (array)
  - `version` (int, monotonically increasing)
  - `created_at`, `updated_at`, `updated_by` (timestamps + actor)
- **Relationships**: 1:N with AdapterEndpoint, 1:1 with RoutingPolicy, 0:1 with FallbackPlan.
- **Validation**:
  - Tool Grant、Contract 引用必须存在且处于可用状态。
  - 每次写入需携带 `version` (ETag) 进行乐观锁校验。

### AdapterEndpoint
- **Identifiers**: `adapter_id` (UUID)
- **Attributes**:
  - `capability_id`, `tenant_id`
  - `transport_type` (enum: http/grpc/mcp/...)
  - `endpoint_url` or `service_ref`
  - `weight` (int >= 0)
  - `max_concurrency` (int, optional)
  - `timeout_ms` (int)
  - `labels` (map; e.g., region)
  - `visibility` (struct: environments, tenants allow/deny list)
  - `health_policy_id` (UUID)
- **Relationships**: N:1 with CapabilityRegistration; 1:1 with HealthPolicy (defined inside RoutingPolicy).
- **Validation**:
  - 权重总和允许为 0，但 Router 必须拒绝调用。
  - `transport_type` 必须在 Transport Adapter 注册列表内。

### RoutingPolicy
- **Identifiers**: `routing_policy_id` (UUID)
- **Attributes**:
  - `strategy` (enum: weighted_round_robin, priority, sticky)
  - `tenant_strategies` (map[tenant]->override)
  - `rate_limit` (struct: limit, window)
  - `fallback_sequence` (array of AdapterEndpoint IDs)
  - `cooldown_seconds` (int; default 60)
  - `sticky_keys` (array; optional)
- **Relationships**: 1:N with AdapterEndpoint; 0:1 with FallbackPlan.
- **Validation**:
  - `cooldown_seconds` >= 30。
  - `fallback_sequence` 中的适配器必须属于同一 CapabilityRegistration。

### HealthProbeResult
- **Identifiers**: `adapter_id`, `probe_window_start`
- **Attributes**:
  - `status` (enum: healthy/degraded/unhealthy)
  - `reason` (string)
  - `confidence` (0-1 float)
  - `next_retry_at` (timestamp; default `now + 60s`)
  - `failure_count` (int)
  - `version` (int;匹配 Registry 版本号)
- **Relationships**: N:1 with AdapterEndpoint。
- **Validation**:
  - `next_retry_at` 不得早于 `probe_window_end + cooldown_seconds`。
  - `version` 必须与注册快照一致，否则需重新拉取。

### DiscoveryCacheEntry
- **Identifiers**: `tenant_id`, `capability_id`, `client_id`
- **Attributes**:
  - `snapshot_version` (int)
  - `payload_hash` (string)
  - `expires_at` (timestamp; default `issued_at + 2m`)
  - `issued_at` (timestamp)
  - `source` (enum: registry/eventbus/local)
  - `policy_digest` (hash)
- **Relationships**: N:1 with CapabilityRegistration。
- **Validation**:
  - `expires_at - issued_at` ≤ 10 分钟。
  - 客户端需在失效前刷新或回退。

### FallbackPlan
- **Identifiers**: `fallback_plan_id` (UUID)
- **Attributes**:
  - `primary_capability_id`
  - `fallback_targets` (ordered array of capability IDs or static responses)
  - `trigger_conditions` (struct: error codes, failure rate, timeout)
  - `notification_channel` (enum: eventbus/webhook)
- **Relationships**: N:1 关联 CapabilityRegistration。
- **Validation**:
  - `fallback_targets` 至少包含一个条目。
  - 静态响应需提供模板与有效期。
