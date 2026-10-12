# PowerX Business Agent Runtime 实施计划

状态：**基于当前代码的实施计划**。本计划以现有 Runtime 为起点增量改造，不将文档中的目标架构误写为已经上线的能力。

## 1. 实施结论

不重写 Agent Runtime，也不把 PowerX 改成任意代码执行器。

当前 `Engine`、Plan Executor、Capability Invocation、Agent-Skill Binding、Run State、Trace、Response Envelope 和 SkillState 已经形成可靠骨架。改造重点是将“一次检测 -> 一次计划 -> 执行结束”的链路，升级为在授权快照内受预算控制的：

```text
Prepare -> Observe -> Plan -> Act -> Verify -> Recover/Re-plan -> Outcome
```

Workflow、Skill 和预绑定 Tool 仍是优先的业务黄金路径；它们不再是 Agent 唯一可以理解和观察的对象。资源发现不等于资源读取，更不等于执行权限。

## 2. 当前实现基线

| 能力 | 当前真实实现 | 结论 |
| --- | --- | --- |
| 任务检测和计划 | `backend/internal/server/agent/runtime/engine.go` 的 `detectTasks`、`BuildPlan` | 已有一次性 Planner；有营销复盘确定性路由，通用路径仍为单次检测。 |
| 候选安全过滤 | `backend/internal/server/agent/manager_tool_calling.go` 的 `CandidateBuildContext`、`isCandidateAllowed` | 已按 tenant、Agent binding、source、grant 过滤；它是**执行候选过滤器**，不是资源发现服务。 |
| DAG 执行 | `backend/internal/server/agent/manager_execute.go` 的 `ExecutePlanWithHooks` | 已支持 stage 并发、依赖、`retry-once`、`continue`、节点 Trace；最终只返回最后一个阶段结果，不能表达整轮业务 Outcome。 |
| Run 调度与模型容量 | `manager_execute.go`、`runtime/execution_budget.go`、`ai/factory/llm/concurrency.go` | 当前执行/限流在进程内；stage 并发与共享模型实际槽位没有统一调度，排队、请求和整轮时限未独立治理。 |
| 运行态存储 | `agent_plan_runs`、`agent_task_events`、`agent_run_task_states`、消息 meta；Event Fabric Redis TaskQueue | 已有局部落库和 Redis 队列基础设施，但 DB 事件通道可丢事件，Redis 驱动未声明租约/消费组；均不是可恢复 Agent Run 的权威存储。 |
| 状态恢复 | `agent_session_skill_states`、`SkillStateService`、`runtime/skill_state.go` | 已有持久化、状态 version、等待参数恢复和 Chat 注入；当前仍有 Runtime 侧的参数合并/确认文本判断。 |
| 结果契约 | `runtime/engine.go` 的 response envelope 校验和 Evidence Ledger | 已能拒绝不合格最终答复；尚未按 Capability 合同验证“业务动作是否真正完成”。 |
| 可观测性 | `agent_run.*`、Trace Node、Plan/Task Event | 已有运行过程；尚无 observation、verification、recovery decision 与 plan revision 的一等事件。 |

因此，已有 SkillState 的实现不能重复建设；文档中声称它“尚未落地”的地方应以本计划为准修订。

## 3. 目标边界

### 3.1 可以观察的资源

只允许经注册、授权和脱敏后的 `ResourceDescriptor`：

- 已发布 Capability 及其输入输出、风险和验证合同；
- 当前主体可读的业务对象、报表、文件、知识和 Artifact 元数据；
- 当前 Agent 的绑定、Team、Skill Revision 和插件运行状态。

禁止把数据库表、插件私有 URL、宿主文件系统或未经登记的接口直接暴露给模型。

### 3.2 可以执行的能力

执行只能来自本轮冻结快照中的已发布、已授权 Invocation Capability。发现到一个资源不会自动授予读取或调用权；写操作必须继续经过 Capability 风险策略和人工确认。

