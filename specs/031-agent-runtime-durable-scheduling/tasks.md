# Tasks: Agent Runtime 持久化调度

全部任务未完成；勾选需附代码、合同、测试和实际运行证据。

- [x] T001 清点现有 SSE/Run logger/DB 快照、007 invocation、024 reducer、004 Redis TaskQueue、010 模型闸门与实际消费者，冻结迁移/兼容矩阵；证据见 [current-baseline.md](./current-baseline.md)。
- [ ] T002 更新 007 OpenAPI、024 Run State schema 与 Framework typed 消费合同；若新增正式 capability，同步 Admin + `core_internal` 服务面和显式 API-key grant 元数据，并验证 capability-check/seed/grant-status。
- [ ] T003 实现 Redis RunStore：幂等受理、版本 CAS、event_seq、同分片 outbox 与低频 Admission 对账；生产检查 AOF/noeviction/健康并失败关闭。
- [ ] T004 在 004 TaskBus 实现并验证 Streams 消费组、租约续期/fencing、重领、ACK/NACK、延迟重试、死信和去重；Agent 禁止 DB polling fallback。
- [ ] T005 实现不可变计划修订、DAG 就绪判断、Run/租户/资源池上限、公平队列和背压；不同资源池可并行。
- [ ] T006 实现跨实例物理模型容量池，实测单槽 Ollama 下一个运行一个排队；Profile 并发策略不得超发物理槽。
- [ ] T007 拆分 queue_wait/request/run/lease 计时与步骤/token/成本预算，输出准确 reason_code 和 Trace 时长。
- [ ] T008 实现 Worker 故障恢复、Redis 对账、过期 fencing 拒写、业务幂等回执和无幂等副作用人工核查。
- [ ] T009 将旧 SSE 执行入口迁为提交/订阅同一 Run；实现 `run_id + event_seq` 续订、历史快照、单消息执行卡片及旧 reducer 兼容。
- [ ] T010 实现对象存储归档、可读校验、locator、保留期清理和归档积压背压；Trace/报告按权限读取。
- [ ] T011 完成两 Core/多 Worker、浏览器断线、重启、重复投递、租约失效、Redis 故障切换、归档故障和 RPO/RTO 演练。
- [ ] T012 完成压测与上线门禁：公平性、队列 P95/P99、模型槽利用率、Redis 内存/写入、DB 写入和归档积压；记录真实 HTTP/SSE 及跨实例证据。

2026-10-04 T005/T007 开发进展：正式 Worker 增加 Redis 原子的 Run/租户共享执行额度、续租和过期回收；单 Core 的 Worker 槽不再复制成额外租户额度。容量等待延后 500ms，不增加业务失败次数、不重置 queued_at，同因等待不重复写事件；修复消费入口缺少延迟项推进的问题。两 Worker 验证同 Run 峰值 1、其他租户正常完成，真实 Redis 的独立进程验证 Run/租户配额隔离。新计划固化 DAG 步骤与 tooling 节点预留预算，规划超额报 budget.exhausted，执行重建并校验。跨 Run 优先级、生产公平性/容量压测、嵌套调用及 token/费用/修订累计预算仍待完成，T005/T007/T012 不勾选。

2026-10-04 双完整 Core 运维验收：独立 18077/18177 实例同时启动通过；停止 A 后 B 持续维护共享归档健康，重启 A 并停止 B 后 A 继续维护。确认 Bootstrap 实际采用 Queue 回退后的 Redis DB 5，修正联调脚本默认 DB 0 的误检和连接回退，增加 Redis 权威状态启动检查。测试进程全部关闭，正式 8077 未重启。此项未执行活动业务 Run，故不能代替 T011 的跨实例接管、浏览器、HA/RPO 或 T012 的压测验收。

2026-09-23 开发检查点：T002 已补目标版 Run/Task/Event schema 并在 `007` 标明当前三帧游标与未来序号游标的区别；Framework typed 客户端和真实 wire 尚未切换，故保持未完成。T003 已有独立 `backend/internal/service/agent_run` Redis AOF/`noeviction` 门禁、Run 状态/事件 CAS、不可变计划版本、初始 Task 状态与原子待投递 outbox；定向及 race 测试通过。启动接线、受理幂等键、Worker Attempt/租约、对账和执行链仍未完成，故 T003 保持未完成。当前本机 Redis `appendonly=no`，不符合生产门禁。

