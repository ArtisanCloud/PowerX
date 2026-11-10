# 系统快照计划：数据库权威与执行保护

2026-11-07，SG-11 系统快照计划切片。普通计划配置迁入数据库；快照内容、采集／恢复记录、维护票据和恢复进度继续使用独立运维账本。本文不是整套 SG-11 或生产验收结论。

## 配置和持久化

计划使用统一 `snapshot-policies` 控制域，复用集中迁移已挂载的 `ops_storage_control_configurations`、`ops_storage_control_revisions` 和正式 TaskBus 历史表。该切片无新增业务表或启动迁移。整份目录是唯一可写配置，保存时同时验证目录版本和单计划版本；单文档验证最多一个当前计划，配置、不可变审计、UUID 回执及 TaskBus 登记原子提交。

旧 `policies.json`／`policy.json` 仅作为只读来源，读取不改写它们。Root 显式接管保存来源定义、ID、启用标志、默认选择及调度锚点的不可变快照，随后暂停全部计划并清除当前选择。历史快照、密钥和恢复记录不随接管删除。定义保存也先暂停并清除其当前选择；选择当前计划后仍保持暂停，开启自动采集必须再次确认。已发布的暂停当前计划可以手动采集。

本机 `policy-control-binding.json` 保存安装归属、已发布水位及来源摘要；`snapshot-policy-control.json` 是经数据库核验的只读执行投影。两者不接受普通配置写入。源数据库、目录、媒体根及实例身份仅以指纹绑定，凭据不进入计划正文。不同归属、来源变化、未发布、投影缺失／篡改及数据库单独回滚会阻止新的准入／删除。

首次发布前再次核对旧来源语义摘要；来源漂移则发布失败，不能自动吞并变化。发布先持久化本机水位再发布投影，故障时保持失效关闭，由正式 TaskBus 重试确认。不得删除绑定、手改水位或改用空目录绕过保护。

## 固定 API、授权与旧入口

前缀来自部署配置，以下相对 `/api/v1`，均只允许真实平台 Root access 用户会话；租户 RBAC、API Key、插件及 STS 不获得宿主机计划权限。这些路由为 `root_runtime_control`，不发布服务能力。

| 方法／路径 | 固定内容 |
| --- | --- |
| GET `/admin/ops/backup/system-snapshots/policy-configuration` | 权威、目录／确认／投影版本、逐计划版本、确认资格 |
| POST `.../adoptions` | `request_id,legacy_digest,confirm:true` |
| POST `.../changes` | `request_id,expected_revision,expected_policy_revision,policy_id,action,confirm:true` |
| GET `.../requests/:request_id` | 原操作者的不可变回执，只读核对 |

动作仅 `save`／`current`／`status`；完整 settings 只有名称、周期分钟、保留天数和份数。新建 ID 等于请求 UUID。周期 1～525600 分钟，天数 1～3650，份数 1～10000，保留天数须覆盖周期。重复 UUID 不新增计划；同 UUID 不同内容冲突。400 为参数错误，409 为版本／来源／请求冲突，428 为未接管或旧合同，503 为权威不可用或提交结果不明，不能据此重发新 UUID。

旧 `SavePolicy`、`PutPolicy`、启停和默认选择在受控实例返回 `SNAPSHOT_POLICY_CONTROL_REQUIRED`。管理端手动采集仍走原创建路由，但须携带 `policy_id` 和确认时的 `policy_revision`。缺少版本返回428；同 ID 被修改或切换后返回409。采集重放也核对原计划及版本，不静默改用新定义。

字段以[计划 OpenAPI](../../specs/027-monitor-center/contracts/snapshot-policy-control.openapi.yaml)及[采集／恢复合同](system-snapshots.openapi.yaml)为准。

## 采集和维护边界

长时间源检查、完整遍历及采集都在 SQL 配置锁外。只有有界任务元数据登记进入当前配置守卫，绑定计划版本、来源指纹及45分钟准入期限；采集正文在事务外执行。修改／暂停阻止新的自动采集及旧版本清理；已经准入的采集可以完成，但不得越过期限。来源变化重试仍受同一期限约束。

