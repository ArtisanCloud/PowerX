# Data Model: Agent Run 持久化调度

**Status**: Implementing；下表保留目标合同，已落地字段和恢复边界见实施补充。

| 实体 | 关键字段 | 权威与保留 |
| --- | --- | --- |
| Run | `tenant_uuid, env, run_id, session_id, message_id, trace_id, actor_ref, idempotency_key, status, plan_revision, event_seq, deadline_at, version` | 活动 Redis；终态归档对象存储。 |
| PlanRevision | `run_id, revision, tasks[], dependency_edges[], failure_policy, checksum` | Redis 不可变版本；归档保留。 |
| Task | `run_id, revision, task_id, status, depends_on[], pool_id, queue_reason, queued_at, started_at, deadline_at, result_ref, version` | Redis；只保留受限结果引用。 |
| Attempt | `task_id, attempt, lease_owner, fencing_token, lease_expires_at, heartbeat_at, request_started_at, finished_at, reason_code, idempotency_key, receipt_ref` | Redis 活动记录；归档保留结论。 |
| ExecutionReceipt | `execution_token, execution_result_ref, execution_evidence_ref` | 当前实现内嵌 Task；调用业务前原子写标记，结果对象校验后写回执。无独立 TTL，重领保留；无回执的既有标记要求人工核查。 |
| RunEvent | `run_id, event_seq, event_type, task_id?, attempt?, occurred_at, sanitized_payload` | Redis 短期事件；归档压缩历史。 |
| DispatchOutbox | `run_id, revision, task_id, attempt, dispatch_state, next_retry_at` | 与 Run 状态在同一 Redis hash slot 原子写；投递器跨分片幂等发布 Stream。 |
| RunAdmission/ArchiveLocator | `run_id, message_id, admission_token, archive_key, final_outcome_ref` | 现有业务存储低频写入；用于发现 Redis 缺失和定位归档。 |
| RunReport | `tenant, run_id, snapshot, event_digest, trace_refs, outcome, checksum, schema_version` | 对象存储；敏感内容按授权、脱敏和保留期处理。 |

Redis Cluster 中同一 Run 的状态、事件与 outbox 键使用 `{tenant:env:run_id}` hash tag；跨 Run/资源池的 Stream 不是同分片原子事务。每次状态转移以 version/fencing CAS 检查旧值，同时递增 `event_seq`、追加事件和待投递记录。投递器及 Worker 以 `(run_id, revision, task_id, attempt)` 去重；恢复扫描器修复状态/outbox/Stream 不一致。排队、领取、执行、验证、完成各自有独立时间戳，便于区分等待与处理耗时。

归档须先写对象、验证可读与 checksum，再写 locator 并标记 `archived`，之后才按保留期删除 Redis 终态详情。对象存储故障维持待归档状态和容量背压。Redis 超过声明 RPO 发生丢失时，Admission 对账只负责发现缺口；恢复须读取业务副作用回执，不能按入队记录盲目重放。

ExecutionReceipt 是 Worker 防止自动重复调用的运行记录，不等于下游业务事务的幂等凭证。业务已执行但对象写入/回执提交失败时，新的 Worker 必须阻塞核查；对象回执已经提交而任务完成状态未提交时，可直接引用回执完成状态收敛。领取、执行占用及回执写入均检查当前 fence，旧 Worker 不得补写新租约下的回执。业务核查解除及 Redis 超 RPO 恢复仍需独立业务证据，禁止删除执行标记后直接重放。

访问 Run/事件/报告时校验 tenant、env、主体和 Session/Message 归属；队列与普通事件不存完整 prompt、密钥或未脱敏 payload。既有 SkillState、Capability 业务对象、模型 Profile 仍由各自业务权威存储管理。

## 已落地的服务会话终态字段（2026-09-26）

`ServiceInvocation` 保留低频 `run_env/admission_state` 受理锚点，新增 `response_envelope` JSONB；`ServiceMessage` 同样新增可空 `response_envelope`。任务状态、续租和事件仍只写 Redis。唯一 assistant 消息沿用 `invoke:<invocation_uuid>` 幂等键，并与 invocation `finished_at/status/output/response_envelope` 在父会话事务锁内提交。恢复扫描仅查询当前部署环境、`admission_state=admitted` 且 `finished_at IS NULL` 的锚点。

终态结果对象中 `durable_response_verified=true` 仅由 ManagerTaskInvoker 在本次可信账本校验成功后设置，不能接受调用方自行声明。恢复器按计划/任务/尝试的对象引用和 SHA-256 校验读取报告，不重跑生成器。取消保留原执行标记与回执；没有执行完成回执的在途任务携带 `manual_review_required`，旧 Worker 无权覆盖取消终态。

## 实施补充：低频归档索引与共享健康

管理端 `agent_admin_run_admissions` 与服务会话 `agent_service_invocations` 均保存 `archive_key`、`archived_at`、`hot_expires_at`。前三个阶段分别是对象读回校验、业务数据库定位提交、Redis 绝对到期设置及确认。`finished_at` 已有值但归档/到期确认尚缺时仍进入恢复扫描；这些字段不承载心跳或 token 状态。环境/受理状态/归档时间/终态时间复合索引用于低频积压聚合。

共享 Redis 健康记录包含待归档数量、最老终态时间、观测时间、阈值判断和配置指纹，TTL 为三倍扫描间隔。无有效共享健康记录时新受理失败关闭，已受理的读订阅不受容量门禁影响。TaskBus 在租户/订阅者分片保存 Run 退休标记，阻止归档后迟到投递复活。

## 2026-10-04 共享执行配额与计划预算实际字段

- 环境策略键：`agent:scheduling:{<env>}:policy`，固定 `tenant_limit:run_limit:lease_ms`，启动发布并校验，配置漂移失败关闭；不写 PostgreSQL。
- 槽键：`agent:execution:{<tenant_uuid>:<env>}:tenant`、`:run:<run_id>` 为 token → Redis TIME 毫秒到期时间的 ZSET；同分片 Lua 原子检查两级额度并取得槽。局部 `:policy` 约束活动租约策略一致。槽键和局部策略在最后续租后 2×TTL 到期，避免完成 Run 留下永久空键；环境策略无自动 TTL。
- ExecuteLease 为内部受限 Run 身份和随机 token。按 TTL/3 同时续租两级槽，过期不能复活。业务结果仍按既有 TaskBus/RunStore fence 写回；容量租约不是副作用幂等凭证。
- 新 Plan 的 `budget` 固化 `max_steps/max_capability_calls`，TaskDefinition 增加 `node_kind`，预算随不可变修订和归档一起保留。先预留整张 DAG 的节点额度，执行重建并核对；旧 Plan 缺省字段保持兼容。模型 token/费用及嵌套调用不包含在这两个字段中。
- Task 状态事件增加 `queue_reason`。容量等待只在原因变化时 CAS 写事件，不修改原排队时间或 attempt；TaskBus 的延后等待不递增投递失败次数。