### 3.3 Outcome 是唯一完成语义

Run 只能以 `completed`、`partial`、`needs_input`、`blocked`、`failed`、`cancelled` 之一结束。`completed` 必须同时有：真实调用结果、能力合同验证证据和用户目标完成证据；LLM 输出 final 或最后一个 Task 成功均不足以成立。

## 4. 目标合同与数据模型

以下合同先以 Go 类型和按数据职责划分的持久化模型落地，再由 Admin/Plugin API 公开所需只读字段；不得先让前端或模型猜测自由 JSON。活动 Run/Task/事件及租约以 Redis 为权威；下表已有的 PostgreSQL Snapshot/Observation/Verification 表是低频治理与证据记录，不作为任务排队、心跳或进度主存储。

| 合同 | 核心字段 | 初始落点 |
| --- | --- | --- |
| `ResourceDescriptor` | `resource_uuid`、`kind`、`display_name`、`source`、`tenant_uuid`、`classification`、`discovery_grant`、`read_capability_id`、`freshness` | `backend/internal/server/agent/runtime/resource` |
| `Observation` | `observation_uuid`、`run_uuid`、`descriptor_ref`、`purpose`、`artifact_ref`、`digest`、`observed_at` | `agent_run_observations` |
| `RuntimeSnapshot` | `snapshot_uuid`、`run_uuid`、`policy_version`、可见资源/可调用 capability 列表、预算 | `agent_run_snapshots` |
| `PlanRevision` | `revision_uuid`、`run_uuid`、`parent_revision_uuid`、`reason_code`、`plan`、`replaced_task_refs` | `agent_plan_revisions` |
| `VerificationEvidence` | `evidence_uuid`、`run_uuid`、`task_ref`、`contract_ref`、`status`、`artifact_ref`、`reason_code` | `agent_verification_evidences` |
| `RuntimeOutcome` | `run_uuid`、`status`、`reason_code`、`recovery_action`、`user_action_required` | 扩展现有 Agent Run 的终态元数据 |

新表及其跨表引用统一使用 UUID；Plan/Task 的内部字符串 ID 仅在单次计划内使用，不充当业务对象身份。

## 5. 分阶段代码改造

### P0：先让失败和部分结果可判断、可解释

目标：解决“底层某节点失败，用户只看到执行失败或假成功”的问题，不引入资源发现。

1. 在 `backend/internal/server/agent/manager_execute.go` 将 `ExecutePlanWithHooks` 的返回值由“最后一个非空结果”升级为 `PlanExecutionReport`：保留每个 Task 的结果、错误、失败策略、重试次数和依赖跳过原因。
2. 在 `backend/internal/server/agent/runtime` 新增 `OutcomeClassifier`。它基于 Report、取消/超时、`awaiting_params`、权限错误和契约校验结果输出六种 Outcome；不得从错误文案做模糊猜测。
3. 将 `runResolvedPlan` 改为以 `PlanExecutionReport + VerificationEvidence` 生成最终答复。`continue` 后的失败必须进入 `partial`，而非被最后一个成功节点掩盖。
4. 将现有 `emitAgentRunFailure`、`agent_run.final/end` 和 Trace 统一带上稳定的 `reason_code`、`outcome`、`recovery_action`；UI 只显示本地化业务说明与下一步，不显示 executor 原始错误。
5. 删除 Runtime 中与具体业务对象绑定的参数猜测和确认词表，迁入 Skill 的结构化 `state_patch`、`missing_fields`、`confirmation` 合同。缺少结构化值时明确 `needs_input`。

验收：部分成功、缺参数、权限拒绝、契约拒绝、超时和取消在 Run State、Trace、History 与最终回复中的 Outcome 一致。

### P0.5：建立 Redis 权威运行态与可恢复调度

目标：同一消息内的独立任务可并行就绪，按真实资源容量领取；跨连接、跨进程、跨实例可追踪和恢复。完整状态机、存储分工及故障规则见 [持久化调度与模型容量设计](./agent_runtime_durable_scheduling.md)。

