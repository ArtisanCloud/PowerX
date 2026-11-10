# AI 在线模型目录

在「设置 → 模型配置」选择 Provider。已配置连接时，下拉框优先读取当前连接最近成功获取的完整在线清单；首次读取没有快照时自动获取。点击 Model 旁的「更新在线模型」可以主动获取最新结果。首次配置可先填写地址与 Key，执行不调用模型的「测试连接」，通过后保存连接并读取在线清单。有未保存的连接修改时，更新弹窗会提示先保存，避免误用旧地址或旧 Key。

`default` 是 `dev` 的历史别名。页面、HTTP／gRPC 配置接口、在线目录与运行时使用同一套读取规则：先读取同租户的 `dev` 对应记录，缺失时读取旧 `default` 记录。规范环境已有记录时整条优先，不拼接旧 Key、地址或参数；也不跨到生产环境或其他租户。旧密文仍按记录实际存储的环境解封。

读取和在线更新不迁移凭据。用户正常保存配置时写入 `dev`，继承的旧密钥先按原环境解封，再按目标环境重新密封，旧记录保留。模型画像、默认路由和旧在线目录快照同样兼容读取；API 返回规范环境名称。显式凭据轮换会分别处理两份物理环境的密文，不混用 RSA 作用域。

管理表单中的 Base URL 留空表示使用当前 Provider／接入方式的默认地址，清空后会在字段下展示该地址。连接测试、试跑与保存都遵守这一规则，不再从旧凭据补回自定义地址。保存空地址时保留“使用默认地址”的含义，后续运行与在线目录按同一规则解析；没有默认地址的服务必须填写地址。服务内部仅更新凭据时，省略 `base_url` 字段仍表示保留旧地址，与显式提交空值不同。单纯查看或编辑表单不会切换运行中的连接，成功保存后才使用新配置。只读连接探测日志记录经校验的实际目标 URL，不记录 Key。

成功结果保存到本地数据库的租户配置表；弹窗显示实际获取地址、协议、最近获取时间及供应商用途声明数量。无需修改源码 YAML 或安装目录。不创建模型画像、不更换活动模型、不改参数、不下载模型、不发送 prompt。

## 逐型号参数画像

同步返回的每个 `models[]` 条目及目录读取的 `items[]` 条目新增 `parameter_profile`，随同一租户、环境、Provider、App 和连接指纹的目录快照原子保存并回读。刷新只更新型号元数据，不覆盖 `AIModelProfile.Defaults`、活动型号或页面内已编辑的参数草稿；上游失败仍保留上一份成功快照。没有参数信息的型号明确返回 `source=unknown` 和空 `defaults`。

```json
{
  "version": 1,
  "provider": "openai",
  "model": "gpt-6-luna",
  "source": "official_docs",
  "source_url": "https://developers.openai.com/api/docs/guides/latest-model",
  "checked_at": "2026-11-02",
  "defaults": {"reasoning_effort": "medium"},
  "sampling": "reasoning_none",
  "reasoning_efforts": ["none", "low", "medium", "high", "xhigh", "max"],
  "token_parameter": "max_completion_tokens"
}
```

- OpenAI `/models` 不返回生成参数。已核实型号关联版本化的官方文档目录，包含默认值、采样约束和输出上限字段；参数合同不等于型号用途、当前账号授权或真实调用验收。未知型号保持未知，不根据名称猜测用途。
- OpenRouter／兼容目录保留实际返回的 `default_parameters`、白名单 `supported_parameters` 与 `top_provider.max_completion_tokens`。Gemini 保留 `temperature`、`topP` 与 `outputTokenLimit`。只保存经过类型及范围校验的参数，不保存原始响应、密钥或任意扩展字段。
- `max_output_tokens` 表示供应商声明的最大输出上限，不会转成默认请求预算。没有明确默认预算时，表单使用标明为平台预设的预算；不得自动填入型号最大输出数。
- 表单选择型号后按“平台预设 → 型号默认值 → 此型号保存值 → 此型号未保存草稿”载入。草稿按环境、模态、Provider、App、型号隔离，连接凭据仍按 Provider 独立管理；切换租户清空型号草稿。`使用型号默认参数` 只重置当前文本参数，仍需点击保存才持久化。
- LLM 保存及试跑 DTO 支持可选 `reasoningEffort`，`temperature`／`topP` 缺省与显式零值不同。不支持采样时控件禁用，请求省略相应字段；新保存请求校验推理枚举及采样约束。既有保存值不会因目录刷新被删除。
- 型号目录、管理试跑、聊天配置解析和 OpenAI 文本／视觉驱动共用 `modelparams` 参数合同。已保存推理强度、Top P、零温度通过运行时配置及 Eino 转换传递；旧配置中的不兼容采样字段在发送前仍按型号合同处理。读取参数不会调用供应商或提交推理。

