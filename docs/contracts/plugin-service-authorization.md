# 插件权限判定与 Core 服务调用身份

## 两个独立问题

Core 判断租户成员是否拥有正式登记的插件权限，并判断调用服务是否被授权。
目标插件判断该主体能操作哪些具体业务对象。查询成员权限不会授予查询方该成员身份，
本合同不包含代用户执行，也不包含 SCRM 的消息收发实现。

## 查询成员插件权限

使用现有能力 `com.corex.iam.authorization.check`，固定绑定
`core://iam/authorization`、`INVOKE`、`preferred_protocol=core_internal`：

```json
{
  "capability_id": "com.corex.iam.authorization.check",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "operation": "check",
      "request": {
        "plugin_id": "com.example.messaging",
        "permission_code": "messages.conversation:reply",
        "member_uuid": "成员 UUID",
        "user_uuid": "该成员关联的用户 UUID"
      }
    }
  }
}
```

也支持 `POST /api/v1/tenant/iam/authorization:check`，body 为上面的 request。
旧的 `resource=iam.member, action=read` 查询保留。
插件查询必须同时提供 `plugin_id`、`permission_code`；可选 resource/action
如提供，必须与登记的权限一致。权限来自 IAM 的 `source=plugin:<id>` 正式同步记录。
API route 引用的 business permission 应查询其正式业务权限码，而非 route 的别名。

调用方需要查询能力的 published + 当前租户 registration + 实时 grant。
查询自身所属插件的权限，或同时获得目标业务能力的实时 grant，才允许查询。
API Key 的 caller owner 来自显式 owner 配置，不从“被授权调用的插件”反推。
租户从经过认证的上下文取得，不接受 tenant、身份或自由代理覆盖字段。

成功返回 `allowed`、`reason_code`、tenant/member/user UUID、plugin_id、permission_code、
resource/action。按实时有效 UUID 角色绑定（含期限）与当前权限状态判定，禁用成员／用户不能获得允许。
`IAM_PERMISSION_DENIED` 是正常的 denied 结果；未登记权限返回 404，越权查询 403，
依赖故障 `IAM_UPSTREAM_DEPENDENCY` 返回 503，不能降为 allowed=false。

API Key grant 保持：`_scope.iam.authorization.check / read / api / authorization-check`。
本次合同同步不会自动为现有 Key 增加权限。

## Core → 插件服务调用

插件为独立服务入口正式声明以下 binding（示例 ID 与路径由插件自行登记）：

```yaml
metadata:
  protocols:
    rest:
      method: POST
      path: /api/v1/service/messages/reply
      auth_type: core_service_ticket
      actor_context: service_actor
      resource_scope: tenant
      schema_ref: contracts/messages.yaml#/ReplyRequest
```

入口必须是插件完整 API 路径，包含插件配置的 HTTP base path。
不允许绝对 URL、查询串、动态路径占位符或 Admin binding 作为回退。
需要 API Key 调用时，仍必须独立声明该插件拥有的显式 APIKeySpec。
该模式为正式 opt-in；旧的 API Key/JWT 插件入口不会因这次 Core 变更自动变成此模式。

调用方使用 `/api/v1/tenant/invocations`，选择这项能力，payload 只允许
`{"body":{...业务参数...}}`。不能提供 endpoint/method/headers/query 或可信身份 context。
Core 校验实时授权、能力发布、当前租户 registration，从进程内挂载表解析目标运行地址，
检查安装运行版本、租户启用和排空状态，然后发送固定方法与路径。

Core 为该次调用生成 `X-PowerX-Service-Invocation` 随机票据，最多 60 秒，
不超过源 STS 凭据的剩余有效期。源 Key、Admin Bearer、用户身份头不发送给目标。
禁止 HTTP 重定向携带票据。调用结束后删除票据；缓存不可用时失败关闭。
票据仅保存于 Core 的隔离缓存空间，审计不保存明文；不能放入日志、URL 或浏览器存储。

这条链路的 STS 主体必须经过 Core Exchange 签发记录校验。Exchange 为令牌生成随机 JTI，
Core 仅缓存该次签发的鉴权字段摘要；知道旧共享 JWT 密钥、手工重签 JWT 或替换主体字段，
都不能制造该记录。升级前的 STS 令牌需要重新 Exchange；签发记录缓存丢失时重新 Exchange，
缓存故障返回 503，不降级到仅验签。既有非本合同的 legacy 入口不因此自动获得这种保证。

