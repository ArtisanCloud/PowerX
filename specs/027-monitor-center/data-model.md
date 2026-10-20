# Data Model: 自动备份闭环（Backup Center）

> 2026-10-15：第 1–5 节为组件级逻辑模型，实际持久化／HTTP 字段以 `model/ops` 和现有合同为准；第 6 节含概念模型与实际持久化映射；以业务库之外的 JSON 文件及 `snapshot_ops/types.go`、`managed.go` 实现，不新增业务表，实际 API 见[合同](../../docs/contracts/system-snapshots.openapi.yaml)。设计见[系统快照计划](../../docs/plan/deploy/system-snapshot-and-restore.md)。

## 1) BackupPolicy（备份策略）

### Fields
- id: string（唯一标识）
- tenant_id: string
- name: string（策略名称）
- enabled: boolean
- schedule_mode: enum(`interval`,`cron`)
- interval_hours: integer（当前默认 1；新建策略默认停用）
- timezone: string（默认 `Asia/Shanghai`）
- retention_count: integer（当前默认 168）
- retention_days: integer（当前默认 7）
- retention_mode: enum(`count`,`age_and_count`)（当前新建默认 `age_and_count`）
- drill_enabled: boolean（当前默认 false）
- drill_interval_days: integer（默认 7）
- target_ref: string（如 `powerx_bak`）
- consecutive_fail_upgrade_threshold: integer（默认 2）
- created_by: string
- updated_by: string
- created_at: datetime
- updated_at: datetime

### Validation Rules
- name 非空且在租户内唯一。
- 旧 `interval_hours` 输入为 1–168；更细周期使用现有 DTO 的 `interval_value/interval_unit` 或 `schedule`，不是该字段的任意扩展。
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

## 6) 系统快照概念模型与实际合同

### SystemSnapshotPolicy

- ID、名称、启停、实际执行间隔／时区、保留天数／份数、保护规则、异地目标。
- 明确必需组件及受支持来源；一次调度创建一个父快照，不能让用户独立设置数据库／媒体周期后自动配对。
- 单实例支持最多 100 套命名配置，表格维护；新建默认关闭，每天一次（1440 分钟）、保留 7 天／最多 7 份。默认选择至多一套，只有该套允许启用，允许全部停用；启用新策略与停用上一套必须原子提交。
- 管理端手动创建从系统默认策略行发起，无需启用定时任务并明确提交 `policy_id`；确认框展示策略名称和规则。默认切换时拒绝旧选择，不静默改用新策略；相同幂等键不得跨策略复用。系统默认选择与定时启停独立：切换默认关闭旧定时、不开启新定时；定时全部关闭时仍允许按默认策略手动采集。
- 策略集合及默认身份在独立私有账本 `policies.json` 一起保存，锚点不通过接口返回；旧 `policy.json` 只读兼容，首次维护时保留旧配置并进入集合账本。策略启停和默认切换不立即启动采集。

### SystemSnapshot

- `snapshot_id`、`contract_version`、源实例／`deployment.env`、逻辑恢复时间和采集起止时间。
- `policy_id/policy_name` 记录采集时所属配置；保留清理按所属策略分组，不因默认切换而套用其他策略规则。
- 触发方式、策略、操作者、Trace、必需组件、排除范围、外部依赖和不支持项。
- 状态：`queued → capturing → verifying → ready`；失败／中断终止本次采集，重试创建可追溯的新执行，不覆写已封存内容。
- 独立记录本地可用、异地状态、导出结果、业务验证报告和保护标记；`ready` 不等于业务已验收。
- 封存 manifest 的完整性信息；敏感配置以密文引用保存，清单不含明文凭据。

系统快照概览通过 Root API 返回 `directory` 和 `directory_source`，目录取实际 Service 存储根的规范路径，不取动态来源解析器的备用目录。来源枚举为 `recovery_profile / environment / configuration / install_root / default / explicit`。`system_snapshots.directory` 为部署配置，`POWERX_SNAPSHOT_DIRECTORY` 可覆盖 YAML；托管 profile 优先。修改不迁移旧快照，候选恢复必须重写到候选目录，快照根不得递归收集为运行配置。

### SnapshotComponent / SnapshotObjectReference