这次增加的是原有目录 JSON 的兼容字段，不新增数据库表、迁移、路由或插件服务授权。参数信息可以来自供应商 API 或已核实文档；`source` 和 `checked_at` 表达实际来源，不能将文档目录标成在线 API 返回值。

## 各 Provider 的获取方式

| Provider | 查询方式 | 鉴权／分页 |
| --- | --- | --- |
| OpenAI | Base URL 下的 `GET models`，默认 `/v1/models` | Bearer Key，保留 Organization |
| OpenRouter | Base URL 下的 `GET models?output_modalities=all` | Bearer Key，读取明确输入／输出模态 |
| DeepSeek | Base URL 下的 `GET models`，默认 `/models` | Bearer Key，读取明确输入／输出模态 |
| Moonshot | Base URL 下的 `GET models`，默认 `/v1/models` | Bearer Key，不根据型号名称猜用途 |
| Hugging Face Router | Base URL 下的 `GET models`，默认 `/v1/models` | Bearer Key，读取明确输入／输出模态 |
| OpenAI Compatible | 保存的兼容服务器的 `GET models` | Key 可选，已保存时会传递 |
| Qwen 国内／国际 | 同一 origin 的原生 `GET /api/v1/models` | Bearer Key；处理 `output.total/output.models` 分页 |
| Anthropic | 原生 `GET /v1/models` | `x-api-key`、`anthropic-version`；处理 `has_more/last_id/after_id` 与对象形式的 `capabilities.image_input` |
| Google Gemini | 版本路径下的 `GET models`，默认 `/v1beta/models` | `x-goog-api-key`；处理 `nextPageToken`；读取 `supportedGenerationMethods` |
| Ollama | 实例 `GET /api/tags`，逐型号 `POST /api/show` | 原样保留 tag，读取 capabilities，不加载模型 |

固定协议路径会保留自定义网关前缀。Qwen 将兼容路径后缀转换为原生目录路径；原生目录可能要求所在区域的 workspace 专属域名及相应目录权限。旧共享域名或第三方兼容网关以实际响应为准。

此轮未接入 Agnes、Volcengine、Hunyuan、Baidu、Coze、ComfyUI、Hash、Sentence Transformers 及 Azure deployment 模式的在线目录。主动同步返回 `AI_MODEL_SYNC_UNSUPPORTED`；普通下拉读取保留并标明本地目录。上游接口失败时显示错误码、实际查询地址、上游 HTTP 状态和可用 trace 信息。

## 视觉分析型号选择

主表单中 Provider 与 Model 输入框保持同一水平线；型号用途和目录来源收为一行状态，更新操作放在该行右侧。打开型号下拉框后再切换「推荐视觉／其他型号」并搜索，分组数量不占用表单标签空间。完整 ID、目录同步时间、用途依据及官方说明位于「型号详情」中，默认折叠；不适用错误始终可见。切换分组、展开详情与更新在线目录都保留当前型号和参数草稿。

