# Agent Run Trace & Report 机制

本文定义 PowerX Agent Runtime 的结构化日志、节点追踪、Root 调试台与智能对话报告下载机制。

该机制不是普通 backend log，也不是仅供 Loki 检索的文本日志；它是 Agent Runtime 的可回放执行轨迹，用于回答一个核心问题：

> 用户在某个 Agent Session 中发送一轮消息后，PowerX 到底做了什么？

## 1. 机制定位

**PowerX Agent Run Trace & Report** 是 Agent Session、Agent Runtime、Planner、Skill/Tool Executor 之间的结构化可观测机制。

它覆盖三个粒度：

1. `Session Trace`：多轮会话级别，按 `tenant_uuid + session_id` 聚合。
2. `Message Trace`：一轮用户输入级别，按 `message_id/run_id` 聚合。
3. `Node Trace`：运行时节点级别，按 `plan_id + node_id` 聚合。

它服务于：

1. 本地开发调试：落盘到 `backend/logs/agents/`。
2. 生产排障：写入 Loki，按 label 查询。
3. Root 调试页面：查看 Session/Message/Node 的执行链路。
4. 报告下载：生成智能对话报告，支持研发、运维、安全审计复盘。

## 2. 设计原则

1. **结构化优先**：所有 Agent Trace 事件必须是 JSON/JSONL 结构化记录，不以纯文本日志作为主数据源。
2. **Root-only**：查询、查看、下载 Agent Trace/Report 必须后端强制 root 权限，前端隐藏菜单不构成安全边界。
3. **Fail-fast**：缺少 `tenant_uuid/session_id/message_id/run_id/trace_id` 等关键字段时，Agent Trace Logger 必须返回明确错误，不做匿名 fallback。
4. **双 Sink 一致**：Local File 与 Loki 使用同一事件模型，只是存储介质不同。
5. **节点可回放**：每个关键 Runtime 节点必须有 start/end/error 事件，可按时间线复原执行路径。
6. **敏感数据可控**：prompt、上下文、tool payload、executor response 必须支持摘要、脱敏和 artifact 分级保存。
7. **不替代业务审计**：Agent Trace 记录执行细节；Skill/Capability/Audit 仍保留治理审计职责，两者通过 `trace_id/run_id/node_id` 关联。

## 3. 本地目录规范

本地开发模式下，默认写入：

```text
backend/logs/agents/
  {tenant_uuid}/
    {session_id}/
      {message_id}/
        run.json
        timeline.jsonl
        nodes/
          001_receive_message.json
          002_context_load.json
          003_intent_recognition.json
          004_planner.json
          005_skill_selection.json
          006_skill_invoke.json
          007_final_response.json
        artifacts/
          prompt.txt
          response.txt
          tool_payload.json
          executor_result.json
        report.md
        report.json
```

文件职责：

1. `run.json`：本轮 Message Run 总览。
2. `timeline.jsonl`：按时间追加的事件流。
3. `nodes/*.json`：节点输入、输出、耗时、错误和摘要。
4. `artifacts/*`：可选明细附件，必须受脱敏策略控制。
5. `report.md/report.json`：下载报告的数据源与人读版输出。

## 4. 标准事件模型

每条 Agent Trace Event 至少包含：

```json
{
  "trace_id": "trace_xxx",
  "run_id": "run_xxx",
  "tenant_uuid": "tenant_xxx",
  "user_uuid": "user_xxx",
  "agent_id": "agent_xxx",
  "session_id": "session_xxx",
  "message_id": "message_xxx",
  "plan_id": "plan_xxx",
  "node_id": "004_skill_selection",
  "node_kind": "skill_selection",
  "node_ref": "mediax.video_rebuilder.cn",
  "phase": "end",
  "status": "success",
  "duration_ms": 128,
  "input_digest": "sha256:...",
  "output_digest": "sha256:...",
  "error_code": "",
  "error_summary": "",
  "created_at": "2026-06-08T12:00:00Z"
}
```

允许扩展字段：

1. `channel`：web、telegram、discord、wechat、scrm、plugin_local_chat。
2. `context_ref`：上下文快照引用。
3. `skill_id/plugin_id/capability_id/executor_path`。
4. `prompt_tokens/completion_tokens/cached_tokens`。
5. `context_layers_size/trim_actions/cache_hit`。
6. `artifact_refs[]`。

## 5. Runtime 节点规范

Agent Runtime 至少需要覆盖以下节点：

1. `receive_message`：收到用户消息，记录 channel、message_id、session_id。
2. `session_restore`：恢复或创建 Agent Session。
3. `permission_check`：校验用户、租户、Agent、Skill/Tool 权限。
4. `context_load`：加载会话摘要、知识片段、能力目录、上下文预算。
5. `intent_recognition`：意图识别与候选能力召回。
6. `planner`：生成 DAG/阶段计划。
7. `node_dispatch`：调度 workflow/skill/tooling/llm/agent_handoff 节点。
8. `skill_selection`：确定命中的 Skill 与原因。
9. `skill_invoke`：调用 PowerX Skill 或插件 Skill Executor。
10. `tool_invoke`：调用 Capability/Tooling。
11. `llm_call`：调用模型并记录 token 与 cache 指标。
12. `final_response`：生成最终回复。
13. `history_persist`：会话消息与摘要持久化。

