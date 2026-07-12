# Agent Run State Protocol 设计

本文定义 PowerX Agent Runtime、Agent Trace、Web Admin 与 PowerXPlugin 调试页面之间共享的运行状态协议。它解决的问题不是“Agent 怎么想”，而是“用户在对话框里如何看懂当前这轮消息发生了哪些任务、哪些 Agent 参与、哪些参数缺失、哪个节点失败、结果在哪里、trace 怎么定位”。

补充边界：`agent_run.*` 只负责 UI 可观察状态，业务任务状态权威由 [`agent_runtime_standard_services.md`](./agent_runtime_standard_services.md) 中的 `SkillStateService` 提供。Runtime 必须先持久化或读取 SkillState，再生成 `awaiting_params/running/completed/failed` 等可见状态。

## 1. 功能背景与目标

当前 Agent 对话和团队任务页面容易把复杂执行压成一段最终文本，导致用户只能看到“执行失败”或“已完成”的自然语言，而看不到任务节点、子 Agent、缺参、执行结果和 trace 入口。对于插件 Skill 调试，这还会造成“模型说成功，但业务没有执行”的假成功。

目标：

1. 建立 `agent_run.*` 标准事件，覆盖一轮 Message Run 的实时状态。
2. 建立 Task State Model，统一表示多任务、多 Agent、Skill/Capability 执行、缺参等待和失败状态。
3. 建立 UI State Reducer，让 PowerX Web Admin 与 PowerXPlugin 调试页渲染同一套运行状态。
4. 建立历史快照，保证刷新页面或从 Trace 页面进入时仍能恢复 session/message/task 状态。
5. 明确与 Google A2A 的关系：A2A 可用于 Agent 间委派，Agent Run State Protocol 是 PowerX 内部 UI/Runtime/Trace 状态协议，二者不等同。

## 2. 与 Google A2A 的关系

Google A2A 关注 Agent 与 Agent 之间的任务互联、状态和 artifact 交换。PowerX `Agent Run State Protocol` 关注的是一轮对话在 PowerX 内部如何被观察和复盘。

关系：

1. A2A `task/status/artifact` 可以映射到 PowerX `task_status/result/links`。
2. PowerX A2A `agent_handoff` 节点必须产出 `agent_run.task_status`，让 UI 看见主 Agent 与子 Agent 的关系。
3. 插件 Skill、Capability Invocation、普通 LLM 节点也必须使用同一套 `agent_run.*` 状态事件。
4. PowerX 不把该协议暴露为跨平台 A2A 标准；它是 PowerX Runtime 与 UI 的权威协议。

## 3. 协议分层

```text
User Message
  -> Agent Runtime
  -> agent_run.* realtime events
  -> AgentRunState reducer
  -> Chat / Team Task / Trace UI
  -> run_state snapshot + Agent Trace artifact
```

分层职责：

| 层 | 职责 | 权威方 |
| --- | --- | --- |
| Runtime Event Protocol | 定义 `agent_run.*` 事件名称、payload 和顺序 | PowerX Core |
| Task State Model | 定义 task、agent、skill、capability、缺参、结果、错误和链接字段 | PowerX Core |
| Skill Manifest Extension | 定义 action 参数、slot 映射、结果展示元数据 | Skill 源定义方，PowerX 校验 |
| UI State Reducer | 将 SSE/WS/history 事件聚合为可渲染状态树 | Framework / Web Admin |
| Trace/History Snapshot | 保存一轮消息的最终状态，支持页面刷新、报告下载和精确定位 | PowerX Core |

## 4. 标准事件

事件统一使用 `agent_run.*` 前缀。`agent_run.*` 是 PowerX Agent Runtime 对外唯一运行状态合同；对话 UI、Trace UI、PowerXPlugin 调试 UI 不得消费 `intent/plan/node_start/node_end/final/end` 等旧事件作为运行状态来源。Runtime 内部可以使用任意实现事件，但进入 SSE/WS/history snapshot 前必须转换为标准事件。

| Event | 语义 | 典型时机 |
| --- | --- | --- |
| `agent_run.started` | 一轮 Message Run 开始 | 收到用户消息并建立 run_id |
| `agent_run.response_plan` | 本轮 response mode、是否执行、目标能力 | ResponsePlanner 完成 |
| `agent_run.intent_detected` | 结构化意图与候选摘要 | Intent 阶段完成 |
| `agent_run.plan_created` | 任务计划创建，含串并行依赖 | Planner 完成 |
| `agent_run.task_status` | 任一任务状态变化 | pending/awaiting/running/completed/failed/skipped |
| `agent_run.task_started` | 任务开始执行 | Executor 开始 |
| `agent_run.awaiting_params` | 缺必要参数，等待用户补充 | 参数校验未通过且可追问 |
| `agent_run.task_completed` | 任务完成并产生结果 | Skill/Tool/A2A 节点成功 |
| `agent_run.task_failed` | 任务失败 | fail-fast 或执行异常 |
| `agent_run.final` | 最终回复内容与可见结果摘要 | Final Response 完成 |
| `agent_run.ended` | 一轮 Message Run 结束 | history/trace 持久化完成 |

