# 插件 WS 身份实时复核合同

日期：2026-10-11。Schema v1。Core 合同与实现；Framework / Skeleton / 插件负责连接关闭与 topic 授权接入。

## 1. 用途与边界

长连接不能只在握手时检查身份。Core 退出会撤销 refresh-session，成员停用和权限撤销会修改 IAM 的当前状态；Framework 必须在处理每条业务入站消息、订阅动作和业务下行消息前复核该连接，并使用返回的当前权限检查 topic。

本合同提供**消息触发的服务端实时复核入口**。不增加定时轮询，不把 ping/pong 当成周期身份请求。空闲连接由 Framework 按有效期关闭；本合同不承诺空闲连接在撤销瞬间得到推送通知。下一条业务消息必须在复核通过后才能执行或投递。

原请求票据仍然只有 60 秒有效期。WS 复核凭证是另一种不透明证明，绑定同一部署、数据库命名空间、插件、租户、用户、成员、原登录 session JTI、原 access 到期时间和原路由权限。它不是新的登录、续期或任意用户查询接口。

## 2. 取得服务端复核凭证

按 [请求身份合同](plugin-request-identity.md) 解析可信 Core 网关注入的 `ws_grant` / `ws_upgrade` 票据。解析成功后额外返回：

```json
{
  "ws_revalidation_handle": "<server-only opaque handle>",
  "ws_revalidation_expires_at": "<original user access deadline>"
}
```

- 仅服务凭据解析 WS 来源的票据时返回；普通 HTTP 票据和 Admin 检查不签发。
- 格式为 `pxwsi_v1_` 加 43 个 base64url 字符；同一票据重复解析返回同一个凭证和固定到期时间。
- 原 ticket、复核凭证、浏览器 WS credential 是不同对象，不能互换。
- 凭证保存在插件服务端的连接状态中，不返回浏览器，不放入 URL、不打印日志。Core 对 invocation 请求和 typed response 的审计副本脱敏该字段。
- WS credential / 连接寿命不得超过 `ws_revalidation_expires_at` 与 `user_session_expires_at`，也不能超过插件自有 WS credential 的更短寿命。

## 3. 正式调用

Capability：`com.corex.iam.identity_context.service_read`。

固定 endpoint：`core://iam/identity-context`，method：`INVOKE`，operation：`revalidate_ws_identity`。

```json
{
  "capability_id": "com.corex.iam.identity_context.service_read",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "operation": "revalidate_ws_identity",
      "request": {
        "schema": "powerx.iam.plugin-ws-identity-query/v1",
        "ws_revalidation_handle": "<server-only opaque handle>"
      }
    }
  }
}
```

Tenant REST：`POST /api/v1/tenant/iam/identity-contexts:revalidate-ws`。Body 是上述 `request`，无 query，最大 4096 字节，未知字段和用户／租户／权限覆盖明确拒绝。

Admin 自身检查：`POST /api/v1/admin/iam/identity-contexts:revalidate-ws`；仅用户 JWT + IAM `iam / identity_context / read`，只检查同用户、同登录会话、同租户的既有凭证，不签发凭证。服务凭据不能调用这个平面。

API Key / STS 均使用原 identity-context 的精确 service grant；继续检查当前凭据、Key/Profile、published capability、tenant registration、插件 owner 和 allowed_capabilities。无需扩大到 admin_manage。租户和插件身份来自可信认证上下文。

## 4. 成功响应与权限判定

REST 标准 Core envelope 的 `data.schema` 是 `powerx.iam.plugin-ws-identity/v1`，typed invocation 的业务结果是 `payload.result`。其余身份、角色、权限、版本、原请求和 Trace 字段沿用请求身份响应。

- `permissions` / `roles` / `policy_version` / `perms_hash` 每次从当前 IAM 读取，不使用握手时的缓存权限。
- `proof_expires_at` 在本操作中表示 WS 复核凭证到期时间；原请求解析中仍表示 60 秒 ticket 的到期时间。
- `ws_revalidation_expires_at` 和 `user_session_expires_at` 固定为原 access 到期上限，复核不续期。
- 原路由必要权限已撤销时直接拒绝；其他 topic 所需权限须由 Framework 使用当前返回权限再次判断。
- Root 仍要求真实有效用户、会话及选定租户；普通用户要求真实有效且相互匹配的用户、成员、租户。Root 无成员规则沿用原合同。
- 返回前再次检查服务授权和主体状态；复核期间角色／权限变化返回 409，旧结果不得授权消息。

## 5. Framework 处置

