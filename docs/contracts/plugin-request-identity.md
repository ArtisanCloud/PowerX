# 插件请求身份上下文：正式 Core 合同

日期：2026-10-10。Schema v1。**Core 已实现；Framework / AI Craft 接入及远程部署需按本文完成。**

依据：[OpenAPI](../../specs/018-plugin-request-identity/contracts/identity-context.openapi.yaml)、[能力声明](../../backend/config/platform_capabilities/plugin-request-identity.yaml)、[实施与证据](../../specs/018-plugin-request-identity/implementation.md)。

## 1. 用途与调用入口

Core 网关验证用户及插件业务权限后，向插件后端注入 `X-PowerX-Identity-Context` 不透明票据。Framework 使用自己的 STS 或 API Key 解析票据，得到 Core 当前权威的用户、成员及目标插件权限。插件收到的业务 JWT 继续用于插件认证，不作为用户 `/me/context` 的凭据。

| 平面 | 正式入口 | 凭据及授权 |
|---|---|---|
| Framework typed Host | `INVOKE core://iam/identity-context` | `service_actor` + STS/API Key + 新能力实时 grant + 票据目标插件授权 |
| Tenant REST | `POST /api/v1/tenant/iam/identity-contexts:resolve` | 同上 |
| Admin 自身检查 | `POST /api/v1/admin/iam/identity-contexts:resolve` | `admin_user` 用户 JWT + RBAC；只能读同用户、同登录会话、同租户票据 |
| 原用户 context | `/api/v1/admin/user/auth/me/context` | 用户 Token，仍要求 `aud=user`；不接受插件业务 Token |

capability：`com.corex.iam.identity_context.service_read`。同一正式能力声明包含 Admin 与 service 平面。

API Key 精确 grant：`_scope.iam.identity_context.service_read / read / capability / iam_identity_context_read / allow`。IAM service permission 必须有 `allow_api_key=true` 和 `api_key_explicit=true`。另校验 Key 的插件 owner policy：`mode=key` 必须明确包含目标插件；旧 permission owner 模式必须有该新 scope 的目标插件精确 binding，不能靠其他权限推导授权。Key/Profile 必须当前有效。

STS：正式 Exchange 签发，issuer `powerx-sts`、audience `powerx:api`、subject 对应当前 client_id、未过期，插件凭据 enabled，allowed_capabilities 包含新能力，且 STS plugin_id 与票据一致。租户由已认证服务凭据派生。

Admin permission_code `corex.iam.identity_context:read` 对应 IAM `iam / identity_context / read`、source `platform_capability`；Core Root 按当前权威 Root 状态处理。服务凭据不能调用此 Admin 平面。

## 2. 票据发行与有效期

- 发行者是 Core 网关的用户业务请求路径；用户/服务调用方没有任意 UUID 的票据签发接口。
- 票据为 `pxctx_v1_` + 32 字节安全随机数的 base64url，共 52 字符。Redis key 使用部署/数据库隔离命名空间及票据 SHA256，正文保存可信身份和原请求关联；不新增数据库表。
- TTL 为 60 秒，并受原用户 access 到期时间上限约束；重复解析不续期。
- Core 删除客户端伪造的票据头和身份覆盖头，注入自己的票据；公开插件路由不会发行票据。
- 绑定目标插件、租户/用户/成员 UUID、Core 原用户登录会话、Root 状态、原 method/path、必要业务权限、request ID、Trace 和 transport。
- body/query/header 中的 tenant/user/member/plugin、roles/is_root/权限声明不能替代这份证明。
- 业务 JWT 的共享 HMAC 密钥不是票据发行证明。Framework 不从未确认的 claims 猜测身份，不向浏览器暴露票据或服务凭据。
- 日志只包含事件、插件、租户及 request/Trace。Invocation 持久记录对 `context_ticket` 脱敏，查询日志也脱敏该字段。

### 登录会话关联

新的用户 access Token 的 `jti` 与其 refresh session JTI 一致。登录、注册及续期保留这一关联；会话持久失败时登录签发失败。解析查询当前 refresh-session 记录，核对 user/member/tenant、到期及撤销状态。退出撤销该 session 后，旧票据立即不能解析。

**升级前不含这项会话关联的旧 access Token 返回 `IAM_CONTEXT_USER_SESSION_INVALID`。用户通过宿主重新登录或正常续期取得新 Token，不增加旧格式推断。** Root 无成员登录支持正常续期。

## 3. 请求

Tenant/Admin REST body：

