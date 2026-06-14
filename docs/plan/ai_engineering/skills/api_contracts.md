# Skills API 契约（Admin / Tenant / Plugin）

本文定义 Skill 管理与调用接口契约，供后端、前端、插件侧统一实现。

插件与 PowerX Agent Runtime 的桥接契约遵循 [`agent_skill_bridge.md`](./agent_skill_bridge.md)。本文件补充 HTTP/SSE/WS 层面的请求与响应样例。

## 0. LLM 基础接口（system + user 双消息位）

### 0.1 非流式

`POST /api/v1/ai/llm/invoke`

```json
{
  "model_key": "openai/gpt-4o-mini",
  "inputs": [
    {"role": "system", "type": "text", "text": "你是资深中文编辑，只输出正文"},
    {"role": "user", "type": "text", "text": "把这段文案改得更有观点"}
  ],
  "params": {
    "temperature": 0.6,
    "max_tokens": 512
  }
}
```

### 0.2 流式

`POST /api/v1/ai/llm/stream`

```json
{
  "model_key": "openai/gpt-4o-mini",
  "inputs": [
    {"role": "system", "type": "text", "text": "你是资深中文编辑，只输出正文"},
    {"role": "user", "type": "text", "text": "把这段文案改得更有观点"}
  ],
  "params": {
    "temperature": 0.6,
    "thinking": true,
    "reasoning": {
      "enabled": true,
      "effort": "medium",
      "expose": "summary"
    }
  },
  "stream_options": {
    "include_usage": true
  }
}
```

说明：

1. `inputs[].role` 推荐使用 `system | user | assistant`。
2. 当前服务端 LLM 会把 `role=system` 聚合为 `system prompt`，其余聚合为 `user prompt`。

### 0.3 多模态接口 Usage 计量增强（开发文档草案，vNext）

适用接口（统一口径）：

1. `POST /api/v1/ai/llm/invoke`
2. `POST /api/v1/ai/llm/stream`
3. `POST /api/v1/ai/image/invoke`
4. `POST /api/v1/ai/video/invoke`
5. `POST /api/v1/ai/tts/invoke`
6. `POST /api/v1/ai/embedding/invoke`

目标：

1. 在 PowerX 底座统一返回 token 计量，避免各模块重复实现。
2. 区分单次调用总量与分阶段（hop）明细，支持排障与成本分析。
3. 流式接口在 `end` 事件输出聚合 usage，避免前端自行累加误差。

请求新增参数（建议）：

```json
{
  "usage_options": {
    "enabled": true,
    "include_hops": true,
    "include_cost_estimate": true,
    "include_provider_raw_usage": false
  },
  "stream_options": {
    "include_usage": true,
    "usage_emit_mode": "final"
  }
}
```

字段说明：

1. `usage_options.enabled`：是否启用 usage 聚合返回，默认 `true`。
2. `usage_options.include_hops`：是否返回每个阶段明细（如 `planner/chat/skill/tooling`）。
3. `usage_options.include_cost_estimate`：是否返回预估成本（按模型费率表计算）。
4. `usage_options.include_provider_raw_usage`：是否透传 provider 原始 usage 字段。
5. `stream_options.usage_emit_mode`：
`final`（默认，仅在 `done/end` 输出 usage）；
`incremental`（允许输出中间 usage 事件）。

非流式响应新增字段（建议）：

```json
{
  "trace_id": "trc_xxx",
  "output": {"type":"text","text":"..."},
  "usage": {
    "total_prompt_tokens": 153,
    "total_completion_tokens": 19,
    "total_tokens": 172,
    "cached_tokens": 0,
    "cache_mode": "auto",
    "cost_estimate": {
      "currency": "USD",
      "input_cost": 0.000015,
      "output_cost": 0.000038,
      "total_cost": 0.000053
    },
    "hops": [
      {
        "phase": "planner",
        "provider": "ollama",
        "model": "qwen3:8b",
        "prompt_tokens": 98,
        "completion_tokens": 12,
        "total_tokens": 110,
        "latency_ms": 8200
      },
      {
        "phase": "chat",
        "provider": "ollama",
        "model": "qwen3:8b",
        "prompt_tokens": 55,
        "completion_tokens": 7,
        "total_tokens": 62,
        "latency_ms": 2300
      }
    ]
  }
}
```

