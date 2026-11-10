# AI 媒体能力合同 v2：Core 与 Framework 对齐

状态：Core 实现与受控验收；不代表已完成真实供应商、部署实例或安装态验收。

## 职责与版本

Core 维护供应商凭据、型号能力、参数校验、媒体任务、文件转存和任务推送。Framework 按本文和 `ai-media-v2.openapi.yaml` 封装类型与运行通道。MediaXStudio 保留页面、业务编排、尝试记录与成果选用。

VLM 同步返回分析内容；Image、Video、TTS 的 v2 调用统一提交到正式 TaskBus，返回稳定 Core 任务。同步供应商也由 Core worker 执行并转存，完成后使用相同成果结构。调用方不得因供应商同步／异步而切换运行通道。

这是显式版本升级：媒体调用要求 `contract_version: 2`。缺失版本返回 `AI_CONTRACT_VERSION_REQUIRED`；不再把旧自由 REST 代理 payload 当作服务绑定。现有 Admin 字符串选模接口继续服务 Admin 页面，服务客户端迁移到下述 typed binding。

## 能力与绑定

所有服务绑定为固定 `core_internal`、`INVOKE`，优先通过 `/api/v1/tenant/invocations` 调用。`payload` 只允许 `body`；拒绝调用方自选 method、endpoint、headers、query、context 和未声明字段。

| capability_id | Core endpoint | operation | API Key scope |
|---|---|---|---|
| `com.corex.ai.catalog.providers.read` | `core://ai/model-catalog` | `list_providers` | `_scope.ai.catalog.providers.read` |
| `com.corex.ai.catalog.models.read` | `core://ai/model-catalog` | `list_models` | `_scope.ai.catalog.models.read` |
| `com.corex.ai.vlm.invoke` | `core://ai/media` | `invoke` | `_scope.ai.vlm.invoke` |
| `com.corex.ai.image.invoke` | `core://ai/media` | `invoke` | `_scope.ai.image.invoke` |
| `com.corex.ai.video.invoke` | `core://ai/media` | `invoke` | `_scope.ai.video.invoke` |
| `com.corex.ai.tts.invoke` | `core://ai/media` | `invoke` | `_scope.ai.tts.invoke` |
| `com.corex.ai.media.tasks.read` | `core://ai/media-tasks` | `get` | `_scope.ai.media.tasks.read` |
| `com.corex.ai.media.tasks.cancel` | `core://ai/media-tasks` | `cancel` | `_scope.ai.media.tasks.cancel` |

服务绑定显式声明 API Key 元数据。执行时检查 published capability、租户 registration、当前 API Key grant 或已安装插件的实时 STS grant；撤销后旧 token 同样失去访问权。任务查询和取消还校验租户及创建者 credential subject。同租户的另一个 API Key／插件不能认领任务。

## 已安装实例的旧注册升级

仅发布 YAML 或确认 `/upgrade` 哈希不会替换历史 adapters。安装态数据库升级的正式 `catalog` 步骤现在同步上述八项媒体能力的已有注册：识别 Core 所有的已知 HTTP／gRPC 旧绑定，追加新版本，统一 `contract_ref=docs/contracts/ai-media-v2.openapi.yaml`，服务注册只保留 `core_internal`／`INVOKE`。Admin REST binding 仍发布在正式能力记录中，继续使用用户 JWT/RBAC。

扫描覆盖所有已有媒体注册所在租户，包括禁用租户；只给活动租户补建缺失能力。保留注册状态、禁用原因、adapter 禁用位、环境开关、可见范围、限流和已有 Tool Grants；不重建凭据，也不添加角色、Profile、API Key 或 STS grants。自定义来源、未知 contract_ref 或非标准 endpoint 返回 `UPGRADE_CAPABILITY_SOURCE_CONFLICT`／`UPGRADE_CAPABILITY_BINDING_CONFLICT`，整笔事务回滚。

升级在同一事务内写入目录、注册新版本及 `UPGRADE_MEDIA_REGISTRATION` 审计，使用租户＋能力锁及 expected version 防止并发追加同版快照；重复执行不产生新注册版本或重复升级审计。历史版本保留。正式升级载荷清单必须使用 `upgrade_registry.py` 重新生成并通过 `--check`，新内容产生新迁移 ID，不改历史履历。

本地开发或受控目录修复可复用正式 Seeder，仅选择这八项能力、仍覆盖全部受影响租户：