| 响应 | 连接与消息处置 |
| --- | --- |
| 200 | 核对绑定身份，检查当前 topic 权限后才处理消息；成功结果不缓存给后续消息 |
| 401 `IAM_CONTEXT_USER_SESSION_INVALID` / `IAM_CONTEXT_USER_SESSION_EXPIRED` | 丢弃消息、关闭对应连接；正常重新授权须走新的受保护 WS grant |
| 403 `IAM_CONTEXT_USER_FORBIDDEN` / `IAM_CONTEXT_MEMBER_REQUIRED` / `IAM_CONTEXT_REQUEST_FORBIDDEN` | 丢弃消息并关闭连接；不能刷新 Token 规避停用／撤权 |
| 401 `IAM_CONTEXT_WS_HANDLE_REQUIRED` / `INVALID` / `EXPIRED` | 缺少、伪造或过期的服务端证明；关闭连接，通过新 WS grant 取得新证明，不恢复旧缓存 |
| 服务凭据／service grant／owner／插件／租户拒绝 | 关闭连接并保留失败平面；不把它包装成人类用户登录过期 |
| 409 `IAM_CONTEXT_POLICY_CHANGED` | 丢弃本条消息和旧身份结果，关闭并通过受保护 grant 重新授权 |
| 503 `IAM_CONTEXT_UNAVAILABLE` | 不授权、不执行／投递本条消息；关闭连接并显示依赖错误，不能使用旧权限继续发送 |

同时检查 Framework 自己的 topic、资源和连接绑定，不允许调用方传入不同 user/member/tenant/plugin 来覆盖结果。取消和退出仍须使用共享 session 代际检查，防止迟到结果恢复已经清理的连接。

### Typed invocation 错误状态

`POST /api/v1/tenant/invocations` 必须保留 Core typed `IAM_CONTEXT_*` 业务错误的原始 HTTP 状态，与上表及直接 REST 调用一致；不能统一改成 502。响应 `code` 是实际 HTTP 状态，`error_code`、`reason_code` 和 `details.reason_code` 保留原业务错误码；请求携带的 invocation `trace_id` 保留在 `details.trace_id`，请求追踪仍由 `X-Trace-ID` / `request_id` 标识。

例如，`revalidate_ws_identity` 的 `IAM_CONTEXT_WS_HANDLE_EXPIRED` 返回 HTTP 401；撤权返回 403；复核期间策略变化返回 409；依赖不可用返回 503。Framework 应读取真实 HTTP 状态和业务码，保持用户会话、服务凭据、证明失效三个失败平面的区分。此修复只补 HTTP 错误映射，不改变身份复核和授权逻辑，不需要迁移或重复授权。

## 6. 接入顺序与验收

1. Core 更新代码与能力目录，加载此合同。
2. Framework 增加强类型 WS query/result 和 `revalidate_ws_identity` 方法；在 grant/upgrade 解析结果中接收并仅保留服务端凭证。
3. 在 WS 业务入站、subscribe 和下行投递前接入实时复核；按表执行关闭。按原 access / WS credential 的期限安排到期关闭。
4. 同步 Skeleton、scaffold、CLI 模板和 AI Craft 等已复制的授权／连接实现。
5. 验证原 ticket 过期后凭证仍可在其固定期限内使用；真实退出、成员停用、角色／权限及 API Key grant 撤销后下一条消息被拒绝；跨租户／插件、伪造凭证和依赖失败均拒绝。

不需要新增数据库迁移。能力声明变化后须运行 capability-check / capability-seed，并核对开发 Host Key 的 grant-status。Core 自动化验收不替代 Framework 真实连接关闭和浏览器联调。

实现：`backend/internal/service/plugin_identity/ws_revalidation.go`；共享权威检查：`service.go`；HTTP dispatcher：`internal/transport/http/{admin,openapi}/plugin_identity/routes.go`；OpenAPI：[identity-context.openapi.yaml](../../specs/018-plugin-request-identity/contracts/identity-context.openapi.yaml)。

验收证据：[plugin-ws-revalidation-20261011.json](evidence/plugin-ws-revalidation-20261011.json)。真实 API Key 与 gRPC STS Exchange 的复核均返回 200；成员停用／权限撤销返回 403；真实用户退出后返回 401；service grant 撤销返回 403。隔离环境已清理。

Typed invocation 状态透传补验：[plugin-iam-invocation-errors-20261011.json](evidence/plugin-iam-invocation-errors-20261011.json)。HTTP 路由回归覆盖 400 / 401 / 403 / 409 / 503 和包装后的业务错误；真实 API Key / STS 经 Selector → Router → Core Invoker 的结果独立记录，不把直接 REST 验收当作 typed invocation 验收。

用户重启开发 Core 后，8077 的真实 Host API Key typed 调用已复验：`IAM_CONTEXT_WS_HANDLE_EXPIRED` 返回 HTTP 401，Trace `9aa5448f-976f-4625-b6ef-c1f430500c49`；非法 schema 返回 HTTP 400，Trace `99dd3991-61b6-40b8-b28e-3ad139a51c7e`。HTTP 状态、业务码、invocation Trace 和 Core 请求 Trace 均已核对。