视觉分析下拉默认展示「已确认支持视觉」，依据供应商明确的图片输入／文本输出声明，或已核对的 OpenAI 官方型号说明。「其他在线型号」保留全部其余在线 ID：未知用途允许显式保存和试跑；明确不支持图片输入、不输出分析文字、已下线或当前调用协议未适配的型号禁用，并显示具体原因。切换分组保留当前绑定与参数；没有确认型号时不自动选择在线列表第一项。

`GET agents/models` 与在线同步结果的每个 item 新增 `vision`：`status`、`reason_code`、`source`、`source_url`、`checked_at`。`source=provider_api` 表示目录接口明确声明；`official_docs` 是 `backend/internal/service/agent/openai_vision_usage.json` 中注明核对日期的官方说明快照，只按完整 ID 精确匹配。它只补充用途提示，不生成型号候选、不按名称前缀猜测，也不冒充 `/models` 的返回字段。新型号没有用途资料仍进入「其他在线型号」，不因资料快照较旧而被禁止选择。扩充资料时需逐项核对官方输入／输出与当前驱动协议，并更新日期及回归；不需要修改 YAML 白名单。

已核对的例子：GPT-5.4 Mini、GPT-5.4、GPT-5.4 nano、GPT-4o／4o Mini、GPT-4.1 Mini 和 GPT-6 Luna 可做图片输入／文字分析；只展示实际在线返回的 ID 及已明确登记的快照。`tts-1` 不接受图片，`gpt-image-2` 输出图片而非分析文本；GPT-5.4 Pro 仅支持 Responses API，当前视觉驱动使用 Chat Completions，因此提示协议未适配。说明链接可在选中项与在线目录中查看。

后台保存校验、typed 媒体目录和真实视觉调用采用相同用途结论；禁止用途有明确错误码。旧版保存的不适用绑定保留并提示用户更换，不会自动修改模型、凭据或图像生成配置。发现用途不等于真实推理成功；服务目录继续单独返回 `configured`、`available` 和 `inference_verified=false`，实际可用性通过「试跑图片」验收。新字段在读取旧快照时补齐，无需迁移、重新同步或改动 grants。

## 在线型号与调用方式

- 型号来源优先采用当前连接的完整在线结果，原样保留供应商 ID，不与本地样例合并、不按本地型号白名单截断。界面按「全部在线／供应商已声明用途／用途未声明」查看；用途为空的型号也可以在当前模态下选择并保存。OpenAI `/v1/models` 不提供图片输入等用途字段，因此其清单标为「用途未声明」，不能把本地登记或管理员选择显示为供应商声明。
- 有明确元数据时保留 `reported_modalities`、`input_modalities`、`output_modalities`。例如 OpenRouter 的图片输入加文本输出可表达视觉分析，Anthropic 的结构化 `image_input.supported` 可以表达视觉输入。型号名相似、ID 前缀和旧人工归类都不构成供应商用途证据。
- 本地 YAML 继续维护接入协议、驱动、参数 Schema、默认值和已知型号的专用参数适配。新在线 ID 使用当前模态已有的固定驱动模板，不能提交任意 endpoint、method、headers 或代理负载。未知视觉型号通过已有 OpenAI／Qwen 视觉协议模板保存；不支持该模态驱动时明确报错。目录发现不会安装新驱动。
- 「保存当前用途」创建该租户、环境、模态的明确画像，未知媒体型号记录 `online-usage-selected`。仅存在 Provider 凭据不会把全部在线 ID 自动标为已配置。服务能力目录同样读取在线型号，返回操作 Schema／defaults、`configured`、`available`、`reason_code`、`usage_source`，并明确 `inference_verified=false`。其中 `available` 表达驱动、配置和当前权限满足调用前置条件，不能当作真实推理成功的证明。
- 选择不等于运行：只有用户单独试跑或正式调用才发送推理请求。供应商仍可能因型号不支持当前用途、账号权限或参数限制拒绝；应按实际错误调整用途或型号。刷新和目录浏览不提交 prompt，不切换活动型号，不改变其他模态配置。
- 已有匹配连接的快照时普通下拉读取该快照，并展示来源和获取时间；手动刷新完整获取后原子替换。失败保留上次成功快照并显示错误，不混入本地样例。没有快照或连接身份已变更时，鉴权、网络、分页失败都明确返回错误，不静默降级成本地列表。
- 只有当前接入方式尚未声明在线目录适配时才保留本地目录，并显示本地来源。这不等于断言供应商没有任何型号 API；新增供应商目录协议需要独立接入。已支持协议的 401／403、404、超时不能当作「没有 API」的证据。

