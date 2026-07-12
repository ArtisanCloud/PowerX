# PowerX Plugin Connectors

This directory hosts reference implementations and runbooks for official connector bundles that ship with PowerX. Each connector documents how to:

- Register an instance through the new `/internal/connector-platforms/{platform}/instances` API (and the matching gRPC RPCs).
- Store OAuth tokens + webhook signing keys exclusively via Vault references or sealed secrets.
- Handle callback signature verification and emit trace context back to the Agent Model Hub.
- Implement instance-level pause/resume hooks driven by the Connector Guard service.

## Trace Correlation

All connector invoke/callback flows **must** propagate the PowerX trace context:

| Header | Description |
|--------|-------------|
| `X-PowerX-Trace-ID` | Canonical trace ID from router or upstream task. |
| `traceparent`       | W3C trace context (optional but recommended). |

When issuing outbound HTTP calls (Coze, n8n), forward `X-PowerX-Trace-ID` so downstream systems participate in the same span tree. Callbacks **must** include the same trace ID in response payloads and headers to satisfy SC-003.

## Plugin Local Agent Chat

插件本地 Chat 只能作为 PowerX Agent Session 的调试入口，不能直连插件业务接口。

标准链路：

```text
Plugin Web Admin /plugins/agent-chat
  -> PowerX Framework Client
  -> POST /api/v1/agents/sessions
  -> GET  /api/v1/agents/stream/sse
  -> PowerX Agent Runtime
  -> Skill Bridge
  -> Plugin Skill Executor
```

禁止链路：

```text
Plugin Web Admin
  -> POST /api/v1/creation/video-automation/ingest
```

本地调试时，插件页面必须携带：

| Field | Description |
|-------|-------------|
| `agent_id` / `agent_uuid` | PowerX Agent 标识，默认可使用 System Default Agent。 |
| `session_id` | 由 PowerX `/agents/sessions` 创建或恢复。 |
| `channel` | 固定为 `plugin_local_chat`。 |
| `message_id` | 当前用户消息 ID，用于 Agent Run Trace。 |

Framework Client 实现应封装 Agent Invoke、Agent SSE、Agent WS 与 STS，不允许每个插件重复实现长期并行的对话系统。

## Files

- [`coze/README.md`](./coze/README.md) – Coze connector quickstart (OAuth, webhook signing, trace IDs).
- [`n8n/README.md`](./n8n/README.md) – n8n workspace connector (API key + signed callbacks).

Each README includes:

1. Instance registration payload (HTTP + gRPC) with mapping templates.
2. Vault secret references expected by Connector Guard.
3. Sample webhook verification snippet in TypeScript.
4. Notes for instance-level degradation (auto-pause + resume approvals).

> **Note**: these documents complement the backend implementation under `backend/internal/service/connector_guard` and the contract tests in `backend/tests/contract/{http,grpc}/agent_model_hub/connector_*`. Update both when connector requirements change.
