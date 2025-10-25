# Data Model: EventBus & Message Fabric

> 所有表默认带 `tenant_id` + `created_at` + `updated_at` + `trace_id`（审计字段），并继承 CoreX 基础模型约束（软删除禁用）。

## TopicDefinition (`event_topics`)
- **Primary Key**: `id` (ULID)
- **Unique**: (`tenant_id`, `namespace`, `name`)
- **Fields**
  - `tenant_id` (string) — 所属租户或 `global`
  - `namespace` (string) — 领域前缀，如 `corex.workflow`
  - `name` (string) — 事件名，如 `approved`
  - `full_topic` (string) — 拼接字段，`<tenant>.<namespace>.<name>`
  - `lifecycle_status` (enum) — `active` / `deprecated` / `retired`
  - `payload_format` (enum) — 默认 `json`，可为 `protobuf` / `avro`
  - `retention_policy` (jsonb) — `{"type":"time","value":"7d"}` 等
  - `versioning_mode` (enum) — `strict` / `backward` / `any`
  - `max_retry` (int) — 默认 5，可按 Topic 覆盖
  - `ack_timeout_sec` (int) — 默认 30
  - `metadata` (jsonb) — 额外标签（告警阈值等）
  - `created_by` (string) — 创建者身份（服务账号/用户）
  - `deprecated_at` (timestamp, nullable)
- **Relationships**
  - 1:N → `AclBinding`（主题授权）
  - 1:N → `ReplayRequest`（回放来源）
  - 1:N → `DeliveryAttempt`（通过 `topic_id` 关联）

## AclBinding (`event_acl_bindings`)
- **Primary Key**: `id` (ULID)
- **Unique**: (`tenant_id`, `topic_id`, `principal_id`, `action`)
- **Fields**
  - `topic_id` (foreign key → `event_topics.id`)
  - `principal_type` (enum) — `service`, `role`, `user`
  - `principal_id` (string) — 服务 ID / 角色名 / 用户 ID
  - `action` (enum) — `publish` / `subscribe` / `replay`
  - `expires_at` (timestamp, nullable) — 到期自动撤销
  - `granted_by` (string) — 审批来源
  - `justification` (text) — 审批说明
  - `audit_ref` (string) — 审计记录 ID

## EventEnvelope Record (`event_envelopes`)
- **Primary Key**: `id` (ULID)
- **Indexes**: (`tenant_id`, `topic_id`, `event_id`), (`trace_id`)
- **Fields**
  - `event_id` (string) — 全局唯一 ID（提供幂等键）
  - `topic_id` (foreign key) — 对应 TopicDefinition
  - `version` (string) — Schema 版本
  - `payload_format` (enum) — 冗余记录
  - `payload_digest` (string) — SHA256 Hash，隐私保护
  - `headers` (jsonb) — 附件元数据（例如 correlation_id）
  - `published_by` (string) — 发布主体
  - `published_at` (timestamp)
  - `retry_count` (int) — 当前重试次数
  - `status` (enum) — `pending` / `delivered` / `failed` / `dlq`
  - `last_error` (text, nullable)

## DeliveryAttempt (`event_delivery_attempts`)
- **Primary Key**: `id` (ULID)
- **Indexes**: (`tenant_id`, `event_id`, `subscriber_id`), (`status`)
- **Fields**
  - `event_id` (foreign key → `event_envelopes.event_id`)
  - `subscriber_id` (string) — 逻辑消费者 ID
  - `delivery_no` (int) — 第几次尝试
  - `status` (enum) — `ack` / `nack` / `timeout` / `scheduled`
  - `latency_ms` (int) — 发布到 ack 花费
  - `nack_reason` (text, nullable)
  - `scheduled_at` (timestamp) — 下一次重试时间
  - `trace_id` (string) — 与 envelope 对齐

## DlqMessage (`event_dlq_messages`)
- **Primary Key**: `id` (ULID)
- **Indexes**: (`tenant_id`, `topic_id`, `status`), (`created_at`)
- **Fields**
  - `event_id` (string) — 原事件 ID
  - `topic_id` (foreign key) — 对应主题
  - `failure_stage` (enum) — `publish` / `delivery` / `ack`
  - `last_error_code` (string)
  - `last_error_message` (text)
  - `payload_snapshot` (jsonb) — 原始载荷（带访问控制）
  - `headers` (jsonb)
  - `status` (enum) — `queued` / `replayed` / `discarded`
  - `replayed_at` (timestamp, nullable)
  - `replay_operator` (string, nullable)

## ReplayRequest (`event_replay_requests`)
- **Primary Key**: `id` (ULID)
- **Indexes**: (`tenant_id`, `topic_id`, `status`)
- **Fields**
  - `topic_id` (foreign key)
  - `time_range_start` / `time_range_end` (timestamp) — 回放窗口
  - `filter_trace_id` (string, nullable)
  - `filter_subscriber_id` (string, nullable)
  - `status` (enum) — `pending` / `running` / `completed` / `failed`
  - `issued_by` (string) — 发起人
  - `result_count` (int) — 回放事件数量
  - `failure_reason` (text, nullable)

## SubscriptionOffset (`event_subscription_offsets`)
- **Primary Key**: (`tenant_id`, `topic_id`, `subscriber_id`)
- **Fields**
  - `last_event_id` (string) — 最近确认事件
  - `last_ack_at` (timestamp)
  - `delivery_cursor` (string) — gRPC 流游标/Redis key
  - `delivery_mode` (enum) — `stream` / `polling`

## Redis Structures（运行时）
- `event:dedupe:{tenant}:{topic}` — Sorted Set，score=过期时间，value=event_id（幂等窗口）
- `event:retry:{tenant}` — Sorted Set，value=`<event_id>#<subscriber_id>`，score=重投时间
- `event:subscription:{tenant}:{topic}:{subscriber}` — Hash，维护流 token 与限流信息
