# 归档目标的数据库权威与执行确认

2026-11-06，SG-11 的归档目标切片。目标配置接入数据库；归档策略随后按[策略合同](archive-policy-control.md)接入。备份／快照等其他域、完整恢复时代次和 SG-09 长期验收仍未完成。本轮不代表正式站已升级或全部商用门槛通过。

## 用户流程

入口是“监控中心 → 存储治理 → 日志归档与追溯 → 存储目标”，资源保留策略中的目标入口复用同一份状态和编辑弹层。

1. 首次只读显示旧目标数量和来源摘要。Root 明确确认“接管目标配置”；没有目标也要确认接管空目录。启动 Core 不会自动导入或开启归档。
2. 原 `targets/*.json` 保持原样，旧定义进入数据库版本的来源证据。每个旧目标的独立版本加一，旧验证失效。原策略的启用值不改写，但其旧目标版本／指纹不再具备执行资格。
3. 查看“已保存版本／已生效版本／执行端快照”。保存成功只表示数据库提交；后台正式 TaskBus 发布并确认后才能测试变更目标。未发布的修改会立即阻止该目标的新传输；未改动目标可继续使用其已确认版本。
4. 编辑明确的 S3 地址、私有桶、专用前缀、region、路径样式和凭据引用。凭据值在独立加密凭据管理中配置，不进入目标数据库文档。SSH 旧目标可经同一固定合同接管，当前 UI 编辑器仅提供 S3。
5. 配置变更后从实际执行节点测试私有桶、版本保留、生命周期约束、写入和完整按版本取回。测试对象随后删除。验证证明单独保存在运行账本中，不改写目标配置或数据库版本。
6. 验证通过后仍须按归档策略明确确认启用。保存目标、接管目标和测试目标都不会自动开启策略、传输历史日志或清理业务数据。

桌面／窄屏沿用纵向分区和弹层。事件刷新不替换表单和原始 CAS 基准；版本冲突保留草稿，用户明确选择“放弃此草稿，载入最新配置”。请求结果未知时禁止再次保存，只能 GET 原 UUID 回执，404 继续视为未确认。普通版本冲突不得重试覆盖。

## 权威与版本

复用 `ops_storage_control_configurations` 和不可变 `ops_storage_control_revisions`，平台 tenant 0；固定 domain 为 `archive-targets`。所有权固定为 `(host_id, filesystem_id, instance_id)`；目标列表最多 128 项，每项有 UUID、独立 revision 和固定 `TargetSettings`。列表 revision 作为 CAS 基准，避免并发编辑丢失；不同目标并发编辑也可能冲突，不自动重基。

首次接管把完整旧目标列表及其 canonical 摘要作为只读 origin 保存。后续修改只变更选中目标；origin 不变。创建目标使用请求 UUID 作为目标 UUID，响应丢失不能重复生成目标。重复请求须符合原操作者、原始动作及参数；查询回执同时返回原操作版本与当前发布状态。

配置、不可变版本、操作者／trace、请求身份和正式 TaskBus 发布意图在同一事务登记。发布回调仅写受控快照，不执行 S3／SSH 网络或文件删除。不存在通用 JSON 或任意远端代理入口；未知字段和凭据值字段拒绝。

## 执行快照和锁

主机预算私有目录中的 `archive-target-control.json` 是执行投影，不是配置权威。包含 scope、列表版本、摘要、完整文档和不可变版本的操作者／时间；原子写入并 fsync。每次解析目标均核对数据库 applied revision、不可变版本和文档摘要。断库、缺表、缺失／篡改／超前投影、身份不符、数据库单独回滚均阻止使用；不回退旧 JSON，也没有离线批准期限。

每个目标的验证摘要绑定 scope、独立版本和配置；未修改目标不因其他目标的列表更新而失效。测试网络在事务之外进行；记录证明前按相同数据库资源锁重新检查当前目标。锁顺序是数据库 → 运行账本；发布不能反向取得旧 `settings` 锁。凭据更换、吊销、主密钥变化和执行节点身份仍按凭据合同使证明失效。

