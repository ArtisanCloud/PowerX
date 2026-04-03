# Image 开放能力调试指南

## 能力 ID

- `com.corex.ai.image.invoke`

REST 入口：`POST /api/v1/ai/image/invoke`

```bash
export HTTP_BASE="http://127.0.0.1:8077/api/v1"
export API_KEY="<api-key>"
```

## 1. 直接 REST 调用

```bash
curl -sS -X POST "$HTTP_BASE/ai/image/invoke?env=dev" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model_key": "openai/gpt-image-1",
    "inputs": [{"type":"text","text":"一只戴墨镜的柴犬，电影海报风格"}],
    "params": {"size":"1024x1024", "quality":"high"}
  }'
```

## 2. 通过 `/tenant/invocations`

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.corex.ai.image.invoke",
    "preferred_protocol": "rest",
    "payload": {
      "method": "POST",
      "endpoint": "/api/v1/ai/image/invoke",
      "query": {"env": "dev"},
      "body": {
        "model_key": "openai/gpt-image-1",
        "inputs": [{"type":"text","text":"一只戴墨镜的柴犬，电影海报风格"}],
        "params": {"size":"1024x1024", "quality":"high"}
      }
    }
  }'
```
