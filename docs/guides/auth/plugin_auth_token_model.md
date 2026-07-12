# PowerX 插件鉴权 Token 模型

本文是 PowerX 插件与底座之间的 token 使用边界说明。目标是统一插件调用底座、底座代理到插件、启动期引导和调试 token 的职责，避免多套凭证混用。

## 结论

插件主动调用 PowerX 底座业务接口时，统一使用 STS Exchange 签发的短期 Bearer token。

`PX_PLUGIN_TOOL_TOKEN` 已废弃。宿主模式不得再注入、读取或依赖该变量。

`plugin:<plugin_id>` audience 的短期 token 只用于 PowerX 动态代理把当前用户请求转发给插件后端，不用于插件主动调用 PowerX 底座。

Root 手工 mint 的插件 token 只用于开发和调试。

## Token 类型

| Token 类型 | 方向 | 签发方 | audience | 用途 | 正式业务主路径 |
| --- | --- | --- | --- | --- | --- |
| STS access token | 插件 -> PowerX | STSService.Exchange | `powerx:api` | 插件调用 PowerX Gateway、Capability、ws-bus、底座开放 API | 是 |
| Plugin request token | PowerX -> 插件 | 动态插件代理 `authz_gate` | `plugin:<plugin_id>` | PowerX 代理当前用户请求到插件后端 | 是，但仅限该方向 |
| Root debug token | Root/Admin -> 插件调试 | Admin system STS handler | `plugin:<plugin_id>` | 开发/调试手工签发 | 否 |

## Claims 标准

PowerX 统一使用 `CoreXClaims` 表达鉴权主体。租户模式下必须区分全局用户和租户成员：

| Claim | 含义 | 用户态 token | 插件服务 STS token |
| --- | --- | --- | --- |
| `tid` | 当前租户 UUID | 必填 | 必填 |
| `tid_n` | 当前租户数值 ID | 必填 | 必填 |
| `uid` | 全局用户 UUID | 必填 | 不填 |
| `uid_n` | 全局用户数值 ID | 必填 | 不填 |
| `mid` | 当前租户成员 UUID | 必填 | 不填 |
| `mid_n` | 当前租户成员数值 ID | 必填 | 不填 |
| `email` | 当前用户邮箱 | 可选，短期 token 可携带 | 不填 |
| `phone` | 当前用户手机号 | 可选，短期 token 可携带 | 不填 |
| `scope` | token scope | `access` | `access` |
| `aud` | token audience | `user` 或 `plugin:<plugin_id>` | `powerx:api` |
| `sub` | JWT subject | `mid`，即当前成员 UUID | `client:<client_id>` |

标准语义：

- `uid/uid_n` 表示全局账号。同一个用户可以加入多个租户。
- `mid/mid_n` 表示该用户在当前租户下的成员身份。权限、部门、角色、状态、通知订阅和审计归属必须优先使用 `tenant + member`。
- 用户登录 token 和 Plugin request token 都属于用户态 token，必须带 `tid + uid + mid`。
- STS access token 属于插件服务 token，代表某个插件实例在某个租户下调用 PowerX，不代表某个登录成员，因此不携带 `uid/mid`。
- 如果未来需要“插件代表某个用户/member 调用 PowerX”，必须新增明确的 on-behalf-of/delegated actor 机制，不得把普通 STS token 混用成用户态 token。

## 资源边界与行为归属

Token 已经能表达 member 身份，但 PowerX 的平台资源边界不因此全部改成 member 级。

标准规则：

1. 平台资源默认仍按 `tenant_uuid` 隔离，例如 topic、queue、scheduler job、capability、plugin installation、gateway API key。
2. 行为记录必须补齐 actor 上下文，例如 audit、invocation trace、scheduler run、event metadata、WS session、notification。
3. 用户态行为 actor 必须记录 `tenant_uuid + uid/uid_n + mid/mid_n`。
4. 插件服务态行为 actor 必须记录 `tenant_uuid + plugin_id/client_id`，不得伪造 `uid/mid`。
5. 系统自动触发行为 actor 必须记录 `actor_type=system` 和系统组件名。

