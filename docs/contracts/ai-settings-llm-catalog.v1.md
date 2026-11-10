# PowerX AI Settings 文本目录 v1

本合同是 Core 实现的正式服务合同。代码与运行实例的验收状态分别记录，不能用本合同或受控测试替代实例升级、真实 grant 和目录一致性证据。

## 能力与调用

| 能力 | operation | body 字段 |
| --- | --- | --- |
| `com.corex.ai.catalog.providers.read` | `list_settings_providers` | 必填 `operation`, `modality=llm` |
| `com.corex.ai.catalog.models.read` | `list_settings_models` | 必填 `operation`, `modality=llm`, `provider`；按 Provider 规则填写 `app` |

通过 `POST /api/v1/tenant/invocations` 调用，选择 `preferred_protocol=core_internal`，固定绑定为 `core://ai/model-catalog`、方法 `INVOKE`。payload 仅允许 typed `body`。

```json
{
  "capability_id": "com.corex.ai.catalog.providers.read",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {"operation": "list_settings_providers", "modality": "llm"}
  }
}
```

型号读取时，将能力改为 `com.corex.ai.catalog.models.read`，body 改为：

```json
{"operation":"list_settings_models","modality":"llm","provider":"ollama","app":""}
```

拒绝调用方传入 `env`、`base_url`、`endpoint`、`method`、`headers`、密钥、自由代理 body 和身份覆盖 context；未知字段即失败。Providers 操作也不允许 `provider/app/capability`，即使值为空。操作必须与 capability ID 对应。

租户来自已认证身份；环境来自该租户的 `ai.current_env`。插件自己的 development/production 选择不覆盖 Core 环境。无环境配置返回 `409 AI_ENV_NOT_CONFIGURED`。

## 返回值与标识

下面是 invocation 成功信封中 `payload.result` 的内容，外层仍包含 `protocol_used`、`fallback_used` 和 trace。

```json
{"contract_version":1,"catalog_kind":"ai_settings","env":"dev","modality":"llm","providers":[{"id":"ollama","name":"Ollama (Local)","apps":[]}]}
```

```json
{"contract_version":1,"catalog_kind":"ai_settings","env":"dev","modality":"llm","provider":"ollama","app":"","items":[{"model_key":"ollama/private-instance:14b","provider":"ollama","model":"private-instance:14b","label":"private-instance:14b"}]}
```

Provider 的 `id/name/apps` 直接复用 `AgentSettingService.Providers("llm")`，显示名称由 Core 管理，不从型号、插件 YAML 或默认文案反推。

型号复用 `AgentSettingService.ModelsForTenant`。`provider` 必须为当前 Provider 目录中的精确 id；若 Provider 提供 App，必须选择它声明的 App。无 App 的 Provider 只能使用空 App。空目录返回 `providers: []` 或 `items: []`，不注入旧型号。

`model/label` 保留 Core 目录中的原始完整型号。`model_key` 为 `provider/model`，有正式 App 时才为 `provider/app:model`。`qwen3:14b`、`bge-m3:latest` 和私有自定义 tag 中的冒号均属于型号，客户端不得拆分或规范化。

OpenAPI 的 `CatalogOperation` 是媒体操作和两个 Settings 请求的互斥联合。正式请求与返回 Schema 位于 [ai-media-v2.openapi.yaml](ai-media-v2.openapi.yaml) 的 `SettingsProvidersOperation`、`SettingsModelsOperation`、`SettingsProviderCatalog`、`SettingsModelCatalog`。现有 `list_providers/list_models` 媒体 v2 合同保持不变，禁止混入 Settings 的 `modality` 字段。

## Ollama 与配置候选语义

- 服务读取必须存在当前租户、当前环境的已保存 Ollama 配置，不读取其他租户、其他环境或全局凭据；无配置返回 `409 AI_SETTINGS_PROVIDER_NOT_CONFIGURED`，不会探测 Core 主机默认实例。
- 已保存配置中的可选 Base URL 为空时，使用 Core 正式 Provider 配置的默认地址；该默认值属于已保存连接语义。连接地址快照在 Core 内部使用，不下发给插件。
- 从对应实例读取 `/api/tags` 和 `/api/show`，按明确能力筛选 LLM，保留原始 tag；不存在或已下线的条目排除。发现失败明确返回错误，不回退 YAML 样例。
- 这里是与管理页一致的**配置候选目录**，不保证型号已建立 profile、已获生成授权、供应商可实时调用或有足够余额。生成仍走既有 Core 模型配置与调用授权检查。
- 对比管理页时，应使用返回的 Core `env`、同租户、同 Provider／App、同一已保存连接。管理页未保存的 Base URL 草稿不属于一致性承诺。