流式 SSE 事件建议补齐：

1. `event: usage`（可选，`usage_emit_mode=incremental` 时输出）。
2. `event: done` 或 `event: end` 必带聚合 usage。

`done/end` 示例：

```text
event: end
data: {"success":true,"trace_id":"trc_xxx","usage":{"total_prompt_tokens":153,"total_completion_tokens":19,"total_tokens":172}}
```

统一约束（不做兼容）：

1. 仅返回新结构：`usage.total_*`、`usage.hops`、`usage.cost_estimate`。
2. 旧字段 `usage.prompt_tokens/completion_tokens` 不再作为契约保证字段。
3. provider 不返回 usage 时，底座允许估算并标记 `usage.estimated=true`。

## 1. Admin API

### 1.1 注册 Skill

`POST /api/v1/admin/skills`

请求体（示例）：

```json
{
  "skill_id": "incident-triage",
  "version": "1.0.0",
  "source": "plugin",
  "bundle_ref": {
    "uri": "s3://powerx-skills/plugin-a/incident-triage-1.0.0.tgz",
    "checksum": "sha256:xxxx"
  },
  "manifest": {
    "description": "故障分诊流程",
    "entrypoints": ["runbook.default"]
  }
}
```

### 1.2 查询 Skill 列表

`GET /api/v1/admin/skills?skill_id=&status=&source=&page=&page_size=`

### 1.3 发布与回滚

- `POST /api/v1/admin/skills/{skill_id}/publish`
- `POST /api/v1/admin/skills/{skill_id}/rollback`

### 1.4 执行轨迹查询（Planner 节点）

- `GET /api/v1/admin/skills/traces?plan_id=&skill_id=&node_id=&node_status=&limit=&offset=`
- `GET /api/v1/admin/skills/traces/{trace_id}?tenant_uuid=`

响应（列表示例）：

```json
{
  "items": [
    {
      "trace_id": "trace_xxx",
      "tenant_uuid": "tenant_xxx",
      "skill_id": "flow.alert.aggregate",
      "status": "completed",
      "plan_id": "plan_xxx",
      "node_id": "task_1",
      "node_status": "completed",
      "retry_trace": "",
      "created_at": "2026-03-19T16:20:00+08:00"
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0
}
```

## 2. Tenant API

### 2.1 直接调用 Skill

`POST /api/v1/tenant/skills/invoke`

```json
{
  "skill_id": "incident-triage",
  "version": "1.0.0",
  "payload": {
    "incident_id": "INC-1001"
  },
  "context": {
    "tool_scope": "ops"
  }
}
```

### 2.2 统一入口调用

`POST /api/v1/tenant/invocations` with `preferred_protocol=skill`

```json
{
  "capability_id": "com.powerx.skill.incident-triage.invoke",
  "preferred_protocol": "skill",
  "payload": {
    "skill_id": "incident-triage",
    "incident_id": "INC-1001"
  }
}
```

## 2.3 Agent 主入口（推荐）

### 2.3.1 非流式调用

`POST /api/v1/agents/invoke`

```json
{
  "agent_id": 1001,
  "q": "请先汇总最近告警，再给出修复建议",
  "session_id": "sess_xxx"
}
```

响应（示例）：

```json
{
  "trace_id": "trc_xxx",
  "plan_id": "plan_xxx",
  "status": "completed",
  "result": {
    "answer": "...",
    "nodes": [
      {"node_id":"n1","kind":"skill","skill_id":"skill.thirdparty.prompt-template","status":"completed"},
      {"node_id":"n2","kind":"llm","status":"completed"}
    ]
  }
}
```

### 2.3.2 流式调用

`GET /api/v1/agents/stream/sse?q=...&agent_id=...&session_id=...`