也就是说，topic 不因为当前登录成员不同而拆成 member topic；消费者需要个人态过滤时，应从 event payload、metadata 或业务 payload 中读取 member actor。

## 插件调用 PowerX 底座

插件调用 PowerX 底座业务接口时必须走 STS。

插件持有租户维度凭证：

```text
POWERX_STS_CLIENT_ID=<plugin_id>.<tenant_uuid>
POWERX_STS_CLIENT_SECRET=<one_time_or_rotated_secret>
POWERX_STS_AUDIENCE=powerx:api
POWERX_STS_SCOPE=access
```

插件启动后或 token 过期前调用：

```text
powerx.auth.sts.v1.STSService.Exchange
```

请求字段：

```text
client_id=<POWERX_STS_CLIENT_ID>
client_secret=<POWERX_STS_CLIENT_SECRET>
audience=powerx:api
scope=access
ttl_seconds=300
```

返回的 `access_token` 用于调用 PowerX：

```http
Authorization: Bearer <sts_access_token>
```

PowerX 底座业务接口应校验：

```text
audience = powerx:api
scope = access
tenant_uuid 存在且有效
subject = client:<client_id>
```

STS access token 的主体是插件服务账号，不是当前登录用户。该 token 的 claims 必须包含 `tid/tid_n`，不应要求或伪造 `uid/uid_n/mid/mid_n`。需要写审计时应记录 `actor_type=plugin` 或 `actor_type=service_account`，并记录 `client_id/plugin_id/tenant_uuid`。

插件 SDK 应在内存中缓存 STS token，并在剩余寿命不足时刷新。遇到 401/403 可强制刷新一次后重试；仍失败则直接返回鉴权错误。

### STS direct HTTP 访问边界

插件调用底座能力的推荐主路径是 `/api/v1/tenant/invocations`。该入口由 Capability Registry 做能力选择、租户授权和协议适配，适合插件不想直接绑定底座 REST/gRPC 细节的场景。

插件也可以用 STS token 直接调用底座 REST 接口，但必须同时满足：

1. 接口已经登记在正式 `backend/config/platform_capabilities/*.yaml` 的 REST protocol endpoints 中。
2. HTTP method 与 capability protocol 中声明的方法精确匹配。
3. 路径没有命中 STS blocklist。

Capability 是业务授权单元，不是 URL。`/api/v1/admin/<resource>`、`/api/v1/<resource>` 与 gRPC service 如果表达同一业务语义、同一授权边界，应登记为同一个 capability 的不同 protocol bindings。路径前缀只表示入口形态和调用身份边界，不应自动生成多个能力。只有资源范围、actor 约束、风险等级或授权开关必须独立时，才拆成不同 capability。

Web、mini-app、customer 入口属于外部业务调用边界，不属于后台管理边界，也不等同于插件服务态 STS。设计这类接口时必须声明 actor：

```text
admin_user      -> /api/v1/admin/*，用户 JWT + member + RBAC
service_actor   -> /api/v1/tenant/invocations 或服务态开放 REST，STS/API Key/OAuth
web_user        -> /api/v1/*，用户 JWT 或业务 session
mini_app_user   -> /api/v1/*，小程序会话、用户 JWT 或 customer/user token
customer_actor  -> /api/v1/customer/*，customer token 或 customer-scoped OAuth/API Key
```

customer/mini-app 自助接口默认必须 owner-scoped/self-scoped，不能复用 admin 全量管理能力。例如客户门户读取自己的账号应使用 `com.corex.customer.account.self_read`，而不是 `com.corex.customer.accounts.admin_manage`。

STS direct route policy 按以下规则生成：

