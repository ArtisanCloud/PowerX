# PowerX Business Agent Runtime Loop 设计规范

状态：**目标架构，待分阶段实施**。本文不把当前的 Plan Executor、Skill Registry、Trace 或已存在的 A2A 机制表述为已经完成的通用自主 Loop。

## 1. 定位

PowerX 的目标不是把 Codex 包装成一个业务聊天入口，也不是让模型绕过治理直接访问数据库、插件私有接口或任意文件系统。

PowerX Business Agent Runtime 是一个面向企业业务世界的受限控制器：它在当前租户、用户、Agent 绑定、数据分级、能力授权、预算与风险策略内，持续选择**当前最可信、可验证、风险可控的下一步**。

它同时满足两个要求：

1. **底座优先与业务确定性**：已经发布、验证、适用的 Core Capability、插件 Capability、Skill 或 Workflow 必须优先复用。
2. **受限自主性**：当没有单一预编排方案覆盖用户目标时，Runtime 可以观察已授权资源、组合已有能力、基于结果修订计划、请求补充信息或明确阻塞；它不能臆造能力、扩大权限或把未验证结论说成完成。

因此，Skill、Tool 与 Workflow 是受治理的业务能力单元和成熟剧本，不是 Agent 能工作的唯一前提；知识库是可选观察源，也不是行动前置条件。

## 2. 当前基线与差距

当前实现已经具备 Session/Message、Agent-Skill Binding、候选硬过滤、任务 Plan、Skill/Tool/Capability 执行、Run State、Trace、审计、结构化结果、ResponsePlanner，以及 `agent_session_skill_states` 驱动的等待参数状态持久化与恢复。这些是 Runtime 的基础，不应推翻。

Run State 的可见协议不等于可恢复调度：当前计划执行与模型容量控制主要在进程内，部分计划/任务事件落库但不能充当权威队列。目标以 Redis 为 Agent 运行态默认驱动，配合对象存储归档；Session、SkillState、业务状态与审计仍归各自权威服务。队列、模型池、租约、断线续订和独立时限的完整合同见 [持久化调度与模型容量设计](./agent_runtime_durable_scheduling.md)。

当前通用主链路仍以一次性计划为中心：

```text
用户消息
  -> 从当前 Agent 已绑定且已授权的候选集中检测任务
  -> 生成 ExecutionPlan
  -> 执行已解析的节点
  -> 结构校验与最终答复
  -> 成功或失败结束
```

主要缺口：

1. 当前候选集主要是预绑定 Skill/Tool/Workflow，不是按需发现的系统资源图。
2. 节点结果通常不会驱动通用的 Plan Revision；校验错误常直接终止整轮。
3. 缺少统一的 Observation、Verifier、替代能力和恢复策略合同。

基于现有代码的具体改造顺序、合同和验收见 [Agent Runtime 实施计划](./agent_runtime_implementation_plan.md)。
4. 用户可能看到内部错误码，而不是业务化的状态、已检查范围和可执行下一步。
5. 运行失败能够进入 Trace，但尚未形成受治理的能力质量评估、草稿、评测、灰度与发布飞轮。

## 3. 总体架构

```text
                 ┌────────────────────────────────────┐
                 │ Policy / Tenant / RBAC / Budget     │
                 └────────────────────────────────────┘
                                   │
User Goal -> Runtime Controller -> Resource Observation Plane
                  │                │
                  │                ├-> Core Capabilities
                  │                ├-> Plugin Capabilities
                  │                ├-> Business Data / Reports
                  │                ├-> Files / Knowledge / Artifacts
                  │                └-> Skills / Workflows / Teams
                  │
                  -> Capability Graph -> Act / Verify -> Observation Store
                                             │                  │
                                             └---- Re-plan <----┘
                                                        │
                                              Final / Partial / Needs Input / Blocked
```

### 3.1 四层职责

| 层 | 责任 | Agent 可自主做的事 | 不可绕过的边界 |
| --- | --- | --- | --- |
| Resource Observation | 资源发现、描述、只读访问、来源定位 | 查询已授权数据、文件、报表、插件状态和能力元数据 | tenant、ACL、字段脱敏、数据等级、来源策略 |
| Capability Graph | 能力合同、适用条件、风险、输入输出、验证和替代关系 | 在可见能力中选择、组合和排序 | 只能调用已发布且当前授权的能力 |
| Runtime Controller | Observe -> Plan -> Act -> Verify -> Re-plan 循环 | 在预算内推进下一小步、追问、部分交付、停止 | 步数/时间/成本限制、HITL、不可无限循环 |
| Evolution Governance | 质量评估、Skill 草稿、测试、灰度、发布、回滚 | 归纳缺口、生成草稿和评测建议 | 不得由生产任务自动修改或发布业务规则、公式、权限 |

## 4. 资源观察与能力发现

### 4.1 资源不是裸访问权