快照按租户、环境、Provider、App 保存，并绑定连接身份指纹。换 Key 或地址后自动获取新连接目录；其他租户／环境不能继承。Key 不写 URL、日志、目录快照，指纹不进入 API 响应。保存新型号时必须在该连接在线结果内，防止任意伪造型号；本地已登记但在线未返回的样例也不能绕过已有在线快照。尚无在线快照的旧配置允许启动，首次连接探测不依赖型号存在性。

管理页 `GET /admin/agents/models` 保持 `models: string[]`，增加 `catalog_source`、`updated_at`、`source` 和 `items`；插件 typed `list_settings_models` 继续以原有 `items` 结构返回同一来源的型号，不改变该操作 DTO。Ollama 原有实例实时发现语义保留：普通下拉读取实际实例，在线同步另存快照。媒体插件的 `list_models` 继续使用固定 typed operation 和原 API Key／STS 授权，不扩大 Profile 或授予管理权限。

## 管理 API

逻辑路由 `POST /admin/agents/models/sync`，API 前缀沿用 Core 配置。只允许用户 JWT 的 Root 或当前租户 owner/admin，拒绝 API Key、STS 和普通成员。它是管理配置入口，已列入能力审计 ignore；不新增插件服务授权或扩大共享 Profile。

```json
{"env":"dev","provider":"openai","app":"","modality":"llm","action":"refresh"}
```

旧版 `assign/unassign` API 为兼容历史客户端保留 revision／单项校验，但不再控制型号是否出现在下拉框，也不代表真实供应商用途。新版页面不调用该入口；用户在相应模态选择并按原保存接口保存配置即可。管理 Body 不接受任意 endpoint、headers、method、Key 或身份字段。

目录查询使用 `capability_registry.default_http_timeout_seconds`，缺省 20 秒；调用方更早的 deadline 仍优先。它与 LLM 推理期限分开。最多 100 页、10000 个型号、每页 8 MiB；拒绝重定向、畸形及不完整分页，完整获取后才保存。

## 验收与恢复

更新失败时看弹窗错误码和上游地址；检查已保存连接的 Key、区域、目录权限及接口支持，然后重试。不会自动替换活动模型；如不需要新候选，保持原型号即可。改变连接身份后旧快照自动失效。

目录请求的 401／403 对前端返回 `AI_MODEL_SYNC_AUTH_FAILED`（Core HTTP 422，避免触发用户 JWT 刷新），详情保留 `source`、`provider`、`protocol` 与 `upstream_status`。供应商明确通知旧版免费 Key 已停用、需绑定 GitHub 时，仅返回固定分类 `upstream_reason=legacy_free_key_retired`，页面提示获取并保存新版 Key；不转发供应商原始错误消息、Key 或账号信息。失败日志带同一请求上下文及上述诊断字段。连接测试结果属于当前表单，切换环境／模态／Provider、修改配置或离开页面后失效，旧请求不得回写成功或失败提示。

受控验收覆盖协议路径、鉴权、分页、原样 ID、显式用途元数据、完整清单、持久化回读、原生未知型号保存与受控视觉调用、跨租户／环境／模态隔离、Key 变更失效、上游和写入失败保留旧值，以及桌面／窄屏弹窗与无自动推理请求。真实账号的目录权限、网络可达性和生成支持，需在重启后的 Core 用对应连接分别验收。