标准状态枚举：

```text
pending | awaiting_params | running | completed | failed | skipped
```

## 4.1 Run Completion 与 Task Completion 边界

`agent_run.ended` 和旧事件 `end success=true` 只表示一轮 Message Run 已经结束，不能表示业务任务完成。UI、Trace 和历史快照必须区分以下两类状态：

| 状态类型 | 权威信号 | 含义 | UI 展示 |
| --- | --- | --- | --- |
| Run 完成 | `agent_run.ended` | 本轮对话流程结束，assistant 回复已生成并持久化 | 可显示“已回复”或收起运行摘要 |
| Task 等待参数 | `agent_run.awaiting_params` 或 task `status=awaiting_params` | Skill 参数不完整，等待用户补充 | 显示缺参卡片 |
| Task 执行中 | `agent_run.task_started` 或 task `status=running` | Skill/Tool/A2A 节点正在执行 | 显示执行中、进度、参与 Agent |
| Task 完成 | `agent_run.task_completed` 或 task `status=completed` 且包含真实 `result` 或 `links` | 业务任务真实执行成功 | 显示任务完成和结果 |
| Task 失败 | `agent_run.task_failed` 或 task `status=failed` 且包含 `error` | 业务任务真实执行失败 | 显示失败、错误摘要、trace 入口 |
| 普通回复 | 没有 task 事件，只有 `agent_run.final/ended` | 本轮只生成自然语言回复，没有执行业务任务 | 不显示任务完成卡 |

硬性规则：

1. `agent_run.final` 和 `agent_run.ended` 不得驱动 task 进入 `completed`。
2. `response_plan.should_call_tool=false` 且没有 task 事件时，UI 不得显示“任务完成”。
3. 只有 `agent_run.task_completed` 或 task snapshot 中的 `status=completed` 可以驱动业务任务完成态。
4. `completed` 必须有真实执行结果来源：Skill result、Capability result、A2A child result 或等价结构化 task result。
5. 如果没有真实执行结果，但 final response 文案包含“已创建、已更新、已删除、已发布、已同步、已完成”等成功性业务结论，Runtime 必须拦截或改写，UI 也不得据此生成成功任务卡。
6. 旧事件 `intent/plan/node_start/node_end/final/end` 只能作为兼容期内部输入，进入 UI 前必须转换为 `agent_run.*`；其中 `final/end` 只能更新 run 级状态，不能更新 task 级状态。

## 4.2 Run Summary 与 Task Graph

`agent_run.plan_created` 必须携带本轮总任务图，作为多任务、多 Agent 串并行展示的权威来源。

```json
{
  "run_id": "run_1782286000000000000",
  "session_id": "session_run_1782285000000000000",
  "message_id": "msg_1782286000000000000",
  "trace_id": "9d38f3fd-7c20-4c3e-8f93-1a0a2d99b000",
  "event": "plan",
  "payload": {
    "planner_mode": "unified",
    "plan": {
      "plan_id": "release_readiness_multi_agent_mvp",
      "tasks": []
    }
  },
  "summary": {
    "status": "pending",
    "total_tasks": 4,
    "pending_tasks": 4,
    "running_tasks": 0,
    "completed_tasks": 0,
    "failed_tasks": 0,
    "current_stage": 0,
    "total_stages": 3
  },
  "tasks": [
    {
      "task_id": "knowledge_analysis",
      "stage": 1,
      "parallel_group": "stage_1",
      "depends_on": [],
      "node_kind": "agent_handoff",
      "node_ref": "release.knowledge_analyst",
      "agent_key": "release.knowledge_analyst",
      "team_id": "release.readiness.team",
      "status": "pending",
      "failure_policy": "continue"
    },
    {
      "task_id": "workflow_planning",
      "stage": 2,
      "parallel_group": "stage_2",
      "depends_on": ["knowledge_analysis"],
      "node_kind": "agent_handoff",
      "node_ref": "release.workflow_planner",
      "status": "pending"
    }
  ]
}
```

字段规则：

1. `summary.total_tasks` 必须等于本轮计划任务数。
2. `tasks[].stage` 表示调度阶段；同一 `stage` 可并行执行。
3. `tasks[].parallel_group` 表示并行组；同一组内任务允许同时 running。
4. `tasks[].depends_on` 表示显式依赖；依赖未完成时不得执行当前任务。
5. `tasks[].parent_task_id` 表示层级关系；多 Agent handoff 子任务必须能回溯到父任务。
6. UI 必须先展示 Run Summary，再展示按 stage/parallel_group 分组的 Task Graph。