```bash
make capability-seed CAPABILITY_SEED_CONFIG=etc/config.yaml CAPABILITY_SEED_FLAGS="-capability-ids com.corex.ai.catalog.providers.read,com.corex.ai.catalog.models.read,com.corex.ai.vlm.invoke,com.corex.ai.image.invoke,com.corex.ai.video.invoke,com.corex.ai.tts.invoke,com.corex.ai.media.tasks.read,com.corex.ai.media.tasks.cancel"
```

该命令只选择要修复的能力，不接收租户 UUID；权限元数据与注册升级同事务提交。省略 `-capability-ids` 才同步完整正式目录。安装态发布继续使用现有 migration plan/apply 流程，不用完整 seed 或重装插件替代升级。

运行中的实例读取目录缓存时校对数据库修订，读取发现快照时校对本地注册版本；Router 根据最新注册清除旧版本健康／sticky 状态，固定 Core 通道不回退到 HTTP 或静态响应。跨区域 replica 继续遵守原复制合同。数据库不可读时发现快照只可标记为 stale，不代表允许执行；执行授权仍查实时数据库。

已有版本锁仍要求管理员在正式 `/upgrade` 接口确认当前发布的 `capabilities_hash`。确认前验证注册已切换到正式 typed binding；未迁移返回 409 `capability.upgrade_binding_mismatch`。不能清空 Redis 或改锁绕过确认。运行二进制也必须包含媒体 v2 执行器；旧进程返回 unsupported capability 时需要由部署负责人按现有启动方式重启。

验收先用升级前的原凭据读取四类供应商和型号目录，分别核对 `contract_version=2`、`protocol_used=core_internal`、`fallback_used=false`，然后检查全部八项 grant-status。真实生成与费用额度仍单独验收；目录读取不会提交生成任务。

### 升级确认的结构化错误

`POST /api/v1/admin/capability-registry/capabilities/{capabilityId}/tenants/{tenant_uuid}/upgrade` 是人工管理员 JWT 确认入口，API Key／STS 不得代为审批。下面约定适用于通过前置鉴权、到达升级确认 handler 的错误；前置认证／Admin 中间件拒绝仍沿用其既有错误合同。

保留原有 `message`：入口解析失败为 `capability.upgrade_invalid`，入口拒绝 API Key 为 `capability.upgrade_forbidden`，Service 返回的失败继续为 `capability.upgrade_failed`。具体机器码同步写入 `error_code`、`reason_code`、`details.reason_code`，不再要求从 `message` 或 `error` 字符串解析。`error` 只包含公开码，不包含原始 SQL、运行存储错误或用户输入。`request_id` 保持标准回包规则；当前 trace 存在时由 `details.trace_id` 返回。

| HTTP | 机器码 | 含义 | retryable |
|---|---|---|---|
| 400 | `capability.upgrade_invalid` | 请求格式、租户、hash 或 reason 无效 | false |
| 403 | `capability.upgrade_forbidden` | 调用方不能确认升级 | false |
| 404 | `capability.upgrade_not_found` | 发布记录或当前有效注册不存在 | false |
| 409 | `capability.upgrade_hash_mismatch` | 确认的 hash 与当前发布不一致 | false |
| 409 | `capability.upgrade_binding_mismatch` | 注册尚未升级为正式 typed binding | false |
| 503 | `capability.upgrade_unavailable` | 升级服务依赖未准备好 | true |
| 503 | `capability.upgrade_persistence_failed` | 读取注册或提交审批审计失败，未进入运行锁确认步骤 | true |
| 503 | `capability.upgrade_runtime_apply_failed` | 审批审计已提交，运行锁确认调用失败，实际运行锁状态不可据此推断 | true |
| 500 | `capability.upgrade_audit_finalize_failed` | 运行锁确认已成功，但审计最终状态更新失败 | true |
| 500 | `capability.upgrade_failed` | 未分类内部失败 | false |

`retryable=true` 表示修复依赖后可以人工重新确认，不授权客户端自动重试审批。审计已确认持久化时附带既有 `audit_id` 和 `approval_persisted=true`；仅在运行锁确认已返回成功、审计收尾失败时附带 `runtime_confirmed=true`。这不改变“先提交审批审计，再确认运行锁”的顺序，也不绕过旧注册迁移、租户授权或版本锁。

