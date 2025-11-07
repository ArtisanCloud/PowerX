# Data Model: Agent Lifecycle & Observability

**Date**: 2025-10-22  
**Source**: `specs/008-agent-lifecycle-observability/spec.md`

## Entities

### AgentProfile
- **Description**: 表示已在平台注册的代理能力及其租户归属。
- **Key Fields**:
  - `id` (UUID) — 全局唯一标识，租户内不可重复。
  - `alias` (string) — 可读名称 + 别名组合，租户内唯一。
  - `tenant_id` (string) — 关联租户标识。
  - `status` (enum) — `pending`, `active`, `paused`, `retired`.
  - `tool_grants` (jsonb) — 可用 Tool Grant 清单与版本。
  - `telemetry_contract_version` (string) — 遥测契约引用。
  - `default_capacity_instances` (int) — 默认实例/Pod 数阈值。
  - `max_capacity_instances` (int, nullable) — 扩容上限，默认=default。
  - `current_capacity_instances` (int) — 当前已分配实例数。
  - `event_topic` (string) — 生命周期事件主题。
  - 审计字段：`created_at`, `created_by`, `updated_at`, `updated_by`, `trace_id`.
- **Relationships**:
  - 1:n 与 `LifecycleEvent`（按代理关联）。
  - 1:n 与 `HealthSignalSnapshot`（按代理关联）。
- **Constraints / Validation**:
  - `alias` + `tenant_id` 唯一索引。
  - `default_capacity_instances` ≥ 1。
  - `max_capacity_instances` ≥ `default_capacity_instances`。
  - `status` 转移遵循状态机（见 LifecycleEvent）。

### LifecycleEvent
- **Description**: 捕捉每次生命周期操作（人工或系统自动）。
- **Key Fields**:
  - `id` (UUID)。
  - `agent_id` (UUID) — 引用 `AgentProfile`.
  - `type` (enum) — `register`, `activate`, `pause`, `resume`, `scale_up`, `scale_down`, `retire`, `auto_degrade`, `auto_recover`.
  - `from_status` / `to_status` (enum) — 状态变化。
  - `requested_capacity` (int, nullable) — 扩缩容目标。
  - `reason` (text) — 触发原因。
  - `triggered_by` (string) — 用户/系统。
  - `trace_id` (string) — 关联追踪。
  - `event_id` (string) — EventBus 发布 ID。
  - 审计字段：`created_at`, `created_by`.
- **Constraints / Validation**:
  - 状态机：`pending -> active`, `active -> paused|retired`, `paused -> active|retired`, `active -> active` 允许 scale 变更。
  - 退役后禁止再次激活。
  - `requested_capacity` 必须在 [1, max_capacity_instances]。

### HealthSignalSnapshot
- **Description**: 汇总指标、追踪、日志计算出的健康评分快照。
- **Key Fields**:
  - `id` (UUID)。
  - `agent_id` (UUID) — 引用 `AgentProfile`.
  - `window_started_at` (timestamp)。
  - `window_duration_sec` (int) — 默认 60。
  - `throughput_per_min` (float)。
  - `success_rate` (float, 0-1)。
  - `p95_latency_ms` (int)。
  - `resource_utilization_pct` (float) — 平均资源占用。
  - `error_rate` (float)。
  - `health_score` (int 0-100)。
  - `status` (enum) — `healthy`, `degraded`, `unavailable`, `unknown`.
  - `anomaly_trace_ids` (jsonb array) — 关联异常追踪 ID 列表。
  - `last_event_id` (string, nullable) — 对应生命周期事件。
- **Constraints / Validation**:
  - `health_score` 与 `status` 对齐（>=80 healthy，50-79 degraded，<50 unavailable，缺数据 unknown）。
  - 防止重复窗口：`agent_id + window_started_at` 唯一。

### AuditRecord (扩展现有审计模型)
- **Description**: 重用 CoreX 审计设施，记录代理相关敏感操作。
- **Key Fields**:
  - `id` (UUID)。
  - `resource_type` — 固定 `agent_profile` 或 `agent_health`.
  - `resource_id` — 对应 `agent_id` 或快照 ID。
  - `action` — 与 LifecycleEvent type 对齐。
  - `actor_id` / `actor_type`。
  - `payload` (jsonb) — 操作摘要。
  - `trace_id`, `created_at`.
- **Constraints / Validation**:
  - 保留 ≥ 13 个月，查询常按 agent_id + 时间范围。
  - 与 LifecycleEvent 保持一对一或一对多映射（视操作复杂度）。

## Derived Views / Indexes
- 代理健康视图：`agent_id` + 最近 5 个 `HealthSignalSnapshot`，输出滚动趋势与健康状态。
- 生命周期最新状态 materialized view：`agent_id` + 最新 `LifecycleEvent`，便于快速查询状态。

## State Machine Summary
- `pending` → `active`（注册后激活）。
- `active` → `paused`（人工暂停） / `retired`（退役） / `active`（扩缩容，仅容量变化）。
- `paused` → `active`（恢复） / `retired`（退役）。
- `retired` → 终态，不可重新激活。
- 自动退化：`active` + 健康评分低于阈值 → `degraded`（在健康视图中标记，生命周期仍为 active，但创建 `auto_degrade` 事件以提示运维）。

## Data Retention
- AgentProfile：长期保留，逻辑删除后标记 `retired_at`。
- LifecycleEvent / HealthSignalSnapshot / AuditRecord：13 个月后可归档到冷存储（需规划批处理 JOB）。
