# Skills API 契约（Admin / Tenant / Plugin）

本文定义 Skill 管理与调用接口契约，供后端、前端、插件侧统一实现。

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

## 6. 鉴权与多租户

1. 所有 Tenant 调用必须从请求上下文解析 `tenant_uuid`。
2. 管理接口仅 Admin 可调用。
3. Skill 调用需通过 ToolGrant 或 Policy 检查。