T004 已新增通用 Event Fabric Redis 租约管理器与 Streams 消费组驱动，覆盖幂等入队、竞争领取、续租、过期重领、递增 fencing token、旧 Worker ACK 拒绝、延迟重试和死信；驱动尚未挂入正式 Worker 启动与运维装配，故 T004 保持未完成。

后续检查点：outbox 投递已连接到该通用 TaskBus 并验证跨分片投递重放去重；单次 Worker 尝试已接入 RunStore 的领取、启动、心跳、结果/验证证据或失败状态写入，以及持久化后 ACK。旧 fencing token 被拒绝；无证据结果进入 `manual_review_required` 而非自动重放。T003/T004 仍需正式启动装配、受理幂等锚点、真实 Agent executor 与跨实例运行验收；Run 最终 Outcome、SSE 续订、归档及模型池到执行链的接线尚未完成。此检查点只证明基础组件，不能代表原截图中的运行链已修复。

T006 已新增独立 Redis 物理模型槽池，部署容量在 Redis 中固定并跨 Core 实例、租户共享；单槽与配置漂移测试通过。尚未注入真实模型调用链、续租/公平调度和多实例 Ollama 验收，故 T006 保持未完成。

T010 已新增 S3/MinIO 报告归档适配、确定性对象键与 SHA-256 读回校验；对象写入失败时 Redis 事件及归档状态保持不变。消息 meta/业务存储 locator、保留期清理、容量背压和实际对象存储验收尚未接线，故 T010 保持未完成。

后续代码检查点：RunStore 新增按租户、环境、会话和请求幂等键稳定生成 Run ID 的受理方法；同键重试返回原始 Trace/期限，不会再产生第二条受理事件。新增不可变计划推进器，自动入队同一 Run 的独立根任务、等待串行依赖、跳过失败依赖，并按验证结果聚合最终 `completed/partial/failed/blocked`；Worker 完成后会推进计划并投递后继任务。Redis 模型池增加跨实例 FIFO 等待、队列上限和过期等待项清理，立即申请也不能插队。RunStore Task/Event 增加领取时间、资源池和实际排队时长。Run 期限改为显式 `ai.runtime.run_deadline` 默认 30m，与任务数/并发波次和单次模型请求期限分开；Agent runtime、config、RunStore 和 Event Fabric 的定向 race 测试通过。正式 Agent Session 入口仍是数据库受理 + 进程内 goroutine + 250ms DB 取消轮询，尚未切换新调度链，以上只能算 T003/T005/T006/T007/T008 的组件进展，不应勾选为生产完成。

超时检查点：`ai.runtime.queue_wait_timeout` 默认 10m；RunStore 将未领取任务超时标记为 `queue.timeout`，且只在领取后记录实际 `queue_wait_ms`。Run 截止时未开始的任务进入 `run.deadline`，不再把依赖任务送入队列；Worker 在过期 Run 上不调用 Executor。上述状态转换连同邻接 Agent Session/HTTP/LLM 包通过定向 race 测试。发现旧 RuntimeBudget 按值复制 `sync.Mutex` 后，PlanController 与 PlanRevisionService 改为传递共享预算指针，并在保存计数时加锁读取；Runtime 定向测试和 `go vet` 已通过。当前实现没有正式入口切换和跨实例实机证据，原故障仍未完成验收。

生产门禁验证：用独立 Redis Server 开启 `appendonly=yes`、`maxmemory-policy=noeviction` 运行 `TestValidateProductionRedisRealServer` 通过，进程已关闭；开发默认 Redis 的 `appendonly=no` 仍会被门禁拒绝。`go test ./internal/server/agent/...` 与 `go vet ./internal/server/agent/... ./internal/service/agent_run ./pkg/event_bus` 通过。正式部署前仍需可恢复 Redis、Worker 装配、实际对象存储和跨实例验收。

