# Data Model — Tool Grants & Security Policy

## Capability

- **Role**: 定义可授权的工具能力（命名空间 + 动作）。
- **Fields**:
  - `id (uuid)` — 主键。
  - `namespace (varchar, unique with action)` — 例如 `event_fabric`.
  - `action (varchar)` — 例如 `publish`.
  - `description (text)` — 能力描述。
  - `risk_level (enum: low|medium|high|critical)` — 风险等级。
  - `default_rate_limit (jsonb)` — 默认速率限制配置。
  - `created_at / updated_at (timestamptz)`.
- **Rules**:
  - `(namespace, action)` 唯一。
  - 高风险能力需指定 `default_rate_limit`.
- **Relationships**:
  - 1:N → `GrantCapability`.

## Grant

- **Role**: 绑定主体、租户、能力范围与条件的授权单元。
- **Fields**:
  - `id (uuid)` — 主键。
  - `subject_type (enum: agent|plugin)` / `subject_id (uuid)`.
  - `tenant_id (uuid)` — 必填。
  - `status (enum: active|revoked|expired|pending)` — 生命周期状态。
  - `source (enum: system_template|tenant_template|session_temp)` — 来源类型。
  - `template_id (uuid, nullable)` — 引用模板。
  - `ttl_expires_at (timestamptz)` — 自动回收时间。
  - `created_by (uuid)` — 创建人。
  - `created_at / updated_at (timestamptz)`.
  - `revoked_at (timestamptz, nullable)` / `revoked_reason (text)`.
  - `version (bigint)` — 递增，缓存失效依据。
- **Rules**:
  - 唯一索引 `(tenant_id, subject_type, subject_id, status in active/pending)`.
  - 状态必须随 TTL 或撤销及时更新。
- **Relationships**:
  - 1:N → `GrantCapability`.
  - 1:N → `GrantCondition`.
  - 1:N → `ApprovalTicket`.

## GrantCapability

- **Role**: Grant 与 Capability 的关联，并标记速率/配额覆盖。
- **Fields**:
  - `id (uuid)`.
  - `grant_id (uuid)` — FK → Grant.
  - `capability_id (uuid)` — FK → Capability.
  - `custom_rate_limit (jsonb, nullable)`.
  - `created_at (timestamptz)`.
- **Rules**:
  - 唯一索引 `(grant_id, capability_id)`.
  - 若 `custom_rate_limit` 为空，使用 Capability 默认值。

## GrantCondition

- **Role**: 定义 Grant 的附加条件（资源白名单、上下文标签、时间窗口）。
- **Fields**:
  - `id (uuid)`.
  - `grant_id (uuid)` — FK.
  - `type (enum: resource|context_tag|time_window)` — 条件类型。
  - `expression (jsonb)` — 条件表达式（列表或结构化定义）。
  - `created_at (timestamptz)`.
- **Rules**:
  - 不同类型的校验逻辑：`resource` 必须提供资源 ID 列表；`time_window` 包含起止时间。

## ApprovalTicket

- **Role**: Challenge 审批实例，用于跟踪审批状态。
- **Fields**:
  - `id (uuid)`.
  - `grant_id (uuid, nullable)` — 若 Challenge 发生在 Grant 创建阶段。
  - `request_fingerprint (uuid)` — 授权评估请求指纹。
  - `tenant_id (uuid)`.
  - `status (enum: pending|approved|rejected|expired)` — 与 SLA 对齐。
  - `assigned_team (varchar)` — 默认 `secops`.
  - `sla_expires_at (timestamptz)`.
  - `decision_by (uuid, nullable)` / `decision_at (timestamptz, nullable)`.
  - `decision_reason (text)`.
  - `created_at / updated_at (timestamptz)`.
- **Rules**:
  - 状态 `pending` 且 `now > sla_expires_at` → 自动转 `expired` 并拒绝请求。
  - `request_fingerprint` 唯一。

## AuditEvent (logical model)

- **Role**: 审计事件统一结构，持久化于 ClickHouse + 对象存储。
- **Fields**:
  - `event_id (uuid)`.
  - `tenant_id (uuid)`.
  - `event_type (enum: grant.created|grant.updated|grant.revoked|evaluation.allow|evaluation.block|evaluation.challenge|challenge.auto_reject|challenge.decision)`.
  - `subject_id (uuid)` / `subject_type`.
  - `actor_id (uuid, nullable)` — 手工操作人。
  - `capabilities (string[])`.
  - `context_tags (string[])`.
  - `request_fingerprint (uuid, nullable)`.
  - `timestamp (timestamptz)`.
  - `metadata (json)` — 决策理由、告警信息。
- **Rules**:
  - 以 `tenant_id` + `timestamp` 分区，保存 ≥ 3 年。

## State Transitions

- **Grant**:
  - `pending → active`（审批通过或无 Challenge 自动生效）。
  - `pending → revoked`（审批拒绝）。
  - `active → expired`（TTL 到期） / `active → revoked`（管理员操作）。
  - `revoked/expired` 状态不可回滚，需新建 Grant。

- **ApprovalTicket**:
  - `pending → approved/rejected`（人工决策）。
  - `pending → expired`（SLA 超时自动处理）。
  - `approved/rejected/expired` 终态不可逆。

## Multi-Tenant Considerations

- 所有表含 `tenant_id`（GrantCondition 通过 Grant 继承但查询需 JOIN 过滤）。
- 索引均需包含 `tenant_id` 以利于查询与隔离。
- Redis 缓存键前缀 `grant:{tenant_id}:{subject_id}:{version}`。