## 5. Task State Payload

`agent_run.task_status` 是 UI 展示的核心合同。

```json
{
  "run_id": "run_1782286000000000000",
  "session_id": "session_run_1782285000000000000",
  "message_id": "msg_1782286000000000000",
  "trace_id": "9d38f3fd-7c20-4c3e-8f93-1a0a2d99b000",
  "task_id": "tc1",
  "parent_task_id": null,
  "depends_on": ["knowledge_analysis"],
  "stage": 2,
  "parallel_group": "stage_2",
  "team_id": "release.readiness.team",
  "agent_id": "18",
  "agent_key": "powerxplugin.template_object.agent",
  "agent_name": "模板智能体",
  "node_kind": "skill",
  "skill_id": "powerxplugin.template.basic.local",
  "capability_id": "com.powerx.plugins.base.local.template.create",
  "action": "create",
  "failure_policy": "fail-fast",
  "status": "awaiting_params",
  "collected_params": {
    "template.title": "测试模板"
  },
  "missing_fields": [
    "template.description",
    "template.content"
  ],
  "result": null,
  "links": [],
  "error": null,
  "updated_at": "2026-06-24T15:20:00+08:00"
}
```

字段规则：

1. `run_id/session_id/message_id/trace_id/task_id/status` 必填。
2. `stage/parallel_group/depends_on` 由 Planner 负责生成；Executor 状态事件必须原样透传。
3. `agent_id/agent_key/agent_name` 至少提供一个可读名称，UI 不得以数据库 ID 作为主要展示。
4. `node_kind=agent_handoff` 时必须包含 `team_id/parent_task_id/child_run_id` 或等价 trace 关联。
5. `node_kind=skill` 时必须包含 `skill_id`；命中插件来源 Skill 且执行业务时必须包含 `capability_id/action`。
6. `awaiting_params` 必须携带 `missing_fields` 与已收集参数摘要。
7. `completed` 必须携带可见 `result` 或 `links`；没有真实执行结果时不得生成成功状态。
8. `failed` 必须携带稳定 `error.code/error.message`，不得只返回 stack 或空 body。

## 6. Skill Manifest 扩展

Core 不写插件业务规则。缺参、结果链接和业务展示要求来自 Agent persona/prompt_seed 与 Skill manifest。

Skill manifest 推荐扩展：

```yaml
action_required_args:
  create:
    - template.title
    - template.description
    - template.content
  update:
    - template_id

action_optional_args:
  list:
    - q
    - page
    - page_size

slot_mapping:
  template.title:
    labels: ["标题", "名称", "模板标题"]
  template.description:
    labels: ["描述", "用途", "说明"]
  template.content:
    labels: ["内容", "正文", "模板内容"]

pending_task_policy:
  enabled: true
  merge_window_messages: 6
  merge_window_seconds: 900
  confirm_before_execute: false

result_presentation:
  create:
    title: "模板已创建"
    primary_link: "template.detail_path"
    visible_fields:
      - template.id
      - template.title
      - template.detail_path
```

约束：

1. `action_required_args` 是参数完整性权威，LLM 不能绕过必填字段直接执行。
2. `slot_mapping` 只帮助自然语言抽取和 UI 展示，不改变业务 schema。
3. `pending_task_policy` 只决定是否允许跨轮补参；不允许时必须 fail-fast。
4. `result_presentation` 只定义结果如何展示，不允许伪造业务结果。

这些字段必须同步进入 SkillStateService 的状态推进协议：

1. `action_required_args` 决定 `missing_fields`。
2. `slot_mapping` 决定自然语言抽取与用户可读字段标签。
3. `pending_task_policy` 决定是否允许跨轮合并，以及合并窗口。
4. `result_presentation` 决定 `completed` 状态下可见 `result/links`。

Core 只执行通用校验和状态持久化；具体字段如 `template.title`、`order.address`、`video.urls` 的含义必须由 Skill manifest 定义。

## 7. UI State Reducer

PowerX Web Admin 与 PowerXPlugin Framework Client 必须使用同一套 reducer 语义：

```text
AgentRunState
  run
  summary
  session
  message
  response_plan
  tasks[]
  task_graph
  stages[]
  agents[]
  pending_params[]
  results[]
  errors[]
  trace_links[]
```

标准 UI 组件：