故障恢复检查点：`RecoverRun` 接受低频受理 locator 提供的 Run 身份，对活动 Run 重放计划推进与未投递 outbox；测试覆盖 Core 在根任务投递前退出、Worker 保存结果后未 ACK/未释放后继任务、重复恢复不重复生成 outbox，已完成任务再次投递不会重做副作用。正式 locator 列表、周期扫描器和 Redis 超 RPO 丢失的业务回执核查仍未接线，T008 保持未完成。

2026-09-24 模型调用接线：`ai.runtime.physical_model_pools` 声明部署物理容量、等待上限和租约 TTL；Bootstrap 使用 Event Fabric Redis 做 AOF/`noeviction` 门禁后才发布配置，启用配置的 Ollama 未匹配 endpoint/model 时失败关闭。LLM `Invoke`、`Stream`、`StreamOrFallback` 先取得原有 Profile 限额，再从共享 Redis 物理池 FIFO 等待；单次 provider timeout 从取得槽位后开始，调用期间续租，结束后释放。规划器此前直接 `NewClient().Invoke` 绕过统一入口，已改走 `llm.Invoke`；池不可用、未配置与排队超时不再被规划器静默降级。测试用模拟 Ollama HTTP 服务在 Profile=2、物理容量=1 时断言峰值实际 provider 并发=1，队列超时独立分类；独立 AOF/`noeviction` Redis 上启动配置门禁通过，进程已关闭。仍需配置真实部署 pool、双 Core/单槽真实 Ollama、Worker/RunStore 正式执行接线，因此 T006 保持未完成。

原因码检查点：Run/Task/Verifier 现优先按类型化错误区分 `queue.full`、`queue.timeout`、`provider.timeout`、`store.unavailable`，避免 provider 自身的 `context deadline exceeded` 被误报成 Run 超时；031 target OpenAPI 已补队列满原因。覆盖相关包的 `go test -race`、`go vet`、OpenAPI YAML 解析和 `git diff --check` 通过。正式 Worker 与 SSE 订阅尚未迁移，T007/T009 保持未完成。

2026-09-24 订阅接线检查点：Agent Session 新增按 STS、租户、会话、受理记录核验的 Redis Run 事件订阅接口；HTTP 同一路由在持久化模式下以十进制 `event_seq` 续订，历史裁剪返回包含当前任务状态的版本一致快照。连接建立时校验数据库归属，长连接每 30 秒复核授权，事件轮询只读 Redis。跨租户/插件、游标重放、未来游标、Redis 不可用与 HTTP SSE 合同的定向 race 测试及 vet 通过。当前正式 `Deps` 仍装配旧进程内 Executor，持久化订阅模式未启用；必须先完成受理、Worker 和结果回写的同一 Run 切换，T009 保持未完成。

2026-09-24 受理握手检查点：正式 `Invoke` 增加可配置的持久化分支，但生产 `Deps` 尚未启用。该分支先在业务数据库写 `pending_create` 低频受理锚点，以同一 invocation UUID 在 Redis 创建 Run，再把锚点标记 `admitted`；失败时原幂等键可补齐创建，不允许另一幂等键绕过活动 Run。已受理锚点若 Redis 状态丢失则失败关闭，不能从数据库重建已可能执行的 Run；`GetInvocation` 对持久化 Run 读 Redis 权威状态，取消在 Worker fencing 接线前失败关闭。新增 `run_env`、`admission_state` 字段由现有 Agent Session AutoMigrate 挂载。定向 race 测试和 vet 通过；规划任务入队、Worker 执行、取消 fencing、终态回写和生产配置/跨实例验收仍未完成，T003/T008/T009 保持未完成。

2026-09-24 规划队列检查点：持久化 `Invoke` 在受理锚点成功后，原子写 Run `planning` 状态、规划任务及同分片 outbox，再向 Redis Streams TaskBus 投递稳定的 `run_id:0:plan:1` 消息。投递失败的同键重试和 `RecoverRun` 可重放 outbox，不产生第二条规划事件；数据库已标记受理、API 在写规划 outbox 前退出时，恢复器也能补建唯一规划请求。规划 Worker 原语使用租约续期及 fencing；规划任务完成与 DAG 修订版 1、初始任务状态在同一 Redis 事务提交，随后恢复器只入队独立根任务。规划排队超时、Run 截止以及已受理但未开始规划的过期状态写入准确 reason_code。测试覆盖上述退出点、完成后 ACK 前退出、重复规划投递、旧 fence 拒写和两个并行根任务；定向 race 测试与 vet 通过。实际 Agent `RunPlanInvoke` 仍把计划生成与整张计划执行耦合，尚无正式 PlanBuilder、单任务业务 Executor、Worker 启动装配、终态回写与真实跨实例验证，T004/T005/T008/T009 仍未完成。

