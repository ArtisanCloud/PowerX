# LLM 开放能力调试指南

## 能力矩阵

| Capability ID | 作用 | REST 入口 |
| --- | --- | --- |
| `com.corex.ai.llm.invoke` | 无状态 LLM 调用 | `POST /api/v1/ai/llm/invoke` |
| `com.corex.ai.llm.models/list` | 模型目录列表 | `GET /api/v1/ai/llm/models` |
| `com.corex.ai.llm.session.create` | 创建会话 | `POST /api/v1/ai/llm/sessions` |
| `com.corex.ai.llm.session.append` | 追加会话消息 | `POST /api/v1/ai/llm/sessions/{session_id}/messages` |
| `com.corex.ai.llm.stream` | 会话流式输出 | `GET /api/v1/ai/llm/sessions/{session_id}/stream` |

## 1. 直接 REST 调用

```bash
export HTTP_BASE="http://127.0.0.1:8077/api/v1"
export API_KEY="<api-key>"
```

### 1.1 无状态调用

```bash
curl -sS -X POST "$HTTP_BASE/ai/llm/invoke?env=dev" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model_key": "ollama/qwen3:8b",
    "inputs": [{"type":"text","text":"解释一下 RAG"}],
    "params": {"temperature": 0.2, "max_tokens": 256}
  }'
```

### 1.2 模型列表

```bash
curl -sS "$HTTP_BASE/ai/llm/models?env=dev" \
  -H "Authorization: ApiKey $API_KEY"
```

返回语义（请按这个理解）：

- 默认返回“混合列表”，不是纯全量，也不是仅已配置。
- `configured` 是响应字段，不是请求参数；接口没有 `configured` 入参，因此不存在“默认 true/false 过滤”。
- 合并范围：
  - Profile 模型：租户 profile + 全局 profile
  - 候选模型：provider catalog（该租户在当前 env 下有 provider 凭据时才会出现）
- 排序：`configured=true` 优先，其次 `profile_configured=true` 优先，再按 `provider/model` 字典序。
- 当前接口不支持 `configured_only=true` 这类过滤参数。

返回字段重点：

- `configured=true`：该模型对应 provider 在当前 env 下已有 credential（通常来自“测试连接通过”或“保存设置”）
- `profile_configured=true`：该模型已在 profile 中落库（租户或全局）
- `profile_configured=false`：该模型来自 provider catalog 候选，尚未保存为 profile
- `source`：`tenant_profile` / `global_profile` / `provider_catalog`

说明：该接口不做逐模型在线探测，因此不代表“连通性已验证通过”。

### 1.3 会话调用

创建会话：

```bash
curl -sS -X POST "$HTTP_BASE/ai/llm/sessions" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model_key":"ollama/qwen3:8b","title":"demo"}'
```

追加消息：

```bash
SESSION_ID="<session_id>"
curl -sS -X POST "$HTTP_BASE/ai/llm/sessions/$SESSION_ID/messages" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "role":"user",
    "content":[{"type":"text","text":"继续上一个问题"}]
  }'
```

流式输出：

```bash
curl -N "$HTTP_BASE/ai/llm/sessions/$SESSION_ID/stream?env=dev" \
  -H "Authorization: ApiKey $API_KEY"
```

## 2. 通过 `/tenant/invocations` 调用

### 2.1 调用模型列表

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.corex.ai.llm.models/list",
    "preferred_protocol": "rest",
    "payload": {
      "method": "GET",
      "endpoint": "/api/v1/ai/llm/models",
      "query": {"env": "dev"}
    }
  }'
```

返回语义说明（与 `1.2 模型列表` 一致）：

- `data.payload.items` 仍是“已配置 + provider catalog”的混合列表。
- `configured` / `profile_configured` 都是响应字段，不是请求参数；不支持通过请求传这两个字段过滤。
- 不会因为走了 `/tenant/invocations` 就自动变成“仅已配置”或“全量连通性校验通过”列表。
- 如需区分，请按 `configured`、`profile_configured` 与 `source` 字段判断。

### 2.2 无状态调用

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.corex.ai.llm.invoke",
    "preferred_protocol": "rest",
    "payload": {
      "method": "POST",
      "endpoint": "/api/v1/ai/llm/invoke",
      "query": {"env": "dev"},
      "body": {
        "model_key": "ollama/qwen3:8b",
        "inputs": [{"type":"text","text":"解释一下 RAG"}],
        "params": {"temperature": 0.2, "max_tokens": 256}
      }
    }
  }'
```

## 3. 常见错误

- `model_key required`
  - 调 `invoke/session` 时未传 `model_key`
  - 调模型列表时应使用 `GET /ai/llm/models`，不要误用 `/ai/llm/invoke`
- `model_key xxx not configured for tenant`
  - 模型不在租户 profile 内，且当前策略不允许未配置模型
- `integration.invoke_failed`
  - 优先用返回的 `trace_id` 到调用链路/日志排查
