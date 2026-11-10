# 归档本机外部命令退出与排空

2026-11-10，T158 的独立工程切片。仅为固定 pg_dump、OpenSSH、rsync 的本机前台进程组提供证明；远端接收器、逃离进程组的自定义守护程序、现场旧执行器与完整主机恢复另行验收。

## 发现与修复

原 pg_dump 使用普通 CommandContext；SSH／rsync 的 Cancel 虽向组发送 SIGTERM，WaitDelay 超时只会杀 leader，不能保证忽略 TERM 的子进程退出。组件崩溃后策略锁释放也不能证明子进程已经排空。

现在三种命令统一走 runArchiveCommand：

1. 在 fork 前原子写入 prepared 记录，创建独立进程组，再写 running／PID／PGID。
2. Linux 使用 waitid(WNOWAIT)，Darwin 使用 kqueue NOTE_EXIT，只观察 leader 退出，不提前回收 PID。正常退出也会结束同组残留；取消先 TERM，最多等待 1 秒，再向组 KILL。
3. 最后一次信号始终发生在直接子进程 Wait/reap 之前，因此该 leader 的 PID／PGID 仍被占用；不在 Wait 后对台账中的裸 PID 发信号。WaitDelay 限制管道等待 2 秒；Wait 后最多检查 2 秒的进程组消失，仅组不存在才确认排空。
4. `drained` 仅表示本机组消失，并不表示命令业务成功；调用方仍检查退出错误和远端 ACK。观察、持久化或管道证明失败保留未终态记录。
5. Service.Close 阻止登记新命令，并等待后台 worker 和直接 API 命令结束；Background 调用也与 Service 取消绑定。

这是原组件内的命令生命周期处理，没有新常驻协调器、端口、私有任务队列或页面轮询。

## 原任务、持久化与缺失证明

新执行开始时，原任务私有记录写入 command_protocol=1、execution_id；`command-executions/<任务UUID>.json` 先初始化 idle。每条命令以新 command_id 原位更新同一任务的序列证明，避免按对象无限增加文件。目标探测使用目标绑定的稳定 UUID；目录总上限 10000，达限停止新登记，不删除未知证明。

记录只含协议、任务／执行／命令 UUID、固定种类、状态、PID／PGID、主机摘要和时间，不保存参数、DSN、环境变量、密码或私钥。使用现有私有目录、文件 fsync／原子 rename／目录 fsync；任务文件与命令文件不是跨文件原子事务，中途失败宁可保留未知。任务每次采集／重传有新 execution_id；当前证明必须匹配原任务和当前执行。

prepared／running 的锁仍占用时显示 active；锁已释放而没有 drained 回执则显示 unconfirmed。未知、损坏、文件缺失、当前执行不匹配、其他主机及旧版本缺少协议都不能被自动认证。元数据中仍待处理的旧执行也计入全局注意项；已导入／重建的历史副本不借用运行身份。

SIGKILL 场景下，此实现**不承诺孤儿命令自动退出**：原 pending 记录会保留。重启不根据 PID 是否存在自动补杀或补写 drained；PID 可能已经重用。人工核对与受控补充证明是下一独立项，本轮不提供清标记按钮。

## 保护接入

- 中断采集的只读内容核验、retain_and_close、未知请求人工结案必须取得匹配本机排空证明；组件原锁可取得不再单独构成许可。
- 新采集／迁移／原任务重传可与其他确认活跃的命令并行，但任何失主未知证明都阻止新接纳；原请求的幂等查询仍只读，不重放。
- 本地到期／CAS 回收、目录裁剪、远端到期预览／执行重查及封存日志清理握手复用该保护。未知或活跃组不授予清理资格。
- 采集取消或缺失排空证明后保留原任务为 interrupted、interruption_pending；已采集 CAS 与源保持。SSH 上传取消后远端状态保持 unconfirmed，不能凭本机组退出认证异地提交。
- 不修改业务／插件调用准入、权限授予、数据库 readiness 或 PostgreSQL 审计配置，不增加数据库迁移。

## HTTP／UI

不新增公开路由。既有 overview 增加 `command_execution={protocol,active,unconfirmed,problem,jobs}`，最多列出 100 个需核对的任务；既有 job GET 增加派生 `command_drain_state`。这两个观测不写证明、不发信号。派生状态不进入归档清单，PID／PGID 和执行 nonce 不暴露给普通历史副本。

归档页面顶部常显未知保护和相关任务入口；弹层显示原因并禁用内容核验。切换任务分区仍可发现注意状态。已有 fsnotify／SSE 包含 command-executions，变化或重连时读接口同步，没有周期性列表／详情请求，也不会自动启动核验或 POST。旧 Core 没有新字段时保留原页面兼容，但不能据此取得新协议排空证明。

## 验收与部署边界

[本轮证据](evidence/storage-archive-command-drain-validation-20261110.json)：真实 pg_dump 在自有 PostgreSQL 表锁上等待后 Close，真实 SSH／rsync 子进程阻塞握手后取消，忽略 TERM 的父子进程组、孤儿组件 SIGKILL 与同根重启、损坏／缺失／旧协议围栏、UI 明暗 1440／390、SSE／空闲请求检查。真实进程测试与 UI controlled prepared 状态夹具分别留证，不拼接为浏览器贯穿整套 Core 崩溃验收。

本机 Darwin 运行验证；Linux amd64 交叉编译单独记录，不能冒称 Linux 运行验收。SSH 远端程序及真实 OSS、完整 Core／DB 崩溃、现场接管、全文件系统回放的独立墓碑、双环境及 7／14 天现场证据仍未完成。

上线／回退须先暂停旧执行并完成进程归属和现场交接。旧执行器不会识别新 command-executions 保护，禁止混跑。不要删证明、重写协议标记、强清锁或按裸 PID 杀进程作为回退。日常服务由用户管理，本轮未重启或部署正式站。
