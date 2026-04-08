# Embedding 开放能力调试指南

## 能力 ID

- `com.corex.ai.embedding.invoke`

REST 入口：`POST /api/v1/ai/embedding/invoke`

```bash
export HTTP_BASE="http://127.0.0.1:8077/api/v1"
export API_KEY="<api-key>"
```

## 1. 直接 REST 调用

```bash
curl -sS -X POST "$HTTP_BASE/ai/embedding/invoke?env=dev" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model_key": "openai/text-embedding-3-small",
    "inputs": ["PowerX", "能力网关"],
    "params": {}
  }'
```

## 2. 通过 `/tenant/invocations`

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.corex.ai.embedding.invoke",
    "preferred_protocol": "rest",
    "payload": {
      "method": "POST",
      "endpoint": "/api/v1/ai/embedding/invoke",
      "query": {"env": "dev"},
      "body": {
        "model_key": "openai/text-embedding-3-small",
        "inputs": ["PowerX", "能力网关"],
        "params": {}
      }
    }
  }'
```

返回结果主要在 `data.vectors`（或经 selector 代理后的 `data.payload.vectors`）。
