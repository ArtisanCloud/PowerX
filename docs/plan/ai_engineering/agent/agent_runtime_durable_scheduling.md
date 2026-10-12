# Agent Runtime 持久化调度与模型容量设计

状态：**分阶段实现中；服务会话与管理端聊天已有可选持久化接线，开发环境尚未启用，生产验收未完成**。本文是 [Runtime 闭环设计](./agent_runtime_loop_design.md)、[实施计划](./agent_runtime_implementation_plan.md) 和 [Run State 协议](./agent_run_state_protocol.md) 的调度与存储合同，不把现有进程内执行器或 Event Fabric TaskQueue 误写成已实现的 Agent Worker。

正式需求、数据模型、待实施任务与验收步骤见 [`specs/031-agent-runtime-durable-scheduling`](../../../../specs/031-agent-runtime-durable-scheduling/spec.md)。

## 1. 目标与当前基线

一条用户消息建立一个 `run_id`；一轮 Run 可以包含串行、并行和多 Agent handoff 任务。任务并行度由依赖图决定，模型、工具和插件服务分别按实际容量排队。排队、断开页面、Core/Worker 重启都不能把已受理的任务变成不可追踪的“执行失败”；灾难恢复承诺以部署声明的 RPO/RTO 为边界。

当前 `ExecutePlanWithHooks` 在一个进程内按 stage 使用 `errgroup` 并发；模型并发闸门也在进程内，键包含租户、环境、provider 和 model。`agent_plan_runs`、`agent_task_events`、`agent_run_task_states`、handoff 和消息 meta 已保存部分状态，但 DBRunLogger 的缓冲区满时可丢事件，不能作为可靠调度账本。`GET /api/v1/agents/stream/sse` 当前仍可触发长时间执行。Event Fabric Redis TaskQueue 已有入队、领取和确认，但驱动声明 `SupportsLease=false`、`SupportsConsumerGroup=false`。上述机制均不能直接证明跨实例恢复。

## 2. 存储和驱动决定

生产 `agent.runtime.store` **默认且要求为 `redis`**；不在 Redis 故障时隐式切换到进程内存或 PostgreSQL。启动时检查驱动、AOF、非驱逐策略和健康状态；容量、复制/备份及恢复演练由部署门禁和持续监控校验。不满足条件则 Agent Run 接入与 Worker 领取失败关闭，并暴露健康状态。开发环境可以显式选择测试驱动，但不得用其通过生产验收。

| 数据 | 权威位置 | 写入规则 |
| --- | --- | --- |
| 活动 Run、计划版本、任务、尝试、依赖、租约、状态序号、短期事件 | Redis | 只持久化状态转换和检查点；Lua/事务原子更新状态、版本与事件；不逐 token 落库。 |
| 就绪任务、延迟重试、领取/确认 | Redis Streams 及索引 | 消费组、可续租、可重领；队列消息只含受限引用与调度元数据。 |
| 大型输入输出、脱敏 Trace、长期 Run 报告与完成快照 | 对象存储 | 内容按 tenant/主体授权、分级、版本和保留期管理；Redis 保存引用与摘要。 |
| Session/Message、Run 受理凭据与归档 locator、SkillState、Capability 业务状态、最终 Outcome 引用、必要审计与副作用回执 | 现有业务存储 | 每轮一次的受理关联和低频业务事实继续由各自权威服务负责；不写排队心跳、进度或每个模型 token。 |
| Loki/本地日志 | 观测副本 | 用于检索和排障，不参与调度、领取、恢复或完成判定。 |

Redis 是运行态权威，不是业务对象/SkillState 的替代数据库。受理成功时在现有消息 meta 留下 `run_id` 与受理凭据，以便 Redis 超出 RPO 发生数据损失后发现缺失 Run；恢复器只能按幂等与副作用回执决定是否安全重建，不能盲目重放。归档成功后才允许按保留策略删除 Redis 中的终态详细事件；归档 locator 写入消息 meta/最终 Outcome 引用，也可由 tenant + run ID 的确定性对象键定位。历史 API 可从对象存储报告恢复同一 `run_id` 的状态和 Trace。归档、删除和保留期需分别配置，并验证归档可读。对象存储不可用时保留待归档状态并施加容量背压，不静默丢弃历史。