2026-09-24 规划业务输入检查点：新增不执行任务的 `BuildInvokePlan` 和 `ServiceSessionPlanBuilder`，把完整 `ExecutionPlan` 放入按 Run 与 SHA-256 定址的对象存储，读回校验后才返回可提交的 DAG；同阶段任务可并行，后续阶段保留 barrier。`DBPlanningInputLoader` 从已受理的数据库锚点、所属 Session/消息、Agent、历史及实时 capability grant 重建输入，队列消息仅携带 Run 身份；撤销授权、过期、取消、环境不一致均停止规划。定向 SQLite 测试验证真实存储路径和授权撤销，相关 Runtime/RunStore/Agent Session race 测试与 vet 通过。仍缺生产 `Deps` 接线、真实任务 Executor、容量池映射、终态回写和跨实例端到端验收，故 T005/T008/T009 仍未完成。

2026-09-24 单任务执行边界检查点：`Manager.ExecutePlanTask` 可从完整计划选择单个任务，使用已验证的上游结果解析 `ParamRefs`，拒绝缺失依赖与无关结果，不再重跑整张计划。`ServiceSessionTaskExecutor` 从 RunStore 核对运行中任务、规划回执与 DAG，从对象存储校验完整计划及已完成上游结果，重新加载授权业务输入，然后仅向声明具备稳定键幂等保证的 `DurableTaskInvoker` 交付该任务；成功结果按任务、尝试和 SHA-256 定址并读回校验。该接口尚无生产业务 Invoker：当前 Skill/Capability 链只把幂等键用于审计或没有显式键，不能保证副作用在 Worker 崩溃后不被重复执行。生产 `Deps` 不启用该 Executor；必须补齐真实调用链的幂等回执或人工核查协议、任务输入/配置快照、终态回写及端到端验证，T008/T009 保持未完成。

2026-09-26 执行回执检查点：Worker 在调用业务前以当前租约和 fence 原子写入 Task 的唯一执行标记；成功结果存储后再提交独立回执，随后写任务完成状态并 ACK。重领时复用已保存回执；只有执行标记但没有回执时进入 `manual_review_required`，禁止自动重做副作用。Redis 标记无独立 TTL，不随租约到期清除；这不承诺 Redis 超 RPO 丢失下的 exactly-once。已增加真实 `ManagerTaskInvoker`，要求 Worker 执行权上下文，单任务入口关闭旧 `retry-once`，失败事件携带 reason_code，业务 panic 转入待核查。同一 Run 并行领取/开始的版本冲突仅重试状态 CAS。新增受控完整链路测试：两个分析任务并行，经 Manager Skill 调用、结果对象校验、依赖推进后汇总，断言仅调用三次、同一 Run 完成；故障注入覆盖已执行未存回执、已存回执未写终态及旧 fence 拒写。此证据使用受控 Skill 和对象存储实现；仍需正式 Worker 生命周期、终态/消息回写、输入配置快照、核查操作入口、业务回执对账与真实营销案例验收。管理端 `chat_handler.go` 的 `Engine.Run` / `RunPlanInvoke` 同样仍未迁移，不能仅以服务会话组件通过代替原页面验收。

此检查点验证：`agent_run`、Manager、Runtime、Agent Session 的 `go test -race` 与 `go vet` 通过；独立真实 Redis（AOF、`appendfsync=always`、`noeviction`）上的受控并行分析/串行汇总测试通过，测试 Redis 已关闭。开发数据库迁移、正式后台重启和真实营销复盘均未执行。

2026-09-26 Worker 与终态回写检查点：

