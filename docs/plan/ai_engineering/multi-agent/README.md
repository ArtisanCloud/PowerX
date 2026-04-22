# Multi-Agent 工程化计划（PowerX）

## 1. 目标与边界

### 1.1 目标
- 建立“主控 Agent + 多执行 Agent”的可治理架构，支持一次用户消息触发多任务协作。
- 在保持现有单入口体验的前提下，实现任务拆解、并发执行、结果汇总与可观测审计。
- 支持租户级隔离、权限约束、可回放与失败恢复。

### 1.2 非目标（当前阶段）
- 不引入完全自治的自由对话型 agent swarm（避免失控与成本不可控）。
- 不在首期实现跨租户团队共享。

## 2. 现状基线（已存在能力）

### 2.1 调度主干
- 已有“主控规划 + 多任务执行”链路：
  - 意图与工具规划：`backend/internal/server/agent/manager_tool_calling.go`
  - 计划编排：`backend/internal/server/agent/manager_plan.go`
  - 分 stage 并发执行：`backend/internal/server/agent/manager_execute.go`

### 2.2 Context 共享（当前）
- 执行内共享：`ResultStore + _deps + param_refs`（任务级上下文共享）。
- 会话级沉淀：`agent_chat_sessions` + 自动摘要（`SummarizeIfNeeded`）。
- 知识快照：`/openapi/knowledge-spaces/qa/memory-snapshot` + `context_snapshot.Store`。

### 2.3 缺口
- 无“Team/团队”实体与成员治理模型。
- 无“主控与子 agent 角色策略”标准化配置。
- 缺统一 context contract（会话摘要、知识快照、执行共享三层尚未统一接口）。

## 3. 目标架构

### 3.1 角色分层
- Main Agent（Orchestrator）
  - 负责：任务拆解、候选 agent 选择、执行计划生成、最终合成答复。
- Worker Agent（Specialist）
  - 负责：领域能力执行（写作、检索、校对、风格化、事实核验等）。
- Guard/Policy 层
  - 负责：权限、配额、工具白名单、租户隔离、审计。

### 3.2 Team 模型（新增）
- `agent_team`
  - `id/tenant_uuid/name/status/default_main_agent_id/context_policy_id/scheduler_policy_id`
- `agent_team_member`
  - `team_id/agent_id/role(weight, priority, concurrency_limit, enabled)`
- `agent_team_policy`
  - 可调用工具白名单、敏感能力黑名单、预算上限、超时策略。

### 3.3 Context 三层模型
- L1 会话层（Session Memory）
  - 最近消息窗口 + 会话摘要（长期记忆）。
- L2 执行层（Execution Shared State）
  - `plan_id` 范围内任务输出、引用关系（`param_refs`）。
- L3 知识层（Knowledge Snapshot）
  - 来自知识空间的 citation snapshot / retrieval plan。

统一 contract：
- `context_packet`（传给每个 worker）
  - `session_summary/recent_turns/shared_state_refs/knowledge_snapshot_ids/tool_grants/constraints`

## 4. 调度与执行策略

### 4.1 一次消息处理流程
1. Main Agent 接收用户消息。
2. Planner 产出任务列表（task graph）。
3. Scheduler 按 team policy 选择 worker（可并发）。
4. Worker 回传结构化结果。
5. Main Agent 聚合、冲突消解、最终回复。
6. 写入审计与会话摘要更新。

### 4.2 并发与失败策略
- 同 stage 并发，跨 stage 依赖屏障。
- 支持 `fail_fast` 与 `best_effort` 两种策略。
- 单任务失败必须记录：`task_id/agent_id/error/type/retry_count`。
- 支持自动重试（指数退避）与降级 fallback agent。

## 5. 接口计划

### 5.1 Team 管理 API（Admin）
- `POST /api/v1/admin/agent/teams`
- `GET /api/v1/admin/agent/teams`
- `POST /api/v1/admin/agent/teams/:id/members`
- `POST /api/v1/admin/agent/teams/:id/policies`

### 5.2 运行时 API
- `POST /api/v1/agents/teams/:id/invoke`
- `GET /api/v1/agents/teams/:id/executions/:execution_id`
- `POST /api/v1/agents/teams/:id/executions/:execution_id/cancel`