Redis 生产前置条件：AOF、非驱逐策略、内存水位与保留量、复制/故障切换、备份与恢复演练、持久化错误监控。当前开发机 Redis `appendonly=no`，不符合该目标。`appendfsync everysec` 不能承诺零数据丢失；必须定义可接受的 RPO/RTO，故障切换和恢复测试据此验收。要求更高持久性的动作须有独立的业务幂等/审计回执，不得仅凭 Redis 确认视为不可逆副作用完成。

目标配置合同（下文包含仍待实现的部署治理字段；已实现的启动配置以 `backend/etc/config_example*.yaml` 为准，示例值不代替负载测试后的容量配置）：

```yaml
agent:
  runtime:
    store:
      driver: redis
      require_aof: true
      require_noeviction: true
    queue:
      max_wait: 10m
      max_pending_per_tenant: 100
    worker:
      lease_ttl: 30s
      heartbeat_interval: 10s
    budget:
      request_timeout: 5m
      run_deadline: 30m
    archive:
      driver: object_storage
```

租约 TTL 必须大于心跳间隔并留出网络抖动；最终参数按环境和服务等级治理，不从 Ollama 当前本机配置推断通用生产默认。生产不得选择 `driver=memory`；配置变更必须有版本与审计。

## 3. 运行身份与状态合同

一次提交只创建一个 Run；`run_id`、`tenant_uuid`、`session_id`、`message_id`、`trace_id` 和调用主体在接入时固定。任务身份为 `run_id + plan_revision + task_id`，尝试再附加 `attempt`。计划修订不能复用旧任务身份掩盖新执行。重试必须保留原尝试、原因、预算消耗和结果引用。

```text
Run: accepted -> planning -> running -> completed | partial |
     needs_input | blocked | failed | cancelled

Task: pending_dependency -> queued -> leased -> running -> verifying
      -> completed | failed | skipped | cancelled
      queued/leased/running -> retry_wait -> queued
      running -> awaiting_params（保留可恢复引用）
```

`queued` 表示已满足依赖但等待 Worker 或资源；`pending_dependency` 表示依赖尚未满足；`leased` 仅表示 Worker 已领取，不能对用户展示成业务执行成功。任务可带 `queue_reason`、`pool_id`、`queued_at`、`leased_at`、`started_at`、`attempt`、`deadline`、`reason_code` 和脱敏结果引用。每次权威状态转换原子递增 `event_seq` 并追加 `agent_run.*` 事件；过期租约的 Worker 不得更新状态或发布完成事件。

同一 Run 的状态与事件键应在 Redis Cluster 中共享 hash tag，保证状态 CAS 与事件追加落在同一分片。状态转为 `queued` 与跨 Run/资源池的队列投递不能假设跨分片事务：先在同分片写权威状态及待投递记录，再由可重试投递器发布到工作 Stream；投递器和 Worker 都按任务身份去重。恢复扫描器定期修复“状态已就绪但尚未投递”和“已投递但未领取”的差异。

终态只能由真实结果及 Verification Evidence 决定。`partial` 保留已完成任务；`failed`、`cancelled` 和 `needs_input` 不删除可复用证据。所有任务状态和事件都按 tenant + env + run 隔离，读取须重新校验主体权限与数据分级；队列负载不得携带密钥、完整 prompt 或未脱敏业务明细。

## 4. 提交、调度与容量

1. 接入 API 验证身份、配额与租户范围，创建 Run、初始事件和低频受理凭据后返回 `run_id`；执行不依赖原 HTTP/SSE 连接。重复提交使用客户端请求幂等键映射到同一 Run。Redis/消息凭据之间的失败窗口须由受理对账与幂等补偿关闭。
2. Planner 保存不可变计划版本。调度器从显式依赖图计算就绪任务，保留现有 stage 约束的兼容语义；只有已通过依赖完成条件与授权快照检查的任务可入队。
3. 调度器按 Run 上限、租户配额、资源池容量和公平策略投递。独立任务可同时就绪；模型容量不足只使需要该模型的任务等待，不阻塞可运行的文件、Capability、不同模型任务。
4. Worker 领取后取得带 TTL 和 fencing token 的租约，续租失败必须停止新副作用并取消可取消的在途调用。状态更新使用 token/版本 CAS；租约过期由恢复器重领。
5. 模型池按实际部署实例及 endpoint/model 统一计量容量，另设租户与 Run 上限。租户不能各自获得同一 Ollama 单槽的完整配额。公平调度避免单租户长期占用；队列长度、等待时限和优先级有界。