权威样例为 [capability-version-upgrade-errors.v1.json](fixtures/capability-version-upgrade-errors.v1.json)。这是已有标准错误信封的细化，媒体 OpenAPI 和 `fixtures/ai-media-v2.json` 的结构不变；Framework 应同步样例和预期 reason，继续原样保留状态、机器码、trace 和 request_id。合同来源与摘要见 [ai-media-v2-source.json](ai-media-v2-source.json)，文件摘要才是当前未提交源码的精确基线。

2026-10-22 本地原 MediaXStudio API Key 已通过上述八次目录读取和八项授权检查；新增的六项授权仅追加到该 Key，原有授权及共享 Profile 保留。`qwen-cn/qwen-vl-max`、`qwen-cn/qwen-image-2.0-pro`、`qwen-cn/qwen3-tts-flash` 已登记在该租户当前 `dev` 环境，目录均返回 `configured=true`、`available=true`。视频凭据由维护人员在 Core 配置后继续核验，未配置时明确返回 `AI_MODEL_NOT_CONFIGURED`。

目录可用不代表任务存储已迁移。本地初查因缺少 `public.ai_media_tasks`，原 Key 对不存在任务的查询／取消返回 502。维护人员随后执行既有 `make db-migrate`，日志在 2026-10-22 20:56:05（UTC+8）记录 `migrate ok`；复查确认任务表、幂等索引已建立，原 Key 查询／取消不存在任务均返回 404 `AI_TASK_NOT_FOUND`，目录、授权及三个已配置型号仍通过。既有入口同步全部 Core／Agent 及按配置启用的 Knowledge 模型，需要按实际数据库维护安排执行；不能在运行时或 GET 中自动建表。本次旧注册升级与授权配置不新增 Framework 方法或合同字段，继续使用已对齐的 v2 客户端。详见 [本地联调证据](evidence/ai-media-local-setup-validation-20261022.json)；该记录不包含付费任务生命周期或安装态 STS 验收。

`/api/v1/admin/agents/models` 与 `/providers` 为 Admin 用户 JWT/RBAC 平面，不能作为 API Key/STS 的自由代理入口。型号 v2 Admin 视图为 `GET /api/v1/admin/agents/models?view=capabilities&provider=...&capability=...`。API Key/STS 客户端不得请求此管理路径。

媒体服务 REST 入口：`POST /api/v1/ai/{vlm|image|video|tts}/invoke`。新任务路由：

- `GET /api/v1/ai/media/tasks/{task_uuid}`。
- `POST /api/v1/ai/media/tasks/{task_uuid}/cancel`，无请求体。
- `GET /api/v1/ai/media/tasks/{task_uuid}/events`，SSE。

服务态媒体 v2 请求禁止通过 query、payload 或 `X-PowerX-Env` 选择环境。环境读取目标租户的 `ai.current_env`；通道由 Framework 启动配置确定。未配置环境明确失败，不回退到其他环境或供应商。

## 精确模型目录

模型目录 body 示例：

```json
{"operation":"list_models","provider":"qwen-cn","capability":"image_generation"}
```

返回 `result: {contract_version: 2, items: [...]}`。每项包含 `model_key/provider/model/app/label/capabilities/configured/available/reason_code/operations`。无型号时 `items: []`，不从 image/video/audio 其他目录回填，不从型号、标签、通配条目或探测缓存猜能力。

本目录只返回已声明媒体 operation 的型号。LLM 使用既有 `ListLLMModels` 专用入口；不能把未声明媒体能力的 LLM 目录条目转换成可执行媒体型号。旧 CatalogModels/CatalogProviders 的 Admin REST 代理调用必须迁移，旧 payload 会明确失败。

与 AI Settings 管理页一致的 LLM 配置候选目录使用独立的 `list_settings_providers`／`list_settings_models` 操作，正式合同见 [AI Settings 文本目录 v1](ai-settings-llm-catalog.v1.md)。它们复用相同的两项目录读取能力与固定 `core://ai/model-catalog` 绑定，返回 `catalog_kind=ai_settings, contract_version=1`；现有媒体操作仍返回 v2，不能混用请求字段或结果。

v2 能力名为 `vision_understanding/image_generation/video_generation/speech_synthesis`。原有 `image_understanding` 仅作为请求筛选别名；旧 LLM 目录维持原声明。`model_key` 为 opaque key，必须原样回传；`app` 单独提供，禁止客户端自行拆冒号，供应商原生型号可以包含冒号。

