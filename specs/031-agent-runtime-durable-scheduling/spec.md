# Feature Specification: Agent Runtime 持久化调度与共享容量

**Status**: Implementing，持久化执行入口已接线，正式营销与生产验收未完成
**Date**: 2026-09-23  
**Design source**: [Agent Runtime 持久化调度与模型容量设计](../../docs/plan/ai_engineering/agent/agent_runtime_durable_scheduling.md)

## 范围与权威边界

一条用户消息受理为一个 `run_id`；一个 Run 内的任务可按依赖图串行、并行或跨 Agent handoff。排队、断线和 Worker 重启不得生成第二条 assistant 消息来承载同一轮任务。现有 `024` 定义 Agent/Skill 及 `agent_run.*` 展示语义；本规格定义持久化 Run 状态、调度、容量和恢复。`004` 是通用 TaskBus 合同权威；`010` 管理模型配置与路由；`028` 的通用定时作业不负责 Agent Run DAG。

当前进程内 `errgroup`、进程内模型闸门、PostgreSQL Run 快照/消息 meta、Local File/Loki Trace 与触发执行的 SSE 是迁移基线，不构成本规格已实现的证据。本文要求按 `tasks.md` 的代码和验收记录逐项核对；已实现组件不等于完整验收通过。

## 用户场景与验收

1. 同一消息规划出两个独立模型任务，部署模型只有一个推理槽。两任务同时就绪；一个 `running`，另一个 `queued` 并展示资源池与等待时长。第二个获得槽位后才开始单次模型请求计时，两个结果最终归入同一个 Run、同一执行卡片和一个最终答复。
2. 模型池满时，不使用该模型的工具任务或使用另一资源池的任务仍可执行；依赖未满足的任务不能提前入队。失败策略与最终 `partial/failed/needs_input` 一致。
3. 浏览器断线、Core 实例重启、Worker 崩溃、租约失效或重复投递后，用户可凭同一 `run_id` 恢复状态和事件；不可安全重放的副作用进入人工核查，不能重复执行。
4. Redis 或对象存储故障时，系统给出可识别的容量/依赖状态；不能通过逐任务写 PostgreSQL 或日志解析制造“成功”。

## 功能要求

