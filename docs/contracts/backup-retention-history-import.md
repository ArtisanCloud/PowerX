# 旧备份保留回执盘点与显式导入

> 后续 T172 已实现完整产物的[人工保留当前意图](backup-cleanup-resolution.md)：维护 v6／凭据 v3 兼容保留本文 v5/v2 历史，并追加固定 operation 标签及新 SQL 审计。原普通保留与导入回执保持原值；下文“下一工程项”及载荷差异属于本切片当时边界。新迁移交付已使用官方生成器冻结当前摘要，完整共享工作区发布评审仍未关闭。

2026-11-09，T171 的源码与自有隔离验收切片。接续[独立恢复凭据](backup-retention-recovery-ledger.md)，不修改普通调用准入、权限、数据库期限、备份保护或原清理锁。

## 覆盖对象与权限

固定来源是当前数据库的 `ops_backup_cleanup_retentions`，包含软删除行检查。Root 人类 access 会话可以盘点、导入当前安装 Scope 的历史，并读取最近 20 次导入回执。插件 API Key、STS 和普通租户管理员不能调用。四个精确管理路由按 `root_runtime_control` 排除出能力自动发布；没有新增服务能力或授权。

盘点分为 `covered` 已有独立身份及结论、`missing` 尚未保存／尚需补入已确认终态、`conflict` 身份或终态冲突、`invalid` 字段／摘要不合法。`checking` 原意是结果未知；导入仍保存 checking，恢复绑定置空，不获得继续保护资格。原 Actor、原因、时间及输入摘要保持原值，导入操作人另记审计。独立终态可以覆盖 SQL 恢复出的 checking；相矛盾的两个终态必须阻断。

当前 Scope 的 `complete=true` 不代表全库可恢复。其他 Scope 单独计数；当前 `restore_ready=true` 仅表示本次观察无归属遗漏且当前 SQL 请求全部有独立覆盖，不证明备份文件、保护状态或整站恢复成功。历史导入回执的 complete 只适用于当时快照，新的 SQL 请求和当前覆盖仍需重新盘点。

## 冻结输入与原子提交

预览快照绑定固定来源指纹、SQL 行身份／更新时间／内容／数量、独立记录摘要和当前恢复代次。分页及过滤不会缩小导入范围；输入不接受客户端行列表、Scope、路径、SQL 或 verified。上限为 4096 条 SQL 请求／独立身份、1024 次独立导入审计、单文件 16 MiB；无自动修剪或按天丢弃 UUID。超限保持原记录并阻断新写入。

1. Root 提交 UUID、预览快照、3～256 字符原因和明确确认；UUID 永久绑定 Scope、操作人及完整输入。
2. 独立维护根先原子登记 pending，然后才发送 SSE accepted；登记失败不进入导入。
3. 持有既有维护根独占锁，读取当前恢复代次；复用正式控制链的已发布暂停、恢复核对和同事务执行守卫，再取固定 SQL 表 SHARE 锁。锁顺序为维护根 → 全局执行控制 → SQL 来源；通过固定 epoch 参数避免在独占锁内重新 flock。
4. 整批重新核对快照、字段、冲突和容量。不一致时只保存 rejected 审计；没有部分成功。成功时，全部历史身份／终态和 completed 导入回执由同一个文件 checkpoint 提交（临时文件 fsync → rename → 目录 fsync）。SQL 仅查询／加锁，无 DML。
5. SSE complete 或 GET 原 UUID读取持久结果。请求丢失、断库、文件写失败或 uncertain 仍锁住原 UUID；只读核对后，可由原操作人明确重试同 UUID／快照／原因。没有自动 POST。跨代次待处理导入会持久拒绝，不能跨代次继续复制。

新导入必须处于正式控制链**确认暂停**，且恢复状态为空或已 acknowledged。只暂停浏览器、未发布配置或本地布尔值均不满足。已完成导入的读取／幂等重放不依赖 SQL 回执表在线，不重新执行导入。

## 恢复与升级

