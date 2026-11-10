# 语音合成开放能力 v2

正式能力为 `com.corex.ai.tts.invoke`，REST 入口为 `POST /api/v1/ai/tts/invoke`。型号必须来自当前凭证的能力目录，示例型号仅说明合同，调用前须完成目标租户配置、授权和费用额度确认。

完整字段、授权绑定、错误和任务推送规则见 [AI 媒体 v2 合同](../../../../contracts/ai-media-v2.md)。

推荐服务调用 `/api/v1/tenant/invocations`，`preferred_protocol` 为 `core_internal`，body 使用固定 typed operation。禁止自选 method/endpoint/query；运行通道和环境由启动配置及目标租户设置确定。

```json
{"capability_id":"com.corex.ai.tts.invoke","preferred_protocol":"core_internal","payload":{"body":{"operation":"invoke","contract_version":2,"model_key":"qwen-cn/qwen3-tts-flash","inputs":[{"type":"text","content":"测试内容"}],"params":{"voice":"Cherry"},"idempotency_key":"unique-attempt-uuid"}}}
```

直接 REST 使用同一 body（可省略 operation），凭证放在 Authorization header；不传 env query 或 X-PowerX-Env。

HTTP 202 返回稳定 Core task_uuid；通过正式任务 GET 读取快照，页面使用 SSE 接收更新。成果转存和校验完成后才返回 succeeded 与 media_asset_uuid。网络重试沿用原幂等键，禁止自动换键重复生成。

音色和语言以所选型号的 parameter_schema 为准。当前 Qwen Flash 适配未声明语速／输出格式，显式传入这些参数会返回结构化错误。