每个节点必须具备：

1. `start` 事件。
2. `end` 或 `error` 事件。
3. `duration_ms`。
4. 输入输出摘要或 artifact 引用。
5. 可稳定排序的 `node_seq`。

## 6. Logger 封装

后端新增独立封装，不允许在 Runtime 中散落调用通用 logger 代替结构化 trace。

```go
type AgentTraceLogger interface {
    StartRun(ctx context.Context, meta AgentRunMeta) (*AgentRunContext, error)
    AppendEvent(ctx context.Context, event AgentTraceEvent) error
    StartNode(ctx context.Context, node AgentTraceNode) error
    EndNode(ctx context.Context, result AgentTraceNodeResult) error
    FailNode(ctx context.Context, failure AgentTraceNodeFailure) error
    CompleteRun(ctx context.Context, result AgentRunResult) error
    BuildReport(ctx context.Context, query AgentReportQuery) (*AgentRunReport, error)
}
```

Sink 设计：

```text
AgentTraceLogger
  -> LocalAgentTraceSink
  -> LokiAgentTraceSink
  -> CompositeAgentTraceSink
```

配置建议：

```yaml
agent_trace:
  enabled: true
  local_enabled: true
  local_dir: backend/logs/agents
  loki_enabled: false
  artifact_policy: redacted
  max_artifact_bytes: 1048576
```

## 7. Loki Label 规范

生产环境写 Loki 时，label 必须低基数优先：

```text
service=powerx-agent
component=agent-runtime
tenant_uuid=...
agent_id=...
session_id=...
message_id=...
run_id=...
node_kind=skill_invoke
status=success|error
```

高基数字段如完整 prompt、payload、response、error stack 不得作为 label，只能作为日志 body 或 artifact 引用。

## 8. Root 调试页面

建议新增 Root-only 页面：

```text
/agent/traces
/agent/traces/sessions/:session_id
/agent/traces/messages/:message_id
```

页面参考 AI Craft 报告视图，布局建议：

1. 顶部：trace/session/message 信息、日志源 Local/Loki、搜索框、刷新、下载。
2. 指标卡片：日志行、节点、请求、回复、Skill 调用、事件、错误、警告、总耗时。
3. 左侧：Message 聚合列表。
4. 中间：节点链路 timeline。
5. 右侧：节点快照、Slot/Context 状态、Skill 输入输出、错误详情。
6. 操作：复制 Message、下载 Message 报告、下载 Session 报告。

权限要求：

1. 菜单仅 root 可见。
2. API 必须 root-only。
3. 非 root 请求返回稳定错误码 `AGENT_TRACE_ROOT_REQUIRED`。

## 9. 管理 API

建议接口：

```text
GET /api/v1/admin/agent-traces/sessions
GET /api/v1/admin/agent-traces/sessions/:session_id
GET /api/v1/admin/agent-traces/messages/:message_id
GET /api/v1/admin/agent-traces/messages/:message_id/timeline
GET /api/v1/admin/agent-traces/messages/:message_id/nodes/:node_id
GET /api/v1/admin/agent-traces/messages/:message_id/report
GET /api/v1/admin/agent-traces/sessions/:session_id/report
```

查询参数：

1. `tenant_uuid`
2. `agent_id`
3. `session_id`
4. `message_id`
5. `run_id`
6. `trace_id`
7. `source=local|loki`
8. `from/to`
9. `node_kind`
10. `status`

## 10. 报告格式

下载格式：

1. `report.json`
2. `report.md`
3. `report.zip`

`report.zip` 内容：

```text
summary.json
timeline.jsonl
nodes/*.json
artifacts/*
report.md
```

`report.md` 模板：

```md
# Agent Message Report

## Summary

## User Message

## Runtime Timeline

## Intent Recognition

## Planner

## Skill / Tool Invocation

## Final Response

## Errors / Warnings

## Appendix
```

## 11. 与 Agent Skill Bridge 的关系

Agent Skill Bridge 负责 PowerX Agent Runtime 与插件 Skill Executor 的调用边界。

Agent Run Trace & Report 负责记录这条链路中发生了什么：

1. 哪个渠道进入 PowerX Agent Session。
2. 哪个 Agent 收到消息。
3. Intent/Planner 为什么选择某个插件 Skill。
4. PowerX 调用了哪个插件 executor。
5. 插件返回了什么结构化结果。
6. 最终会话回复如何生成。

插件本地 Chat、Web Chat、Telegram/SCRM 等入口必须共享相同的 trace 字段。

## 12. MVP 范围

第一阶段建议交付：

1. `AgentTraceLogger` 标准接口与 DTO。
2. `LocalAgentTraceSink` 写入 `backend/logs/agents`。
3. Agent Runtime 关键节点接入 start/end/error。
4. Root-only Message Trace 查询 API。
5. Root-only Message Report 下载 API。
6. Web Admin Root 页面展示一轮 Message timeline 和节点详情。

第二阶段：

1. Loki Sink。
2. Session 级报告。
3. ZIP 下载。
4. Prompt/Tool Payload 脱敏策略。
5. 节点耗时图和异常聚合。
6. 与 Skill/Capability/A2A 审计页面互跳。