### 目标插件验证

目标插件在专用服务入口收到票据后，先使用自身 API Key 或 STS 凭据调用：

```json
{
  "capability_id": "com.corex.iam.identity_context.service_read",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "operation": "resolve_service_invocation",
      "request": {
        "schema": "powerx.iam.service-invocation-query/v1",
        "invocation_ticket": "从服务票据请求头取得",
        "capability_id": "入口固定对应的能力 ID",
        "capability_version": "入口固定对应的契约版本",
        "method": "POST",
        "path": "/api/v1/service/messages/reply",
        "body_sha256": "收到的原始 HTTP body 字节的 SHA256 小写 hex"
      }
    }
  }
}
```

目标凭据需要 `com.corex.iam.identity_context.service_read` 的正式租户 grant，
并且 STS plugin_id 或 Key 的显式 owner 必须包含目标插件。
这个操作只在 typed core_internal 入口开放，不新增 Admin 检查入口。
普通用户／root Admin JWT 不能调用此操作，也不能签发服务票据。

返回 `schema=powerx.iam.service-invocation/v1`、`actor_type=service_actor`、
tenant、真实 caller_subject、可确定时的 caller_plugin_id、目标插件、能力、契约与
provider 版本、catalog/protocol hash、固定 method/path、body 摘要、Core request_id、trace_id、expiry。
多 owner／未绑定 owner 的 Key 不会凭空变成某个插件；caller_subject 始终保留 Key UUID。
目标插件必须以验证结果建立业务身份，不能以普通租户头、用户 JWT 或 payload 中的 caller 字段代替。
目标插件还必须将返回的 provider_version 与自身运行版本核对；请求／追踪标识以验证结果为准。

Core 回查时重新核对源凭据是否撤销、其 grant、能力发布、最新 registration、版本/hash、
目标租户启用状态。撤权无需等待票据 TTL。请求不匹配返回
`IAM_SERVICE_INVOCATION_REQUEST_MISMATCH`，目标不匹配返回
`IAM_SERVICE_INVOCATION_TARGET_FORBIDDEN`；过期／已结束返回 401，依赖不可用返回 503。

目标插件取得可信服务身份后，还需要检查业务对象隔离、消息入口权限、发送条件和幂等。
票据验证不是幂等接单机制；同一有效请求可能重复到达。异步工作应在接单时验证并保存必要的
非凭据业务上下文，不能持票据延迟执行，也不能自动获得代用户权限。

## 部署和验收边界

- Core 代码、能力合同与安装运行服务需同步更新；日常 Core 进程由所有者重启。
- 目标插件需实现专用服务入口、票据回查及业务授权，然后按正式插件流程发布能力、更新 registration 和精确 grant。
- 既有只读／共享 Profile、订单支付权限不因本次变更扩大。
- Core 受控回归覆盖权限允许／撤销、API Key 与 STS、真实 HTTP 目标、目标／租户／请求错配、Admin 拒绝、版本变化、过期与代理字段覆盖。
- Core 受控回归不代表 SCRM 已适配或消息实际发送成功；双方部署后需使用真实服务身份联调。

代码：`internal/service/iam/plugin_authorization.go`、`authorization_invoker.go`、
`internal/service/capability_registry/plugin_service_invocation.go`、
`internal/service/plugin_identity/service_invocation.go`、
`internal/infra/plugin/manager/router/service_target.go`。

## 失败返回与恢复信息（2026-11-02 补充）

`core_service_ticket` 目标返回非 2xx 时，Core 只从完整、合法 JSON 的顶层
`error_code`，或正式 `success=false` envelope 的 `error.code` 读取机器码（两处并存必须一致）：3–128 个字符，首字符大写字母，其余为大写字母、数字或下划线。
插件不能使用 `CAPABILITY_`、`IAM_`、`STS_`、`RUNTIME_IDENTITY_` 前缀冒充 Core 鉴权／运行错误。
非法、缺失、保留前缀或非 JSON 响应统一使用 `CAPABILITY_PLUGIN_REJECTED`。
不透传上游的 message、error、原始响应体或任意 details。

