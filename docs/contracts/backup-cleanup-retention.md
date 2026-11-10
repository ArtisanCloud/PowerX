# 备份清理意图：人工确认保留并保护

2026-11-09，T172 的非破坏性处置切片。源码及自有隔离验收，不代表日常／生产部署或资源接管。

## 操作含义与权威

Root 对一条 `pending/unconfirmed` 清理意图明确选择保留。服务端重新完整读取文件，通过后在同一有界 SQL 事务中设置 **原备份 Job 的 protected=true**，并完成持久请求回执。**不修改 artifact 的 cleanup_state/cleanup_token，不删除文件，不确认删除完成，也不释放磁盘。** 恢复、清理结果确认或解除原意图不由此操作提供。

浏览器此前的完整校验结果不构成授权。输入中没有 `verified`、路径、SQL、保护布尔值或任意目标；未知字段拒绝。仅平台 Root 人类 access 会话，租户管理员、API Key、STS、插件服务身份均拒绝。

## 暂停、归属与执行器边界

操作必须满足：全局自动执行为已发布且 confirmed 的 paused；目标备份计划已接管、已发布，disabled、非当前且关闭自动演练；配置行／快照／不可变发布版本、Scope 和执行来源指纹一致；独立恢复代次已核对。初次进入及最终提交都在既有配置锁顺序中复核。期间策略、全局开关或恢复围栏改变，不能沿用原 Fence。

同一数据库会话同时持有请求 UUID 锁和既有备份策略作业锁；需要至少两条 DB 连接。另一同 UUID 执行者不能启动第二次核验；其他 UUID 对同意图的活动请求留下 rejected 回执。遗留 running/pending 备份记录、queued/running 恢复演练也阻止操作，不能因会话锁已释放便视为任务排空。源 artifact／Job 在有界事务内行锁定后比较版本，再执行保护写入。

本合同约束遵守既有锁／暂停协议的 Core 执行器，不构成对旧外部脚本或直接文件操作的停机证明。旧执行器升级、盘点与现场唯一执行权仍按 T171 交接；本接口不替用户执行 stop/disable/restart。

## UUID、版本与持久回执

新表 `ops_backup_cleanup_retentions` 是无 tenant_id 的平台运维请求／审计，不是配置表。request_id 必须有真实有效唯一索引；活动 intent 使用辅助部分唯一索引。该表已注册系统表容量／时间范围清单，分类 protected，排除普通日志清理，防止删除去重履历后重用 UUID。

第一次接纳时绑定 request_id、Actor、artifact、token、review_version、Reason、安装 Scope 和输入摘要。生命周期为 `checking → retained/rejected`，终态不可重写；不提供修改／删除回执接口。`review_version` 是服务器对原 artifact/Job 关联、状态、大小、校验值、时间及保护状态，加上独立维护锚点协议／安装／AnchorID／恢复代次的摘要，不是前端自行计算的版本。即使 SQL 行恢复到完全相同的值，旧页面请求仍不能跨恢复代次。

同 UUID／相同输入重放已完成回执，不重新读取或写保护；不同内容为幂等冲突，其他 Actor 不能接管原 UUID。Root 可只读查看本安装回执，checking 的 can_resume 仅对原 Actor 为真，含义是可明确重试原请求，不是当前可写保护的授权。接纳后每次重试仍重新核验，不能跳过文件或配置检查。

回执描述**该次操作完成时**的事实，不证明当前文件仍完整、当前保护设置未被其他明确操作修改或 SQL 恢复回退，也不允许以后删除。历史回执不能替代当前状态检查。v4 之后的新请求在 SQL 接纳前写入[独立恢复凭据](backup-retention-recovery-ledger.md)，已保存的身份与历史结果不随 SQL 回放丢失。v4 之前尚未导入的 SQL 履历仍可能回退；404 或恢复后的 checking 不证明此前未执行，不能自动换新请求。

## 请求与状态推送

以运行配置 API 前缀为准：