- `WorkerService` 已实现按部署环境扫描低频受理锚点、恢复 outbox、有限并发消费、失败终态补写及进程取消退出。`Deps.AgentRunWorker` 由 Bootstrap 装配，`cmd/app` 启停；`ai.runtime.durable_sessions` 默认关闭，仅选择服务会话消费者。启用必须通过迁移字段检查、Redis AOF/noeviction 与指定 S3 桶写入/读回检查，失败不退回旧 Executor。管理端聊天仍是另一消费者，不能通过此开关视为完成迁移。
- 服务会话终态在父会话事务锁内写入唯一助手消息、清理 pending 状态并更新受理锚点。结构化报告通过 `response_envelope` 保存和返回，文本留空，避免重复呈现。报告必须在任务本次可信计算账本中验证并随对象回执保存；缺少有效报告的业务最终任务不能变为 completed。终态查询和 SSE 在消息提交成功后才向客户端报告结束，SSE 会读完终态前的全部分页事件。
- 对象存储不可用或数据库回写失败会保留待回写锚点；重复补写不重复执行任务、不生成第二条消息。数据库旧过期清理不再修改已受理的 Redis Run。后续会话上下文包含结构化历史，长度预算包含报告字节。
- 取消在 Redis 同分片事务内写入 Run/未完成任务状态和事件、清理 outbox，阻止新任务及旧 Worker 结果写入；已有执行标记但无完成回执的任务保留 `manual_review_required`，不宣称撤销外部副作用。规划器明确失败或 panic 会留下终态记录，不因同一消息重领而重复调用模型；已分类的模型/队列/存储错误保留原因码。
- 故障测试覆盖对象读取失败、消息插入后事务回滚、重复补写、跨租户拒绝、Worker 重启恢复、取消后迟到结果与旧租约拒写、缺失报告拒绝完成。相关 Go 包的 race 测试和 vet 通过；独立真实 AOF/always/noeviction Redis 上的受控双分析并行与报告汇总验证通过，测试进程已停止。
- 受理截止时间先规范为微秒精度，再同时写 PostgreSQL 与 Redis，避免数据库精度截断触发身份校验误报。
- 已执行正式 `go run ./cmd/database migrate -config etc/config.yaml`，退出 0，并查询确认本机 `powerx.public` 的 invocation `run_env/admission_state/response_envelope` 和 message `response_envelope` 字段。正式后台尚未重启，开发配置未开启 durable_sessions。

下一验收缺口：管理端 Stream/REST 正式入口及其授权/输入配置快照、跨实例运维与归档保留期、实际营销复盘和断线恢复证据。T002—T010 不因上述组件测试而整体勾选；当前不能通知用户开始完整业务验收。

### 2026-09-26 管理端正式入口与断线续订

- `/agents/stream/sse`、会话 SSE 和同步 `invoke` 已按 `durable_sessions.admin_chat_enabled` 接入同一 Redis Worker；独立管理用户受理表保存主体、输入对象 locator/hash、快照与唯一结果索引。输入正文进入 S3，API key/JWT 不落快照；Worker 恢复时复核用户、成员、Agent、技能、资源授权与模型配置，并加载当前凭据。
- 管理用户 Run 与 service session 在共享队列按受理记录分派。受理扫描可修复 `pending_create` 窗口；已 admitted 的 Redis Run 丢失不重建；未开始且已过期的 pending 凭据只生成 `run.deadline` 失败终态。Run 期限不得超过冻结授权有效期。
- SSE 重连使用相同 Run + event_seq，前端丢弃断开的不完整帧后续订，复用当前助手消息。终态在会话事务内幂等保存一次。未来游标在响应头前拒绝。新的消息追踪详情/时间线/报告从 Redis 读取；追踪列表和会话聚合仍是旧日志来源，尚未完成迁移。
- 已执行正式 migrate 并核对 `public.agent_admin_run_admissions` 字段。相关 Go race 测试、续订 3 项 Vitest、消息重构检查、Nuxt build 通过。
- 开发后台未重启；开发配置尚未开启 durable 分支。本机 Redis AOF 未启用，配置的 MinIO `127.0.0.1:9000` 未运行。真实营销案例、浏览器刷新恢复、跨实例与归档生命周期验收尚未完成，因此 T009/T010/T011/T012 保持未勾选。前置响应规划目前仍在 HTTP 请求内完成，正式受理后 DAG 才交给 Worker；不把该阶段描述为全生命周期持久化。