### 5.3 Context API
- `POST /api/v1/agents/context/snapshot`（执行后落快照）
- `GET /api/v1/agents/context/snapshot?session_id=...`

## 6. 数据与审计

### 6.1 审计字段（必备）
- `tenant_uuid/team_id/main_agent_id/worker_agent_id/execution_id/task_id`
- `trace_id/request_id`
- `input_hash/output_hash`
- `latency_ms/token_usage/cost_estimate`
- `status/retry/fallback`

### 6.2 幂等键
- `tenant + team + execution_intent_hash + client_request_id`

## 7. 里程碑

### M1（1-2 周）
- 固化现有主控调度链路的 contract（不改交互入口）。
- 输出统一 `context_packet` 并接入 worker 入参。
- 增加 execution 级审计明细。

### M2（2-3 周）
- 上线 Team 数据模型与 Admin API。
- 支持 team 级策略：可调用 agent 白名单、并发上限、超时策略。
- 运行时支持指定 team invoke。

### M3（2 周）
- 引入故障恢复与回放（execution replay）。
- 增加质量评分回传（worker output quality score）用于后续路由优化。

## 8. 验收标准
- 单条用户请求可稳定触发 2+ worker 并发执行。
- 任一 worker 失败时，主控可返回可解释结果（非空白失败）。
- 全链路可按 `trace_id` 回溯：主控决策、任务分发、子任务结果、最终合成。
- 租户隔离验证通过：跨租户 team/agent 无法调用。

## 9. 风险与应对
- 风险：任务图过深导致延迟不可控。
  - 应对：限制最大 stage 深度与最大任务数。
- 风险：主控 prompt 膨胀。
  - 应对：context_packet 压缩与摘要刷新策略。
- 风险：工具权限越权。
  - 应对：team policy + tool grants 双重校验。

## 10. 实施建议（从你当前代码出发）
- 先不推翻现有 `BuildPlan/ExecutePlan`，在其上加 Team 约束层。
- 先把“调度治理”做出来（team + policy + audit），再做“更聪明的 planner”。
- 优先保证可观测与回滚，再扩展模型复杂度。

## 11. Web Admin 交互落地（本轮已实现）

### 11.1 菜单与路由
- 智能体管理：`/settings/ai/agents`
- 团队管理：`/settings/ai/agent-teams`
- 智能会话：`/agent/sessions`
- 团队任务：`/agent/team-tasks`
- 统一工作台：`/agent`
  - 单智能体模式：`/agent?workspace=smart`
  - 团队任务模式：`/agent?workspace=team&team_id=<id>`

### 11.2 页面职责
- 智能体管理
  - 展示 agent 列表。
  - 支持快速新建、重命名。
  - 快速跳转到智能会话或团队管理。
- 团队管理
  - 配置主 Agent 的团队策略（dispatch/failure）。
  - 维护团队状态（active/disabled）。
- 智能会话
  - 单智能体会话模式入口页。
  - 引导进入 `/agent?workspace=smart`。
- 团队任务
  - 输入 `parent_agent_id` 加载团队列表。
  - 选择 `team_id` 后进入 `/agent?workspace=team&team_id=...`。

### 11.3 统一工作台模式切换策略
- 当前采用“同一工作台 + Query 参数”的轻量方案，不拆分两套大页面。
- 页面顶部显式展示当前模式：
  - `智能会话` 或 `团队任务` Badge。
  - 团队模式时显示 `team_id`，便于调试与审计对齐。
- 模式切换通过顶部按钮完成，避免用户迷路。

### 11.4 最小验证用例（UI）
1. 打开侧边栏 Agent 菜单，确认出现 4 个二级入口：
   - 智能体管理
   - 团队管理
   - 智能会话
   - 团队任务
2. 在“团队管理”创建一个团队，记下 `team_id`。
3. 进入“团队任务”，输入同一个 `parent_agent_id` 并加载团队。
4. 选择团队并点击“打开任务工作台”。
5. 在 `/agent` 顶部确认：
   - 模式为“团队任务”
   - 可见 `team_id` 标识
6. 切换到“智能会话”，确认工作台标题与模式 Badge 切回单智能体。
