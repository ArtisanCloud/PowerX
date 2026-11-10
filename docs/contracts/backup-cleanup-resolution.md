# 未知清理意图的人工保留处置

2026-11-09，T172 的源码与自有隔离验收切片，接续[保留请求](backup-cleanup-retention.md)和[旧回执覆盖](backup-retention-history-import.md)。只实现完整、可核验备份的非删除处置。当前采用的协作备份／验证执行器必须遵循既有 policy 会话锁与正式执行守卫；现场旧执行器交接仍须单独证明。

> 后续隔离切片已实现：[缺损备份人工隔离](backup-cleanup-quarantine.md)。当前协议为 v7／凭据 v4；下文 v6/v3 为本切片引入版本，旧操作合同保持。

## 结果与历史

固定操作 `retain_cleanup`，仅 Root 人类 access 会话可用。新 UUID、安装 Scope、Actor、artifact/token、预览版本、历史摘要、原因及恢复代次不可变。新 SQL 审计表 `ops_backup_cleanup_resolutions` 单独记录 checking→retained/rejected；原 `ops_backup_cleanup_retentions` 和独立原请求记录均保持原值，不将 checking 改写成“此前未执行”。

成功事务同时提交：`Job.protected=true`、精确产物的 `cleanup_state=retained` 和新处置终态。**原 cleanup_token 保留**。没有 unlink、删除、原位清空 token、开启计划或自动重做清理。已人工保留的产物退出当前 pending/unconfirmed 清单；终态回执只证明历史操作，不证明当前文件或恢复资格。即使以后撤销 Job 保护，非空 cleanup_state 仍阻止自动清理；重新纳入清理需未来明确流程，本轮不提供。

## 前置条件与核验顺序

1. 当前 policy 受数据库权威管理；全局暂停已发布并确认，恢复已 acknowledged，计划、当前计划及 drill 均关闭。
2. 获取原 UUID 会话锁和既有 policy 会话锁；同一策略的协作备份、清理、验证必须排空。SQL 中没有 pending/running 备份和 queued/running drill。页面预览只是当时观察，提交仍重新获取锁与核对，不能用浏览器 verified 或“停了页面”替代。
3. 当前 artifact/token、作业、版本与状态有效；只支持已登记策略的成功备份（policy_id 非零）。原请求历史全部有独立覆盖，其他 Scope、坏行、冲突、超限均阻断。同代次未完成的新处置必须先核对原 UUID；没有新的自动请求。跨代次未知历史可以在新明确核验中保留原结论。
4. 服务端冻结当前元数据、完整原 SQL／独立历史摘要及独立恢复绑定。整个 dump 完整 SHA256、大小、manifest 和 sha256 sidecar 校验，在长 SQL 事务外完成，持有 policy 锁和已打开文件。
5. 最终短事务重新经过正式全局／计划／恢复 Fence，锁住产物、Job 和处置行，重读历史和版本，确认无活动作业，复核全部文件描述符／路径身份。保护、状态 CAS 和终态回执同提交；失败回滚全部当前状态。

缺文件、缺 sidecar、校验不符、孤立／软删行和状态漂移均不能伪装为保留成功，也不因失败删除文件。受控执行器以外的外部工具、旧脚本、第三方进程及整盘覆盖，不由会话锁作出排空承诺。

## 两阶段审计与恢复

既有维护根 `storage-retention-ledger.json` 的 Records 增加固定 operation 标签、历史摘要与 Job/Policy 身份。维护协议 **v6**／凭据 **v3**；共享 4096 个身份、16 MiB，导入审计仍上限 1024。完整 v5/v2 升级保留原请求和导入回执；已登记凭据丢失不重建。旧程序失效关闭；协议变化使旧未完成请求成为历史，必须以当前版本明确复核。

先原子登记独立 checking，再接纳 SQL。只在新登记的同一次调用创建 SQL 行；已存在独立 UUID 而 SQL 缺失／冲突时不补建。SQL 已确认终态后，明确 POST 补存独立终态，完整路径成功才答复完成。

