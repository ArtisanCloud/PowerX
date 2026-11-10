# 归档未知处置请求的人工结案

2026-11-10，T158 的独立工程切片。源码、隔离验证与现场接管分别验收。

## 用户行为与边界

中断采集的 retain_and_close 请求在落盘 registered 后可能因断库、退出或确认丢失，没有可确认的终态。Root 可先只读核对，再明确选择 **保留保护并结案请求**。结论为 `closed_unknown`：历史动作是否执行仍未知，人工结束的是待核对请求，原采集任务仍 interrupted / interruption_pending。

此操作不写任务、不扫描或认证 CAS 内容、不删除内容、不重放保留、不创建采集、不解除 GC／采集保护、不启用策略。结束原采集仍须独立完整核验 CAS 并生成新的 retain_and_close 请求。不能把“未找到回执”“任务看起来没变”或本次锁可取得当作此前未执行的证据。

## HTTP 与身份

默认前缀 `/api/v1/admin/ops/backup/archives`；跟随部署 API 前缀，测试覆盖自定义 `/custom/v2/admin/ops/backup/archives`。

| 方法与相对路径 | 行为 |
| --- | --- |
| GET `/interruption-requests/:request_id/review` | 同步、只读元数据核对，无后台任务；no-store |
| POST `/interruption-requests/:request_id/closures` | 原子记录人工未知结论；不自动重试 |
| GET `/interruption-requests/:request_id` | 既有原请求回执，新增 closed_unknown 和 closure 审计 |

三者都必须是真实平台 Root 用户会话；插件、API Key 和仅有 root 标记的服务身份均不可。新增两条路由为明确的 root_runtime_control 审计忽略，不发布租户／插件能力，不需要 capability-seed 或扩大授权。

POST 输入：

```json
{
  "request_id": "另一个全新 UUID",
  "expected_version": "本次 review_version 的 64 位摘要",
  "action": "close_unknown_keep_protection",
  "reason": "人工核对后保留未知历史与全部原任务保护",
  "confirm": true
}
```

路径是原处置 UUID，输入 request_id 是新结案 UUID。原因须首尾无空白、至少 5 个 Unicode 字符、最多 1024 UTF-8 字节，无 NUL 或 CR。拒绝未知字段、查询覆盖、超大 JSON 和错误身份。

## 核对、锁与正式控制

只接受完整且一致的原请求台账，状态 registered、无 result 或 closure；原任务必须仍是未完成中断采集，来源及当前暂停策略一致。完整／导入／恢复／迁移任务不能走此路径。

先取得原请求独占锁，然后取得原 GC／策略锁。核对摘要绑定原台账全文及输入摘要、当前任务全文、当前策略、主机与安装身份、独立恢复代次；observed_at 不进入摘要。当前策略必须暂停归档及远端清理，正式控制部署必须通过已发布配置 Guard。锁证明仅限该组件的原任务／GC／策略执行，不证明外部 pg_dump、SSH、rsync 或远端接收器已退出。

当前代次与原登记代次是否一致会明确返回；不同不意味着旧动作失败。人工核对可以在新代次作出“历史仍未知，当前保护保留”的结论，POST 必须与刚核对的当前代次和任务一致。

## 持久化、幂等与响应丢失

不增加数据库表、队列或第二个处置任务。原 `interruption-requests/<原 UUID>.json` 同一次原子写入包含原不可变身份、原输入及原因、closed_unknown、结案 UUID／摘要／核对版本／原因／人员／时间／原登记代次／当前代次／当前任务摘要。使用既有私有目录、文件 fsync、原子 rename 及目录 fsync。结案唯一效果是这条台账，因此无需跨任务文件或 SQL 的原子提交承诺。

全目录登记锁与最多 10000 条的有界扫描防止结案 UUID 被另一请求复用；后续 retain 请求也拒绝已用于结案的 UUID。同一结案 UUID 和输入重放仅返回既有原回执；换编号／换输入拒绝。原 retain UUID 一直只读，closed_unknown 不能重新执行 retain。

若正式 DB 确认失败或落盘确认丢失，POST 返回 `ARCHIVE_CLOSURE_UNCONFIRMED`。不自动恢复或重做。GET 原请求保持只读：当前任务全文和当前恢复代次仍匹配时，`current_protection_matches=true`；恢复、任务变化、任务缺失或锚读取失败不提供当前证明。保留历史 closed_unknown、人员及原因，不用旧结论重新设置保护。闭合文件损坏直接 503。

浏览器在 POST 前保存结案最小意图，包含原身份、原输入摘要、结案 UUID、版本和摘要；不保存原因正文、票据或 JWT。回复缺失后只 GET 原编号；仅同一原请求和输入、同一结案摘要（若本浏览器提交过）、当前保护匹配的回执才能解除待确认。404／读取失败／历史不匹配不解除；刷新、SSE 断开和分区切换不自动 POST。更换登录身份中止旧请求，旧结果不进入新会话。

## 页面与兼容

“日志归档与追溯”中既有待核对提示进入弹层，使用“概览 → 核对未知请求 → 确认人工结案”纵向分区，窄屏选择分区，原因保留。未知状态、连接异常和原编号始终可见。结案后展示原因、人员、时间和结案编号。

Core overview 新增 `interruption_request_closure_version: 1`；旧 Core 不展示可提交的结案操作。旧执行器不识别新台账格式时会失败关闭，不能混合接管；上线与回退均须先暂停执行并保存台账。禁止删除新台账或清空浏览器意图作为回退。

## 验证与未覆盖

[证据](evidence/storage-archive-request-closure-validation-20261110.json)记录 Service 并发／锁／回放／恢复、正式 PostgreSQL 断库、Root HTTP、真实 Nuxt 页面代理自有后端的桌面／窄屏和确认回复截断。UI 使用自有台账故障状态，不能替代实际 Core 崩溃现场；未停止用户服务、迁移日常库、部署正式站、改变现场策略或触碰用户对象。

全文件系统连同独立恢复锚一起回放后的 UUID 墓碑、整套 Core／DB／外部命令排空、阿里云 OSS 真传输与取回、旧执行器现场交接、双环境及 7／14 天证据仍是总门槛。