现有 LLM 同步与流式入口已接入显式配置的 Redis 物理模型池。部署时在 `ai.runtime.physical_model_pools` 为每个 Ollama endpoint/model 指定稳定的 `pool_id`、真实 `capacity`、`max_waiting` 与长于 `request_timeout` 的 `lease_ttl`；启动时要求 Event Fabric Redis 通过 AOF/`noeviction` 门禁。Profile `max_concurrent_requests` 仍作为租户策略限额；物理槽位跨 Core 实例共享。等待先于单次请求期限开始，队列超时单独返回；租约续期失败会取消调用上下文。若已启用物理池但 Ollama 目标没有精确匹配的部署规则，调用失败关闭。正式 Agent Session Worker、RunState 订阅与归档的接线仍须独立完成。
6. Capability/Skill/Workflow 执行时重新校验授权快照和当前对象权限。可重试动作必须有业务幂等键；无幂等保证且无法确认副作用结果的动作以 `manual_review_required` 原因阻断 Run，不得自动重复调用。
7. 任务完成后持久化结果引用和事件，更新依赖并唤醒后继；Run 的最终 Outcome、答复与归档由同一状态机推进。消费语义是至少一次投递与幂等处理，不宣称恰好执行一次。

Event Fabric TaskQueue 应扩展通用租约、消费组、续租、重领、死信和去重合同后供 Agent 使用；不得让 Agent 直接依赖目前 `SupportsLease=false` 的实现，也不得在 Agent 包内维护第二套无治理的队列。

目标服务接口至少提供：`SubmitRun`（幂等受理）、`GetRun`（权威快照）、`ListRunEvents(after_seq)`（有界续订）、`CancelRun`、`GetTaskAttempts` 和 `GetRunReport`。所有读写按租户、主体、Session/Message 归属校验；管理端 Trace 查询继续走 Root 授权。迁移时现有会触发执行的 SSE 入口需切换为提交后订阅，旧客户端兼容期由服务端映射到新 Run，不能在一次请求里再起第二个执行器。

## 5. 时限、预算与背压

| 时限 | 起点 | 到期结果 |
| --- | --- | --- |
| 排队等待 `queue_wait_timeout` | 任务满足依赖并进入队列 | `queue.timeout`；明确容量等待耗尽，不伪装成模型调用超时。 |
| 租约 `lease_ttl` | Worker 领取 | 续租或回收；旧 Worker 的写入被 fencing 拒绝。 |
| 单次请求 `request_timeout` | 真正发起模型/能力请求 | `provider.timeout` 等明确原因；排队时间不计入。 |
| Run 总预算 `run_deadline` | Run 受理 | 停止新任务，按已验证结果输出 `partial/failed/cancelled` 与恢复动作。 |

总预算同时限制步骤、模型/Capability 调用、token/成本、重试与计划修订，不能只按 `ceil(stage_tasks/max_concurrent_tasks) * request_timeout` 推导。应在受理时根据服务等级和任务图设定硬上限，执行中按实际排队、处理、验证和剩余预算判定；预算不足不能无限等待。排队满、Redis 内存/归档背压或 Worker 不足时返回明确的容量状态/拒绝原因，不静默丢任务。

## 6. 事件、UI 与追踪

`agent_run.*` 是用户与调试界面的状态合同；SSE/WS 仅订阅，不拥有运行生命周期。客户端使用 `run_id + event_seq` 去重并在断线后续订；若事件已过期则读取权威快照或归档报告。`agent_run.final` 仍是运行状态，普通 `final` 才是主 assistant 正文。一个 Run 的内部并行任务始终属于同一消息执行卡片。