1. 定义 `AgentRunStore`、`AgentWorkQueue`、`WorkerLease`、`ResourcePool` 与 `RunEventJournal` 的强类型合同。生产默认驱动为 Redis；启动校验 AOF、非驱逐策略与健康状态，部署门禁检查容量、复制/备份和恢复演练，不允许静默回退内存或 PostgreSQL。
2. 将受理与执行分离：提交创建 `run_id` 和初始事件，任务由调度器和 Worker 执行；SSE/WS 只订阅状态，断线按 `event_seq` 续订。历史 API 读取 Redis 快照或已归档报告。
3. 扩展 Event Fabric TaskQueue 的消费组、可续租、过期重领、fencing、死信、延迟重试和幂等去重能力；Agent 使用统一基础设施，不直接复用当前无租约驱动，也不维护私有队列。
4. 将 stage/依赖计划编译成就绪任务；显式限制 Run/租户/服务实例的并发，模型池按共享 endpoint/model/deployment 实际槽位调度。队列等待时不占用单次模型请求时限。
5. 分离 `queue_wait_timeout`、`request_timeout`、`run_deadline` 和租约 TTL；停止按任务波数直接推导整轮硬期限。重试、取消、部分结果和副作用幂等必须保留可恢复证据。
6. Redis 中只写状态转换/检查点，模型 token 不写权威事件流；大 payload 和长期 Trace 归档对象存储，Session/SkillState/业务审计保留原权威服务。定义归档失败背压和完成态保留策略。
7. 在 Web Admin/Framework 共用 Run State 协议中加入排队、领取、验证、重试及原因码；同一 Run 的任务留在一张消息执行卡片，Trace 显示排队/执行耗时与每次尝试。

验收：单槽 Ollama 同一消息双任务、不同资源并行、两 Core 多 Worker、公平配额、重领/重复投递、浏览器断线、Redis 故障切换、对象存储归档、AOF 恢复和无幂等副作用保护全部通过；不能以单进程单元测试或 HTTP 200 代替。

### P1：建立 Resource Observation Plane

目标：让 Agent 能在没有 Workflow 或知识库时，观察其被授权读取的系统资源；不扩大可写范围。

1. 新增 `ResourceCatalogService`，聚合正式 Capability Registry、插件发布清单、业务资源提供者和文件/知识元数据提供者；每个提供者仅返回 `ResourceDescriptor`，不得返回原始敏感内容。
2. 新增 `ObservationService`，按 descriptor 的 `read_capability_id` 调用只读适配器，并在字段分级、tenant、ACL、数据域和脱敏检查之后产出摘要或 Artifact Ref。
3. 新增 `RuntimeSnapshotService`，在 `Engine.Run` 的 Prepare 阶段冻结 discovery grant、read grant、invocation grant、资源版本和预算。后续 Re-plan 只能使用此快照。
4. `CandidateBuildContext` 保持为**执行过滤**；新增 discovery context，禁止把“发现到的未绑定资源”直接塞入 `BuildToolCallCandidatesWithContext` 变成可执行 Tool。
5. 第一批只接入低风险资源：能力目录、当前 Agent/Skill/Team、已授权活动报表元数据、用户明确附带的文件和知识条目。高敏业务明细、跨租户数据和写能力不纳入首期。

验收：同一请求可发现与读取资源严格受快照约束；无 Workflow/Knowledge 的只读分析能完成，越权资源不会出现在模型上下文或 Trace 内容中。

### P2：将一次性 Engine 替换为受控 Loop

目标：在同一授权快照中完成有限次数的 Observe、Act、Verify、Re-plan，而不是发生错误后直接终止或任意重试。

