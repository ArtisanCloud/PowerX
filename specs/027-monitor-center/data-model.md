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


## SG-11 配置事务基础（2026-11-05）

新增 `ops_storage_control_configurations` 和 `ops_storage_control_revisions`，集中挂载于 `pkg/corex/db/database/migration.go`。其身份、版本、幂等请求、审计和同事务 TaskBus 登记见[实际实现合同](../../docs/contracts/storage-control-authority.md)。两表是普通日志清理排除项。系统表容量监控及后述逐来源文件日志、归档目标已接入数据库权威并显式接管旧 JSON，详见[专用合同](../../docs/contracts/storage-table-monitoring-control.md)；其他资源尚未接管，不能将本节理解为全部模块已经迁移。

### 逐来源文件日志配置（2026-11-06）

复用配置／不可变版本表，不新增通用日志配置表。域为 `file-lifecycle.`＋source_id 的 SHA256 前32位；完整来源ID与指纹在固定正文校验。来源正文只包含协议版本、来源、指纹、四项 settings 以及旧策略正文／摘要 evidence。安装 scope_id 算法保持不变。

旧 lifecycle.json 的 policies 只读，published 保存已批准执行快照，runs 保留不可变请求、在途删除意图和结果。最终删除与配置更新共用数据库资源锁，固定 DB→主机锁顺序；pending、断库、未知快照阻止新删除。相同请求键返回原操作版本，与当前期望／确认版本分开。详见[合同](../../docs/contracts/storage-file-lifecycle-control.md)。

### 归档目标列表（2026-11-06）

固定域 `archive-targets` 复用平台配置及不可变版本表，不新增第三套配置表。scope 仍为 host／filesystem／instance；列表最多128个目标，各项为 UUID＋独立 revision＋固定 TargetSettings，仅保存 credential_ref。列表 CAS 防止并发丢失；新目标 UUID 等于创建 request_id，旧请求核对返回原 operation_revision，不能用当前版本替代原请求结果。

正文保留旧目标 origin 与 canonical digest，以及原操作的 typed 参数；接管独立版本加一并使旧证明失效。归档元数据中的 `target-verifications/*.json` 是运行验证账本；预算私有目录的 `archive-target-control.json` 是经 DB applied_revision 和不可变版本核对的投影，二者均不接受配置编辑。完整合同见[归档目标控制](../../docs/contracts/archive-target-control.md)。归档策略随后按下一节接入；运维任务和验证账本仍不是数据库配置正文。

### 归档策略列表与执行守卫（2026-11-06）

固定 `archive-policies` 域复用配置及不可变版本两表。每项为UUID、独立revision、固定PolicySettings、enabled／remote_cleanup_enabled、source_fingerprint、服务端target_revision／target_fingerprint及created_at；来源连接只参与内存摘要，正文不含DSN或凭据。正文另保存原策略来源、摘要与typed请求参数。首次接管和定义保存均关闭两个开关。

已保存列表revision与单条revision同时CAS；新策略ID等于创建request_id。`archive-policy-control.json` 是核对数据库确认版本的投影；`policy-control-binding.json` 是新版执行器禁止回退旧JSON的本地绑定，不是第二份配置权威。运行前通过 `WithCurrentTx` 在同一事务按策略→目标顺序锁定，作用于任务准入、S3写入、远端清理及本地保留。见[策略合同](../../docs/contracts/archive-policy-control.md)。此段仅记录归档策略切片；其他配置域的当前进度见后续章节及 SG-11 任务，联合恢复时代次仍待完成。

### 2026-11-07 主机预算配置域

复用tenant 0控制配置／不可变版本表，固定domain为`host-budget`。正文含固定Settings、authority UUID、完整旧BudgetPolicy及摘要；scope为可信host／filesystem／owner installation。共享budget账本中的control投影是运行确认与唯一归属记录，legacy policy及活动预留保留；其他安装仅使用该归属的已核验DB配置。发布前保守合并限制，准入与配置共用DB锁，再核对主机账本；释放不依赖DB。完整联合恢复时代次另属T171。


## 数据库备份计划配置接管（2026-11-07）

实际平台全库计划复用 `ops_backup_policies`，不新增第二套编辑表；逻辑模型前述 tenant_id 不作为本切片授权边界。新增 `control_scope_id varchar(64)`、`control_revision bigint`、`execution_fingerprint varchar(64)`；非空 scope 的 `is_current=true` 有部分唯一索引。目录及单行版本同时 CAS，旧 UUID／ID 和历史引用不变。统一配置／不可变审计／TaskHistory 在同一事务提交，详见[固定合同](../../docs/contracts/database-backup-policy-control.md)。

`ops_backup_jobs.policy_revision` 记录准入时的单计划版本；`ops_backup_artifacts.cleanup_state/cleanup_token` 保存逐产物删除意图，结果不确定不自动重做；软删后历史作业保留。来源快照只记录原定义与标志，执行连接只保存指纹。集中迁移添加字段／索引，不自动接管、启用或删除。系统快照／数据库期限接管及完整恢复时代次仍待完成。


## 系统快照计划控制（2026-11-07）

普通计划以 `ops_storage_control_configurations` 的 `snapshot-policies` 单文档为唯一可写来源，逐计划 ID／revision、当前选择／开关、调度锚点和源指纹随目录 CAS 原子提交，不新增第二套编辑表。不可变 `ops_storage_control_revisions` 保存来源快照、操作者、UUID回执和正式发布意图，复用集中迁移已有挂载。