### 2026-10-02 环境门禁实测与受理恢复补齐

- 已核对共享开发 Redis 的数据量、消费者库和磁盘空间后启用 AOF，并 `CONFIG REWRITE` 持久保存；noeviction 保留，appendfsync=everysec。新增 Redis 启动健康检查，拒绝加载中、AOF 未启用、最后写入或重写失败的节点。真实 Redis 检查测试通过；未进行共享 Redis 重启和 RPO/RTO 演练。
- 新增可重复的本机 MinIO/独立 Core 启动与 S3 预检命令。实测发现 MinIO 区域与 Core 不一致及自带 S3 SigV4 客户端 canonical request 多一空行；已固定区域并修复普通签名/预签名。AWS 官方固定向量及真实 MinIO（含中文对象键）Put/Get、预签名读写和删除测试通过。
- 持久化配置的独立 Core 在 18077 启动成功，健康 HTTP=200，门禁已实际验证。正式 8077 进程未重启，默认开发配置未被自动启用。
- 服务会话扫描器补查 pending_create，在会话锁下幂等恢复受理；过期未开始凭据生成 run.deadline 失败终态。旧数据库过期清理只处理旧进程模式，避开持久化锚点。测试覆盖未开始恢复、重复扫描、过期恢复与禁止读取业务输出；相关包 race/vet 通过。
- 真实营销 SSE 验收命令已准备，可验证一次提交后断开续订、Redis 追踪、唯一回复和 v4 报告。数据库管理员记录有效，但密码为 bcrypt 哈希，刷新表没有原始令牌；root/root 登录失败，浏览器连接不可用，尚未取得实际 HTTP 验收所需的登录会话。因此不勾选 T006/T009/T011/T012 的业务/跨实例验收。

- 补充真实模型证据：通过观测代理连接真实 `qwen3:8b`，调用正式 `llm.Invoke` 与真实 AOF Redis，Profile 并发 2 / 物理容量 1 时两次调用均成功、provider 并发峰值 1；并未验证两个 Core 同时执行完整营销 DAG。
- 已运行 `go test ./...`，全仓仍有 Media/Event Fabric 测试替身接口缺失、Skill Bridge 旧构造器引用，以及 Capability/Workflow/IAM/Knowledge 等合同或夹具失败，不能报告全仓通过。本轮修改的相关包 race/vet 与 S3 子模块签名和实机测试通过。

2026-10-03 本轮收尾：最新构建再次通过独立 Core 的启动/健康检查；新 SSE 未登录访问返回 401。所有本轮启动的独立 Core 和 MinIO 测试进程已停止，固定 MinIO 数据目录与私有联调配置保留，原 8077 后台仍运行。营销验收脚本与合成输入已准备，尚未运行已认证的真实营销请求。

2026-10-03 终态归档接线：正式 durable Worker 的 FinalizeRun 先幂等完成消息，再将完整事件、规划任务及各版计划/任务写入对象存储并读回校验，最后保存 SQL `archive_key/archived_at`。管理端和服务会话扫描器均保留 finished-but-unarchived 锚点；对象失败或 SQL locator 失败不会重跑任务或重复写助手消息。订阅只负责消息完成，归档失败由 Worker 后续扫描恢复。启动门禁要求新增归档列存在。

- 新增 `TestTerminalArchiveRecoversObjectAndLocatorFailuresWithoutNewMessages`：对象不可用、SQL locator 写入失败、已验证对象重用、唯一助手消息、完整事件和规划任务归档、跨身份拒绝；独占 miniredis 热状态清空后仍可按原权限读取对象报告，损坏归档拒绝读取。
- 服务会话测试补充 finished-but-unarchived 扫描、归档 locator 幂等、消息绑定和不同 locator 拒绝。
- 本机集中 `cmd/database migrate` 成功，随后查询 information_schema 确认两张锚点表的四个新增列。原配置含示例 JWT secret，迁移使用 0600 临时配置及仅供该进程的随机密钥；没有修改运行后台的密钥或配置。
- T010 仍有保留期清理、归档积压背压和生产容量/故障演练待完成。当前不自动删除 Redis 历史；未归档数据不会因新增流程被清理。

