# VLM 开放能力调试指南

VLM 面向视觉多模态输入（图文理解等），REST 入口：`POST /api/v1/ai/vlm/invoke`。

```bash
export HTTP_BASE="http://127.0.0.1:8077/api/v1"
export API_KEY="<api-key>"
```

## 1. 直接 REST 调用

```bash
curl -sS -X POST "$HTTP_BASE/ai/vlm/invoke?env=dev" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model_key": "openai/gpt-4o-mini",
    "inputs": [
      {"type":"text","text":"描述这张图"},
      {"type":"image_url","url":"https://example.com/demo.png"}
    ],
    "params": {"temperature": 0.1}
  }'
```

## 2. 与 Selector 的关系

当前建议优先直连 REST 入口进行 VLM 调试。若后续发布稳定的 VLM 专属 `capability_id`，再统一迁移到 `/tenant/invocations`。

## 3. 常见错误

- `model_key required`：缺少模型键
- `model not configured for tenant`：租户未配置该模型
- `provider driver not implemented`：对应 provider 的 VLM 驱动尚未实现