- 父快照 ID、类型（数据库／媒体／配置／程序版本）、必需性、状态、内容摘要、大小及对象引用。
- 数据库：事务快照标识、数据库版本、扩展与迁移版本；`captured_at` 取实际 MVCC 导出时间。在线采集父任务记录 `attempt`、`retry_at`；冲突时 `status=capturing/phase=retry_wait`，同一 ID 最多 4 次，重试不混用旧临时内容。在线包不具备同点 Redis 合同，`recovery_supported=false`；受控停写回退点保留完整合同。
- 媒体：驱动、bucket／相对 key、内容版本、SHA256 和引用关系；必须可解析到包内实际内容。
- 配置／程序：生效配置摘要、密文引用与加密版本、插件身份／绑定、Core／插件版本和平台产物摘要。
- GC 以所有存活快照／恢复／导出任务的引用集合为依据；与旧 CAS 清理器使用兼容的锁与保护机制。

### SnapshotPackageJob

- 导出／导入任务 ID、快照 ID、格式版本、状态、包大小／摘要、内容可用性与解密检查结果。
- 完整导出必须物化全部必需引用；不能只复制父 manifest 或依赖源机器路径。
- 导入预检查可用不代表恢复完成；未知版本、缺对象、错误解密材料和路径越界必须阻断。

### SystemRestoreTask

- 任务 ID、源快照／导入 ID、目标模式（隔离测试／同实例正式恢复）、目标实例及资源身份。
- 目标新库／新媒体目录、配置和部署身份／绑定映射、程序兼容计划、隔离策略；不接受任意服务器命令或自由 DSN。
- 聚合阶段：`queued → prechecking → restoring → validating → ready_for_review`；正式模式经显式确认后进入 `switching → active`。
- 失败、取消、中断及 `rolling_back → rolled_back / rollback_failed` 独立记录；隔离测试验证通过不进入正式生效状态。
- 组件任务 ID、技术／业务报告、维护确认、当前回退点、停写和切换检查点、补偿结果、源／目标未误改证明。
- 元数据位于被恢复业务库之外；协调器与 Core 生命周期分离。恢复写入后回退需重新决策，不沿用停写阶段的自动补偿承诺。

### Relationships / Integrity

- SystemSnapshotPolicy 1:N SystemSnapshot；SystemSnapshot 1:N SnapshotComponent／SnapshotObjectReference。
- SystemSnapshot 1:N SnapshotPackageJob／SystemRestoreTask；恢复关联各现有组件任务，但最终状态由父任务验证条件决定。
- 必需组件完整且校验通过才可封存；正式切换必须有通过的联合验证、维护确认和当前环境回退点。
- 旧 BackupJob／媒体 Job 保持组件类型，不能仅按时间相近自动生成完整 SystemSnapshot。

### 实际持久化与状态映射

- 稳定私有目录内 `snapshots/<UUID>.json`、`restores/<UUID>.json`、`requests/`、`audit/` 保存父记录、幂等凭据和脱敏审计；`contents/<UUID>/` 保存独立内容、manifest 和包装密钥。
- manifest 合同 `powerx.system-snapshot/v1`：`id/instance/version/platform/captured_at/database_snapshot/files/components`；媒体引用清单位于 `metadata/media-inventory.json` 并与文件清单交叉校验。
- `created_at` 是任务时间，`captured_at` 是进入采集边界后的时间；不是任意时间点 PITR。
- 配置为 `configuration/runtime.enc`，数据密钥由本机主密钥包装；导出时仅在独立口令加密包内部携带数据密钥，导入后重新包装。
- 历史数据副本仍为 `queued/running → prepared/failed/interrupted`，其 `business_verified=false`、`cutover_available=false`。
- 托管任务 `kind=capture/prepare/cutover/resume`，实际状态 `queued/running/verified/complete/failed/interrupted/rolled_back/attention_required/resolved`；上面的 `ready_for_review/switching/active` 是概念词，不是 HTTP 枚举。
- 独立 `jobs/`、检查点历史、`active.json`、候选描述、心跳和只读票据密钥不在业务库内。`active` 记录一个整套部署和代次，不拆分各组件指针。
- 正式切换使用 `expected_generation` 和幂等键；恢复写入前可补偿回原部署，可能开放写入后保持候选现场并进入 `attention_required`。
- 已生成托管采集合同的包才有 `recovery_supported=true`；历史包只有兼容数据副本。实际字段以 [API 合同](../../docs/contracts/system-snapshots.openapi.yaml) 为准。
