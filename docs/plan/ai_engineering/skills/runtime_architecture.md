# Skills 运行时架构设计

本文定义 Skill 在 PowerX 的双路径运行时接入方案。

补充机制：

1. PowerX 与插件之间的 Agent Skill Bridge 统一桥接规范见 [`agent_skill_bridge.md`](./agent_skill_bridge.md)。本文的 Agent + Skill 主路径必须遵循该桥接边界：渠道进入 PowerX Agent Session，PowerX Agent Runtime 选择 Agent 已绑定 Skill，Skill action 映射到 capability_id，最终通过 PowerX Capability Invocation 执行业务。
2. Agent Runtime 结构化日志、节点追踪与报告下载机制见 [`agent_run_trace_report.md`](./agent_run_trace_report.md)。所有 Agent 主入口、Skill/Tooling 节点、插件 Skill Bridge 调用都必须写入同一套 Agent Run Trace。
3. PowerX Core 自有 A2A 多智能体协作机制见 [`multi_agent_a2a.md`](./multi_agent_a2a.md)。A2A 是底座 Agent Runtime 内部的 `agent_handoff` 编排能力，不依赖插件 capability handler；插件 Skill 只是在后续可作为子 Agent 绑定能力进入候选池。
4. Agent 最终回复的 ResponsePlanner、Context Builder 与 Final Response 分层机制见 [`agent_response_planning.md`](./agent_response_planning.md)。自然语言回答不得直接复述全局候选池，必须先生成 `response_plan`，再按 `response_mode` 选择上下文并落库 message meta。
5. Agent 对话、团队任务、插件调试页展示多任务/多智能体执行过程时，必须遵循 [`agent_run_state_protocol.md`](./agent_run_state_protocol.md)。`agent_run.*` 是 Runtime 与 UI 的共享状态协议，覆盖 task 状态、缺参等待、执行结果链接和 trace 精确定位。
6. Agent Runtime 标准服务面见 [`agent_runtime_standard_services.md`](./agent_runtime_standard_services.md)。Core 只提供 session/context/skill state/capability invocation/trace/artifact/progress/model policy/tenant authz 等通用服务；业务字段、缺参规则、状态合并和执行就绪判断必须来自 Skill。

## 1. 总体架构

Skill 运行时支持两条路径：

1. 路径A：Agent 内 SkillRunner
2. 路径B：Capability Gateway 的 SkillAdapter（`preferred_protocol=skill`）
3. 路径C：Agent Skill Bridge 将插件 Skill action 解析为 capability invocation（`executor.type=capability`）

两条路径共享：

- SkillRegistry
- ToolingRegistry（capability registry，数据库权威源）
- SkillManifest 校验
- 审计与追踪模型
- 安全策略（tool_grants/safe_mode）
- PowerXPlugin Framework Client 的 Agent SSE/WS/HTTP 调用封装

插件侧本地 Chat、渠道插件、移动端调试面板均不得直接调用业务插件私有接口来模拟 Agent；必须经 PowerX Agent Session/Stream API 进入同一条运行时链路。

## 2. 路径A：Agent + Skill

### 2.1 触发条件

当 Agent Planner 识别到任务节点类型为 `skill` 时，进入 SkillRunner。

### 2.0 Runtime Intent / Control Command

Agent 主入口必须先区分“控制面 Runtime Intent”和“自然语言任务意图”：

1. Runtime Intent 是结构化控制命令，由请求字段显式传入，例如 `intent=agent.bound_capabilities`。
2. Runtime Intent 不依赖自然语言关键词匹配，不进入 LLM，不进入 Planner。
3. Runtime Intent 只执行确定性 handler，例如 `agent.bound_capabilities` 只读取当前 Agent 的 `agent_skill_bindings`。
4. 普通自然语言请求必须进入 `intent_classifier -> planner -> node executor` 主链路。
5. 前端、插件 Chat 或渠道若需要展示“当前 Agent 已绑定能力”，必须调用结构化 Runtime Intent，不得让 LLM 从候选池或 prompt 中猜测。

首批 Runtime Intent：