`operations[capability]` 包含：

- `parameter_schema`：JSON Schema 的 object/type/properties/enum/minimum/maximum/default 子集，`additionalProperties: false`。
- `defaults`：该型号、该操作的有效默认参数；Framework 先据此建立表单。
- `input_types/max_images/max_videos`：实际接入支持的输入集合及上限。
- `max_image_bytes/max_total_image_bytes/max_image_pixels`：可选的参考图解码字节及像素限制；与图片数量共同生效，不能只检查数量。

- `available/reason_code`：该操作在当前凭证下的可执行判断。
- `async`：供应商适配器是否使用原生异步任务；Core 生成接口均返回 Core 任务。

OpenAI GPT-Image-2 的参考图协议、限制及受控验收见 [参考图输入合同](openai-image-reference.md)。

`configured` 表示目标租户具有有效的型号配置和可解封凭据；`available` 还要求当前调用主体具备相应生成授权。这是调用前的配置／授权判断，不承诺供应商实时成功或余额充足。调用时再次验证。

当前明确接入的媒体 operations 包括 Qwen 已声明 VLM 型号、Qwen Image 2.0 系列的文生图、Qwen3 TTS Flash、OpenAI 已登记图像生成型号，以及 Seedance 已登记视频型号。未声明 operation 的供应商目录条目不宣称可执行。Qwen TTS 当前仅声明已适配的 voice/language_type，不把不支持的语速、输出格式参数静默接受。

### Agnes AI

`provider=agnes`，Bearer API Key 由 Core 保存，默认 Base URL 为 `https://apihub.agnes-ai.com/v1`。供应商配置入口为 Core 的 `/settings/ai`，在目标租户当前环境分别保存图像、视频型号。Framework 与 MediaXStudio 继续使用现有 v2 目录、调用和任务客户端，传回目录给出的完整 `model_key`。

| model_key | 能力 | 本轮支持的参数／引用 |
|---|---|---|
| `agnes/agnes-image-2.5-flash` | `image_generation` | `size` 为 1K／2K／3K／4K，`ratio` 为目录枚举；最多三张参考图，可使用公开 URL 或 Data URI |
| `agnes/agnes-image-2.1-flash` | `image_generation` | 与 Image 2.5 Flash 使用相同请求合同 |
| `agnes/agnes-video-2.5` | `video_generation` | `resolution` 为 720p／1080p／2K，`max_duration_sec` 为 4–12 的整数，`aspect_ratio` 与 `mode` 为目录枚举；最多八张参考图 |
| `agnes/agnes-video-2.5-flash` | `video_generation` | 分辨率仅 720p，其余通用参数同上；最多五张参考图 |

图像请求的返回编码和参考图放入供应商要求的 `extra_body`。图片同步供应商响应仍经过 Core TaskBus 和媒体入库后返回成果。视频采用原生异步提交，必须保存响应的 `video_id`，不能用 `id`／`task_id` 替代；查询固定在同一供应商的 `/agnesapi`，携带 `video_id` 与 `model_name`。只有顶层 `status=completed` 且包含 `url` 时接纳结果，不依据 `internal_status` 判断完成。

视频 `mode=text` 不接受参考图，`mode=reference` 必须有参考图，`mode=keyframe` 按输入顺序将一到两张图绑定为首帧、尾帧。模式组合在创建 Core 任务前验证。视频只接受可被供应商读取的 HTTP(S) 图片地址，本轮未声明音频或视频参考输入。原生 1K 视频固定为方形，与通用比例参数的语义不同，本轮不发布此档位；显式传入时返回参数错误。未实现的参数（如 FPS、生成质量和 seed）不能显式传入。Agnes 未发布原生取消协议：Core 排队中可以取消，已提交时返回 `cancel_status=unsupported` 并继续读取真实结果，不发送推测的 DELETE。查询、刷新和客户端重连均不重复生成。

Core 配置页为 Agnes 显示对应尺寸、分辨率和时长范围，隐藏该接入未支持的质量、编码格式与 FPS；实际创作的比例、模式等表单仍由消费方读取 operation Schema 构建。`configured/available` 沿用当前配置、驱动与授权判断，不能替代真实供应商调用验证。

