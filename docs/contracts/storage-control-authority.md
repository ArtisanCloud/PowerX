# 存储配置权威与发布事务

日期：2026-11-05。进度以 [T166～T174](../../specs/027-monitor-center/tasks.md#us9-commercial-gates) 为准；[验收记录](evidence/storage-commercial-foundation-validation-20261105.json)区分源码、隔离测试和本地 UI。后续[表监控接入证据](evidence/storage-monitoring-control-validation-20261105.json)记录该资源的实际写入与推送闭环。

## 当前边界

Root 门禁已实现于 HTTP 新旧备份路径、存储治理和五个旧 gRPC 备份方法。系统表容量监控已接入统一数据库配置与正式 TaskBus 发布，支持显式接管、保存回执和执行端确认；详见[表监控合同](storage-table-monitoring-control.md)。文件日志保留策略已按来源接入显式接管、数据库版本与逐文件删除前的权威锁，见[日志合同](storage-file-lifecycle-control.md)。归档目标已通过[目标控制合同](archive-target-control.md)接入显式接管、列表／目标版本、执行快照和独立验证证明；归档策略已按[策略合同](archive-policy-control.md)接入暂停式接管、版本与执行守卫；主机预算也按[预算合同](storage-host-budget-control.md)接入唯一归属与同盘守卫；数据库备份计划、普通快照计划、调用日志期限和两项专项系统表期限也已接入版本化数据库控制。紧急暂停与交接锁定见[执行保护合同](storage-execution-control.md)。各域源码接入不等于日常／生产已接管，联合恢复和旧执行器现场证明仍待完成。

模块不提供通用 JSON HTTP 写入口；表监控通过专用 Root 接口显式接管，默认暂停。启动不自动导入旧 JSON、不运行迁移、不启用策略、不执行数据删除。本轮不重启日常 Core、不迁移日常库，生产未部署。

## Root 与兼容

`/admin/ops/backup/**`、`/admin/backup/**`、`/admin/monitor/backup/**` 均要求真实平台 Root 用户 access JWT，覆盖配置、启停、当前计划、任务、文件、归档、快照、恢复及 SSE。旧别名保留，旧租户管理员授权不再授予整机访问。`/admin/ops/storage/**` 复用相同身份判定。

旧 `OpsAdminService` 的 `ListBackupPolicies`、`UpsertBackupPolicy`、`TriggerBackupJob`、`ListBackupJobs`、`TriggerRestoreDrill` 在统一 JWT 拦截器后执行同一门禁。服务调用需另行声明最小权限合同，不能借用这些 Admin 方法。

签名、issuer、audience、有效期、吊销及 scope 仍由统一认证执行；`auth.IsPlatformRootSession` 只检查已验证上下文：有效 user_id、Root、scope=access，且没有插件、STS、API Key 或 service actor 身份。角色名、`admin:root` 字符串及自报请求头不能替代。

无真实用户会话为 HTTP 401 `ROOT_USER_SESSION_REQUIRED`，已登录非 Root 为 403 `BACKUP_ROOT_REQUIRED`；gRPC 使用 Unauthenticated／PermissionDenied。备份菜单和整个页面仅显示给 Root；撤销 Root 后清空缓存、关闭窗口、丢弃旧权限周期的迟到结果。其他租户运维模块的通用 RBAC 未收窄。

## 数据与事务

| 表 | 内容与约束 |
| --- | --- |
| `ops_storage_control_configurations` | tenant_id=0；范围＋域唯一；期望／确认版本、正文／摘要、发布状态及操作者 |
| `ops_storage_control_revisions` | 范围＋请求 UUID 唯一；范围＋域＋版本唯一；不可变正文、摘要、操作者、Trace 与任务 ID；兼作幂等回执、审计和发布意图 |
| 既有 `event_task_histories` | 使用正式 StreamDriver.Register 在同事务登记，不新增私有队列 |

范围键为 `SHA256(host_id + NUL + filesystem_id + NUL + instance_id)`；原始身份在读取／发布时复核。它是安装范围身份，不替代跨实例唯一的主机协调者和执行所有权。

`ControlService` 只接受预先绑定的固定业务域与 DTO 校验器，绑定必须拒绝未知字段。枚举文件生命周期、预算、归档目标／策略、快照、数据库期限及表监控，**当前接入表监控、逐来源文件日志策略、归档目标及策略、主机预算、数据库备份计划、系统快照计划与调用日志期限；两类专项系统表期限亦已接入独立固定控制域；未审查的其他表不因枚举而获得清理资格**。原有数据库备份计划不另建第二份权威。

保存要求 UUID 请求键和 expected_revision，首次为 0。配置 CAS、不可变回执／审计及 TaskBus 登记同事务提交，任一步失败则全回滚。相同操作者、请求键及规范化载荷返回原回执；同键不同内容或操作者返回 409。operation_revision 是原请求版本，desired_revision 是当前版本。

迁移只集中挂载在 `pkg/corex/db/database/migration.go` 的 MigrateStorageGovernanceModels；构造及启动不迁移。新增两表是普通日志清理排除项，系统表清单由 63 项增为 65 项。其专项审计／回执保留仍需完成，不能直接清表。

JSONB 格式变化通过有界规范化消除摘要误判。正文拒绝凭据字段，但独立凭据值由[归档凭据提供器](archive-credential-vault.md)处理；外部 KMS 与完整联合恢复仍待验收。错误不回显正文或密钥。主要错误均带 STORAGE_CONTROL_ 前缀：400 PRECONDITION_REQUIRED／DOCUMENT_INVALID／SECRET_FORBIDDEN；409 REVISION_CONFLICT／IDEMPOTENCY_CONFLICT／DIGEST_MISMATCH／SCOPE_MISMATCH；503 SCHEMA_NOT_READY／SAVE_UNAVAILABLE。

## 发布与未关闭项

内部状态为 unconfigured、pending、applied、failed。保存后仍为 pending；发布回调成功，且确认版本与正式任务完成同事务提交后，才为 applied。提交后 ACK，ACK 失败不倒写成配置失败。

任务仅携带范围、域、版本与摘要；执行端核对不可变记录、任务登记身份和正文。在配置行锁下仅发布最新期望版本，旧消息不调用回调。回调必须有界、幂等，仅发布受控配置，不执行备份或删除。失败不增加确认版本。

消费者启动、传输恢复及提交后入队失败唤醒使用正式 TaskBus 恢复；常驻消费是阻塞读取，不周期扫描业务配置。Changed 仅通知状态变更，不引入页面轮询。

接管前仍需固定各域 DTO 与唯一写入口、原子执行快照、身份／所有权校验、紧急停用与离线期限、当前计划唯一性、密钥存储、旧配置 dry-run／接管／回退及 UI 期望／生效版本。这些 T166／T167／T169～T174 子项不能由本模块测试替代。

归档目标凭据现另有[独立文件密钥提供器](archive-credential-vault.md)，符合密钥不放普通数据库配置正文的边界；归档目标另按[目标控制合同](archive-target-control.md)接管；归档策略也已接入专用版本与执行守卫，不代表 T170 或 SG-11 全部完成。

2026-11-07 调用日志期限切片见[专用合同](database-retention-control.md)：两张调用表采用配置数据库权威和同事务预览／批次／回执，旧 JSON 只保留来源证明。专项系统表规则和整体恢复门槛继续按正式任务追踪。

2026-11-07 专项系统表切片见[接管合同](system-table-retention-control.md)：健康探测历史与退休发现缓存独立 CAS／发布，同事务预览、清理与回执，保留最新探测及有效／未知缓存；部署与恢复总门槛仍未关闭。


2026-11-08 托管原地恢复新增[独立执行栅栏](storage-execution-control.md#2026-11-08托管原地恢复栅栏)：在业务覆盖前提交独立维护代次，数据库／运行投影一起恢复旧副本仍须 Root 核对当前代次、保持暂停，之后才单独恢复。该接线不代表外部整盘回退、跨资源水位重建、凭据／目标重新验证或真实旧进程交接已经完成。隔离验证与生产接管继续区分。