## 授权与错误

Admin 页面保留 `admin_user + user_jwt + RBAC`；服务客户端使用 API Key／STS 的 `service_actor`，不代理 Admin REST。

两个正式服务 scope 分别为 `_scope.ai.catalog.providers.read` 和 `_scope.ai.catalog.models.read`，`action=read`、`resource_type=api`，resource pattern 分别为 `ai-catalog-providers` 和 `ai-catalog-models`。正式 IAM permission 必须为 `allow_api_key=true` 且 `api_key_explicit=true`。每次调用检查 published capability、当前租户 registration 和当前凭据的实时 grant；一个目录 scope 不能代替另一个。

| HTTP | 错误码／含义 |
| --- | --- |
| 400 | `AI_OPERATION_INVALID`：未知字段、错误操作、模态或操作／能力不匹配 |
| 400 | `AI_TYPED_OPERATION_REQUIRED`：自由代理或身份覆盖 payload |
| 401/403 | 未认证、未授权、撤销或跨租户；沿用正式授权错误 |
| 409 | `AI_ENV_NOT_CONFIGURED`：目标租户未选择 Core 环境 |
| 409 | `AI_SETTINGS_PROVIDER_NOT_CONFIGURED`：目标租户／环境未保存 Ollama 连接 |
| 422 | `AI_SETTINGS_PROVIDER_INVALID`、`AI_SETTINGS_APP_REQUIRED`、`AI_SETTINGS_APP_INVALID` |
| 422/502/504 | `AI_MODEL_DISCOVERY_*`：实例配置、响应、网络或超时错误 |
| 503 | `AI_DEPENDENCY_UNAVAILABLE`：Core 目录依赖不可用 |

发布／绑定版本不匹配时沿用 Registry 的显式升级错误；不得通过 fallback 或自动扩权绕过。错误与结果不得包含密钥、已保存地址或供应商原始敏感响应。

## 验收与运行更新

1. 比较 Provider 精确 id/name/apps；验证原生 tag、App、空目录、未保存连接、租户／环境隔离和发现失败。
2. API Key 与 STS 分别验证 granted、无 grant、scope 隔离、撤销即时生效、跨租户 403、未发布／停用 registration 拒绝、自由 payload 拒绝。
3. 运行 `make capability-check`、受影响测试和 `go vet`；通过 scoped `make capability-seed CAPABILITY_SEED_FLAGS='-capability-ids com.corex.ai.catalog.providers.read,com.corex.ai.catalog.models.read'` 同步正式合同。该步骤不附加凭据授权。
4. 检查开发 Host API Key 的 `POST /api/v1/tenant/capabilities:grant-status`；需要显式版本确认时按现有版本升级流程处理。
5. 由运行进程管理者重新编译、重启 Core 后，用原始 API Key／STS 验证两个操作；记录实际版本／PID／发布时间和同租户同环境目录比较结果。受控测试与未加载新代码的旧进程不能标记为真实接通。

2026-10-25 开发实例已完成 API Key 实测：原 Framework Host Key 两项 grant 均为 `granted`，两个操作返回 200、`core_internal`、无 fallback；与同租户 `dev` 管理页底层服务的 13 个 Provider（含名称、Apps）及 Ollama 完整 tag `qwen3:14b` 一致。STS 的授权、撤销与隔离通过受控集成测试，未将它标记为真实实例 STS 验收。详见 [验收记录](evidence/ai-settings-llm-catalog-validation-20261025.json)。

## 管理页在线同步

管理页新增「更新在线模型」，按 Provider 原生目录协议获取并保存租户范围快照。云 Provider 的本契约读取共用该快照，保持原有输出和授权边界；同步写入仅限用户管理员，不作为插件服务能力。操作方式及协议表见 [AI 在线模型目录](ai-online-model-catalog.md)。