| 组件 | 用途 |
| --- | --- |
| `AgentRunSummary` | 展示总状态、总任务数、完成数、失败数、当前阻塞原因 |
| `AgentRunTimeline` | 展示本轮 Message Run 的阶段与任务流 |
| `AgentTaskGraph` | 按 `stage/parallel_group/depends_on` 展示串并行编排 |
| `AgentLaneView` | 按 Agent 泳道展示主 Agent 与子 Agent 的任务状态 |
| `AgentTaskCard` | 展示单个 task 的 Agent、Skill、状态、耗时 |
| `AgentPendingParamsCard` | 展示缺参字段和已收集字段，支持用户自然语言补充 |
| `AgentTaskResultCard` | 展示真实执行结果、业务对象摘要和链接 |
| `AgentTraceButton` | 跳转到 `tenant_uuid/session_id/message_id/run_id/task_id` 精确 trace |

页面对齐：

1. PowerX Agent Chat：每条 assistant 消息下方展示本轮 `AgentRunTimeline` 摘要。
2. PowerX `/agent/team-tasks`：显示多 Agent / 多 task 状态、stage、依赖和并行组，不只展示最终文本。
3. PowerX `/agent/traces`：按 session -> message -> task/node 聚合，支持下载报告。
4. PowerXPlugin Agent Chat 调试：消费同一 `AgentRunState`，不得自定义一套插件私有状态协议。

## 8. 存储与历史恢复

实时链路：

```text
Agent Runtime -> SSE/WS agent_run.* -> UI reducer
```

历史链路：

```text
Agent Runtime -> message meta / run state snapshot / Agent Trace artifact
```

存储要求：

1. SSE/WS 只负责实时，不是历史权威。
2. PostgreSQL 保存 session/message/message meta 与可查询 run state snapshot。
3. Local File/Loki 保存完整 Agent Trace 与报告 artifact。
4. Redis 只允许做短 TTL 状态缓存，不得作为运行历史权威源。
5. 页面刷新后必须能从历史 API 恢复 `AgentRunState`，不能要求用户重新执行。

## 9. 执行与缺参闭环

缺参闭环：

```text
User asks create template
  -> planner selects skill/action=create
  -> param extractor collects template.title
  -> required args check finds missing description/content
  -> agent_run.awaiting_params
  -> UI renders PendingParamsCard
  -> user provides description/content
  -> Runtime merges slots under pending_task_policy
  -> task_status running
  -> capability invocation
  -> task_completed with result links
  -> final response
```

禁止行为：

1. 缺少必填参数时直接调用业务 capability。
2. 没有 `task_completed` 或真实 result 时，Final Response 声称“已创建/已更新/已删除/已完成”。
3. 执行失败后只返回空消息或把错误吞掉。
4. UI 只显示 message id，不展示用户消息前缀、任务状态和 trace 入口。

## 10. 验收标准

1. 用户发起创建类任务但缺参数时，两个 UI 都展示 `awaiting_params` 卡片，并列出业务语言字段。
2. 用户补充参数后，同一 task 从 `awaiting_params -> running -> completed`，并展示结果链接。
3. 多 Agent 任务中，UI 可看到主 Agent、三个子 Agent、每个子任务状态和失败策略。
4. 任一 task 失败时，Chat 消息和 Trace 页面都能精确定位到 `message_id/task_id/node_id`。
5. 没有真实 Skill/Capability/A2A 执行结果时，Final Response 不允许输出成功文案。
6. PowerXPlugin 调试页与 PowerX Web Admin 对同一 SSE/history payload 渲染一致。
7. 多任务计划中，同一 stage 的并行任务在 UI 中显示为同一并行组；后续 stage 必须显示依赖关系。
8. 多 Agent handoff 中，用户能看到主 Agent、子 Agent、父子任务关系、失败策略和最终汇总状态。

## 11. 代码映射

计划落点：

| 范围 | 文件/模块 |
| --- | --- |
| DTO/Event | `backend/internal/server/agent/runtime/*`, `backend/pkg/dto/*` |
| Runtime Emit | `backend/internal/server/agent/runtime/engine.go`, `backend/internal/server/agent/manager_execute.go` |
| Trace Snapshot | `backend/internal/service/agent_trace/*` |
| History API | `backend/internal/transport/http/admin/agenttrace/*`, `backend/internal/transport/http/admin/agent/*` |
| Web Admin UI | `web-admin/app/components/agent/*`, `web-admin/app/pages/agent/traces/`, `web-admin/app/pages/agent/team-tasks*` |
| Plugin Framework | `PowerXPlugin/framework/backend/go/runtime/powerx/agent/*`, `PowerXPlugin/skeleton/web-admin/*` |

## 12. 变更记录

| 日期 | 变更 |
| --- | --- |
| 2026-06-24 | 新增 Agent Run State Protocol 设计，定义实时事件、任务状态、缺参闭环、UI reducer 与 Core/Plugin 页面统一要求。 |