| intent | handler | 数据源 | LLM/Planner |
| --- | --- | --- | --- |
| `agent.bound_capabilities` | `BoundCapabilitiesHandler` | `agent_skill_bindings` + `skills_registry_records` | bypass |
| `agent.bound_skills` | `BoundCapabilitiesHandler` | `agent_skill_bindings` + `skills_registry_records` | bypass |

Runtime Intent 与自然语言意图的入口顺序：

```text
Agent Session Request
  -> RuntimeIntentRouter
      -> matched: deterministic handler -> final/end
      -> not matched: NaturalLanguageIntent -> Planner -> NodeExecutor
```

### 2.1.1 决策分层（Intent / Planner / Executor）

Skill 是否执行，不由单一阶段直接拍板，而是三层决策：

1. Intent 层：给出 `candidate_skills[]`（候选）。
2. Planner 层：结合依赖、权限、上下文，决定是否把候选落为 `skill` 节点。
3. Executor 层：按 `node.kind/use` 分发，`kind=skill` 才进入 SkillRunner。

### 2.1.1a 候选分层（System + Agent）

进入 Intent/Planner 前，先构建统一候选池，必须满足：

1. 按类型分区：`workflow`、`skill`、`tooling`。
2. 按来源分层：`system builtin`（平台固有）+ `agent custom`（Agent 自定义）。
3. 先合并去重，再做硬过滤（tenant/scope/status/source/tool_grants/binding）。
4. 仅将过滤后候选注入 LLM 决策输入。

同名候选去重优先级（落地约束）：

1. 先按 `name + node_kind` 分组去重。
2. 若同时存在 `system` 与 `agent`，优先保留 `agent custom`。
3. 同层冲突按 `updated_at`（新优先）与 `binding_status=active` 决定保留项。
4. 任何未通过硬过滤（tenant/source/tool_grants/visibility）的候选不得进入 LLM。

建议在 Intent 输出增加结构化字段：

```json
{
  "intent": "incident_triage",
  "candidate_skills": [
    {"skill_id": "incident-triage", "confidence": 0.91, "reason": "match keywords + context"}
  ]
}
```

### 2.1.2 Skill 候选识别（Intent 内增强）

不建议每个 Agent 独立硬编码；建议在现有 Intent 层统一增加 `Skill Resolver`：

1. 结构化 Runtime Intent：由 `RuntimeIntentRouter` 处理，不进入 Skill Resolver。
2. 快捷命令：`/command` 可走规则，必须显式命令格式。
3. 硬过滤：按 Agent 绑定、租户、发布状态、权限、source allowlist、tool grants 过滤候选。
4. 召回：关键词/标签/向量检索只在硬过滤后的可见范围内执行。
5. 重排：LLM 或 reranker 输出 top-k；不得臆造未授权 `skill_id`。

最终由 Planner 消解冲突并定案。

### 2.1.2a 节点级模型选择策略

Agent Runtime 必须支持节点级模型选择。首版可以全部继承 Agent 默认模型，但运行时接口和 Trace 必须保留节点模型选择结果。

标准节点：

| 节点 | 默认模式 | 后续可选模型 |
| --- | --- | --- |
| `runtime_intent` | deterministic | 不使用模型 |
| `intent_classifier` | inherit Agent default | 小模型 / 分类模型 |
| `planner` | inherit Agent default | 中/大模型 / 推理模型 |
| `skill_param_extractor` | inherit Agent default | 小/中模型 |
| `final_response` | inherit Agent default | Agent 默认模型 |
| `reviewer` | inherit Agent default | 中模型 / 审核模型 |

策略对象：

```json
{
  "default_provider": "openai",
  "default_model": "gpt-4o-mini",
  "selections": {
    "runtime_intent": {"mode": "deterministic", "source": "runtime_command"},
    "intent_classifier": {"mode": "inherit_default", "provider": "openai", "model": "gpt-4o-mini"},
    "planner": {"mode": "inherit_default", "provider": "openai", "model": "gpt-4o-mini"},
    "final_response": {"mode": "inherit_default", "provider": "openai", "model": "gpt-4o-mini"}
  }
}
```