有效业务机器码及 4xx／5xx HTTP 状态保留，例如目标返回
`409 / MESSAGING_IDEMPOTENCY_CONFLICT`，调用入口同样返回 409，且顶层
`error_code`、`reason_code` 及 `details.reason_code` 保留该码。
`details.upstream_status` 保存目标状态；Core 本次 trace 保存在 `details.trace_id`。
目标顶层 `trace_id` 仅接受标准小写 UUID 或 32 位十六进制 trace，返回为 `details.upstream_trace_id`，
不允许替换 Core trace。3xx 不跟随，调用入口返回 502。

SCRM 的嵌套 `error.details` 采用同一规则。顶层有效 UUID `request_id` 返回为 `details.upstream_request_id`。
只保留下列 `details` 恢复字段；可选 `details.recovery` 也采用相同白名单，不递归：

| 字段 | 校验 |
|---|---|
| next_action_code | 3–64 字符的机器标识（字母开头，后续只允许字母、数字、下划线） |
| retryable | 布尔值 |
| retry_after_ms | 0–86400000 的整数 |
| retry_after | 0–86400 的整数（秒） |
| operation_id、operation_uuid、request_id | 标准小写 UUID |
| current_revision、expected_revision | 0–9007199254740991 的整数 |

Core 运行解析错误独立返回：

- `409 / CAPABILITY_PLUGIN_RUNTIME_VERSION_MISMATCH`，包含 `expected_provider_version`、`actual_provider_version`。
- `503 / CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE`：运行不存在、查询不可用或未启用。

二者在生成服务票据、发送目标请求前失败；不暴露运行查询的内部原文。
普通能力调用失败也保留 Core 已生成的 trace，Framework 无需解析 error 字符串。

## 流式调用边界（2026-11-02 补充）

`POST /api/v1/tenant/invocations/stream` 复用普通调用的 Selector policy、authorizer、
实时服务 grant、能力发布、模型授权与版本锁检查。用户态和服务态都要求当前租户存在有效的最新 registration。
STS 还必须经过 Core 签发记录验证。当前支持 Core 的正式 `.stream` REST POST 绑定；
插件票据流式调用返回 `400 / CAPABILITY_SERVICE_STREAM_UNSUPPORTED`，不能回退到用户代理。

推荐只传业务参数：

```json
{
  "capability_id": "com.corex.ai.llm.stream",
  "preferred_protocol": "rest",
  "payload": {
    "query": {"env": "dev"},
    "body": {"model_key": "openai/实际型号", "inputs": [{"type": "text", "text": "你好"}]}
  }
}
```

流式地址只从 Core 配置的 `HTTPBaseURL` 原点和能力正式绑定取得。
请求 Host、X-Forwarded-Proto 和 payload 均无权改变原点。
旧 payload.method／endpoint 仅允许与正式绑定完全相同的断言；headers 仅兼容
`Content-Type=application/json`、`Accept=text/event-stream` 断言，实际请求头由 Core 重建。
禁止任意 URL、方法、鉴权／身份头、context；query 当前仅允许 env=dev/staging/prod。
原调用凭据只发送给固定 Core 入口，不发送给插件，不跟随重定向。
超时沿用 Core 配置产生的 AI HTTP client。非 SSE 或失败响应不透传原始内容。

成功响应携带 `X-Trace-ID`。审计在流结束／中断时记录状态、耗时和 trace，不保存流内容、输入图片、
原始代理头或服务凭据。客户端中途断开时关闭上游响应体。

受控验收包含真实 API Key HTTP 中间件、正式路由注册、真实 InvocationService、隔离数据库和
本地 httptest 目标：合法 SSE、撤权／撤销凭据／最新 registration 禁用、跨租户 registration、
模型与版本锁、Selector 拒绝、任意 URL／方法／鉴权头、Adapter 地址漂移、非 SSE／重定向／上游失败、
插件业务错误与运行错误的 HTTP envelope。目标插件身份验证另由 plugin_identity 回归覆盖。
这些证据不替代真实 SCRM／Framework 部署后的服务身份联调。

## 服务 descriptor 登记路径（2026-11-02 补充）

`metadata.protocols` 与 `permissions[].protocol_bindings` 的服务路径都填写实际完整 API 路径，
例如 `/api/v1/service/messaging/executions.reply`。自定义 base path 同样保留原样。
Core 不自动补删 `/api/v1`、自定义前缀或 `/_p/<plugin_id>`。