旧 JSON 不覆盖；`policy-control-binding.json` 为安装归属／发布水位，`snapshot-policy-control.json` 为核对数据库后的只读投影。运行 Snapshot 新增 `policy_revision`、`policy_fingerprint`、`admission_deadline`、`retention_token` 和 `cleanup_error`。先隔离后删除，未知结果保留状态并停止重做；恢复／维护进度继续独立，不转为业务库队列。见[快照计划合同](../../docs/contracts/snapshot-policy-control.md)。


### 2026-11-07 调用日志期限接管（实际实现）

普通设置属于现有控制配置／版本表的 `database-retention` 域，不另建策略表。固定 DTO 区分保存期限和启用状态，保存后暂停清理；原 JSON 保留历史回执和来源摘要。`database-retention-binding.json` 保存 scope、来源摘要和单调水位，`database-retention-control.json` 为经数据库核对的执行投影，无离线清理资格。

预览与批次复用 `ops_storage_cleanup_runs`，固定 `tenant_id=0`、`instance_id=db-retention:<scope_hash>`；预览记录 actor、revision、cutoffs、expires_at 和 policy_snapshot，消费后标为 consumed；业务行变更与完成回执同事务。手动结果以原 UUID 读取，自动零候选批不写完成记录。终态 30 天、有界回收和未知结果边界见[合同](../../docs/contracts/database-retention-control.md)。集中迁移已有模型可满足本切片，不新增运行时迁移或业务表。两项专用系统表期限规则的控制域接管仍待完成。


### 2026-11-07 专项系统表期限接管（实际实现）

`capability_registry_health_probe_results` 和 `capability_registry_discovery_cache` 各自拥有 `system-table-retention.<fixed_id_sha256_first32>` 控制域，仅识别这两个固定值。CAS／发布和旧来源摘要分别独立，原共享 JSON 不覆盖。`system-retention-{probe,discovery}-binding.json` 记录 scope／单调水位，对应 control 文件为只读投影；不能离线删除。

预览和终态复用 `ops_storage_cleanup_runs`，按 `sys-retention:<scope_hash>:<fixed_adapter_id>` 隔离，固定 tenant_id=0；源数据处理、一次性预览消费、完成回执同事务。保留字段为既有 policy_snapshot、cutoff、actor、revision，加 preview_id／expires_at／trigger；不新增数据表或启动迁移。保护规则、30 天账本窗口和未知结果核对见[合同](../../docs/contracts/system-table-retention-control.md)。其他未批准表不获得普通日志清理资格，恢复交接和完整商用验收继续待办。


### 2026-11-07 安装级受控执行保护（实际实现）

固定 `execution-control` 域复用既有控制配置／版本表：`mode` 为 paused／handoff／open，`review_policies` 保留交接后的策略复查要求，`operation` 保存 UUID、expected_revision、固定 action、原因、确认及两个运维声明。初始无配置为 unmanaged，不自动接管。暂停及交接无须等待消费者，恢复需确认当前发布。

`execution-control-binding.json` 存 scope／单调 revision／digest；`execution-control-snapshot.json` 存正式发布投影。暂停成功答复前同步写独立水位，DB-only 回滚不能忽略该水位。共享／独占 PostgreSQL advisory lock 按开关→资源的固定顺序覆盖资源 CAS 和最终受控步骤；同连接取不可变版本。不新增表、列、启动迁移、轮询队列或第二常驻程序。完整联合恢复时代次与旧进程停止证明仍未完成，见[合同](../../docs/contracts/storage-execution-control.md)。


### 2026-11-08 托管恢复独立代次（实际实现）

既有维护根的 `installation.json.storage_protocol` 标记 `powerx.storage-recovery/v1`，`storage-recovery-binding.json` 保存协议、安装 ID、anchor UUID；`storage-recovery-epoch.json` 保存同一身份、当前 epoch UUID、恢复任务／快照 UUID 和时间。它们不属于普通运维目录或业务恢复载荷，不被补偿或完成启动清除。登记协议后缺失或不安全文件不能自动重建。共享／独占文件锁隔离有界守卫与恢复代次写入，不新增数据库表或常驻执行器。

`execution-control` 文档增补可选 `recovery_epoch`；固定操作 `acknowledge_restore` 携带 `expected_recovery_epoch`、环境核对及旧执行器／已接纳任务声明，沿原 CAS／不可变回执／正式 TaskBus 发布。代次不符覆盖状态为 recovery 并拒绝执行；确认发布后仍 paused，保留策略复查要求。完整字段与未覆盖的外部回退边界见[合同](../../docs/contracts/storage-execution-control.md)。


### ops_backup_cleanup_retentions（T172，2026-11-09）

平台 Root 运维请求／审计；不含 tenant_id。request_id 真唯一，Actor／artifact／原 token／review_version／输入摘要／安装 Scope 在接纳时固定，记录 reason、内部配置 Fence 及 checking→retained/rejected 终态。成功时与原备份 Job.protected=true 同事务，artifact 清理状态与 token 不变。模型挂载 `pkg/corex/db/database/migration.go`，普通日志清理排除，容量清单按 created_at 观测；不作为业务能力调用就绪依赖。终态回执不允许更新／删除，数据库恢复范围保证见 `docs/contracts/backup-cleanup-retention.md`。