```json
{
  "schema": "powerx.iam.plugin-request-identity-query/v1",
  "context_ticket": "<redacted>"
}
```

无 query；body 上限 4096 字节，未知字段、非法操作和身份覆盖明确 400。上面票据仅为脱敏示例，真实值从可信 Core 转发头读取。

Framework typed invocation：

```json
{
  "capability_id": "com.corex.iam.identity_context.service_read",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "operation": "resolve_request_identity",
      "request": {
        "schema": "powerx.iam.plugin-request-identity-query/v1",
        "context_ticket": "<redacted>"
      }
    }
  }
}
```

使用 `POST /api/v1/tenant/invocations`。固定 binding 不接受任意 method、endpoint、headers、query 或 raw proxy。内层操作返回 `{result: IdentityResult}`，外层保留 Registry invocation envelope。

## 4. 身份、权限及 Root 规则

每次解析复核服务 grant、目标插件 owner、用户会话、账号、租户、成员及插件启用状态，再读取当前目标插件权限。普通成员的角色绑定及有效期也参与过滤；通过共享 business permission code mapper 返回权限，技术路由 permission 不伪装成业务 grant。

- 普通用户：user_uuid / member_uuid / tenant_uuid 必须是当前有效且相互匹配的 Core 对象。
- Core Root：当前 Core user 必须仍为 Root 且有效；选定租户必须有效，不能使用空租户或伪造租户。Root 可以没有该租户成员，此时 `member_uuid=null`；若存在成员引用，仍必须有效且匹配。
- Root 身份变更、账号/成员/租户停用、插件停用均拒绝；不依据客户端传入的 Root 标志或角色名提升权限。
- 返回当前插件范围权限，`policy_version` 是本次身份/角色/权限内容的摘要，`perms_hash` 是排序后的权限摘要。
- 原请求必要权限已撤销时返回 `IAM_CONTEXT_REQUEST_FORBIDDEN`。Root 仍需真实身份及会话，业务 Root 权限按 Core 规则执行。
- 解析结束前再次检查服务授权及主体状态，避免授权撤销期间返回已无资格的身份。

REST 成功使用标准 Core envelope，data 结构示例：

```json
{
  "schema": "powerx.iam.plugin-request-identity/v1",
  "mode": "delegated",
  "plugin_id": "com.powerx.plugins.ai-craft",
  "tenant_uuid": "11111111-1111-4111-8111-111111111111",
  "user_uuid": "22222222-2222-4222-8222-222222222222",
  "member_uuid": "33333333-3333-4333-8333-333333333333",
  "is_root": false,
  "roles": ["role_user"],
  "permissions": ["operations.order:read"],
  "policy_version": "<current-identity-permission-hash>",
  "perms_hash": "<current-permission-hash>",
  "source_method": "GET",
  "source_path": "/admin/operations/ai-craft/payment-orders",
  "source_required_permission": "operations.order:read",
  "transport": "http",
  "source_request_id": "<original-request-id>",
  "source_trace_id": "<original-trace-id>",
  "trace_id": "<current-resolve-trace-id>",
  "proof_expires_at": "2026-10-10T08:00:30Z",
  "user_session_expires_at": "2026-10-10T08:14:00Z"
}
```

时间使用 UTC RFC3339Nano。`proof_expires_at` 为票据有效上限；`user_session_expires_at` 为原 access 有效上限。二者不能互换或通过解析延长。

## 5. 错误与恢复动作

| HTTP / 机器码 | 失败平面 | Framework/插件动作 |
|---|---|---|
| 400 `IAM_CONTEXT_INVALID_ARGUMENT` | 调用合同 | 修正请求；不重试旧格式 |
| 401 `IAM_CONTEXT_SERVICE_CREDENTIAL_INVALID` / `IAM_CONTEXT_SERVICE_CREDENTIAL_EXPIRED` | 插件服务凭据 | 检查/正常续期服务凭据；不清空人类用户登录态 |
| 403 `IAM_CONTEXT_SERVICE_FORBIDDEN` | capability / live grant | 检查正式授权；不当作用户会话过期 |
| 401 `IAM_CONTEXT_PROOF_REQUIRED` / `IAM_CONTEXT_PROOF_INVALID` / `IAM_CONTEXT_PROOF_EXPIRED` | 网关证明 | 重新发起已认证的原业务请求获取新票据；不从缓存/原 claims 补身份 |
| 401 `IAM_CONTEXT_USER_SESSION_INVALID` / `IAM_CONTEXT_USER_SESSION_EXPIRED` | 人类登录会话 | 交给宿主正常续期或重新登录 |
| 403 `IAM_CONTEXT_USER_FORBIDDEN` / `IAM_CONTEXT_MEMBER_REQUIRED` / `IAM_CONTEXT_REQUEST_FORBIDDEN` | 用户资格 / 业务权限 | 显示权限问题，检查成员或角色；不刷新 Token 规避授权 |
| 403 `IAM_CONTEXT_TENANT_FORBIDDEN` / `IAM_CONTEXT_PLUGIN_FORBIDDEN` | 租户 / 插件 owner / 启用状态 | 拒绝跨域，检查服务绑定或插件配置 |
| 409 `IAM_CONTEXT_POLICY_CHANGED` | 解析期间权限变化 | 丢弃旧结果，重新走原请求和实时授权；不延长旧票据 |
| 503 `IAM_CONTEXT_UNAVAILABLE` | Redis / IAM / 上游依赖 | 显示依赖不可用并保留 Trace；不变成匿名/空成功 |