- **FR-001 单一身份**：提交时固定 tenant/env、session/message、调用主体、trace、幂等键和 `run_id`；计划修订与 task attempt 有独立身份。相同幂等键返回同一 Run。
- **FR-002 依赖图**：Planner 保存不可变计划版本；调度器只将依赖已满足且授权仍有效的任务转为 `queued`。独立任务允许同时就绪；Run、租户、资源池各有可治理的并发上限。
- **FR-003 存储权威**：生产默认且要求 `agent.runtime.store.driver=redis`。Redis 保存活动 Run/Task/Attempt、租约、序号和短期事件；对象存储保存长期脱敏报告及完成快照；PostgreSQL 只保存原有业务事实、Session/Message、低频 Run 受理和归档引用。日志/Loki 不是状态权威。生产不自动降级到 memory 或 DB polling。
- **FR-004 Redis 门禁**：生产启动检查 AOF、`noeviction` 和健康状态；部署还须声明容量、复制/备份、可接受 RPO/RTO 与恢复演练。门禁失败时停止新 Run 接入和 Worker 领取。Redis 的持久化确认不等同不可逆业务副作用的恰好一次执行。
- **FR-005 队列与租约**：复用经扩展的 Event Fabric TaskBus，具备 Redis Streams 消费组、ACK/NACK、续租、重领、fencing、去重、延迟重试、死信及可观测能力。当前声明 `SupportsLease=false/SupportsConsumerGroup=false` 的实现不可直接用于 Agent Worker。至少一次投递由幂等处理保障。
- **FR-006 原子状态**：每次权威状态转换以版本 CAS 原子更新状态、递增 `event_seq` 并追加事件；跨 Redis 分片的任务投递使用待投递记录与对账器，不能假设跨分片事务。过期 fencing token 不得写回完成状态。
- **FR-007 共享模型池**：按实际部署 endpoint/model/实例计量物理槽位，跨 Core 实例和租户共享；Profile 的 `max_concurrent_requests` 是策略上限，不能复制成每租户物理容量。队列有上限、公平性、优先级及等待时限；非模型任务不被无关模型队列阻塞。
- **FR-008 独立预算**：分别记录 `queue_wait_timeout`、`lease_ttl`、单次 `request_timeout`、Run `run_deadline`、重试/步骤/token/成本预算。请求超时从实际调用开始，排队超时只能报 `queue.timeout`；不能以并发波次数乘请求超时替代完整 Run 预算。
- **FR-009 副作用与授权**：Skill/Capability/Workflow 调用前重验租户、授权及对象权限；可重试动作使用业务幂等键和结果回执。结果不明且无幂等保证时停止自动重试，标记 `manual_review_required`。取消或权限撤销阻止后继与新副作用。
- **FR-010 订阅与历史**：SSE/WS 仅订阅 Run，使用 `run_id + event_seq` 去重/续订；事件过期时读取权威快照或归档。一个 Run 的任务列表映射到同一消息卡片。终态必须有真实结果与验证证据，`agent_run.final/ended` 不自动表示业务任务完成。
- **FR-011 归档与对账**：归档可读后才清除 Redis 终态详细事件；对象存储中断时背压并保留待归档标记。低频受理凭据可发现 Redis 数据损失；恢复器按 RPO 和副作用回执判断能否重建，禁止盲目重放。
- **FR-012 隔离与数据最小化**：所有状态、事件和报告按 tenant/env/run 隔离，读取重验主体权限；队列只放受限引用，不能放密钥、完整 prompt 或敏感业务 payload。
- **FR-013 服务合同**：服务提供幂等 `SubmitRun`、`GetRun`、`ListRunEvents(after_seq)`、`CancelRun`、`GetTaskAttempts`、`GetRunReport`。已有会话 invocation 接口是服务态受理/读取/取消/事件的兼容入口，实施时须更新 `007` OpenAPI 与契约测试并迁移触发执行的 SSE。不得在旧入口和新 Worker 同时执行同一 Run。
- **FR-014 Capability 授权**：若该运行态合同以 Core capability 供 Framework Host/委托绑定消费，正式平台能力声明必须同时覆盖存在的 Admin `admin_user` JWT+RBAC 面与固定类型 `core_internal` `service_actor` 面；本地 Host 使用 `PX_GATEWAY_API_KEY` 时声明显式 `api_key` 元数据。服务绑定使用稳定 `core://` + `INVOKE` + typed operation DTO，拒绝任意 HTTP method/endpoint/headers/raw payload，且只授予所需 `service_read` 或明确的执行权限。变更时通过 capability-check、seed、grant 和开发 Host key `grant-status` 验证。

## 状态与错误语义

Run：`accepted -> planning -> running -> completed|partial|needs_input|blocked|failed|cancelled`。Task：`pending_dependency -> queued -> leased -> running -> verifying -> completed|failed|skipped|cancelled`；可经 `retry_wait -> queued`，缺参进入 `awaiting_params`。旧 `pending/running` 客户端状态须有版本化兼容映射；`queued` 与 `leased` 不能显示为业务成功。失败原因至少区分 `queue.full`、`queue.timeout`、`provider.timeout`、`run.deadline`、`store.unavailable`、`archive.backpressure`、`manual_review_required`。

## 非功能验收

- 两个 Core 实例和多个 Worker 不能超额使用单槽模型；跨租户公平性、队列上限及 P95/P99 等待可测。
- 断线、重启、租约过期、重复投递、Redis 故障切换后的状态、事件和归档报告使用同一 `run_id` 且序号连续；无幂等副作用不重复执行。
- AOF/复制/备份恢复演练符合环境声明的 RPO/RTO；生产配置 `appendonly=no` 必须被门禁拒绝。
- 压测给出 Redis 内存/写入量、归档积压、模型槽利用率、队列超时、数据库写入量；业务数据库不承载逐任务进度/心跳。

实施路线见 [plan.md](./plan.md)，实体与原子性见 [data-model.md](./data-model.md)，未完成任务见 [tasks.md](./tasks.md)。
