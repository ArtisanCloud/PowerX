# 目标探测命令范围与人工排空确认

2026-11-10，T158。扩展[原任务命令确认合同](storage-archive-command-confirmation.md)，复用原三条 Root 接口。只确认某一次原目标探测的**本机前台命令组已经退出**，不验证当前目标、不证明远端接收器退出、不启用策略、不删除数据。

## 范围和兼容

- job 命令维持协议 2、原输入摘要；新增探测命令为协议 3，没有 job_id。record_id 是服务端按 `archive-command-probe/<target_id>` 派生的 UUID，每次实际探测生成新的 execution_id／command_id。
- fork 前绑定实际传入 SSH 的 Target：ID、版本、配置控制摘要、设置和凭据指纹。只保存凭据引用和摘要，不保存私钥、AccessKey、命令行、环境或令牌。
- 同一目标的探测由专用锁串行。原命令未知时不能覆盖记录或再探测；人工证明匹配原记录后，下一次显式探测建立全新执行身份，旧回执成为历史。
- 协议 1／2 的旧无任务探测没有原目标绑定，UI 可只读查看 `probe_scope_missing`，不能根据现有目标补造历史、挪用其他 job 或按裸 PID 杀进程。损坏记录继续告警并保护。
- 对正式 DB 权威，必须完整发布策略与目标两份配置；不从 JSON 缓存回退。legacy 独立组件夹具保留本地实现，不是正式安装的权威替代。

## 核对与事务

1. Root 用户会话取得原命令锁、原目标探测锁、GC 和本地设置锁；不能接管仍被执行器持有的命令。
2. 读取原目标、完整策略引用集合、原 OS 启动身份与独立恢复代次。仍须同主机／同 boot／namespace、有效启动标记、leader absent 且 group absent；identity_mismatch／prepared／缺证均不能确认。
3. 正式权威 guard 先核对发布快照，再在同一事务按执行开关→策略→目标行顺序锁定，校验作用域、revision=applied、文件投影和整份文档摘要。全部引用策略必须归档与远端清理都暂停，并在该快照锁内取得其执行锁。
4. 任一目标、凭据指纹或策略文档变化使旧预览失效，包括新增引用和未发布配置。断库／投影损坏不是“目标删除”。仅完整发布快照明确找不到原目标，才显示 deleted。
5. 目标 changed／deleted 后可重新核对原命令，明确确认原范围的本机退出；不会把原目标的测试结果赋给新目标。取回、归档或删除仍须独立完成其原有校验。

文件锁不阻塞等待；冲突返回保护状态。正式事务内不借用第二数据库连接；验证覆盖单连接池。预览只读，只有显式提交才写证明。

## 固定 API 和输入

默认 `/api/v1/admin/ops/backup/archives`，支持既有自定义前缀：

| 方法 | 相对路径 | 作用 |
| --- | --- | --- |
| GET | `/commands/:record_id/review` | 原目标／命令／引用策略和当前退出证据 |
| POST | `/commands/:record_id/confirmations` | 原版本下记录人工本机退出证明 |
| GET | `/command-confirmation-requests/:request_id` | 只读核对原 UUID，不重做提交 |

沿用严格 Root 身份、no-store、严格 JSON；不新增租户／Framework 能力，不改变业务调用存储准入或发布权限。

POST 在原输入末尾增加 `target_id`（job 不发送，omitempty 保持原摘要）：

```json
{"request_id":"新请求UUID","expected_version":"本次预览SHA256","execution_id":"原执行UUID","command_id":"原命令UUID","action":"confirm_local_command_drain","reason":"核对原目标的完整启动身份，记录本机退出","confirm":true,"target_id":"原目标UUID"}
```

路径必须对应 target_id 的固定派生 record_id；服务端重新核对，不能替换范围。review／receipt 新增 `subject=probe`（仅 review）、target_id 和 probe；probe 展示原名称／版本、当前版本／状态和引用策略的暂停情况。overview 的 `probe_command_confirmation_version=1` 才开放探测入口；旧 job 功能版本仍为 1，命令协议为 3。注意清单独立于最近任务列表，即使没有 job 仍可发现。

## 不可变证明与未知响应

沿用原命令单文件不可变正向证明、原 UUID 全局互斥、输入摘要、actor／原因／时间、恢复代次和 OS 身份绑定。不改原目标、策略、命令或数据对象。

- 同 UUID、同输入只读返回；换输入／换目标不允许。其他 UUID 不能再次确认同一命令。
- 进入正向写入前已知拒绝可记录不可变 rejected。正向写入开始后的任何错误都是未知；只能 GET 原请求，404／503 不证明未提交。
- current_record_matches 仅证明原命令文件／执行／主机／boot／namespace／独立恢复代次仍匹配，不是目标验证有效或原任务完成。后续目标编辑不推翻已经发生的本机退出；新执行或恢复代次不能继承它。
- 浏览器仅以当前 session／tenant 保存原 UUID、record／target／execution／command、版本／输入摘要和时间，不保存原因、令牌或凭据。精确范围、摘要和当前证明匹配才解除未知状态；换身份不消费旧结果。
- SSE 变化使预览失效但保留草稿。手动核对／重连同步可 GET，无周期查询，无自动重发 POST。

## 验证与发布边界

[指南](../guides/deploy/027-monitor-center/usecase-us9-archive-probe-confirmation.md)与[本轮证据](evidence/storage-archive-probe-confirmation-validation-20261110.json)。真实 SSH 夹具、原生 PostgreSQL、Root HTTP 和真实 Nuxt 覆盖不同层；HTTP／SQL 夹具是在真实 ssh 退出后回放丢失 final ack 的 running 标记，不能称为整套 Core 崩溃恢复。Linux 仅交叉编译，不是原生运行。没有部署正式站、迁移日常库、接管现场执行器或完成真实 OSS 验收。T158／SG-09／SG-11 整体仍待办。