Runtime Intent 控制面查询使用结构化参数，不依赖自然语言短语：

```text
GET /api/v1/agents/stream/sse?intent=agent.bound_capabilities&agent_uuid=...&session_uuid=...&tenant_uuid=...&env=...
```

约束：

1. `intent=agent.bound_capabilities|agent.bound_skills` 可以不传 `q`。
2. Runtime Intent 命中后直接执行确定性 handler，不进入 LLM / Planner。
3. 普通自然语言请求不得通过关键词穷举触发控制面逻辑。
4. SSE `final.metadata` 必须包含 `runtime_intent`、`runtime_intent_kind`、`llm_bypassed`、`planner_bypassed` 和 `model_selection`。

事件语义（建议）：

1. `intent`：多候选 skill（top-k）
2. `plan`：DAG/阶段计划
3. `node_start` / `node_end`：节点执行生命周期
4. `token` / `data`：节点流式输出
5. `final` / `end`：最终收敛输出

补充约束：

1. `node.kind=skill` 通过 Skill InvokeService/AdapterService 真实执行。
2. `node.kind=tooling` 通过 Capability InvocationService 真实执行（tooling 数据来自 capability registry 落库）。
3. 未命中可执行节点时仅输出 `intent + final(+end)`，不发 `node_start/node_end`。
4. `plan.tasks[]` 节点建议补充 `source_scope=system|agent`，用于区分系统固有能力与 Agent 自定义能力来源。
5. `meta` 事件建议输出 `model_policy`，说明 `runtime_intent/intent_classifier/planner/skill_param_extractor/final_response/reviewer` 各节点使用的模型选择。首版可全部继承 Agent 默认模型。

## 3. Plugin / 第三方接口

### 3.1 导入 Skill

`POST /api/v1/admin/skills/import`

### 3.2 绑定 capability

`POST /api/v1/admin/skills/{skill_id}/bind-capability`

```json
{
  "capability_id": "com.powerx.skill.incident-triage.invoke",
  "tool_grants": ["ops.incident.read"]
}
```

### 3.3 插件 Skill 发现

`GET /api/v1/plugin/skills`

响应示例：

```json
{
  "items": [
    {
      "skill_id": "mediax.video_rebuilder.cn",
      "provider": "com.powerx.plugin.mediax-studio",
      "version": "1.0.0",
      "title": "视频智能重构",
      "description": "根据视频链接和模板要求创建视频自动化重构任务",
      "intent_examples": ["帮我重构这个 shorts"],
      "input_schema": {
        "type": "object",
        "required": ["urls"],
        "properties": {
          "urls": {"type": "array", "items": {"type": "string"}}
        }
      },
      "executor": {
        "type": "plugin_http",
        "method": "POST",
        "path": "/api/v1/plugin/skills/invoke",
        "capability": "creation.video_automation.ingest"
      }
    }
  ]
}
```

### 3.4 插件 Skill Executor

`POST /api/v1/plugin/skills/invoke`

请求：

```json
{
  "skill_id": "mediax.video_rebuilder.cn",
  "version": "1.0.0",
  "input": {
    "urls": ["https://example.com/video.mp4"],
    "template_hint": "篮球模板"
  },
  "context": {
    "tenant_uuid": "tenant_xxx",
    "user_uuid": "user_xxx",
    "agent_id": "agent_xxx",
    "session_id": "session_xxx",
    "message_id": "message_xxx",
    "channel": "telegram",
    "locale": "zh-CN",
    "trace_id": "trace_xxx"
  }
}
```

响应：

```json
{
  "success": true,
  "skill_id": "mediax.video_rebuilder.cn",
  "status": "queued",
  "message": "已创建视频重构任务",
  "task_id": "video-automation-task-xxx",
  "data": {
    "task_url": "/creation/video-automation/tasks/xxx"
  },
  "trace_id": "trace_xxx"
}
```

强约束：

1. `tenant_uuid/user_uuid/agent_id/session_id/trace_id/skill_id` 缺失时必须失败。
2. 插件必须校验 `skill_id` 存在且启用，executor capability 与声明一致。
3. 不提供匿名 fallback，不绕过租户上下文。