官方协议参考：[OpenAI](https://developers.openai.com/api/reference/resources/models/methods/list)、[OpenRouter](https://openrouter.ai/docs/api/api-reference/models/list-all-models-and-their-properties)、[DeepSeek](https://api-docs.deepseek.com/api/list-models/)、[Hugging Face](https://huggingface.co/docs/inference-providers/hub-api)、[DashScope](https://www.alibabacloud.com/help/en/model-studio/list-models)、[Anthropic](https://platform.claude.com/docs/en/api/models/list)、[Gemini](https://ai.google.dev/api/models)、[Ollama](https://docs.ollama.com/api/tags)。


## 视觉分析的真实图片试跑

在「视觉分析」选择型号并填写当前连接后，直接点击连接检查区的「试跑图片」，无需先保存。先选一张 PNG／JPEG／WebP（不超过 5 MB），或填写供应商可读取的 HTTP(S) 图片 URL，再输入识别要求。上传图片在浏览器内编码，不写入媒体库或浏览器持久存储；URL 预览由用户主动触发。只有「开始识别」提交一次真实模型请求并产生用量，打开弹窗、预览、切换分区和原「检查参数」均不执行推理。

「图片与要求」「识别结果」分区保留本次草稿。结果显示分析文字、实际型号和耗时；展开技术详情可查看 trace 和 usage。失败保留输入并显示错误码、上游状态和可用的 provider trace。关闭弹窗、停止等待或离开页面会中止等待并丢弃迟到结果，不承诺供应商已停止计费；切换环境／租户／登录身份清空图片草稿，普通 token 刷新不清空草稿。

管理入口复用 `POST /admin/agents/test/call`，API 前缀沿用 Core 配置。`execute` 缺省为 false；显式为 true 时只支持 `modality=vlm`。新页面提交 `useFormConnection=true` 以及当前表单的 Key、Base URL、认证方式、组织及参数，不提交旧的 `reuseCredential` 标记。旧 Core 会在推理前拒绝这一请求，页面提示重启升级，避免忽略新标记后误用旧连接。调用复用正式视觉用户 RBAC、图片校验、固定驱动、参数 Schema 和分析解析；管理试跑不经过已保存画像或在线目录准入，服务凭据不能使用这一管理试跑入口。它是管理测试入口，不新增服务 capability，不修改插件 grants。

```json
{
  "env": "dev",
  "modality": "vlm",
  "execute": true,
  "useFormConnection": true,
  "vlm": {"provider": "openai", "model": "gpt-6-luna", "baseURL": "", "maxTokens": 2048, "reasoningEffort": "none", "jsonMode": false},
  "inputs": [
    {"type": "text", "content": "描述图片并提取清晰可见的文字"},
    {"type": "image_url", "url": "https://example.com/picture.png"}
  ]
}
```

当前表单字段优先：填写新 Key 时仅使用新 Key；Key 留空才读取当前租户、当前环境已保存的 Key。Base URL 显式留空使用供应商默认地址，不继承旧自定义地址；组织、认证方式和 deployment 也按当前表单值处理。没有已保存型号画像、没有目录快照或整个连接尚未保存，都不阻止明确发起的试跑。测试期间不写入凭据、型号画像、活动路由或在线目录；只有点击页面「保存」才持久化。检查参数使用当前型号的固定协议 Schema，不依赖已保存连接，也不发起推理。

成功响应必须同时具有 `ok=true`、`generation_submitted=true`、`result.type=analysis`、非空 `text` 和 `trace_id`，并返回 `env`、`elapsed_ms` 以及可用的 `usage`。连接或参数检查成功不能当作识别成功。旧 Core 若仍返回 `generation_submitted=false`，页面明确提示 `AI_VISION_TRIAL_NOT_SUPPORTED`，需要重启更新后的 Core。

试跑遵守 `capability_registry.ai_multimodal_http_timeout_seconds` 配置（通过启动依赖传入）；前端此请求不使用通用 30 秒超时，也不自动重试。审计记录调用人、租户、环境、型号、成功与否及 trace，不记录图片、完整提示词或识别正文。该入口使用当前表单的连接与显式环境；只有 Key 留空才读取同租户、同环境凭据，不修改租户运行环境。