约束：

1. 缺省策略必须等价于“所有模型节点使用 Agent 默认模型”。
2. Planner 调用必须读取 `planner` 节点选择结果。
3. Trace/SSE metadata 必须输出 `model_policy`，便于排障。
4. 后续引入 DB 路由策略时，只能覆盖节点选择结果，不得绕过 Agent 权限边界。

### 2.1.3 提示词策略（统一模板）

建议使用统一提示模板，而不是每个 Agent 各写一套：

1. 输入：`user_message + allowed_skills + agent_profile + context`。
2. 约束：只能从 `allowed_skills` 选择。
3. 输出：结构化 JSON（`intent/candidate_skills/confidence`）。
4. 无命中：返回空数组，不得臆造 skill_id。

补充：LLM 输入不应只是一段未分区的“工具列表文本”，应包含结构化分区：

1. `workflow_catalog[]`（含 `source=system|agent`）
2. `skill_catalog[]`（含 `source=system|agent`）
3. `tooling_catalog[]`（含 `source=system|agent`）
4. 每项附参数 schema 与约束标签（授权/可见性/来源策略）

### 2.2 执行流程（目标态：多 Skill + DAG）

1. Intent 输出多候选 `candidate_skills[]`（top-k）
2. Planner 生成计划 DAG（`serial stages + parallel groups`）
3. 每个节点落盘 `plan_id/node_id`，并执行硬过滤（tenant/source/tool_grants/scope/status）
4. Tool-Calling 选择节点执行参数（仅限 allowlist 中技能）
5. Runner 拉取 Manifest 与 Bundle 并执行 entrypoint
6. 节点结果回填 Planner 上下文（供后续节点引用）
7. 输出到 Agent stream（intent/plan/node_start/token/node_end/final）
8. 写审计与指标（trace_id + plan_id + node_id）

当计划节点命中 `source=plugin` 的 Skill 时，SkillRunner 不调用插件 Skill 私有 executor。它必须读取 Skill Manifest 的 `action_capabilities` 或 `executor.action_map`，将 planner 提取的 `action` 解析为 `capability_id`，再进入 PowerX Capability Invocation。

```text
Agent skill node
  -> Skill Manifest action_capabilities[action]
  -> capability_id
  -> PowerX Capability Invocation
  -> Plugin capability handler
```

如果 action 为空、映射缺失、capability 未注册、租户无权访问或插件实例不可用，必须 fail-fast，并写入 Agent Trace。不得回退到插件私有 Skill 执行入口或插件私有业务 URL。

组合规划可追溯元信息（落地约束）：

1. 每个计划节点必须包含：`node_kind`、`node_ref`、`source_scope`。
2. `workflow` 节点可声明 `depends_on=[skill_node, tooling_node]`。
3. `node_start/node_end` 事件 payload 必须携带上述三字段，便于前端与审计复盘。

### 2.3 失败语义

1. 可重试错误：依赖临时不可用、网络抖动
2. 不可重试错误：鉴权失败、manifest 非法、签名不通过

## 3. 路径B：Capability Invocation

### 3.1 触发条件

租户调用：

- `POST /api/v1/tenant/invocations`
- `preferred_protocol` 根据 capability 绑定的协议选择，例如 `rest/grpc/mcp/skill`

### 3.2 执行流程

1. Selector 解析 capability/policy
2. Router 选到对应 transport adapter
3. Adapter 装配执行上下文
4. 调用真实 capability handler
5. 返回统一 envelope（trace/status/protocol/result）
6. 写 InvocationTrace 与审计事件

约束：

1. Agent 业务执行优先使用 `Skill action -> capability_id -> Capability Invocation`。
2. 不再把 `/tenant/skills/invoke` 作为标准业务执行路径。
3. 若历史环境仍存在直接 Skill 调用接口，只能作为迁移期内部调试入口，不得写入新规范和插件开发指南。

## 3.1 插件 Capability 路径（Agent Skill Bridge）

触发条件：