```text
static plugin runtime contracts
+ formal platform capability REST endpoints
- STS blocklist
```

`/api/v1/admin/*` 是后台用户态 API 命名空间，不等于“禁止插件后台页面使用”。浏览器中的 PowerX Admin、插件 Admin 页面、以及任何携带用户 JWT 的后台请求，仍然按用户鉴权、租户成员、RBAC 和业务权限判断。STS direct route policy 只约束 `issuer=powerx-sts`、`audience=powerx:api` 的插件服务态 token。

普通 STS token 不携带 `uid/mid`，不能代表登录用户通过 `/api/v1/admin/*` 绕过用户 RBAC。插件后端如果要代表当前用户调用底座后台 API，必须引入明确的 delegated/on-behalf-of 机制，不能复用普通 STS token。

对服务态 STS direct call，`/admin/*`、`/internal/*`、`/public/*`、`/auth/*`、`/setup/*`、debug、migration、root、drain、bootstrap、mock、health、根级动态路径等默认不允许。少量确认为插件服务运行时合同的入口，可以进入 static allow，但必须有明确用途说明和 `auth_subject_validator` 测试。

因此，新增插件可直接调用的底座 REST API 时，不应手工改鉴权白名单绕过能力目录。正确顺序是：实现真实 transport/service/permission/test，登记正式 platform capability REST protocol，运行 `make capability-check`，再验证 STS direct policy。

遇到 STS direct HTTP 403 时，按顺序排查：

1. STS token 的 `audience=powerx:api`、`scope=access`、`tenant_uuid` 是否正确。
2. 目标 endpoint 是否存在于正式 platform capability REST protocols。
3. HTTP method 是否与 protocol 声明一致。
4. 路径是否命中 STS blocklist。
5. 如果走 `/tenant/invocations`，再查租户 registration、adapter、permission code 是否启用。

## 托管插件运行态凭证

PowerX 托管的插件进程是全局运行态，按 `plugin_id` 启动，不按业务租户复制进程。

因此，插件进程启动 env 不得绑定某个普通业务租户。PowerX 在启动全局插件进程时，使用 `system` tenant 生成插件 runtime STS 凭证，并注入：

```text
PX_GATEWAY_BASE_URL
PX_GATEWAY_AUTH_SCHEME=bearer
POWERX_STS_CLIENT_ID=<plugin_id>.<system_tenant_uuid>
POWERX_STS_CLIENT_SECRET=<rotated_runtime_secret>
POWERX_STS_AUDIENCE=powerx:api
POWERX_STS_SCOPE=access
POWERX_GRPC_UPSTREAM_ADDRESS
POWERX_GRPC_UPSTREAM_TENANT_UUID=<system_tenant_uuid>
```

该凭证只表达“插件运行态服务身份”，用于启动期注册、taskbus/ws-bus/capability 等平台调用。它不代表任何业务租户成员，也不能作为当前用户身份使用。

租户是否能看到菜单、访问 `/_p/<plugin_id>/admin/*` 或 `/_p/<plugin_id>/api/*`，仍由当前请求的用户 token 和该租户的 `TenantPluginInstance` 启用状态决定。

## PowerX 代理当前用户请求到插件

浏览器访问插件 API 的路径是：

```text
Browser -> PowerX /_p/:plugin_id/api/* -> Plugin Backend
```

PowerX 动态代理负责：

1. 校验当前浏览器用户登录态。
2. 解析租户、用户、成员上下文。
3. 执行插件 RBAC 判定。
4. 签发短期 plugin request token。
5. 将上游请求头改写为：

```http
Authorization: Bearer <plugin_request_token>
```

该 token 的 audience 必须是：

```text
plugin:<plugin_id>
```

插件后端使用该 token 识别当前访问用户和租户上下文。该 token 可包含当前用户的基础身份字段，例如：