协议依据：[Image 2.5 Flash](https://www.agnes-ai.com/en/docs/agnes-image-25-flash)、[Image 2.1 Flash](https://www.agnes-ai.com/en/docs/agnes-image-21-flash)、[Video 2.5](https://www.agnes-ai.com/en/docs/agnes-video-25)、[Video 2.5 Flash](https://www.agnes-ai.com/en/docs/agnes-video-25-flash)。本轮受控验证见 `evidence/agnes-provider-core-validation-20261022.json`；不包含真实 Key 的供应商调用或桌面／窄屏浏览器验收。

## 调用、幂等与成果

Typed body 示例：

```json
{
  "operation":"invoke",
  "contract_version":2,
  "model_key":"volcengine/seedance:doubao-seedance-1-0-pro-250528",
  "inputs":[{"type":"text","content":"生成测试视频"}],
  "params":{"resolution":"720p","max_duration_sec":5},
  "idempotency_key":"project-uuid-shot-uuid-attempt-uuid"
}
```

REST 也可使用 `Idempotency-Key` header；与 body 同时提供时必须一致。幂等键作用域为租户＋credential subject＋能力。同键同请求返回同一个任务，同键不同请求返回 409 `AI_IDEMPOTENCY_CONFLICT`。查询、订阅和重连只读状态，绝不重复生成。网络错误／503 后沿用原键重试，禁止自动换新键重提。

VLM 返回 `type: analysis`、`text`、provider/model、usage、trace_id。生成调用的 REST 返回 HTTP 202，data 为任务；Core binding 的 `result` 为同一任务类型：

```json
{
  "contract_version":2,
  "task_uuid":"11111111-2222-4333-8444-555555555555",
  "status":"queued",
  "phase":"queued",
  "model_key":"volcengine/seedance:doubao-seedance-1-0-pro-250528",
  "capability":"video_generation",
  "revision":1,
  "trace_id":"core-trace-id",
  "cancel_status":"none",
  "assets":[],
  "created_at":"2026-10-19T00:00:00Z",
  "updated_at":"2026-10-19T00:00:00Z"
}
```

状态为 `queued/running/succeeded/failed/cancelled`。`phase` 区分 `queued/submitting/waiting_provider/materializing/completed`。Core 重启后通过同一 task_uuid 读取；worker 从 TaskBus 注册历史与租约恢复。异步任务默认最多执行 24 小时。

仅在供应商产物下载、大小／类型／SHA-256 校验和媒体入库完成后标记 succeeded。`assets` 为数组，每项包含 `media_asset_uuid/type/mime_type/size_bytes/checksum/checksum_algorithm`。资产 UUID 在同一任务中稳定；通过现有媒体服务读取或重新申请下载凭证。供应商 URL、原生 task_id、poll_url 和供应商凭据不出 Core，下载 URL 的有效期不等于资产标识的生命周期。

在供应商不支持提交幂等的情况下，Core 不能承诺供应商侧 exactly-once。提交前先持久化 submitting；若崩溃后没有取得原生句柄，任务以 `AI_SUBMISSION_OUTCOME_UNKNOWN` 明确失败，绝不盲目重提。该错误不保证供应商没有接单或没有计费，应由部署负责人核查后决定新尝试。

## 取消与推送

取消结果通过 `cancel_status` 表达 `none/requested/confirmed/unsupported`。排队中尚未提交的 Core 任务可直接 confirmed。供应商任务仅在供应商确认取消后进入 cancelled；请求取消不承诺停止计费或退款。Seedance 仅取消 queued，running 明确 unsupported；不会对已完成任务调用 DELETE 删除原生记录。

SSE 必须接入页面常驻状态更新，不能用详情／列表轮询替代：

- `status` 事件携带完整任务快照，`id` 为 `task_uuid:revision`。
- 先订阅再读取初始快照，事件到达后读取最新状态；按 revision 去重。
- `Last-Event-ID` 只放 header。重连始终返回最新完整快照，允许跳过中间阶段，不提供完整事件日志重放承诺。
- 心跳是注释 `: keepalive`，不会查询任务详情；授权会在心跳时复核。
- `reconnect` 表示刷新鉴权后有上限退避重连；`error` 表示访问撤销，结束连接。
- 流在 token 过期前或最多五分钟后要求重新鉴权；Redis 断线后重连并同步快照。
- 连接与快照均有租户／创建者隔离，身份切换和离页必须关闭旧订阅。不将 token 放 URL 或浏览器持久存储。

## 结构化错误

保留 HTTP 状态、Core `error_code/reason_code`、`trace_id/request_id`、参数字段及错误原因；存在供应商响应时保留 `upstream_status/upstream_code/provider_trace_id`。异步失败在任务快照中保存错误码与供应商追踪字段，不把供应商原文当作公共文案。

REST 错误使用标准 envelope；网关错误也保留 `details.reason_code` 中的 AI 错误码。读取响应 `X-Trace-ID`、`X-Request-ID` 及 body 的 request_id，不能只解析 message 或把所有错误统一改写为 502。

常见错误：`AI_PARAMETER_INVALID`（details.parameter/reason）、`AI_INPUT_UNSUPPORTED`、`AI_INPUT_LIMIT_EXCEEDED`、`AI_MODEL_CAPABILITY_UNSUPPORTED`、`AI_MODEL_NOT_CONFIGURED`、`AI_ENV_NOT_CONFIGURED`、`AI_IDEMPOTENCY_CONFLICT`、`AI_SUBMISSION_OUTCOME_UNKNOWN`、`AI_PROVIDER_TASK_FAILED`、`AI_MEDIA_TRANSFER_FAILED`。未授权／撤销保持 401/403，不降为 502 或伪造空目录。

## Framework 必须对齐的工作

1. 在既有 AI Settings Catalog 与 GenerativeService 中添加 v2 目录、operation、分析结果、任务、成果、取消状态、结构化错误类型。CatalogModels 不再以 `[]string` 承载服务态媒体目录。
2. 使用上述固定 Core binding，发送 typed body，不再发送 method/endpoint/query 的 Admin REST 代理 payload；保留 Admin 用户与 service actor 的边界。
3. VLM/Image/Video/TTS 请求添加 contract_version=2；生成请求携带稳定幂等键。Image/TTS 也要接收 Core 任务，不能假定立即获得图片／音频 URL。
4. 增加任务 get/cancel；根据正式 SSE 路由订阅完整快照，支持 header 鉴权、Last-Event-ID、revision 去重、取消与身份切换后的释放。
5. 完整保留错误状态、Core/供应商错误码和 trace。local/delegated 启动通道保持固定；local 未接入这些能力时明确 unsupported，不能退到另一个通道。
6. 使用 `docs/contracts/fixtures/ai-media-v2.json` 同一份样例测试 Core 与客户端。受控 API Key/STS 验收与真实安装态／付费供应商验收分别报告。

Framework 对齐位置（本轮仅核对，未修改 Framework）：`framework/backend/go/runtime/aisettings/{client,types}.go`、`framework/backend/go/runtime/powerx/ai/{client,types}.go`、`framework/backend/go/runtime/ai/runtime.go`。建议新增媒体目录与任务专用类型／方法，避免继续用 `ModalInvokeOutput.Data` 承载不受约束的原始 JSON；SDK、Skeleton 与模板的调用示例一并更新。

最低受控验收矩阵：API Key 与 STS 各覆盖目录和调用、稳定幂等键重试、重新连接后读取同一任务、任务 get/cancel、撤销授权、跨租户／跨主体拒绝、SSE revision 去重与断线同步、错误状态／字段／trace 保留。后续真实验收再覆盖参考图分析、图片、配音播放与异步视频播放。

## 部署与验收边界

新增表通过 `pkg/corex/db/database/migration.go` 集中挂载；运行时启动不执行 AutoMigrate。部署时执行正常数据库迁移、`make capability-check`、`make capability-seed`，再检查 Host API Key 的 grant-status；授予目录、对应生成、任务读取／取消及所需媒体上传／读取能力。保留用户现有服务，重编译与重启由部署负责人执行。

本轮验收使用隔离数据库、真实 TaskBus 驱动＋受控 Redis、受控供应商 HTTP、真实媒体本地存储。真实供应商调用、余额、租户权限和部署运行态由后续单独验收确认。

供应商适配依据：[Qwen Image 官方文档](https://www.alibabacloud.com/help/en/model-studio/qwen-image-api)、[Qwen TTS 官方文档](https://www.alibabacloud.com/help/tc/model-studio/qwen-tts-api)、[Seedance 取消语义](https://docs.volcengine.com/docs/ark/cancel-or-delete-video-generation-tasks-api)。