“PowerX 系统里的资源可被 Agent 观察”仅指已发布、已授权、可审计的资源合同。不得把该原则实现为 Agent 直读任意数据库表、插件私有 URL、本机文件系统、密钥或跨租户对象。

资源应以只读 Observation Capability 暴露，例如：

```text
resource.discover
resource.describe
data.query
report.inspect
file.search
file.read_excerpt
artifact.read
capability.explain
```

每个 Resource Descriptor 至少应声明：

```text
resource_uuid, resource_kind, source, tenant_uuid, display metadata,
read capability, data classification, ACL/field policy, freshness,
artifact/source reference, cost hint, audit requirements
```

`resource_uuid` 是业务资源和跨服务引用的稳定身份；Capability ID、协议、端点和 Schema Ref 是机器合同，不得在普通业务答复中作为主标签展示。

### 4.2 能力优先级

Runtime 每轮从经过硬过滤的能力图中排序，顺序是：

1. 满足前置条件的已验证 Core Capability 或发布 Workflow；
2. 满足前置条件的已授权插件 Capability；
3. 当前 Agent 已绑定、已发布的 Skill/Tool/Team；
4. 用只读资源补充信息后再评估上述能力；
5. 明确 `needs_input`、`blocked` 或创建受治理的能力缺口记录。

“发现能力”不等于“取得调用权”。Discovery Grant 只允许看到安全元数据；Invocation Grant、对象归属校验和风险策略仍在调用时重新校验。

## 5. 受限 ReAct Loop

### 5.1 Run 状态

每个 Run 必须持有以下权威或可追溯状态：

```text
Goal + Constraints + ResourceSnapshotRef + CapabilitySnapshotRef
+ PlanRevision + Observations + VerificationEvidence + Budget + Outcome
```

其中快照必须固定 `tenant_uuid`、`agent_id`、用户/调用主体、授权指纹、资源版本和能力版本；缓存不得改变本轮可见边界。

### 5.2 循环步骤

```text
1. Prepare: 解析目标、策略、预算与风险；建立本轮资源/能力快照。
2. Observe: 只读取当前最能降低不确定性的资源。
3. Plan next action: 选择一项最小、可验证、已授权的下一步。
4. Act: 调用 Capability、Skill、Workflow、Tool 或 human.ask/approval。
5. Verify: 校验结果合同、来源、业务完成条件和副作用证据。
6. Decide: completed / partial / needs_input / blocked / retry / re-plan。
```

Runtime 不是追求无法证明的“全局最优解”，而是在当前证据和策略下选择最合理下一步。每轮必须设定最大步骤数、总超时、Token/调用预算、同类重试数和任务并发上限；资源池另设实际模型/能力容量。排队等待、单次请求与整轮预算分别计时；达到上限时输出可解释的 `partial` 或 `blocked`，不能静默循环。

### 5.3 计划修订

Plan 是版本化的工作假设，不是一次生成后不可改变的命令。每次修订必须记录：

```text
plan_revision, trigger_observation_ref, superseded_tasks,
new_tasks, rationale, policy_decision, budget_delta
```

允许修订 Plan、替换调用路径、追加观察、降级为澄清或部分报告；不允许在生产 Run 中直接修改 Skill Revision、公式、权限、数据分级或已发布 Workflow。

## 6. 验证、错误恢复与用户答复

### 6.1 失败分类

| 类别 | Runtime 行为 | 用户可见结果 |
| --- | --- | --- |
| 可机械修复的结构问题 | 受限修复或平台归一化，例如去重、重新编码、一次 schema repair | 不暴露内部错误；继续或给出部分结果 |
| 临时依赖故障 | 按声明退避重试；超过预算则换已授权路径 | 说明系统暂不可用及重试建议 |
| 可替代能力存在 | 重新规划并调用替代能力 | 说明采用的资料/口径，不虚构等价性 |
| 缺业务数据或证据 | 继续观察或 `human.ask` | `needs_input`，列出缺失内容与用途 |
| 权限或对象范围不足 | 不枚举不可见对象；不尝试旁路 | `blocked`，说明需要何种授权/角色 |
| Runtime/契约缺陷 | 记录结构化诊断和 Trace；创建质量信号 | 稳定、脱敏的业务化失败说明与恢复动作 |

`generic_fact_duplicate`、executor 路径、内部错误码、原始 Stack、Trace ID 不得作为普通用户的最终答复。它们只应在诊断/Trace 视图显示。用户必须得到：已完成什么、验证依据是什么、还缺什么、下一步可以做什么。

### 6.2 完成语义

```text
completed   已达到声明完成条件，且有验证证据。
partial     已交付可验证部分，未覆盖范围明确。
needs_input 缺用户输入、文件、授权或审批，任务可恢复。
blocked     当前权限、能力或外部依赖不满足，不能安全推进。
failed      已尝试的可用路径出现不可恢复系统故障。
cancelled   用户或策略取消。
```

Run 结束不等于业务任务完成；最终自然语言回复不等于有执行证据。该规则补充现有 Run State 协议。

