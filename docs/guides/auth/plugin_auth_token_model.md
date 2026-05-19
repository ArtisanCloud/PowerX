# PowerX 插件鉴权 Token 模型

本文是 PowerX 插件与底座之间的 token 使用边界说明。目标是统一插件调用底座、底座代理到插件、启动期引导和调试 token 的职责，避免多套凭证混用。

## 结论

插件主动调用 PowerX 底座业务接口时，统一使用 STS Exchange 签发的短期 Bearer token。

`PX_PLUGIN_TOOL_TOKEN` 只允许作为 bootstrap/过渡凭证，不作为插件业务调用底座的主凭证。

`plugin:<plugin_id>` audience 的短期 token 只用于 PowerX 动态代理把当前用户请求转发给插件后端，不用于插件主动调用 PowerX 底座。

Root 手工 mint 的插件 token 只用于开发和调试。

## Token 类型

| Token 类型 | 方向 | 签发方 | audience | 用途 | 正式业务主路径 |
| --- | --- | --- | --- | --- | --- |
| STS access token | 插件 -> PowerX | STSService.Exchange | `powerx:api` | 插件调用 PowerX Gateway、Capability、ws-bus、底座开放 API | 是 |
| Plugin request token | PowerX -> 插件 | 动态插件代理 `authz_gate` | `plugin:<plugin_id>` | PowerX 代理当前用户请求到插件后端 | 是，但仅限该方向 |
| `PX_PLUGIN_TOOL_TOKEN` | 插件 -> PowerX | 插件生命周期注入 | 当前实现依赖宿主配置 | 启动期探活、过渡期引导 | 否 |
| Root debug token | Root/Admin -> 插件调试 | Admin system STS handler | `plugin:<plugin_id>` | 开发/调试手工签发 | 否 |

## 插件调用 PowerX 底座

插件调用 PowerX 底座业务接口时必须走 STS。

插件持有租户维度凭证：

```text
PX_STS_CLIENT_ID=<plugin_id>.<tenant_uuid>
PX_STS_CLIENT_SECRET=<one_time_or_rotated_secret>
PX_STS_AUDIENCE=powerx:api
PX_STS_SCOPE=access
```

插件启动后或 token 过期前调用：

```text
powerx.auth.sts.v1.STSService.Exchange
```

请求字段：

```text
client_id=<PX_STS_CLIENT_ID>
client_secret=<PX_STS_CLIENT_SECRET>
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

插件 SDK 应在内存中缓存 STS token，并在剩余寿命不足时刷新。遇到 401/403 可强制刷新一次后重试；仍失败则直接返回鉴权错误。

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
  "uid": "user_uuid",
  "mid": "member_uuid",
  "email": "user@example.invalid",
  "phone": "0000000000"
}
```

插件不得把该 token 当作调用 PowerX 底座业务接口的凭证。

## PX_PLUGIN_TOOL_TOKEN

`PX_PLUGIN_TOOL_TOKEN` 是插件生命周期注入到插件进程环境变量里的宿主契约 token。

统一后的定位：

```text
bootstrap only
```

允许用途：

- 插件启动期探活。
- 拉取 STS 配置或完成引导检查。
- 过渡期兼容尚未迁移到 STS 的内部链路。

禁止用途：

- 作为插件业务请求调用 PowerX 底座的长期凭证。
- 作为当前访问用户身份。
- 作为插件 SDK 对外暴露的默认业务 token provider。

新代码不得新增依赖 `PX_PLUGIN_TOOL_TOKEN` 的业务调用路径。

## Root Debug Token

Root 手工 mint token 仅用于开发和调试。

它不属于插件正式运行链路，不应写入插件 SDK 默认流程，也不应作为生产调用凭证。

## 环境变量契约

插件正式调用 PowerX 底座时，推荐环境变量为：

```text
PX_GATEWAY_BASE_URL
PX_STS_CLIENT_ID
PX_STS_CLIENT_SECRET
PX_STS_AUDIENCE=powerx:api
PX_STS_SCOPE=access
```

过渡期可继续注入：

```text
PX_PLUGIN_TOOL_TOKEN
```

但所有新业务调用必须优先接入 STS。

## 迁移规则

1. 插件 SDK 增加统一 token provider：`getPowerXAccessToken()`，内部只负责 STS Exchange、缓存和刷新。
2. PowerX Gateway 业务接口统一接受 STS token，并校验 `audience=powerx:api`。
3. `plugin:<plugin_id>` token 只允许出现在 PowerX 代理到插件后端的方向。
4. `PX_PLUGIN_TOOL_TOKEN` 调用路径逐步改为 bootstrap-only。
5. 文档中“业务调用优先使用 PX_PLUGIN_TOOL_TOKEN”的描述必须改为“业务调用使用 STS token”。

## 代码映射

| 职责 | 代码位置 |
| --- | --- |
| STS Exchange | `backend/internal/transport/grpc/auth/sts_handler.go` |
| STS proto | `backend/api/grpc/contracts/powerx/auth/sts/v1/sts.proto` |
| gRPC STS 注册 | `backend/internal/server/grpc/server.go` |
| Plugin request token 签发 | `backend/internal/infra/plugin/manager/router/authz_gate.go` |
| PowerX 动态代理改写 Authorization | `backend/internal/infra/plugin/manager/router/router.go` |
| `PX_PLUGIN_TOOL_TOKEN` 注入 | `backend/internal/infra/plugin/manager/lifecycle.go` |
| Root 调试 token | `backend/internal/transport/http/admin/system/sts_handler.go` |

## 验收

- 插件业务调用 PowerX 底座时，使用 `Authorization: Bearer <sts_access_token>`。
- STS token 的 `aud` 为 `powerx:api`。
- PowerX 代理到插件后端时，使用 `Authorization: Bearer <plugin_request_token>`。
- Plugin request token 的 `aud` 为 `plugin:<plugin_id>`。
- 插件业务代码不直接读取 `PX_PLUGIN_TOOL_TOKEN`。
- 旧的 `X-PowerX-CTX*` 和 `px_ctx*` 链路不得恢复。