1. 在 `backend/internal/server/agent/runtime/loop` 引入 `Controller`、`Budget`、`ObservationStore`、`PlanRevisionStore` 和 `RecoveryPolicy`；由 `Engine.Run` 直接委托 Controller，不保留两套生产执行路径。
2. Prepare 创建快照和 revision 0；Planner 只接收当前目标、已验证 Observation、可调用 Capability Graph 和剩余预算。
3. 每次执行后由 Verifier 输出结构化 verdict：`pass`、`retryable`、`replaceable`、`needs_input`、`blocked`、`fatal`。只有 `retryable` 可以按合同有限重试；`replaceable` 必须从冻结 capability graph 中选择替代能力。
4. Re-plan 必须保存 parent revision、触发 Observation/Verifier、被替换 Task 和预算消耗；旧 Plan 不可被原地篡改。
5. 设置硬上限：总步骤、总耗时、模型调用、Capability 调用、同类重试、并发数。上限耗尽输出 `partial` 或 `blocked`，绝不静默循环。

验收：可复现“读取报表 -> 发现口径缺失 -> 询问用户”“调用失败 -> 用已发布替代能力修复”“达到预算 -> 部分结果”的 Trace 回放，并证明没有超出初始快照。

### P3：把验证变为能力合同的一部分

目标：不再仅验证最终 Markdown/Envelope，而验证业务动作的真实完成条件。

1. 扩展正式 Capability/Skill Manifest：增加 `verification_contract`、`side_effect_evidence_schema`、`retry_policy`、`alternative_capability_ids`、`human_approval_policy`；缺少所需验证合同的写能力不得被 Agent 调用。
2. 新增 `VerificationService`，负责 schema、来源、调用回执、业务完成条件和副作用证据验证；验证器必须是确定性代码或经登记的验证 Capability，不能靠模型自由判断。
3. 营销活动复盘作为首个垂直样板：复用 Evidence Ledger，但把“引用合格”与“复盘任务完成”分开记录；重复 generic fact、单位/口径冲突、数据缺失分别进入可修复、需补充或阻断路径。
4. 对高风险写能力接入 `human.ask/approval`，审批事件成为 Verification Evidence；审批前不得提前声称完成。

验收：模型生成的漂亮结论没有真实来源/验证证据时，Run 不能标记 `completed`。

### P4：能力质量飞轮，且必须人审发布

目标：从线上失败学习，但不允许线上 Agent 自行修改生产业务规则。

1. 从脱敏 Trace 和用户反馈生成 failure cluster、缺口候选与评测样本。
2. 生成的是 Draft Skill/Capability Revision、测试样本与回归报告，不是直接修改已发布 Revision。
3. 运营/业务/安全审批后才可灰度；版本、指标阈值和回滚点必须可追溯。

验收：任一 Skill 公式、权限、数据策略或高风险动作的变化都有人工审批、离线评测、灰度指标和回滚 Revision。

## 6. 推荐实施顺序与依赖

```text
P0 Outcome/Report
  -> P0.5 Redis Run Store + Queue/Worker + Resource Pool
    -> P1 Snapshot + Observation
      -> P2 Controller + Revision + Budget
        -> P3 Capability Verification Contract
          -> P4 Offline Evaluation and Human-approved Evolution
```

P0 可以立即开始，且应先于营销 Skill 的继续扩展；P0.5 解决长任务执行与容量瓶颈，不能被“把模型并发数改为 1”替代。P1/P2 是让 Agent 从“固定候选调用器”变成“受控业务 Agent”的最小架构拐点。P3 不等待所有资源接入，但任何新写能力必须在上线前具备验证合同。

## 7. 测试与上线门槛

每个阶段都要新增单元、集成和端到端运行追踪测试：

1. P0：DAG 部分失败、继续策略、超时、取消、等待参数、原始错误不泄漏。
2. P0.5：Redis 权威状态/队列、任务池和模型池、三类时限、事件续订、跨实例恢复、幂等副作用与真实单槽模型负载。
3. P1：tenant/ACL/字段分级隔离、discovery/read/invocation grant 分离、快照一致性、Artifact 脱敏。
4. P2：预算耗尽、有限重试、替代能力、Plan Revision 可回放、循环终止性。
5. P3：每类 Capability 的验证通过/失败、审批阻断、无真实副作用证据不得 completed。
6. P4：离线评测阈值、审批、灰度、回滚和版本比较。

