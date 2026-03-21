# Skills 运行时架构设计

本文定义 Skill 在 PowerX 的双路径运行时接入方案。

## 1. 总体架构

Skill 运行时支持两条路径：

1. 路径A：Agent 内 SkillRunner
2. 路径B：Capability Gateway 的 SkillAdapter（`preferred_protocol=skill`）

两条路径共享：

- SkillRegistry
- ToolingRegistry（capability registry，数据库权威源）
- SkillManifest 校验
- 审计与追踪模型
- 安全策略（tool_grants/safe_mode）

## 2. 路径A：Agent + Skill

### 2.1 触发条件

当 Agent Planner 识别到任务节点类型为 `skill` 时，进入 SkillRunner。

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

1. 召回：关键词/标签规则匹配。
2. 语义：向量检索候选 skills。
3. 重排：LLM 打分并输出 top-k；规则仅用于 `/command` 快捷命令，不参与普通自然语言主路由。

最终由 Planner 消解冲突并定案。

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

组合规划可追溯元信息（落地约束）：

1. 每个计划节点必须包含：`node_kind`、`node_ref`、`source_scope`。
2. `workflow` 节点可声明 `depends_on=[skill_node, tooling_node]`。
3. `node_start/node_end` 事件 payload 必须携带上述三字段，便于前端与审计复盘。

### 2.3 失败语义

1. 可重试错误：依赖临时不可用、网络抖动
2. 不可重试错误：鉴权失败、manifest 非法、签名不通过

## 3. 路径B：Capability + Skill

### 3.1 触发条件

租户调用：

- `POST /api/v1/tenant/invocations` 且 `preferred_protocol=skill`
- 或 `POST /api/v1/tenant/skills/invoke`

### 3.2 执行流程

1. Selector 解析 capability/policy
2. Router 选到 `transport=skill`
3. SkillAdapter 装配执行上下文
4. Runner 执行 Skill
5. 返回统一 envelope（trace/status/protocol/result）
6. 写 InvocationTrace 与审计事件

## 3.3 Agent 主入口（闭环入口）

建议将以下接口作为完整闭环主入口：

1. `POST /api/v1/agents/invoke`（非流式）
2. `GET /api/v1/agents/stream/sse`（流式）

约束：

- 调用方仅传 `message + agent_id(+session_id)`，不强制传 `skill_id`。
- 系统自动执行 `intent -> plan -> tool/skill nodes -> final`。
- tenant `/tenant/skills/invoke` 与 `/tenant/invocations` 保留为执行层接口，用于直接调用与治理复用。

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
