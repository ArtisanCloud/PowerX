# Video 开放能力调试指南

## 能力 ID

- `com.corex.ai.video.invoke`

REST 入口：`POST /api/v1/ai/video/invoke`

```bash
export HTTP_BASE="http://127.0.0.1:8077/api/v1"
export API_KEY="<api-key>"
```

## 1. 直接 REST 调用

```bash
curl -sS -X POST "$HTTP_BASE/ai/video/invoke?env=dev" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model_key": "volcengine/doubao-video",
    "inputs": [{"type":"text","text":"生成一个 5 秒钟海边日落镜头"}],
    "params": {"duration": 5}
  }'
```

## 2. 通过 `/tenant/invocations`

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.corex.ai.video.invoke",
    "preferred_protocol": "rest",
    "payload": {
      "method": "POST",
      "endpoint": "/api/v1/ai/video/invoke",
      "query": {"env": "dev"},
      "body": {
        "model_key": "volcengine/doubao-video",
        "inputs": [{"type":"text","text":"生成一个 5 秒钟海边日落镜头"}],
        "params": {"duration": 5}
      }
    }
  }'
```