维护协议升至 `powerx.storage-recovery/v5`，同一凭据文件升至 `powerx.backup-retention-ledger/v2`，增加 imports 操作审计。不新增数据库表、列或迁移 payload。完整 v4/v1 凭据保留原 LedgerID、身份和历史后原子升级，接续合法中断状态；v1/v2/v3 的首次凭据初始化仍沿原流程。已登记 v4/v5 凭据缺失或损坏时禁止重建。旧程序不识别新协议时失效关闭；升级必须先停止旧写入者，不删除协议标记回退。

受控当前实例恢复和数据库补偿，在覆盖 SQL **之前**调用 `BeginChecked`：从实际当前 DSN／Core schema 进行有界只读快照检查，核对所有 Scope 的 SQL 请求与独立记录。遗漏、坏行、冲突、来源不可用、超限均阻断覆盖，不先推进恢复代次。表确实不存在表示当前库没有该表请求；结构损坏或读取失败不能当作空表。成功报告增加 `backup_request_history_coverage`。此检查不挂到插件 Lookup／普通业务容量准入，不将配置或历史问题包装成容量耗尽。

## 管理接口与页面

默认 API 前缀 `/api/v1`，可由现有部署配置改变；[OpenAPI](backup-retention-history.openapi.yaml) 使用相对管理路径。

| 方法与路径 | 含义 |
| --- | --- |
| GET `/admin/ops/backup/cleanup-retention-history` | page 默认 1、page_size 默认 20（最大 50）、coverage 固定枚举；严格拒绝未知／重复查询参数 |
| POST `/admin/ops/backup/cleanup-retention-imports` | 严格正文，accepted／complete SSE；接纳后失败发送 uncertain，结果仍未知 |
| GET `/admin/ops/backup/cleanup-retention-imports` | 当前 Scope 最近 20 次独立导入审计 |
| GET `/admin/ops/backup/cleanup-retention-imports/:request_id` | 原 UUID 读取；404 不证明未执行；其他 Scope 返回拒绝 |

备份中心 → 数据库备份 → **历史回执覆盖**。内部纵向“盘点／导入／回执”一次展示当前任务，窄屏改用选择导航。主要覆盖状态与失败保持可见，切换保留草稿和未知 UUID。POST 使用单次 SSE，既有 backup_cleanup 推送触发只读同步，页面无周期查询；退出／换身份中止旧连接和结果应用。预览读取失败保留旧内容但禁止提交；历史回执读取失败单独展示。

## 验证、边界与下一项

[证据](evidence/backup-retention-history-import-validation-20261109.json)记录源码、隔离 PG、SQL dump/restore、文件原子故障、Root HTTP/JWT/SSE、真实 Nuxt 浏览器和临时资源收尾；不等同日常或正式站部署。操作步骤、排障及回退见[指南](../guides/deploy/027-monitor-center/usecase-us9-backup-history.md)。

整包发布检查仍有共享工作区门槛：`upgrade_registry.py --check` 当前返回 `UPGRADE_PAYLOAD_CHANGED`（core 组）。本切片没有修改 COMMON/GROUPS 中的生产输入；保留其他模型／迁移变更，不自动重生成其载荷或迁移 ID。需相应变更合并评审后再通过整包检查，不能将隔离测试称为发布验收。

未覆盖外部工具覆盖维护根、整盘回滚、同机全部副本丢失、拥有维护目录写权限者篡改（SHA256 不是外部签名）、跨 Scope 自动导入或未知清理锁的解除。T171／SG-11 不关闭。

下一独立工程项按 T172 推进**跨代次未知清理意图的人工终结**：先定义保留／隔离处置与原未知历史的关系，验证执行器排空、当前 artifact/token 与内容重新核验、UUID/CAS/审计和丢响应边界，再提供 Root 操作。不得把历史 checking 改成“此前未执行”，不得在解除锁时顺带删除文件。可先用自有 DB／维护根开发；现场 T155 长期私有目标及取回、旧执行器接管、整站和 7/14 天运行验收仍需独立完成。