1. Agent Planner 选择 `source=plugin` 的 Skill。
2. Skill Manifest 中 executor 声明 `type=capability`。
3. 当前租户已安装并启用对应插件实例。

执行流程：

1. PowerX 从治理态 Skill Registry 读取插件 Skill Manifest 快照。
2. 校验 Skill 已发布、租户可见、Agent 已绑定、tool_grants/source allowlist 通过。
3. 从 `action_capabilities` 或 `executor.action_map` 解析 action 对应的 `capability_id`。
4. 组装 Capability Invocation，注入租户、用户、Agent、Session、Message、Channel、Trace 上下文。
5. 调用 PowerX Capability Invocation，由 Gateway/Router 选择插件 capability adapter。
6. 插件 capability handler 校验上下文并执行领域任务。
7. PowerX 归一化 capability result，写入 Agent Stream、会话消息、trace/audit。

失败约束：

1. 缺少关键上下文必须返回 fail-fast 错误。
2. action 无映射、capability 不存在或 capability 与插件来源不匹配时必须拒绝调用。
3. 不允许降级为匿名调用、跨租户调用、Skill 私有 executor 或渠道直连业务接口。

## 3.3 Agent 主入口（闭环入口）

建议将以下接口作为完整闭环主入口：

1. `POST /api/v1/agents/invoke`（非流式）
2. `GET /api/v1/agents/stream/sse`（流式）

约束：

- 调用方仅传 `message + agent_id(+session_id)`，不强制传 `skill_id`。
- 系统自动执行 `intent -> plan -> tool/skill nodes -> final`。
- 业务执行统一落到 `/tenant/invocations` / Capability Invocation；Skill 不能脱离 Agent 与 capability grant 独立执行业务。

## 3.4 ResponsePlanner / Context Builder / Final Response

Agent 主入口的 `final` 不是“把所有上下文直接塞给 LLM 后生成回答”。标准链路必须是：

```text
User Message
  -> Intent / Tool Planner
  -> Response Planner
  -> Context Builder
  -> Final Response LLM
  -> Persist Message + Meta
```

ResponsePlanner 负责输出结构化 `ResponsePlan`：

```json
{
  "response_mode": "capability_intro",
  "should_call_tool": false,
  "target_capability_ids": ["powerxplugin.template.basic"],
  "use_capability_context": true,
  "include_examples": true,
  "include_schema": false,
  "repeat_full_intro": false,
  "needs_clarification": false,
  "missing_fields": []
}
```

标准 `response_mode`：

1. `capability_intro`：介绍当前 Agent 已绑定能力。
2. `capability_howto`：说明某项能力怎么用、需要哪些输入。
3. `skill_execution`：总结 skill/tool/agent_handoff 执行结果。
4. `clarify_params`：执行意图明确但缺少必要参数。
5. `normal_chat`：普通上下文对话。
6. `error_explain`：把执行错误转换成用户可理解的说明。

Context Builder 必须按 `response_mode` 动态注入上下文：

| Mode | 上下文要求 |
| --- | --- |
| `capability_intro` | 当前 Agent 已绑定能力摘要、title/description/examples、最近 message meta |
| `capability_howto` | 目标能力详情、input_schema.required、action enum、prompt_spec 摘要 |
| `skill_execution` | 执行结果摘要、artifact refs、目标能力 title |
| `clarify_params` | 缺失字段、字段说明、示例提问 |
| `normal_chat` | Agent profile、结构化会话摘要、最近消息；默认不注入完整能力目录 |
| `error_explain` | error_code、error_summary、failed_node、可操作下一步 |

硬约束：

1. 用户可见能力上下文只能来自当前 Agent 的绑定能力，不能来自全局候选池。
2. 平台内部 runtime 工具可以用于执行，但未被显式绑定/授权时不得作为“我能做什么”暴露给用户。
3. Final Response LLM 只负责自然语言表达，不负责重新选择 capability。
4. assistant message 必须保存 `response_mode/capability_ids/response_plan_id/used_context_layers/tool_calls/final_response_model` 等 meta，用于后续去重和追问。
5. SSE/debug event 必须输出 `response_plan`，Agent Trace 必须记录 `response_planner/context_builder/final_response/history_persist` 节点。

