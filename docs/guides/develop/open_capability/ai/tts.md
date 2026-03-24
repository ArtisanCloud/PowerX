# TTS 开放能力调试指南

## 能力 ID

- `com.corex.ai.tts.invoke`

REST 入口：`POST /api/v1/ai/tts/invoke`

```bash
export HTTP_BASE="http://127.0.0.1:8077/api/v1"
export API_KEY="<api-key>"
```

## 1. 直接 REST 调用

```bash
curl -sS -X POST "$HTTP_BASE/ai/tts/invoke?env=dev" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model_key": "openai/gpt-4o-mini-tts",
    "inputs": [{"type":"text","text":"你好，这是一段测试语音"}],
    "params": {"voice":"alloy"}
  }'
```

## 2. 通过 `/tenant/invocations`

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.corex.ai.tts.invoke",
    "preferred_protocol": "rest",
    "payload": {
      "method": "POST",
      "endpoint": "/api/v1/ai/tts/invoke",
      "query": {"env": "dev"},
      "body": {
        "model_key": "openai/gpt-4o-mini-tts",
        "inputs": [{"type":"text","text":"你好，这是一段测试语音"}],
        "params": {"voice":"alloy"}
      }
    }
  }'
```