`target-verifications/<id>.json` 为独立私有验证账本，0600、有界读取并拒绝软链／硬链；只能由测试最终确认写入。缺失或不匹配时显示“需要验证”。本地私有 `target-control-binding.json` 和运行配置 `target_control_required: true` 阻止**新版**独立 worker 读取旧 JSON 继续运行。旧二进制不理解新标记，部署仍必须停止旧临时任务并完成 T171 接管核对；不能只替换配置文件后宣称无双执行。

## 接口与授权

所有路径均位于配置 API 前缀下的 `/admin/ops/backup/archives`，兼容 `/admin/backup` 别名沿用相同 Root 守卫：

| 方法和相对路径 | 固定合同 |
| --- | --- |
| GET `/target-configuration` | 目标权威、版本、来源摘要和期望目标列表 |
| POST `/target-configuration/adoptions` | `request_id, legacy_digest, confirm:true` |
| POST `/targets` | `request_id, expected_revision, expected_target_revision, target_id, settings` |
| GET `/target-configuration/requests/:request_id` | 原操作者的只读不可变回执 |
| POST `/targets/:target_id/test` | 发布后独立测试，空对象请求 |

`target_id:""` 和 `expected_target_revision:0` 表示新建；其他更新必须明确 UUID 和版本。旧 `Target` 原样 POST 不再兼容，返回 400，不降级成文件写入。服务内部旧 `SaveTarget` 在绑定后返回 428。无数据库控制面时新增写入返回 503。

只接受平台 Root 的真实用户 access session；匿名、普通用户、API Key 和 STS 服务身份不允许。接口明确忽略为 `root_runtime_control`，不是租户／插件能力，不新增服务 grant。错误详见固定 OpenAPI；并发 409、接管 428、不可用 503，禁止把错误或 404 回执当作未提交证明。

UI 状态使用原备份 SSE。合并平台配置变更事件和私有文件事件，发布失败也发送失效通知；推送不含配置正文或凭据。初次载入、变更、手动刷新、重连才读接口；没有常驻状态轮询。连接断开提示保留草稿。

## 升级、故障处理与回退

- 集中升级入口需先迁移既有 storage control 两表，并准备正式 TaskBus 与可信主机／文件系统／实例身份。本轮没有增加新表，也不在构造函数自动迁移。
- 数据库或 worker 未就绪先修部署依赖，不尝试向旧文件保存。目标显示只读不意味着策略已迁移。
- 首次接管前旧文件摘要变化会拒绝接管；提交后、首次发布前变化会使发布失败；即使已有更高期望版本取代接管版本，首次投影仍核对原来源。保留来源证据，查清漂移后维护处理，不自动覆盖。
- 快照不符时暂停目标使用并核对数据库不可变版本、执行 scope 和账本。不要删除绑定文件或手工改 JSON 绕过检查。
- 回退到旧二进制会绕开新权威，不能在线双跑。须先停用并停止相关执行器，在授权维护窗口内核对两份配置和证明；数据库、凭据独立账本与主密钥的联合恢复仍属于 T171。
- 全套数据库＋快照＋绑定同时回滚、外部 KMS、跨节点部署接管、长期真实 OSS/S3 传输取回及上线后的增长曲线仍未验收。本轮不删除现有本地归档对象。

## 代码与验证

- 目标固定 DTO、执行钩子、私有证明及新版 worker 绑定：`backend/internal/service/archive_ops/target_control.go`。
- 数据库列表适配、来源摘要、CAS、回执、发布及最终验证锁：`backend/internal/service/storage_governance/archive_target_control.go`。
- 路由／Root 守卫：`backend/internal/transport/http/admin/backup/{archive_handler,routes,events_handler}.go`。
- 页面：`ArchiveGovernanceBody.vue`、`ArchiveTargetControlStatus.vue`，API client 为 `archiveOpsService.ts`。
- 隔离证明见 [验收记录](evidence/archive-target-control-validation-20261106.json)；区分原生 PostgreSQL／HTTP／正式 TaskBus 驱动、Redis/S3 协议仿真和真实浏览器，不等同真实云服务或正式部署验收。
