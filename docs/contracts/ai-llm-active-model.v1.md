# 读取租户默认文本模型

插件后端用已验证的 STS Bearer 令牌调用 `POST /api/v1/tenant/invocations`。Core 检查正式能力发布、当前租户登记及调用插件的实时授权。插件需通过正式授权流程获得 `com.corex.ai.llm.active_model.read`。

```json
{
  "capability_id": "com.corex.ai.llm.active_model.read",
  "preferred_protocol": "core_internal",
  "payload": {"body": {"operation": "get_active_model"}}
}
```

固定绑定是 `INVOKE core://ai/llm/active-model`。业务结果位于 `data.result`，包含以下字段。

```json
{"env": "dev", "modality": "llm", "model_key": "qwen-cn/qwen3.8-flash"}
```

租户取自 STS 身份。Core 优先采用身份显式绑定的环境，其次读取该租户的 `ai.current_env`。默认路由来自「模型配置」中保存的 `llm` 项，限定同一租户和环境。开发环境优先读取 `dev`，兼容该租户旧的 `default` 记录。插件应原样传递 `model_key`。读取能力和 LLM 执行能力分别授权。

在 STS 文本模型调用中，`env` 请求参数只用于确认服务端解析出的环境。发送其他环境时返回 `403 AI_FORBIDDEN`，便于调用方发现读取后发生的环境切换。

| HTTP | `details.reason_code` | 含义 |
| --- | --- | --- |
| 400 | `AI_OPERATION_INVALID` 或 `AI_TYPED_OPERATION_REQUIRED` | 操作参数非法 |
| 400 | `AI_ENV_INVALID` | 解析出的 AI 环境超出支持范围 |
| 409 | `AI_ENV_NOT_CONFIGURED` | 租户尚未选择当前 AI 环境 |
| 409 | `AI_DEFAULT_MODEL_NOT_CONFIGURED` | 该环境尚未保存默认文本模型 |
| 503 | `AI_DEPENDENCY_UNAVAILABLE` | AI 配置读取失败 |

部署时运行 `make capability-check`，发布包含此能力的新 Core，并在目标环境执行 `make capability-seed CAPABILITY_SEED_FLAGS="-capability-ids=com.corex.ai.llm.active_model.read"`。给目标插件授予该能力后，使用该插件的 STS 调用 `/api/v1/tenant/capabilities:grant-status` 确认状态为 `granted`，再发起上述查询。
