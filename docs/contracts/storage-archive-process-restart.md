# T158：归档执行中断与重启合同

2026-11-10。延续 [原请求核对](storage-archive-run-request.md)，本轮完成归档组件源码、真实 OS 子进程与独立 HTTP/SSE／实际 Nuxt 验收。**不是完整 Core 进程、整站恢复、长期 OSS 或生产接管验收。**

## 主要行为

旧 `running` 记录不能证明当前存在执行器。当前进程只认本次实际执行注册的 job ID + execution UUID；这个 UUID 仅保存在本地私有任务记录，不通过 Job DTO、清单或原请求接口公开。其他进程即使持有原策略锁，本进程也只报告 `unconfirmed`，不会冒称自身正在执行。

| 本地记录 | 当前执行证明 | 页面／对账结果 | 自动行为 |
| --- | --- | --- | --- |
| running／完整快照远传 pending | 匹配当前进程的执行 UUID | active；分别呈现本地与远端状态 | 原执行继续 |
| running／success + pending | 没有匹配证明，但原策略或迁移锁仍忙 | unconfirmed；原记录不改写 | 不接管、不替代 |
| running | 原策略锁已排空 | status=interrupted；interruption_pending=true | 保留任务、局部内容，阻止替代采集 |
| success + pending | 原策略锁已排空 | 本地 success 保留；remote_state=unconfirmed，待处理标记开启 | 不自动重传或创建替代任务 |
| 历史终态，包括 remote success | 无当前执行 | recorded | 历史成功不能代替实时远端读取校验 |

`execution_state` 是只读观察值，不持久化。`interrupted_at`／`interrupted_phase` 留作历史；`interruption_pending` 是当前待处理围栏。overview 声明 `execution_version=1`，与原 `run_request_version=1` 独立；新 UI 仅在两项均支持时允许手动采集。旧 Core 需要由用户按现有流程编译重启，页面不会绕过旧协议。

## 重启识别、锁与调度

- NewService 和既有分钟 Tick 在扫描有界任务目录后，只对未形成终态的本地任务做对账。普通任务持原策略锁；迁移任务先持自身 migration 锁，再持原策略锁。忙锁跳过，读取／目录故障停止归档动作；不靠超时或心跳猜测死亡。
- GET Job／Catalog／Overview／原请求只观察，不为读取接管锁、改写任务或触发重试。独立列表 `execution_attention` 不随近期 100 条任务截断，旧中断任务仍能在顶部发现。
- 新采集在原策略锁内重新扫描，存在中断／未确认待执行任务返回 `409 ARCHIVE_INTERRUPTED_JOB_PENDING`。原 Idempotency-Key 仍指向原 Job ID，不能被此围栏误变成第二任务。
- Tick 先对账，再看策略；中断任务对应策略跳过新采集及自动远端清理，中断标记未解除前不进入自动重传。没有新增队列、页面轮询或业务能力准入依赖。
- 中断采集可能只留下 CAS 对象而尚未发布 Files 清单。因此只要归档根存在待处理的中断任务，保留共享对象回收围栏；其他任务的本地回收尝试也不能删除这些局部对象。staging 不在本轮自动清理范围内。

## 显式重传与历史副本

完整本地快照可在“日志归档与追溯 → 检索与取回 → 重传原任务”确认后，复用既有 `POST /api/v1/admin/ops/backup/archives/jobs/:job_id/retry`。在原策略锁内重新读取任务，使用新执行 UUID，沿用原任务、请求摘要、快照内容及中断历史。当前策略／凭据／目标检查和完整读回保持；只有完整远传成功才清除待处理标记。失败继续保留，页面／SSE 不自动 POST。

**采集未完成不能走 Retry**：当前没有冻结采集计划，无法证明重新读取仍是同一快照。本轮保留并阻止替代执行，不提供直接清除标记或删除局部对象的按钮。下一项实现有审核、版本约束及回执的原任务处置，不能通过手改 JSON 解除保护。

S3／SSH 发布清单会剥离当前执行归属和待处理门禁，保留中断时间作为历史；导入／恢复副本不从历史字段取得当前执行资格。已经记录的 remote success 必须再次读取精确远端版本、内容及来源握手，才能用于既有源清理流程。本轮不改变清理策略，不清理业务数据。

## 验收边界与证据

- `archive_ops/process_restart_test.go`：自有测试二进制运行**生产归档 Service 与调度器**，真实 SIGKILL 后同根启动新 OS 进程。采集场景由自有 PostgreSQL 的媒体引用表锁精确阻塞，文件已进入 CAS、pg_dump 尚未启动；另两场景在 S3 协议服务收下对象／清单、回复前杀进程。原请求／任务保留、局部字节 SHA 相同、无替代任务、重传前源清理握手 false、显式同任务重传后完整校验成功。
- 真实分钟调度观察 62 秒，原记录时间在测试中回拨 48 小时，确保策略确实到期；未发生自动新建／重试。这种时间回拨只属于夹具。
- 计数以显式 TestTarget 的测试对象创建／删除之后为基线；归档故障、重启及重传未新增 DELETE。不能把目标连通性探针删除误认为归档数据删除，也不能宣称整个测试零 DELETE。
- `archive_fault_browser_test.go` 的新 opt-in 模式在同进程重建 Service、保留 pending 记录，独立验证真实 Root JWT／HTTP／fsnotify／SSE；实际 Nuxt 明暗 1440／390、断流／重连、503、上下文和显式 Retry。此 UI 场景不冒称浏览器连着被 SIGKILL 的整个 Core。
- 原生 PostgreSQL／正式 TaskBus 的四类已接纳故障回归通过；Redis 为 miniredis，S3 为版本协议夹具，不替代阿里云 OSS。
- [证据索引](evidence/storage-archive-restart-validation-20261110.json)，[操作指南](../guides/deploy/027-monitor-center/usecase-us9-archive-restart.md)。源码和隔离验收通过不代表日常 Core 已加载；日常服务、策略、数据库、正式站及旧执行器未在本轮改动。

## 下一项和仍未完成的门槛

下一独立任务仍属于 T158：**中断采集原任务的受控处置**。先冻结原 UUID、采集版本／内容证明、执行器排空、保护保留、审计和响应丢失回执合同；在服务端重查后才能明确结束阻塞或启动另一次经授权的采集。通过并发、旧版本、断库／恢复、未知响应、活动执行及 UI 验收前，保持现有围栏。无需此时重启日常服务。

T155 待未来阿里云 OSS 配置后通过正式 UI 完成独立前缀真实归档、完整验证、源消失后精确取回与测试清理；整套 Core／worker 联合重启、完整停库、旧执行器现场交接、逐资源接管、整站恢复、外部密钥、双环境及 7／14 天长期证据仍待办，不勾选 T158／SG-09／SG-11 总门槛。