| 方法与路径 | 合同 |
| --- | --- |
| POST `/admin/ops/backup/cleanup-reviews/:artifact_id/retentions` | `request_id, expected_token, review_version, reason, confirm:true`；reason 3～256 字。一次请求绑定 SSE。 |
| GET `.../cleanup-reviews/:artifact_id/retentions?expected_token=...` | 当前安装、精确 artifact/token 的最近 20 个请求；找回未完成请求。 |
| GET `/admin/ops/backup/cleanup-retentions/:request_id` | 原 UUID 的持久回执，不能用当前配置代替其历史结果。 |

POST 不接受 query。所有路径 ID、UUID、字段、正文大小及重复 query 都做边界校验。公开字段及错误见 [OpenAPI](backup-cleanup-review.openapi.yaml)。SSE 分为 accepted（持久 checking）、progress（仅字节）、complete（持久终态）。uncertain、提前 EOF 或网络失败均不能推断未执行。

计算最多五分钟，SQL／连接获取与释放有界；完整 SHA256 在事务外进行。保留登记路径的目录／文件描述符，拒绝链接、非普通文件、越界、内容或清单错配；最终事务只做有界身份／大小／修改时间复核、当前版本检查及元数据写入。没有新后台队列、worker 或自动重试；普通 TaskBus 配置发布合同不变。

## 失败与响应丢失

- 接纳前：非法输入、请求忙碌或结构不可用返回明确 HTTP 错误。新表／必要唯一索引缺失为 `BACKUP_RETAIN_SCHEMA_NOT_READY`，只影响本功能；未加入 ControlService.Ready 或能力调用准入。
- 接纳后、确定尚未尝试最终提交：失败／取消留下 rejected 回执，原保护与意图不变；写失败则保留 checking，不假装结束。
- 最终提交结果不确定：不得把回执改成 rejected。UI 保留原 UUID、输入和冻结意图，只读取回执，或由原 Actor 明确点击重试同一请求；不自动重发、换 UUID、清空标记或恢复旧启用值。
- 进程中断后 checking 不自动消费。重新打开意图可找回请求，原 Actor 显式恢复；围栏或元数据已变化则留下拒绝结果。终态回执与保护来自同一事务，不允许“有成功回执却没完成本次保护”的半提交。

页面只在初载、手动操作、事件及重连时同步；没有周期状态查询。共享 SSE 不替换冻结请求或自动 POST。离开／换身份会取消当前流；服务器持久状态仍可找回。

## 页面使用

备份中心 → 数据库备份 → 备份清理待核对 → 检查文件现状 → **确认保留**。三个阶段使用纵向分区，窄屏使用选择器。填写原因并明确勾选，再点击“重新核验并确认保留”。

结果未知时，源切换、重新检查和父弹窗关闭会锁住；可以停止并核对、读取原回执或明确重试同一请求。完整性失败、活动任务、版本变化等均不显示成功。已完成后展示 UUID、时间和操作事实，可重新进入读取，不重复提交。原清理锁保留，因此该意图仍会出现在待核对清单，不能将本操作当成释放容量或解除意图。

## 迁移、启用与升级

模型定义集中挂载 `pkg/corex/db/database/migration.go` 的 `MigrateBackupCleanupRetentionModels`，不在 Service 构造器或应用请求中迁移。函数在 AutoMigrate 后验证必需列及 request_id 真唯一约束；冲突不得静默放过。

开发／测试先核对配置指向的数据库和 schema，备份后执行：

```bash
cd backend
go run ./cmd/database migrate -config /path/to/dev-config.yaml -scope backup-cleanup-retention
```

此 scope 仅允许 dev/test，使用统一迁移锁和事务，只迁移这张请求表，不夹带 IAM、业务表、seed 或 reset。正式安装使用既有受控升级 plan/apply 与维护窗口；不能将 dev scope 冒充正式升级。官方 `deploy/powerx/container/upgrade_registry.py` 已按当前源码重生成内容寻址载荷，`--check` 需通过；旧迁移 ID 不覆盖。

迁移并重新编译重启 Core、刷新前端后，人工操作可立即测试，不需等待自动周期。本轮只对自有临时集群执行上述增量验证，没有迁移日常库或重启用户服务。

## 验收与回退

[本轮证据](evidence/backup-cleanup-retention-validation-20261109.json)区分真实 PG/JWT/HTTP/SSE、受控文件、故障注入和浏览器；未将仿真文件叫作真实 pg_restore 或生产恢复。原生多包验收用 `-p 1`，因为同一个自有 PG 集群的整库备份锁跨 schema 共享，不能把测试互斥误当成业务故障。