独立执行器只可完成已登记、来源匹配且未过期的采集；不能从旧 JSON 新开受控计划或做无数据库守卫的保留清理。受控标志随加密来源合同传递，删除本机绑定也不会让已登记的受控任务改走旧清理器。

**旧独立协调器模式暂不允许开启自动计划**：尚未接入数据库保留守卫，返回 `SNAPSHOT_POLICY_EXECUTOR_UNSUPPORTED`，手动采集可继续。默认同进程模式已接入；不另加常驻程序或端口。

内部维护恢复点采集是独立的明确操作，不读取普通计划、不触发其自动保留，恢复任务及限时进度票据维持既有合同。它不通过管理端计划 API 暴露为绕过版本校验的入口。

## 到期清理和不明结果

受控保留只处理本次已准入计划所属的历史包，按其捕获版本再次核验当前启用计划。最新成功、显式保护、导入包、恢复引用和未完成维护任务优先保护；不使用其他计划或默认规则兜底。该切片未新增字节预算、已验证恢复点最低数量或跨 archive／snapshot 的统一引用系统，仍属 SG-07。

实际清理先写 `pruning` 和唯一 `retention_token`，再在短配置守卫内将单包目录原子移到同根 `retention/<token>`，同步目录后释放数据库锁。隔离父目录必须是根目录内的真实私有目录，链接／越界时拒绝；递归物理删除在锁外执行；暂停晚于原子隔离时，已获准的隔离清理可以完成。全部完成后保留 `pruned` 历史记录。

任一步骤不确定时保持 `pruning`／`prune_unconfirmed`，停止自动重做，采集记录显示 `cleanup_error`。不得把未知结果报告成删除0项或成功。Root 可在核对具体快照和 token 后，使用已有明确确认的单份删除入口清理原目录、对应隔离目录及展示记录；审计和恢复历史保留。未开发自动猜测或后台修复不明意图的功能。

## UI 与验收

快照列表默认展示当前计划摘要、采集按钮和记录；完整计划在独立纵向“系统快照计划”分区。`SystemSnapshotPolicies.vue` 同时用于备份中心和存储治理，显示计划版本、保存／执行确认状态、自动开关和当前选择；编辑按需打开弹层。关闭／分区切换保留草稿及原 CAS 版本，SSE 不自动前移草稿版本。响应丢失后锁住写入，只读核对原请求，404 也不新建。

计划写入待核对时，其他分区仍有提示且阻止新的手动采集；已有快照的查看／导出／维护恢复保持可用。复用一条 Root SSE，初载、事件、重连和手动刷新读取，空闲时无周期状态查询。

[验收记录](evidence/snapshot-policy-control-validation-20261107.json)区分真实 PostgreSQL／Gin／JWT／SSE／Nuxt 与合成采集适配器、miniredis 传输。未运行日常数据库迁移、未重启用户服务、未修改正式站；未据此证明真实 pg_dump、独立恢复或7／14天运行。

系统快照仍是恢复资格和业务恢复验证分别记录，`ready` 不等于业务验收完成。T171 旧二进制交接、跨环境迁移与数据库＋运维状态一起回滚的独立时代次，以及恢复演练等剩余配置路径继续待办；专项系统表期限现见[接管合同](system-table-retention-control.md)；两张调用日志表期限现见[接管合同](database-retention-control.md)；维护窗口必须先停止不理解新合同的旧执行器，不能新旧双跑。源码中的旧方法保护只约束新版程序，不能代替部署交接。

代码映射：`snapshot_policy_control.go`（storage_governance）管理固定配置、发布和SQL守卫；`snapshot_ops/policy_control.go` 管理只读来源与本机绑定；`policy_retention.go` 管理准入及隔离清理；`service.go` 和 `coordinator_access.go` 接入采集；bootstrap 在控制消费者启动前绑定，保留原恢复进度链路。