上线看板至少包含：`verified_completion_rate`、`partial_rate`、`needs_input_rate`、`recovery_success_rate`、`budget_exhausted_rate`、`raw_internal_error_exposure_rate`、越权发现/调用拒绝率、排队 P95/P99、租约回收率、资源池利用率、归档积压和回归失败率。

## 8. 首个实施切片

首个可交付切片选“营销活动复盘”：

1. P0 先让 Evidence 契约失败被归类为 `needs_input`、`partial` 或 `failed`，并给出稳定的业务修复动作。
2. P1 只注册活动、指标、渠道报表和用户上传文件的只读 descriptor。
3. P2 只允许一次受控恢复：补读同一活动的已授权来源，或请求具体缺失指标；不自动扩大活动范围、不自动改写 Skill。
4. P3 将活动复盘的来源、指标口径、计算证据作为 `verification_contract` 样板。

它能验证 Runtime 新机制，但不会把营销业务规则硬编码进 Core。

### 2026-10-03 归档实施记录

正式 durable Worker 已接入终态自动归档与 finished-but-unarchived 恢复扫描。对象写入及读回校验后再保存低频数据库 locator；对象/locator 失败可重试，不增加模型调用或助手消息。归档保留规划任务、各版计划/任务及完整事件序列。管理端追踪报告/SSE、服务会话 Query/SSE/状态快照在热状态不存在后可按现有所有权读取校验后的对象。新增归档列已通过集中迁移在开发数据库确认。

尚未交付保留期清理与归档积压背压；真实营销授权请求、浏览器刷新、跨 Core/Redis 故障演练和生产容量证据仍待验收。本轮未操作 8077 后台重启，未修改正式开发配置以切换 durable 分支。

### 2026-10-04 存储生命周期与页面恢复记录

保留期清理、共享归档积压背压及页面刷新恢复已实现并接入持久化分支。归档对象、低频 SQL 定位、热状态到期确认分阶段恢复；共享队列按 Run 退休，避免取消后留存无消费者的任务。页面从服务器历史与 Run 定位恢复，不重发输入。两独立进程调用真实 Ollama 已验证共享单槽排队，真实 Redis 队列退休也已验证。

这些开发与组件运行证据不替代已认证的营销 HTTP/SSE、真实浏览器、完整多 Core 故障演练和压测；031 对应任务保持这些验收缺口。当前没有由本轮操作正式 8077 后台重启或配置切换。

隔离 AOF 强制终止/重启用例已实际执行：always fsync 下 Run/事件不丢失，原消息重领和旧 fence 拒绝，规划只执行一次，约 1.189s 完成恢复及重领。共享开发 Redis 和正式后台未由本用例重启；everysec 与完整 Core/业务 RPO/RTO 仍需独立验收。

### 2026-10-04 共享执行配额与步骤预算

Worker 正式接入 Redis 的 Run/租户原子执行槽，包含规划、续租失败取消、过期回收、部署配置漂移门禁。容量不足的任务保留原排队时间和 attempt，延后 500ms；同原因不写重复进度事件，消费入口自动推进到期延迟项。两 Worker 共用单 Run 配额且另一租户完成、独立进程在真实 Redis 上的 Run/租户共享配额验证通过。

新不可变计划预留完整 DAG 的步骤/Capability tooling 节点预算；超额终止规划，执行按已验证完整计划重建检查，旧计划兼容读取。上述结果尚不包含嵌套调用、token/成本、重规划累计预算或生产跨 Run 公平性压测。本轮不需要数据库迁移，正式后台仍未重启；新 Worker 代码需在安排重启时才会生效。