## 7. 风险与人工协同

| 风险级别 | 示例 | 默认策略 |
| --- | --- | --- |
| R1 只读 | 查询、摘要、分析、发现资源 | 自动执行 |
| R2 可逆低风险 | 创建草稿、保存未发布配置 | 策略允许时自动，记录审计 |
| R3 有副作用 | 发通知、更新业务对象、创建工单 | 预览后按策略或用户确认 |
| R4 高风险 | 发布、删除、付款、权限修改、跨域共享 | 强制人工审批与执行后验证 |

`human.ask` 与 `human.approval` 是 Loop 的标准 Action。人工确认只批准具体计划修订与具体副作用，不授予未来任意操作的泛化权限。

## 8. Skill 与能力演进闭环

在线任务闭环和能力演进闭环必须分离：

```text
在线：观察 -> 调用已有能力 -> 验证 -> 修订 Plan -> 交付/阻塞

演进：Trace/反馈 -> 失败聚类与评测 -> Skill/Capability 草稿
      -> 自动契约与回归测试 -> 人工审批 -> 灰度 -> 发布/回滚
```

数据库保存 Skill 的原因是让其 Revision、绑定、版本、依赖、权限和发布状态可治理、可审计、可回滚；不是允许生产 Agent 因一次失败自行覆盖或发布 Skill。

AI 可以提出 Skill 草稿、Workflow 候选、测试样例、评测数据和风险说明。业务公式、数据访问范围、权限扩大、正式发布和回滚必须由有权限的人或明确的发布策略负责。

## 9. 可观测性与质量指标

除现有 Trace 节点外，目标 Runtime 需记录 `resource_discovery`、`observation`、`verification`、`plan_revision`、`recovery_decision`、`approval` 和 `outcome`。敏感内容以分级 Artifact Ref 或摘要保存。

核心指标：

```text
task_completion_verified_rate
partial_delivery_rate
needs_input_resolution_rate
raw_internal_error_exposure_rate
recovery_success_rate
repeat_failure_rate
capability_gap_rate
plan_revision_count
human_approval_rate
skill_revision_quality_regression_rate
```

`raw_internal_error_exposure_rate` 的目标是零；“模型回复成功”不能替代已验证业务完成率。

## 10. 分阶段交付与验收

### Phase 0：失败体验与结果语义

1. 将内部错误分类为可修复、可替代、需补充、无权、系统失败。
2. 修复前端/最终答复，禁止普通用户看到 executor/error code/trace 原文。
3. 支持机械去重、一次受限修复和 `partial/needs_input/blocked` 输出。

验收：重复结构化事实、缺字段、未授权和临时依赖失败均不会只返回“执行失败”；Trace 保留完整诊断。

### Phase 0.5：持久化调度与容量治理

1. 接入 Redis 权威 Run/Task 状态、可靠队列与 Worker，拆分执行生命周期和 SSE/WS 连接。
2. 按任务依赖和实际资源池容量调度，允许同一消息内可并行任务同时就绪，容量不足的请求有界排队。
3. 持久化事件序号、租约、重领、重试、取消及归档引用；排队等待、单次调用和整轮预算独立计时。
4. 生产启动必须校验 Redis 持久化与恢复前置条件；Redis 不可用时不降级为进程内任务池。

验收：单槽 Ollama 的消息内双任务、浏览器断线、跨实例 Worker 崩溃/重领、Redis 故障切换及无幂等副作用保护均按 [调度规范](./agent_runtime_durable_scheduling.md) 验证。

### Phase 1：Resource Observation Plane

1. 为 Core、插件、数据、文件、报表建立可发现的只读 Resource Descriptor 与 Observation Capability。
2. 实现 discovery grant 与 invocation grant 分离、租户/ACL/字段分级校验和快照引用。
3. Agent 能在无 Workflow、无知识库时使用已授权的只读业务资源完成分析或明确缺口。

### Phase 2：Bounded ReAct Controller

1. 引入 Observation Store、Verifier、Plan Revision 和预算控制。
2. 为 `human.ask/approval`、部分结果、替代能力和恢复策略定义合同。
3. 所有重新规划都可由 Trace 回放，且不会突破原始授权快照。

### Phase 3：能力质量飞轮

1. 从 Trace 与用户反馈生成脱敏失败聚类和能力缺口。
2. 生成草稿 Revision、测试样例与离线评测；禁止直接发布。
3. 灰度、指标门槛、人工审批、回滚和版本对照完成闭环。

## 11. 非目标

1. 不提供无限制 Shell、数据库直连或插件私有 URL 调用。
2. 不通过自然语言猜测未发布 Capability、权限或业务字段。
3. 不把知识库、Workflow 或某个固定 Skill 当作所有任务的必经入口。
4. 不让在线 Agent 自动修改和发布生产 Skill、公式、权限或数据策略。
5. 不以“Agent 做了很多步骤”替代可验证的业务结果。