| 故障 | 行为 |
| --- | --- |
| 独立登记前失败 | 不接纳 SQL，不改变保护或意图 |
| 独立登记后 SQL 接纳失败／历史丢失 | 保留 UUID 和未知；不补建、不重放，需维护代次／历史核对 |
| SQL checking，当前代次和身份一致 | 原 Actor 可显式同输入重试；重新获取锁、完整读取和复核 Fence |
| SQL 当前状态／终态提交失败 | 原子回滚保护和清理状态；能确认回滚且代次一致时另记 rejected，否则未知 |
| SQL 已终态、独立 checkpoint 失败 | GET 保持 checking，给出 `BACKUP_RESOLUTION_CHECKPOINT_REQUIRED`、`sql_committed` 和原 Actor 的 can_resume；继续原 POST 只补回执，不重写保护或清理状态 |
| 独立终态、SQL 回放或缺表 | GET／同 UUID POST 只读独立历史，不重新保护当前库 |
| 跨代次 checking | recovery_blocked，无执行重放；当前有效产物另行明确预览和处置，原历史不改 |

GET 不修补任何存储。UUID 的 404／查询失败不证明未执行。新处置 pending／retained 存在时，原普通保留 checking 也不能恢复执行；其原记录保持不变。受控当前恢复／补偿前的覆盖检查同时核对新 SQL 审计表所有 Scope；缺口阻断 SQL 覆盖及代次推进。

## API 与页面

[OpenAPI](backup-cleanup-resolution.openapi.yaml)，相对既有可配置 API 前缀：

- GET `/admin/ops/backup/cleanup-reviews/:artifact_id/resolution-preview?expected_token=...`：固定当前条件与历史摘要。
- POST `/admin/ops/backup/cleanup-reviews/:artifact_id/resolutions`：严格 UUID/token/version/history_digest/reason/confirm，accepted／progress／complete SSE；未知为 uncertain／EOF。
- GET 同 `/resolutions?expected_token=...`：精确意图最近 20 条独立处置历史。
- GET `/admin/ops/backup/cleanup-resolutions/:request_id`：原 UUID 只读核对。

所有接口拒绝未知／重复查询和自由来源；四条 Root 管理路由不发布为插件能力，无 API Key／STS 授权。页面入口为备份中心 → 数据库备份 → 备份清理待核对 → 文件详情 → 纵向 **终结意图并保留**。冻结草稿／请求，未知时聚合锁住父弹层关闭及重选意图；确定恢复阻断后保留 UUID 但允许退出。完成先展示历史回执，没有新表单覆盖结果。共享 backup_cleanup SSE 同步，离开／换身份释放旧流，无轮询或自动 POST。

## 迁移与交付边界

新平台运维表由 `pkg/corex/db/database/migration.go` 集中挂载，目录分类 protected、排除日志清理。新功能单独检查列和实际唯一索引；不加入 ControlService.Ready 或普通数据库／插件容量准入。dev/test 有精确 `migrate -scope backup-cleanup-resolution`，只准备该表；日常和正式站未执行。

官方载荷生成器已冻结本次 schema 和中央入口。`cmd/database/migrate.go` 属于四组 COMMON，因而四组内容寻址 ID 均变化；之前共享工作区已有 core 差异记录，不代表其他改动已全面评审。仅载荷一致性和自有 Core 升级回调验收通过；完整发布／其他 payload 回调／现场升级仍需独立审查。不能降协议或删除独立凭据来回退。

[指南](../guides/deploy/027-monitor-center/usecase-us9-cleanup-resolution.md)与[证据](evidence/backup-cleanup-resolution-validation-20261109.json)给出操作和隔离边界。完整 T172／SG-11 未关闭。下一独立工程项为缺失、损坏或无法完整校验产物的**人工隔离处置**：先定义诊断证据、当前身份和维护 Fence，只记录隔离与阻止自动处理，不伪称有效备份、不删除、保留 token／原历史；以 UUID/CAS、并发、恢复和响应丢失验收。长期私有目标及真实取回、旧执行器接管、外部密钥、双环境和 7/14 天证据仍待完成。
