# AI 开放能力调试指南

本目录聚焦 PowerX AI 相关开放接口，按子模块拆分文档，便于插件侧按能力快速联调。

默认环境约定：

- HTTP Base：`http://127.0.0.1:8077/api/v1`
- 鉴权（默认）：`Authorization: ApiKey <API_KEY>`
- 兼容鉴权：`Authorization: Bearer <TENANT_TOKEN>`
- 租户作用域：默认从 API Key 绑定关系解析；若使用 JWT，则从 claims 读取 `tenant_uuid`

## 子模块文档

- [LLM（含模型列表、会话、流式）](./llm.md)
- [VLM（视觉多模态）](./vlm.md)
- [Image（图像）](./image.md)
- [Video（视频）](./video.md)
- [TTS（语音合成）](./tts.md)
- [Embedding（向量）](./embedding.md)

## 统一入口说明

如需统一走 Selector/Gateway，可调用 `POST /api/v1/tenant/invocations`：

- `capability_id` 使用各子文档列出的能力 ID
- `preferred_protocol` 常用 `rest`
- `payload` 里写 `method`、`endpoint`、`query/body`

参考主文档：[开放能力总览](../readme.md)