UI 显示等待依赖、排队原因与时长、执行中、验证中、部分成功及可恢复动作；参与 Agent 数和任务完成数不能充当业务完成率。Trace 记录任务入队、领取、租约、模型池等待、请求开始/结束、重试、回收、归档和 Outcome，至少含 `run_id/task_id/attempt/event_seq/pool_id/queue_wait_ms/execution_ms/reason_code`。日志/Loki 是诊断副本，查运行状态以 Redis/归档为准。

## 7. 故障与恢复规则

| 故障 | 行为 |
| --- | --- |
| Core/SSE 断开 | Run 继续；客户端按序号续订或读快照。 |
| Worker 崩溃或租约失效 | 恢复器检查状态与副作用回执，安全任务重领；旧 Worker 写入被 fencing 拒绝。 |
| Redis 暂时不可用 | 停止接入新 Run、停止新领取；在途 Worker 不得声称未持久化的完成。恢复后按快照、Stream 与租约协调。 |
| 对象存储不可用 | 阻止终态事件清理，限流/背压并告警；Trace 不标记已归档。 |
| 重复投递或重试 | 同一任务尝试幂等归并；Capability 按业务幂等合同判断是否允许重试。 |
| 用户取消/权限撤销 | 停止后继任务和新副作用，取消可取消请求；保留审计和已有结果。 |

## 8. 实施与验收门槛

实施顺序：运行态驱动合同和 Redis 配置门禁 → 权威状态机与事件序号 → TaskQueue 租约/消费组能力 → DAG 调度与模型池 → 独立预算/恢复 → SSE/WS 续订与 UI → 归档/运维与跨实例验收。迁移时新旧运行链不得同时执行同一 Run；已有 Session/Message 和 SkillState 不迁为 Redis 权威。

必须通过以下验证后才能称为已交付：

1. 同一消息内两个独立模型任务、单槽 Ollama：两个任务同时就绪，一个运行一个排队；第二个拿到槽后才开始单次请求计时，最终同一 Run 正确结束。
2. 不同模型/非模型任务能在单槽模型等待期间并行；依赖任务不会提前执行，部分失败策略与最终 Outcome 一致。
3. 两个 Core 实例、多个 Worker 下无超额模型并发；跨租户公平、队列上限和等待超时可观测。
4. 浏览器断线/刷新、Core 重启、Worker 崩溃、租约过期、重复投递、Redis 故障切换后可恢复或给出明确的不可恢复原因；不重复执行无幂等副作用。
5. AOF/复制/备份恢复演练符合声明的 RPO/RTO；当前 `appendonly=no` 配置必须被生产启动门禁拒绝。
6. Run 快照、SSE/WS、历史报告、Trace 的状态、事件顺序和 `run_id` 一致；原始错误、密钥和敏感 payload 不进入普通用户事件。
7. 压测报告覆盖队列吞吐、Redis 内存/写入、归档积压、模型池利用率、排队 P95/P99、超时率和业务数据库写入量；没有用业务数据库承载每次调度/进度写入。

本规范是开发目标；当前代码、配置和运行实例是否达到上述门槛须以迁移、测试和真实多实例运行证据逐项确认。

## 2026-09-26 实现核对

管理端以 `ai.runtime.durable_sessions.admin_chat_enabled` 显式接入共享 Worker（同时要求 `enabled=true`）。`agent_admin_run_admissions` 保存低频受理与唯一结果索引；完整输入放对象存储，Redis 保存调度状态和事件。HTTP 同步/流式入口在正式受理后不再执行旧 Engine/HistorySink；Worker 终态事务保存一条助手消息。SSE 断线续订复用 Run 与原消息，追踪详情直接读取 Redis。

恢复授权取当前数据库主体与权限，并校验冻结合同；凭据只在执行时重新加载。受理期限不超过冻结授权有效期。`pending_create` 可以幂等恢复，已 admitted 丢失 Redis 状态失败关闭。

源码回归、前端构建与管理端表迁移已完成；尚未执行更新后的真实营销 HTTP/SSE 验收。追踪聚合列表、完整刷新恢复、前置响应规划持久化、归档清理/背压和跨实例故障演练仍需完成。详情见 `specs/031-agent-runtime-durable-scheduling/tasks.md` 的当日记录；不得仅凭启用开关将整个目标架构标记交付。