### 3.5 插件调用 PowerX Agent Stream（Framework Client 封装）

插件本地 Chat 或插件内调试页面调用 PowerX Agent 主入口：

```text
POST /api/v1/agents/invoke
GET  /api/v1/agents/stream/sse
WS   /api/v1/agents/stream/ws
```

插件侧不直接拼接 SSE/WS 事件协议，应通过 PowerXPlugin Framework Client 使用。

SSE 事件最小语义：

1. `intent`
2. `plan`
3. `node_start`
4. `node_end`
5. `token`
6. `final`
7. `end`

## 4. 统一响应模型

```json
{
  "trace_id": "trc_xxx",
  "plan_id": "plan_xxx",
  "status": "completed",
  "protocol_used": "skill",
  "fallback_used": false,
  "result": {
    "summary": "..."
  }
}
```

## 5. 错误码

- `skill.not_found`
- `skill.version_not_found`
- `skill.permission_denied`
- `skill.execution_failed`
- `skill.source_untrusted`
- `skill.plugin_not_installed`
- `skill.plugin_executor_unavailable`
- `skill.plugin_context_missing`
- `skill.plugin_capability_mismatch`
- `AGENT_TRACE_ROOT_REQUIRED`
- `AGENT_TRACE_NOT_FOUND`
- `AGENT_TRACE_SOURCE_UNAVAILABLE`

## 6. 鉴权与多租户

1. 所有 Tenant 调用必须从请求上下文解析 `tenant_uuid`。
2. 管理接口仅 Admin 可调用。
3. Skill 调用需通过 ToolGrant 或 Policy 检查。

## 7. Agent Run Trace & Report API

Agent Trace API 属于 root-only 调试能力，用于查看 Agent Session/Message/Node 的结构化运行轨迹并下载智能对话报告。

### 7.1 Message Trace 详情

```text
GET /api/v1/admin/agent-traces/messages/{message_id}
```

Query：

1. `tenant_uuid`
2. `source=local|loki`

响应：

```json
{
  "run": {
    "trace_id": "trace_xxx",
    "run_id": "run_xxx",
    "tenant_uuid": "tenant_xxx",
    "agent_id": "agent_xxx",
    "session_id": "session_xxx",
    "message_id": "message_xxx",
    "status": "completed",
    "node_count": 7,
    "event_count": 16,
    "duration_ms": 1820
  },
  "timeline": [],
  "nodes": [],
  "artifacts": []
}
```

### 7.2 Message Timeline

```text
GET /api/v1/admin/agent-traces/messages/{message_id}/timeline
```

Query：

1. `source=local|loki`
2. `node_kind`
3. `status`

事件模型：

```json
{
  "trace_id": "trace_xxx",
  "run_id": "run_xxx",
  "message_id": "message_xxx",
  "node_id": "006_skill_invoke",
  "node_kind": "skill_invoke",
  "node_ref": "mediax.video_rebuilder.cn",
  "phase": "end",
  "status": "success",
  "duration_ms": 320,
  "created_at": "2026-06-08T12:00:00Z"
}
```

### 7.3 Message Report 下载

```text
GET /api/v1/admin/agent-traces/messages/{message_id}/report?format=markdown|json&source=local|loki
```

报告必须包含：

1. Summary
2. User Message
3. Runtime Timeline
4. Intent Recognition
5. Planner
6. Skill / Tool Invocation
7. Final Response
8. Errors / Warnings

### 7.4 Session Report 下载

```text
GET /api/v1/admin/agent-traces/sessions/{session_id}/report?format=json|markdown|zip&source=local|loki
```

首版可先实现 Message 级报告，Session 级报告作为扩展接口保留。

### 7.5 权限错误

非 root 用户访问任何 Agent Trace API 必须返回：

```json
{
  "code": "AGENT_TRACE_ROOT_REQUIRED",
  "message": "Agent trace inspection requires root permission"
}
```