### 3.4.2 Response Guidance 来源与边界

PowerX Agent Runtime 的通用 prompt 只表达平台级约束，不承载业务 Agent 的专属话术或字段规则。最终回复规范按以下来源合并：

1. Core runtime rules：安全、权限、租户隔离、不得暴露内部字段、不得编造未绑定能力。
2. Agent `persona`：Agent 身份、服务对象、表达边界。
3. Agent `prompt_seed`：当前 Agent 的默认回答策略，例如如何介绍能力、如何引导用户测试。
4. Skill `response_guidance`：能力级说明，来自 `manifest_json.response_guidance` 或 `manifest_json.prompt_spec.response_guidance`。
5. ResponsePlan `answer_requirements`：本轮用户消息拆出的组合回答要求。

Core Runtime 只负责抽取和拼装这些材料：

- `SkillRegistryRecord.manifest_json.response_guidance`
- `SkillRegistryRecord.manifest_json.prompt_spec.response_guidance`
- `ToolCallCandidate.ResponseGuidance`
- `CapabilityContextItem.ResponseGuidance`
- `[CONTEXT-L1 CAPABILITIES]` 中的 `回复规范`

严禁在 Core Runtime 中写入某个业务 Agent 的专用字段规则、行业示例或执行话术。例如模板对象的 `template.name/template.description/template.content` 要求必须来自模板 Skill 包，而不是 `final_response_prompt.go`。

### 3.4.1 上下文与 SkillState 存储驱动

Agent 上下文不是 runtime 内存单点驱动。标准分层：

1. Runtime Memory：只保存本轮请求过程态，例如 ResponsePlan、节点输出、短生命周期执行状态；不是权威源。
2. PostgreSQL：保存 Agent Session/Message、assistant message meta、Skill Registry、Agent-Skill Binding、模型策略、结构化摘要、context_ref metadata；是业务与治理权威源。
3. Redis：保存短 TTL planner/response_plan 缓存、候选快照、recent meta hot window；只能加速，不能改变可见能力边界。
4. Local File：本地开发保存 Agent Trace artifact，路径为 `backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}`。
5. Loki：生产保存 Agent Trace 事件，用于 root-only 调试检索。
6. Object Storage：保存大 prompt/context/tool payload artifact，DB 只保存引用与 checksum。

Context Builder 必须优先读取 DB 权威记录；Redis 或内存命中只能作为缓存。去重判断必须读取 assistant message meta，不允许靠自然语言文本匹配。

