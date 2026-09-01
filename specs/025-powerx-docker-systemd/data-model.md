# Phase 1 Data Model: PowerX 运维治理域

> UUID 规则优先：下列早期模型中的通用 `id`、`plugin_id`、`policy_id`、`job_id` 等是历史计划遗留。新增或修正实现必须为可寻址/可审计业务对象提供稳定 UUID，并使用 `*_uuid` 建立关系；API、事件和审计不得暴露 numeric ID，也不得提供 numeric-to-UUID 兼容翻译。第 9 节 `PluginDatabaseBinding` 已按当前规则定义。

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

## 8. DeploymentIdentity（配置对象）

- Purpose: 表示整套 PowerX 实例的稳定部署身份；权威来源是 Core 实际加载的 `config.yaml`，不建立数据库表。
- Fields:
  - `env` (`dev` / `test` / `staging` / `prod`)
- Validation Rules:
  - 必填且只允许上述枚举。
  - 不从 `POWERX_ENV`、版本、运行模式、目录、域名或插件安装 metadata 推导。
  - 首次插件安装后不能通过普通 setup 或运行配置修改；变更必须走显式迁移。

## 9. PluginDatabaseBinding

- Purpose: 记录插件安装对应的 Schema/Database 与 Role/User，是恢复、迁移、清理和审计的权威引用。
- Fields:
  - `binding_uuid`（自身稳定业务 UUID）
  - `tenant_uuid`
  - `plugin_uuid`
  - `plugin_key`（用于命名的稳定 manifest 标识，不作为关系键）
  - `deployment_env`
  - `driver`
  - `database_name`
  - `schema_name`
  - `role_name`
  - `status` (`provisioning` / `active` / `repair_required` / `purging` / `purged` / `failed`)
  - `created_at`
  - `updated_at`
- Validation Rules:
  - 所有 API、审计、事件和跨表引用使用 `binding_uuid` / `tenant_uuid` / `plugin_uuid`，不得暴露 numeric ID。
  - 有效绑定按 (`tenant_uuid`, `plugin_uuid`, `deployment_env`) 唯一。
  - 对象名必须符合带环境段与稳定 hash 的命名规范。
  - 密码只进入受控 secret/host-values 写入链路，不进入本模型、日志、trace 或审计。
  - 旧绑定缺少 UUID、环境或对象名时进入 `repair_required`，不得自动翻译。

## Relationships

- `BackupPolicy` 1:N `BackupJob`
- `BackupJob` 1:N `BackupArtifact`
- `BackupJob` 1:N `RestoreDrillRecord`（按演练来源关联）
- `DeployReleaseRecord` 与 `ApprovalPolicyProfile` 按 `environment` 关联验证
- `PluginDatabaseBinding` 通过 `plugin_uuid` / `tenant_uuid` 关联业务对象，并以 `binding_uuid` 被审计和 repair 任务引用

## State Transitions

- `DeployReleaseRecord.status`: `pending -> running -> success|failed`
- `BackupJob.status`: `pending -> running -> success|failed`
- `RestoreDrillRecord.status`: `running -> success|failed`