固定 service endpoint 的认证中间件错误使用服务平面机器码。`/_p/.../api/...` 的用户认证失败使用用户会话机器码。通过 Registry invocation 调用时，路由/认证/注册在 typed handler 之前失败可能使用现有 `registry.*` / `CAPABILITY_*` envelope；其失败平面仍是服务调用，不能据此注销用户。Framework 保留原 HTTP、error/reason code、source/current Trace 及上游 request ID。

## 6. WS 授权合同

票据仅用于请求开始时的身份证明，不能作为浏览器 WebSocket Token：

1. 插件 WS grant 请求经 Core 用户网关认证与真实 route permission 检查，Core 注入票据，结果 `transport=ws_grant`。
2. Framework/插件解析后，用返回的 user/member/tenant/plugin 与当前有效权限签发已有 WS 专用 credential；topic 权限仍要单独检查。
3. WS credential 的有效期不能超过 `user_session_expires_at`，不得使用 ticket 字符串作为 Token 或把 ticket 的 60 秒续期成登录会话。
4. 若 WS upgrade 经 Core 代理，握手须具备 Core 能验证的用户凭据；Core 注入 `transport=ws_upgrade` 票据，插件先解析并检查连接权限再 upgrade。插件自己签发的 WS audience 不能被拿去查询 Core 用户 `/me/context`。
5. 服务凭据解析 `ws_grant` / `ws_upgrade` 后返回 `ws_revalidation_handle` 和固定的 `ws_revalidation_expires_at`。Framework/插件在业务消息、订阅和下行投递前调用正式 `revalidate_ws_identity` 操作；退出、成员停用和权限撤销在实时复核中明确失败。详见 [WS 实时复核合同](plugin-ws-revalidation.md)。
6. WS credential 过期、复核拒绝后的连接关闭由 Framework/插件的 WS lifecycle 执行。空闲连接最迟原 access / WS credential 到期关闭；本轮实时复核入口不承诺撤销瞬间向空闲连接推送事件。续期重新走受保护 grant 路径，不添加轮询或匿名降级。

Core 已有真实 101 代理握手与票据解析测试，以及 WS grant 分类测试。插件 topic token、长连接关闭和具体浏览器重连验收由消费端完成，不能以 Core 的测试替代。

## 7. Framework 接入要求

- 新增 typed delegated identity 输入/结果，严格映射所有 UUID、nullable member、Root、权限版本、原请求及两类过期时间。
- `RefreshFrameworkIAMContext` 等 middleware 读取可信 Core 票据，使用本插件服务凭据调用本合同；结束把 plugin Bearer 送往用户 `/me/context` 的调用链。
- Root 同样解析票据，不能保留“Root 不刷新身份”的旁路。结果作为权威身份写入 tenant context，随后执行统一业务 RBAC。
- Local provider 使用已有 Local 身份合同。缺票据/缺服务绑定明确失败，不落到 Local 或原 claims 解码。
- API Key 必须同时有新 grant 和目标插件 owner；STS 插件 manifest/批准及 allowed_capabilities 按正式能力声明配置。
- 同步 Go/.NET、skeleton/scaffold/CLI，以及 AI Craft 已复制的 adapter/middleware；最新源码联调通过后再发布制品。
- 先部署 Core，再升级消费端；Core 网关改动及会话 JTI 关联需要重新编译/重启。受影响旧 access 需宿主续期/重新登录。

**本次仅 Core 实现及合同交付，未修改外部 Framework/AI Craft，也未部署或重启远程服务。**