多轮业务任务不得只依赖最近消息窗口或上下文摘要恢复参数。跨轮业务参数、缺参状态、确认状态和执行就绪状态必须进入 `SkillStateService`，其权威协议见 [`agent_runtime_standard_services.md`](./agent_runtime_standard_services.md#23-skillstateservice)。Core 可以持久化业务状态 envelope，但不能写死某个 Skill 的字段含义。

## 4. 统一结果模型

`SkillExecutionResult` 建议字段：

- `trace_id`
- `status`：`completed/failed/denied`
- `output`
- `artifacts`
- `latency_ms`
- `protocol_used`：固定 `skill`
- `fallback_used`
- `plan_id`（Agent 闭环时必带）
- `nodes[]`（可选，非流式汇总返回）

## 5. 与现有模块映射

- Agent：`backend/internal/server/agent/*`
- Selector/Invocation：`backend/internal/service/capability_registry/*`
- Tenant API：`backend/internal/transport/http/openapi/capability_registry/*`
- PowerXPlugin Framework：`powerx-plugin/`（目标落点，提供 Skill Runtime 与 Client 封装）
- Plugin Runtime/Gateway：`backend/internal/infra/plugin/manager/*`
- A2A Team/Handoff：`backend/internal/service/agent/team_service.go`, `backend/internal/server/agent/manager_execute.go`, `backend/pkg/corex/db/persistence/model/agent/*`

## 5.1 落库权威（Skill vs Tooling）

1. Skill：`skills_registry_records` / `skills_capability_bindings` / `skills_execution_traces`。
2. Tooling：`capability_records`（目录）+ `invocation_traces`（调用追踪）。
3. Redis/ToolStore：仅缓存与策略快照，不作为唯一事实源。

## 6. 观测要求

每次执行必须上报：

- Metrics：`skill_invocations_total`, `skill_invocation_latency_ms`
- Trace 标签：`skill_id`, `skill_version`, `tenant_uuid`
- Audit 字段：`source`, `entrypoint`, `tool_grants`
- Plan 字段：`plan_id`, `node_id`, `node_kind`, `node_status`, `depends_on`, `retry_count`
- Query API：`GET /api/v1/admin/skills/traces`（支持 `plan_id/node_id/node_status` 过滤）

### 6.1 Agent Run Trace & Report 观测基线

SkillExecutionTrace 只覆盖 Skill 调用治理维度；Agent Run Trace 覆盖 Agent Runtime 的会话、消息、计划、节点与报告维度。两者必须通过 `trace_id/run_id/plan_id/node_id/skill_id` 关联，但不得互相替代。

Agent Runtime 每轮消息必须创建 `run_id`，并按以下粒度写结构化事件：

1. `Session`：`tenant_uuid/session_id/agent_id/channel`。
2. `Message`：`message_id/run_id/user_uuid/user_message_digest`。
3. `Plan`：`plan_id/candidate_count/node_count/failure_policy`。
4. `Node`：`node_id/node_kind/node_ref/phase/status/duration_ms`。
5. `Artifact`：prompt、上下文、tool payload、executor response 的脱敏引用。

本地开发写入：

```text
backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}/
```

生产环境写入 Loki，且必须使用低基数 label：

```text
service=powerx-agent
component=agent-runtime
tenant_uuid=...
agent_id=...
session_id=...
message_id=...
run_id=...
node_kind=...
status=...
```

Root-only 查询与下载入口：

```text
GET /api/v1/admin/agent-traces/messages/:message_id
GET /api/v1/admin/agent-traces/messages/:message_id/report
GET /api/v1/admin/agent-traces/sessions/:session_id/report
```

非 root 请求必须返回 `AGENT_TRACE_ROOT_REQUIRED`，禁止只依赖前端菜单隐藏。

### 6.2 A2A 多智能体观测基线

A2A handoff 节点必须作为 Agent Run Trace 的一等节点记录，不能只保存在普通 backend log 中。每个 `agent_handoff` 节点至少携带：

1. `team_id/team_name`
2. `parent_agent_id/parent_agent_key`
3. `child_agent_id/child_agent_key`
4. `handoff_task_id`
5. `failure_policy`
6. `context_ref_ids`
7. `child_run_id` 或 `handoff_trace_id`

主 Agent 最终回复必须能关联到所有子 Agent 的节点结果。root 下载 Message 报告时，应能看到“主 Agent 拆分了什么、每个子 Agent 收到什么、返回什么、失败策略如何生效”。

Core-only MVP 使用 `release.readiness.team` 作为 seed 演示团队，详见 [`multi_agent_a2a.md`](./multi_agent_a2a.md)。

## 7. 决策流程图（三层抉择）

```mermaid
flowchart TD
    U[User Message] --> I[Intent Layer]
    I --> C[candidate_skills top-k]
    C --> P[Planner Layer]
    P --> D{Build skill node?}
    D -->|No| N1[Other nodes]
    D -->|Yes| N2[node.kind=skill]
    N2 --> E[Executor Dispatch]
    E --> S[SkillRunner]

## 8. Planner 编排图（串并行）

```mermaid
flowchart TD
    U[User Message] --> I[Intent Top-K Skills]
    I --> P[Planner DAG]
    P --> S1[Stage 1: serial node]
    S1 --> G{Stage 2 parallel}
    G --> N21[node A]
    G --> N22[node B]
    N21 --> S3[Stage 3 merge node]
    N22 --> S3
    S3 --> F[Final Response]
```
```
