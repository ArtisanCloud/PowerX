# Data Model: 自动备份闭环（Backup Center）

## 1) BackupPolicy（备份策略）

### Fields
- id: string（唯一标识）
- tenant_id: string
- name: string（策略名称）
- enabled: boolean
- schedule_mode: enum(`interval`,`cron`)
- interval_hours: integer（默认 6）
- timezone: string（默认 `Asia/Shanghai`）
- retention_count: integer（默认 14）
- drill_enabled: boolean（默认 true）
- drill_interval_days: integer（默认 7）
- target_ref: string（如 `powerx_bak`）
- consecutive_fail_upgrade_threshold: integer（默认 2）
- created_by: string
- updated_by: string
- created_at: datetime
- updated_at: datetime

### Validation Rules
- name 非空且在租户内唯一。
- interval_hours >= 1 且 <= 24。
- retention_count >= 1。
- timezone 必须是有效时区标识。
- drill_interval_days >= 1。

### State
- `enabled=false`：策略存在但不触发调度。
- `enabled=true`：策略纳入调度。

---

## 2) BackupJob（备份作业）

### Fields
- id: string
- policy_id: string（FK -> BackupPolicy.id）
- tenant_id: string
- trigger_type: enum(`scheduled`,`manual`,`retry`)
- started_at: datetime
- finished_at: datetime|null
- status: enum(`queued`,`running`,`success`,`failed`,`canceled`)
- artifact_count: integer
- error_code: string|null
- error_message: string|null
- trace_id: string|null
- duration_ms: integer|null
- created_at: datetime

### Validation Rules
- `running` 状态必须存在 started_at。
- `success/failed/canceled` 状态必须存在 finished_at。
- failed 状态必须记录错误摘要（error_code 或 error_message）。

### State Transitions
- `queued -> running`
- `running -> success | failed | canceled`
- 禁止终态回退到 running。

---

## 3) BackupArtifact（备份产物）

### Fields
- id: string
- job_id: string（FK -> BackupJob.id）
- tenant_id: string
- storage_uri: string
- checksum: string
- size_bytes: integer
- status: enum(`available`,`expired`,`corrupted`,`deleted`)
- expires_at: datetime|null
- created_at: datetime

### Validation Rules
- success 作业至少对应 1 个可追踪产物记录。
- `available` 产物必须有 storage_uri 与 checksum。

### Lifecycle
- `available -> expired -> deleted`
- 若校验失败可转为 `corrupted`。

---

## 4) RestoreDrillJob（恢复演练作业）

### Fields
- id: string
- tenant_id: string
- source_artifact_id: string（FK -> BackupArtifact.id）
- trigger_type: enum(`scheduled`,`manual`)
- started_at: datetime
- finished_at: datetime|null
- status: enum(`queued`,`running`,`success`,`failed`,`canceled`)
- result_summary: string|null
- error_message: string|null
- trace_id: string|null
- created_by: string
- created_at: datetime

### Validation Rules
- 演练只允许引用 `available` 产物。
- 终态必须有 finished_at。
- 失败时必须包含 error_message。

---

## 5) BackupAlert（备份告警）

### Fields
- id: string
- tenant_id: string
- policy_id: string
- source_type: enum(`backup_job`,`retention_cleanup`,`restore_drill`)
- source_id: string
- level: enum(`info`,`warning`,`high`)
- rule_key: string（如 `consecutive_failures`）
- message: string
- acknowledged: boolean
- acknowledged_by: string|null
- acknowledged_at: datetime|null
- created_at: datetime

### Rules
- 连续失败次数达到阈值（默认 2）时，产生 `high` 告警。
- 告警支持确认，但不应删除历史。

---

## Relationships
- BackupPolicy 1:N BackupJob
- BackupJob 1:N BackupArtifact
- BackupArtifact 1:N RestoreDrillJob（逻辑上可被多次演练）
- BackupPolicy 1:N BackupAlert

## Query Views（供监控页）
- policy_health_view: 策略启停、下次执行时间、最近作业状态、连续失败次数。
- backup_job_timeline_view: 备份作业时间线。
- drill_job_timeline_view: 演练时间线。
- backup_alert_active_view: 未确认高优先级告警。