同日补充：管理端 SSE 的订阅前游标校验、事件分页和终态消息，以及服务会话 Query/SSE/快照均接入已授权归档读取。Redis 返回运行不存在时才允许使用已完成的归档锚点；Redis 连接故障不能被当作运行过期。测试模拟独占 miniredis 热数据到期后验证原 Run 事件与同一终态消息恢复、未来游标拒绝、损坏归档拒绝。当前仍没有开启 Redis TTL/删除。

最终验证：相关 agent_run/agent_session/runtime/bootstrap/OpenAPI agent_session 包通过测试和竞态检查；go vet 与独立 Core 编译通过。全仓 `go test ./...` 重新执行仍失败，存在 Media/Event Fabric 的旧 mock 缺失接口、Skills 未定义旧符号，以及 IAM/Workflow/Knowledge/Capability 等合同或 fixture 失败。完整输出保存在私有 `/tmp/powerx-agent-runtime/go-full-test.log`，不把相关包通过作为全仓通过。

2026-10-04 生命周期实施：T010 的保留期清理与积压背压已接入正式 Worker。默认已归档热状态保留 168h、积压 1000 个或最老 1h 触发新受理限制；绝对到期不被补写重试延长。增加 `hot_expires_at` 确认字段、归档积压复合索引，集中迁移后查询确认。扫描补齐对象/SQL/TTL 中断点，到期后确认只读归档，不重建运行。共享 TaskBus 按 Run 清理排队、延迟及死信残留，并以退休标记拒绝迟到入队，其他 Run 不受影响；投递游标写入也与终态互斥。

T009 补充页面恢复：按用户/租户/环境保存无输入和凭据的 Run 定位；初始化或重新选择会话后强制读服务器历史，已完成的唯一助手消息直接展示，否则仅按原 Run 续订，从零重建任务状态。不增加用户消息，不重发 q。新增 Vue composable 集成测试覆盖实际续订 URL、历史用户消息不重复、唯一结果、已有结果不再订阅，另有定位隔离/过期和断线帧恢复测试。仍需真实浏览器页面验收，因此 T009 不整体勾选。

运行证据：真实本机 Redis 的退休/NACK 丢弃/迟到投递防护测试通过；真实 Ollama 两独立测试进程共享同一物理池验证通过，provider calls=2、峰值=1，第二进程在第一个持槽时进入 Redis 等待队列。这里没有伪造管理员 token 或宣称完整两 Core/营销验收完成。浏览器连接仍返回 `nodeRepl.fetch request failed`，且有效管理员会话仍未提供；真实营销、完整 Core 故障切换与 RPO/RTO/压测证据待补。

2026-10-04 最终验证补充：相关后端九个包的 race 测试、go vet 与 Core 编译通过；新增恢复扫描/背压 HTTP 原因码定向 race 测试通过。前端四个文件共 10 个测试、Nuxt build 与 check-refactor 通过。全仓 go test ./... 重跑仍失败，缺口仍集中于 Media/Event Fabric/Skills 旧 mock/符号及其他 Capability/Workflow/IAM/Knowledge 等合同或 fixture，私有日志 /tmp/powerx-agent-runtime/go-full-test.log；没有把定向验证当成全仓通过。

T011 隔离 AOF 故障证据：TestIsolatedRedisAOFRestartRecoversRunAndPendingDelivery 启动独占 Unix socket、独占临时目录的 Redis 子进程，appendonly=yes/appendfsync=always/noeviction，强制终止后以原 AOF 重启。已确认 Run 和 event_seq 全量一致、原 PEL 消息由另一 Worker 重领、fence 递增且旧 ACK 拒绝、规划调用一次。本次启动恢复与重领约 1.189s，进程和临时目录已清理，未停止共享 Redis。此证据只覆盖该声明 fsync 策略的运行/队列组件，不是当前 everysec 配置的生产 RPO/RTO，也不是已认证营销或完整多 Core 故障验收。