完整路径例外只适用于当前 permission 的 channel、method、path 与一条正式 metadata protocol
精确匹配，且该 protocol 明确声明 `auth_type=core_service_ticket` 的情况；
能力里另一条路径或另一种 method 的票据绑定不会使普通绑定获得豁免。
该绑定仍要求 rest、service_actor、tenant scope、有效 method 和 schema，权限必须为
`type=api`、`independent=true` 且不引用 Admin／业务权限别名。
API Key 还要满足原有独立显式声明、capability/plugin owner 精确匹配规则。
普通绑定的路径和权限规则保持不变。

服务路径必须为固定、规范的路径：拒绝绝对／带主机 URL、宿主 `_p` 路径段、查询串（包括空 `?`）、
fragment、动态参数、通配符、反斜线、重复斜线、路径穿越和编码后改变的路径。
运行时仍由实际挂载表核对 API base path、租户启用及排空状态；登记通过不代表已获得调用授权。

服务票据的本地 schema_ref 支持打包合同文件和 JSON Pointer，例如
`docs/contracts/messaging-service.openapi.yaml#/components/schemas/ReplyRequest`。
同步时分别读取文件并验证片段，拒绝缺文件、错误文档、缺失／非法片段和逃出插件根目录的相对引用；
保存的正式 schema_ref 保留完整原值。其他绑定的 schema 处理规则保持不变。

回归素材来自 SCRM 五份真实服务 descriptor，原文副本、实际 OpenAPI 文件与 SHA256 清单位于
`backend/internal/service/capability_registry/testdata/scrm-service-registration/`。
验证包含协议／权限检查、正式 ProcessArtifact 的重复同步与租户 registration、
安装 Loader／启动权限映射，以及默认／自定义 API base path 的运行解析。
路径／method 不一致、身份／scope 错误、缺 schema、错误插件 owner 和无 grant 均需拒绝。
测试不写入本机运行库、不扩大现有 Key/Profile 或默认角色权限，也不重启日常服务。

## REST 适配器标识与完整清单同步（2026-11-02 补充）

同一能力可以包含多个同方法、不同路径的 REST 绑定。未声明 ToolRef/RPC 时，
自动 ID 使用 `capability_id.METHOD.sha256`：摘要输入为小写 channel、NUL 分隔符、
大写且去除首尾空白的 method、NUL 分隔符、既有目录规则规范化后的 endpoint。
使用完整 SHA256；不使用声明下标、随机值或同步次数。该规则适用于 rest/http，
显式 ToolRef 优先、RPC 次之的既有标识语义及其他协议保持不变。

完全重复的绑定仍生成相同 ID，由登记校验明确拒绝，不静默去重。
`metadata.protocols` 的 mapping 按 channel 名排序后转换，保证相同清单重复同步的
`capabilities_hash`、`protocol_hash` 稳定；各通道内部的数组顺序保持不变。
绑定的地址、method、schema、身份、票据验证与授权规则不因此改变。

下一次正式同步会生成新的 REST ID；既有历史 registration 不作手工改写。
多通道旧清单首次采用固定顺序时，其 hash 也可能变化，此后同一内容保持稳定。
这些变化只在正常同步产生的新 registration 中生效，不要求迁移或改写授权数据。

完整 SCRM 回归素材位于
`backend/internal/service/capability_registry/testdata/scrm-full-registration/`，
保留 27 项能力、五组同方法多路径旧能力及五项票据服务，附原文件 SHA256。
测试经过正式安装 Loader、权限同步、整包 ProcessArtifact 和两轮租户登记，
检查全部适配器数量、唯一性、ID/hash 稳定及重复绑定拒绝；有邻接 SCRM 源码时同时验证该实际清单。
这属于临时副本和隔离数据库验收，不替代运行安装、在线授权或实际消息收发验收。

## LLM 流式服务绑定补充（2026-11-06）

Framework 的 LLM 流式调用现按[LLM 流式服务合同 v1](ai-llm-stream-service-v1.md)使用正式 core_internal 绑定及独立显式服务 grant。Core 既有四元组精确匹配规则保持不变，但旧 REST 路由 grant 不代表该独立服务 grant。Core 的 done(ok=true) 是成功终止帧；Framework 必须完整适配 final/end，不能用 EOF 当作完成。
