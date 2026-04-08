# Phase 1 Data Model: PowerX 运维治理域

## 1. DeployReleaseRecord

- Purpose: 记录每次部署发布与回滚动作。
- Fields:
  - `id` (唯一标识)
  - `environment` (目标环境)
  - `backend_version`
  - `web_admin_version`
  - `action` (`release` / `rollback`)
  - `status` (`pending` / `running` / `success` / `failed`)
  - `operator`
  - `trace_id`
  - `started_at`
  - `ended_at`
  - `error_message`
- Validation Rules:
  - 同一环境同一时刻仅允许一个 `running` 任务。
  - 回滚动作必须指定可用目标版本。

## 2. PluginLifecycleAudit

- Purpose: 记录插件安装、切换、回滚、卸载等关键动作。
- Fields:
  - `id`
  - `plugin_id`
  - `from_version`
  - `to_version`
  - `action` (`install` / `switch` / `rollback` / `uninstall`)
  - `result` (`success` / `failed`)
  - `gate_result`
  - `gate_reason`
  - `operator`
  - `trace_id`
  - `created_at`
  - `detail`
- Validation Rules:
  - `switch/rollback` 必须存在目标版本。
  - 失败动作必须有 `gate_reason` 或错误说明。

## 3. BackupPolicy

- Purpose: 定义备份策略与调度行为。
- Fields:
  - `id`
  - `name`
  - `backup_type` (`logical` / `physical` / `wal`)
  - `schedule`
  - `retention_days`
  - `enabled`
  - `storage_target`
  - `created_by`
  - `updated_by`
  - `updated_at`
- Validation Rules:
  - `retention_days` > 0。
  - `schedule` 必须可被调度器解析。

## 4. BackupJob

- Purpose: 记录一次备份执行任务。
- Fields:
  - `id`
  - `policy_id`
  - `status` (`pending` / `running` / `success` / `failed`)
  - `trigger_type` (`manual` / `scheduled`)
  - `started_at`
  - `ended_at`
  - `error_message`
  - `operator`
  - `trace_id`
- Validation Rules:
  - `running` 状态必须在结束时转为终态。
  - `failed` 状态必须写入错误信息。

## 5. BackupArtifact

- Purpose: 记录备份产物元数据。
- Fields:
  - `id`
  - `job_id`
  - `storage_uri`
  - `size_bytes`
  - `checksum`
  - `created_at`
- Validation Rules:
  - `checksum` 必须存在。
  - `size_bytes` 必须 > 0。

## 6. RestoreDrillRecord

- Purpose: 记录恢复演练结果。
- Fields:
  - `id`
  - `source_job_id`
  - `status` (`running` / `success` / `failed`)
  - `rto_seconds`
  - `report_uri`
  - `operator`
  - `trace_id`
  - `created_at`
- Validation Rules:
  - 成功演练需记录 `rto_seconds`。
  - `rto_seconds` 必须 <= 3600（首版目标）。

## 7. ApprovalPolicyProfile

- Purpose: 按环境管理高风险操作审批策略。
- Fields:
  - `id`
  - `environment`
  - `approval_mode` (`none` / `dual_approval`)
  - `updated_by`
  - `updated_at`
- Validation Rules:
  - 每个 `environment` 仅允许一条生效策略。

## Relationships

- `BackupPolicy` 1:N `BackupJob`
- `BackupJob` 1:N `BackupArtifact`
- `BackupJob` 1:N `RestoreDrillRecord`（按演练来源关联）
- `DeployReleaseRecord` 与 `ApprovalPolicyProfile` 按 `environment` 关联验证

## State Transitions

- `DeployReleaseRecord.status`: `pending -> running -> success|failed`
- `BackupJob.status`: `pending -> running -> success|failed`
- `RestoreDrillRecord.status`: `running -> success|failed`