检查并发、原 UUID 重放、旧快照、其他 Actor、损坏、取消／中断后恢复、围栏变化、SQL 回滚、唯一索引冲突、真实权限与响应丢失。前端检查明暗 1440/390、分区／草稿／回执保留、无轮询、未知状态不丢原请求。临时环境必须收尾。

回退时先停用新入口并保持资源暂停；保留回执表、原 token、protected 状态和版本，不通过删表／删审计／手改状态来恢复旧行为。新的已删除结果确认、解除意图、跨资源引用与整站恢复仍未完成。

## 代码映射与下一项

- `backup_cleanup_retention.go`、对应 repository：平台请求模型、唯一性、接纳／回执 CAS 与原子保护。
- `backup_ops/cleanup_retention*.go`：Root、双会话锁、版本／输入绑定、重新核验、失败和未知处理。
- `storage_governance/backup_policy_control.go`：既有共同政策核验与固定 RetainGuard，暂停／恢复围栏。
- `cleanup_review_handler.go`、`BackupRetainReview.vue`、`backupRetentionService.ts`：严格 HTTP/SSE、原请求核对与纵向分区。

## 2026-11-09 T171：恢复代次与正式升级联合验收

此段记录 v3 阶段；当前独立凭据协议已接续升级为 v4，详见[新增合同](backup-retention-recovery-ledger.md)。该阶段采用 `powerx.storage-recovery/v3`。原地维护启动显式升级现存完整 v1/v2 锚点，保留 AnchorID、恢复任务和 Epoch，重复升级不创建新代次。先写 binding，再写 epoch，支持中断后接续；登记后证明丢失不重建。旧程序不认识 v3 就失败关闭；旧快照包含 v2 Core 时，既有开发恢复适配流程保留当前执行器，不能报告旧程序逐字节恢复成功。不提供降低协议、删除锚点以便回退的路径。

可写核对版本由独立锚点绑定；控制链或锚点不可读时列表与结构诊断仍可读，但省略 `review_version`，页面显示原因并禁用新确认。原请求登记后、围栏捕获前中断也不能跨代次继续。完整读文件期间发生恢复，最终有界事务复核失败，保护和原 token 均不改。未完成旧请求的明确重试可留下 `BACKUP_RETAIN_REVIEW_CHANGED` 拒绝回执；它不证明恢复前没有发生过操作。

已完成的回执重放只读历史，即使当前 Job.protected=false 也不会重新保护。恢复代次核对与发布完成后必须重新读取当前记录，用新版本和明确的新请求确认；不会自动恢复原 enabled、drill 或其他策略。所有读回与异常保持原 SSE/手动同步合同，没有增加轮询、私有队列或依赖能力准入的新表。

[联合验收证据](evidence/backup-retention-recovery-validation-20261109.json)包含：真实 PostgreSQL schema 级 pg_dump/pg_restore、原始 UUID 的 checking/缺失边界、代次变化与丢失证明、正式 installed core callback + UpgradeRunner 的 plan/apply/重复/事务失败/重试。隔离恢复的是自有测试 schema 和受控产物；不是日常／生产整站恢复、完整物理恢复、旧二进制现场运行或长期目标验收。正式 core 回调按既有 public schema 合同在独立数据库执行；不通过修改其他域迁移来迁就测试。

本轮无新表、列、授权或迁移 payload；上轮新增回执表仍须通过中央迁移准备。启用本轮源码需要用户在既有维护流程重新编译／重启 Core；本轮未替用户执行。升级 v3 后回退程序必须支持 v3；不支持时保持暂停并恢复受支持版本，不能手改锚点协议。

该项已接入既有维护根并完成源码与隔离验收，详见[独立恢复凭据合同](backup-retention-recovery-ledger.md)。下一独立工程项是旧回执的只读盘点、显式导入与覆盖证明；新请求的独立登记不等于旧历史已全部导入。T155 长期私有目标、现场旧执行器接管、整站业务恢复和 7/14 天运行证据继续待办，不关闭整个 SG-11。
