# 缺损备份的人工隔离处置

> 接续实现：[隔离产物台账与只读复查](backup-quarantine-inventory.md)，提供退出待核对清单后的固定入口。下文“下一独立工程项”保留为本切片的历史计划；当前现场优先项仍为 T155 长期目标验收。

2026-11-09，T172 源码与自有隔离验收切片。接续[人工保留处置](backup-cleanup-resolution.md)，固定操作 `quarantine_cleanup`，只允许 Root 人类 access 会话。**逻辑隔离阻止备份被继续使用，不修复文件、不释放空间，也不是归档或删除。**

## 当前状态与历史事实

成功短事务同时提交精确 artifact/token 的 `cleanup_state=quarantined` 和新请求终态。Job.status、Job.protected、原 cleanup_token、原请求及未知历史保持原值；不创建“有效备份”结论，回执 `valid_backup_proof=false`。可用列表排除隔离产物，历史仍可见并标记隔离；下载、保护变更、恢复验证和恢复副本来源拒绝使用。恢复副本在策略锁内重读来源，防止计划后隔离、执行时继续沿用旧来源。

原 checking 不能改写为“此前未执行”。新 UUID 记录的是当前明确处置，独立终态是历史事实，不能在 SQL 回放后重新隔离当前记录。普通删除器仍拒绝所有非空 cleanup_state。没有自动解除隔离、重建文件、清 token、迁移文件或删除操作。

## 诊断与证据

只接受稳定的 `missing`、`size_changed`、`checksum_mismatch`、`sidecars_missing`、`sidecar_mismatch`、`metadata_invalid`。后三类也可能源于登记或旁文件错误，不等同于 dump 必然损坏。`verified`、`changed`、`unsafe`、`unavailable` 和中断不产生隔离资格。父目录缺失／不可固定时为 unavailable，不能冒充已证明的文件缺失。

预览是显式按需 GET SSE，持有既有 policy 会话锁，执行最长五分钟诊断；不在进入页面、切换分区或接收失效通知时自动全量读文件。冻结当前元数据、原 SQL／独立历史摘要、维护代次、分类及证据摘要。摘要包含私有文件身份，不输出路径、内容或设备明细。

从根目录逐级 no-follow 打开父目录，固定父目录和 dump／manifest／checksum 三个路径的存在性；文件仅接受普通文件、单硬链接。持有文件描述符，核对设备、inode、大小、模式、mtime、ctime 和链接数；缺失路径通过固定父目录与最终否定查找复核。完整诊断后、最终事务前再次核对；改写后恢复 mtime 也不能通过 ctime 检查。预览不授予最终执行权；POST 重做相同诊断、分类和摘要匹配。

既有 policy 锁与正式全局／计划暂停、恢复 acknowledged／Fence、无活动备份／drill、精确版本及 CAS 同时成立才允许提交。外部进程、旧脚本和任意文件系统写入不受该锁全面约束，不承诺与非协作进程的跨文件系统原子事务。

## 审计、恢复与升级

复用 protected 运维表 `ops_backup_cleanup_resolutions`，增加 `operation`（旧行默认 retain_cleanup）、`expected_diagnosis` 和 `diagnostic_digest`。中央迁移和精确 dev/test scope 挂载保持不变；缺列只阻止处置，不进入 ControlService.Ready 或普通插件能力容量准入。

维护协议 **v7**／独立凭据 **v4**，在既有维护根保存两种固定操作。真实 v6/v3 升级保留 typed retain 记录、原 UUID 和导入审计；已登记凭据缺失／损坏不重建。共享 4096 身份／16 MiB／1024 导入审计上限不变，不自动删除墓碑。旧程序不能降协议继续写。

顺序仍为独立身份登记 → SQL checking → 最终状态和 SQL 终态同事务 → 独立终态 checkpoint → 返回成功。缺 SQL 不补建，SQL 回滚保留当前状态，checkpoint 失败只允许原 UUID 补回执。跨代次 checking 无恢复执行资格；历史终态 GET 和同输入 POST 不重放隔离。恢复前覆盖证明包含两个操作所有 Scope；任一缺口阻止覆盖 SQL 和推进代次。

## HTTP 与 UI

[OpenAPI](backup-cleanup-quarantine.openapi.yaml)，路径相对可配置 API 前缀，默认 `/api/v1`：

| 方法／路径 | 行为 |
|---|---|
| GET `/admin/ops/backup/cleanup-reviews/:artifact_id/quarantine-preview?expected_token=...` | 一次 SSE progress／complete，失败为 HTTP 错误或 incomplete／EOF |
| POST `/admin/ops/backup/cleanup-reviews/:artifact_id/quarantines` | 严格 UUID/token/version/history/reason/confirm/expected_diagnosis/diagnostic_digest；accepted／progress／complete 或 uncertain／EOF |
| GET 同 `/quarantines?expected_token=...` | 精确意图最近 20 条隔离独立回执 |
| GET `/admin/ops/backup/cleanup-quarantines/:request_id` | 原 UUID 只读核对；不修补存储 |

拒绝未知字段／重复查询、自由路径和浏览器 verified 授权。独立 Root 运维控制不发布插件能力。retain 与 quarantine 的读入口互不混淆。EOF、404 和错误不证明未执行，不自动创建新请求。

页面入口：备份中心 → 数据库备份 → 备份清理待核对 → 文件详情 → 纵向“隔离缺损产物”。显式检查、分类说明、原因和确认后提交。未知请求冻结全部输入、聚合锁住父弹层；原 UUID GET 核对或显式同输入恢复。只读诊断可取消，身份切换／卸载释放流。共享 SSE 仅使诊断失效并同步回执，不自动重扫或 POST；无状态轮询。

## 边界与下一项

[指南](../guides/deploy/027-monitor-center/usecase-us9-cleanup-quarantine.md)、[证据](evidence/backup-cleanup-quarantine-validation-20261109.json)。仅源码与自有隔离验收；未迁移／重启日常或正式站，未处置现场产物，未接管旧执行器。内容寻址载荷一致性不能替代整个共享工作区的发布审查。

下一独立工程项是 **T172 隔离产物台账与只读复查入口**：当前产物退出注意清单后，Root 仍能按隔离状态定位产物、原 token 与 UUID 回执，查看处置分类／时间／原因并显式复查现状。验收覆盖分页／过滤、SQL 回放后历史与当前状态分开、桌面／窄屏和无轮询；不顺带开放解除隔离或删除。可先在自有数据库开发，无需重启日常服务。T155 长期私有目标与真实取回、旧执行器现场交接、整站业务恢复、外部密钥及 7/14 天运行证据继续待办，T172／SG-11 总门槛不关闭。