```json
{
  "tid": "tenant_uuid",
  "tid_n": 1,
  "uid": "user_uuid",
  "uid_n": 10,
  "mid": "member_uuid",
  "mid_n": 20,
  "email": "user@example.invalid",
  "phone": "0000000000"
}
```

Plugin request token 必须表达当前登录成员身份。插件后端做业务归属、通知、审计或权限二次判断时，应使用 `tenant + member`，不要只使用 `user`。

插件不得把该 token 当作调用 PowerX 底座业务接口的凭证。

## 废弃变量

`PX_PLUGIN_TOOL_TOKEN` 和 `PX_TOOL_TOKEN` 不再属于宿主模式插件鉴权契约。

禁止用途：

- 作为插件业务请求调用 PowerX 底座的凭证。
- 作为 ws-bus、taskbus、capability invoke、IAM 查询的出站 token。
- 作为当前访问用户身份。
- 作为插件 SDK 对外暴露的默认 token provider。

新代码不得新增读取 `PX_PLUGIN_TOOL_TOKEN` 或 `PX_TOOL_TOKEN` 的宿主模式路径。

## Root Debug Token

Root 手工 mint token 仅用于开发和调试。

它不属于插件正式运行链路，不应写入插件 SDK 默认流程，也不应作为生产调用凭证。

## 环境变量契约

插件正式调用 PowerX 底座时，环境变量为：

```text
PX_GATEWAY_BASE_URL
POWERX_STS_CLIENT_ID
POWERX_STS_CLIENT_SECRET
POWERX_STS_AUDIENCE=powerx:api
POWERX_STS_SCOPE=access
POWERX_GRPC_UPSTREAM_ADDRESS
POWERX_GRPC_UPSTREAM_TENANT_UUID
```

## 迁移规则

1. 插件 SDK 增加统一 token provider：`getPowerXAccessToken()`，内部只负责 STS Exchange、缓存和刷新。
2. PowerX Gateway 业务接口统一接受 STS token，并校验 `audience=powerx:api`。
3. `plugin:<plugin_id>` token 只允许出现在 PowerX 代理到插件后端的方向。
4. 移除 `PX_PLUGIN_TOOL_TOKEN` / `PX_TOOL_TOKEN` 注入、读取和探活路径。
5. 文档中所有 “业务调用使用 PX_PLUGIN_TOOL_TOKEN” 的描述必须改为 “业务调用使用 STS token”。

## 代码映射

| 职责 | 代码位置 |
| --- | --- |
| STS Exchange | `backend/internal/transport/grpc/auth/sts_handler.go` |
| STS proto | `backend/api/grpc/contracts/powerx/auth/sts/v1/sts.proto` |
| gRPC STS 注册 | `backend/internal/server/grpc/server.go` |
| Plugin request token 签发 | `backend/internal/infra/plugin/manager/router/authz_gate.go` |
| PowerX 动态代理改写 Authorization | `backend/internal/infra/plugin/manager/router/router.go` |
| Root 调试 token | `backend/internal/transport/http/admin/system/sts_handler.go` |

## 验收

- 插件业务调用 PowerX 底座时，使用 `Authorization: Bearer <sts_access_token>`。
- STS token 的 `aud` 为 `powerx:api`。
- STS token 代表插件服务身份，包含 `tid/tid_n`，不包含用户态 `uid/mid`。
- PowerX 代理到插件后端时，使用 `Authorization: Bearer <plugin_request_token>`。
- Plugin request token 的 `aud` 为 `plugin:<plugin_id>`。
- Plugin request token 包含当前用户态 claims：`tid/tid_n`、`uid/uid_n`、`mid/mid_n`。
- 用户登录 token 和 refresh 后的新 access token 都保留 `tid + uid + mid`。
- 插件业务代码不读取 `PX_PLUGIN_TOOL_TOKEN` 或 `PX_TOOL_TOKEN`。
- 旧的 `X-PowerX-CTX*` 和 `px_ctx*` 链路不得恢复。
