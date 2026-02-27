# Data Model: EventBus & Message Fabric（Topic / Subscriber / Subscription）

> 本文档采用“Topic 注册制 + Subscriber 注册制 + ACL 授权 + JWT 租户鉴别”模型。

## 1) EventTopicRegistry（当前物理源：`event_topics`）

- **用途**：记录系统可用 Topic 的统一注册定义（语义层）。
- **唯一键建议**：`(scope_type, scope_id, namespace, name)`（等价 `full_topic` 唯一）
- **当前实现说明（2026-02）**：
  - 物理落表与运行期治理均使用 `event_topics`。
  - `scope_type` 支持 `system/tenant/plugin/third_party`。
  - `scope_id` 对应作用域标识（如 `global`、tenant UUID、plugin id、第三方源 id）。
- **核心字段（现有+演进）**：
  - `id` (ULID/UUID)
  - `scope_type` / `scope_id`
  - `namespace`（如 `_topic.knowledge.space.feedback`）
  - `name`（如 `reprocess`）
  - `full_topic`（如 `global._topic.knowledge.space.feedback.reprocess`）
  - `lifecycle_status` (`active/deprecated/retired`)
  - `payload_format` (`json/protobuf/avro`)
  - `versioning_mode` (`strict/backward/any`)
  - `ack_timeout_sec` / `max_retry`
  - `retention_policy` / `metadata`
  - `created_by` / `updated_by`
  - `created_at` / `updated_at`

## 2) TopicPolicy（可选：租户/环境覆盖）

- **用途**：对注册 Topic 做稀疏覆盖（不是每租户复制）。
- **唯一键**：`(scope_type, scope_id, topic_id)`
- **核心字段**：
  - `scope_type` (`tenant/global/system`)
  - `scope_id`（tenant_uuid 或固定值）
  - `topic_id`（FK -> `event_topics.id`）
  - `ack_timeout_sec_override` (nullable)
  - `max_retry_override` (nullable)
  - `lifecycle_override` (nullable)

## 3) SubscriberRegistry（新增规划：`subscriber_registry`）

- **用途**：记录消费者目录（系统内置、租户扩展、插件、第三方）。
- **唯一键建议**：`subscriber_id`
- **核心字段（规划）**：
  - `subscriber_id`（如 `_subscriber.knowledge_space.reprocess`）
  - `source_type`（`system/tenant/plugin/third_party`）
  - `source_id`（插件 id / 第三方标识 / tenant id）
  - `scope_type` / `scope_id`
  - `status`（`active/paused/offline`）
  - `capabilities`（jsonb）
  - `heartbeat_at`
  - `created_at` / `updated_at`

## 4) TopicSubscriptions（新增规划：`topic_subscriptions`）

- **用途**：维护 Topic 与 Subscriber 的多对多订阅关系。
- **唯一键建议**：`(topic_id, subscriber_id)`
- **核心字段（规划）**：
  - `topic_id`（FK -> `event_topics.id`）
  - `subscriber_id`（FK -> `subscriber_registry.subscriber_id`）
  - `enabled` (bool)
  - `delivery_policy` (jsonb)
  - `retry_policy` (jsonb)
  - `created_by` / `updated_by`
  - `created_at` / `updated_at`

## 5) AclBinding（`event_acl_bindings`）

- **用途**：谁可以对某 Topic 执行何种动作。
- **唯一键**：`(scope_id, topic_id, principal_id, action)`
- **核心字段**：
  - `scope_id`（tenant_uuid 或 global/system）
  - `topic_id`（FK -> `event_topics.id`）
  - `principal_type` (`service/role/user`)
  - `principal_id` (string)
  - `action` (`publish/subscribe/replay`)
  - `expires_at` (nullable)
  - `granted_by` / `justification` / `audit_ref`

## 6) EventEnvelope（`event_envelopes`）

- **用途**：事件信封持久化（用于重试、重放、审计）。
- **关键点**：
  - `tenant_uuid` 来自 JWT 上下文
  - `topic_id` 指向 `event_topics`
  - `topic_key` 可冗余存储用于检索
- **核心字段**：
  - `event_id`（幂等主键之一）
  - `tenant_uuid`
  - `topic_id`
  - `version` / `payload_format` / `payload_digest`
  - `headers` / `trace_id` / `published_by`
  - `status` / `retry_count` / `last_error`

## 7) DeliveryAttempt / DLQ / ReplayRequest / SubscriptionOffset

这些表保持原职责不变，但 topic 关联统一使用 `topic_id`（指向 `event_topics`），
不再依赖“租户实例化 topic 表”。

## 7.1) 兼容状态标注（必须）

- `event_topics`：**当前生产真相源**（Topic 注册与治理统一入口）。
- `subscriber_registry` / `topic_subscriptions`：**下一阶段补齐项**（用于插件/第三方 subscriber 动态注册）。
- 迁移目标：管理台与调度链路从“内置常量聚合”演进到“注册表驱动”。

### Topic 治理迁移落地点

- 迁移入口：`backend/pkg/corex/db/migration/202602120001_event_topics_governance_migration.go`
- 正向函数：`EnsureEventTopicsGovernanceMigration(db)`
- 回滚函数：`RollbackEventTopicsGovernanceMigration(db)`
- 执行链路：`backend/pkg/corex/db/database/migration.go` 的 Event Fabric 迁移阶段。

## 8) 运行时缓存（Redis）

- `event:topic:resolve:{topic_key}` -> 解析结果（topic_id + scope）
- `event:acl:{scope}:{topic_id}:{principal}:{action}` -> ACL 结果
- 策略：`cache-aside`（先写 DB，再删缓存），TTL 60~300s。

## 9) 关键约束

1. 请求体中的 `topic` 仅传语义 key（`_topic.<domain>.<name>`）。
2. 租户由 JWT 唯一确定，不从 header/body 覆盖。
3. 运行时解析链路：`tenant -> global -> system`。
4. 未注册 topic 返回业务错误（404），不得返回 500。
5. `subscriber_id` 命名统一采用 `_subscriber.<domain>.<handler>`。
6. `kind` 命名统一采用 `_kind.<domain>.<action>`。

## 10) ACL 治理视图模型（Phase 13）

> 该章节描述“系统设置 / 事件权限”页面所需读模型，属于 API 聚合层，不改变核心存储真相源。

### 10.1 TopicAclView（按 Topic 查看授权）

- `topic_id`
- `topic_key`（语义 key）
- `scope_hint`（`tenant/global/system`）
- `bindings[]`
  - `principal_type`
  - `principal_id`
  - `action`
  - `expires_at`
  - `updated_at`

### 10.2 PrincipalAclView（按主体反查 Topic）

- `principal_type`
- `principal_id`
- `permissions[]`
  - `topic_id`
  - `topic_key`
  - `action`
  - `scope`

### 10.3 约束

1. 治理页必须可见共享 Topic（`global/system`），不得仅返回当前 tenant 自有 Topic。
2. 显示层以语义 key 为主，UUID 仅作为内部引用。
3. ACL 写入仍以 `AclBinding` 为权威落库结构，治理视图仅做聚合投影。

## 11) 三表关系（必须统一口径）

- `event_topics`：定义“有什么事件”
- `subscriber_registry`：定义“谁可以消费”
- `topic_subscriptions`：定义“谁订阅了哪些事件”

关系：

- `event_topics (1) -- (N) topic_subscriptions (N) -- (1) subscriber_registry`

说明：这是支持插件/第三方 subscriber 动态接入的最小闭环模型。
