# 插件会话终止与宿主登录恢复 v2

日期：2026-11-07。Core 源码和受控验收见[验证收据](evidence/plugin-session-termination-validation-20261107.json)；Framework、AI Craft 的接入、日常服务部署和实际用户登录验收分别交付，不把本合同视为三方已完成上线。

## Core 行为与期限规则

沿用现有 session bridge v2、用户 JWT、服务端 `iam_refresh_tokens` 登录记录和宿主 `powerx.session` 快照。宿主浏览器 `sessionId` 是浏览器会话代际，不是 JWT 的 JTI；通知必须使用已接收快照的 sessionId/revision，不能自行解码 JTI 代替。

登录、Root 登录、切租户和 SaaS 注册的签发共用 `issueSessionTokens`：

1. 依据当前 `RefreshTTL` 生成固定期限的 Refresh Token。
2. 数据库会话的 `expires_at` 与该 Refresh Token 的实际 exp 一致。
3. Access Token 的 exp 为 `min(当前时间 + AccessTTL, 登录会话截止时间)`。
4. 刷新仍检查原 JWT、登录记录、撤销、绑定和当前用户/租户/成员状态；只返回 Access Token，不更新会话截止时间、不滑动续期、不轮换原 Refresh Token。
5. 刷新响应省略 `refresh_token`，宿主在受检查的同一刷新事务内保留原值。`expires_in` 取实际 Access Token 的剩余时间，不再直接返回配置的 AccessTTL。

现有配置即使是 AccessTTL=720h、RefreshTTL=336h，也不会签发超出登录会话期限的 Access Token。本次不修改这些配置值，不回写历史登录记录，也不删除或延长历史会话。已经结束的会话必须重新登录。

## 当前用户的权威核验

宿主调用 `GET <Core API prefix>/admin/user/auth/me/session`，使用自己当前的用户 Access Token；不经插件、STS、API Key 或 delegated 能力代理。

- 请求不接受用户、租户、会话选择字段；query 和 body 均拒绝。身份只取 Core 已认证上下文。
- 读取当前 JTI 对应的登录记录，检查固定期限、撤销和 user/tenant/member 绑定；再次检查用户、租户和成员状态。Root 没有成员记录时保持现有合法规则。
- 成功返回 `data: {status:"active", session_expires_at:<Unix ms>, access_expires_at:<Unix ms>}`，`Cache-Control: no-store`。
- 不签发 Token，不延长会话，不修改撤销状态。
- 属于用户认证私有入口，明确进入 capability audit ignore；不发布为租户能力，不增加 Key/Profile/grant，不加入 STS direct 入口。

| HTTP / code | 宿主处理 |
|---|---|
| 401 `AUTH_ACCESS_EXPIRED` | 明确是 Access Token 过期；由宿主按既有 Web Locks 续期一次并重新核验，不能循环 |
| 401 `AUTH_SESSION_EXPIRED` | 登录会话已到期，当前代际清理并进入登录 |
| 401 `AUTH_SESSION_REVOKED` | 登录会话已撤销，当前代际清理并进入登录 |
| 401 `AUTH_SESSION_INVALID` | 当前用户凭据/会话不存在或绑定不符，当前代际清理并进入登录 |
| 刷新 401 `AUTH_REFRESH_INVALID` | 原刷新凭据已无效，进入登录，不尝试绕过 |
| 403 `AUTH_USER_FORBIDDEN` | 保留宿主会话，显示错误，不当作 Access Token 过期 |
| 503 `AUTH_SESSION_UNAVAILABLE`、网络错误 | 保留宿主快照、页面和草稿，显示暂时不可用，不改写为 401 |
| 未分类 401 或服务/proof 错误 | 不据此清除宿主登录 |

全局 JWT 中间件在到达核验 Handler 前拒绝请求时，也为此专用入口保留上述可区分错误；不会因此改变其他业务路由的错误合同。

## 插件 → 宿主通知合同

沿用 `protocolVersion=2`。新增的报告是核验提示，不是退出命令。

```ts
window.PowerXBridgeClient.reportSessionInvalid({
  sessionId: snapshot.sessionId,
  revision: snapshot.revision,
  code: 'IAM_CONTEXT_USER_SESSION_INVALID',
});
```

注入消费器从最后接收的 auth-token 得到 sequence，发送：

```ts
{
  source: 'plugin', protocolVersion: 2,
  pluginId, instanceId,
  type: 'auth:required', sequence,
  sessionId, revision,
  code: 'IAM_CONTEXT_USER_SESSION_INVALID'
}
```

不发送 Access/Refresh Token、用户自报身份、跳转 URL 或错误原文。该方法只接受 `IAM_CONTEXT_USER_SESSION_INVALID`；调用方不能用服务错误、权限错误或未知 401 触发宿主验证。

初始化 context 读取失败时，注入消费器也会在 `auth:rejected` 中带上 sessionId/revision 和这个经过白名单检查的机器码。普通适配失败继续只显示错误。Framework 必须保留结构化原始错误，不能一律包装为 `PLUGIN_AUTH_CONTEXT_FAILED` 后丢失原因。

Core 宿主依次检查：已注册 iframe window、精确 origin、pluginId、instanceId、协议版本、当前 auth sequence、当前 sessionId/revision 和机器码。通过后才调用宿主核验接口：

- 服务器仍确认 active：保留登录，只重新同步当前快照；抑制同一轮适配失败循环。
- 当前会话已经结束：清除当前代际，广播 auth-cleared，再进入统一登录。
- 失败或临时不可用：保留登录并显示核验错误。没有后台轮询；手动重试/后续显式报告才再次检查。

同一快照的并发报告共享核验请求；窗口内重复报告被合并。后续显式报告仍可发现刚发生的到期/撤销。frame 卸载会撤销自己的恢复等待；所有等待者都离开时取消实际核验请求。新登录、切租户或 revision 改变后，旧结果不能清除新状态或跳转。

登录返回地址取宿主当前 route 的 pathname/query/hash，禁止由插件消息指定。Token 不加入 URL，不重复进入登录循环。插件 Document 被拒绝的恢复也接入同一宿主核验流程，仍保留既有 Cookie 单次修复预算。

## Framework 需要对齐

只需要通用前端会话客户端对齐，不让 Framework 插件调用新的用户认证私有接口。

1. 在 `framework-client/bridge.ts` 的 `PowerXFrameBridgeClient` 类型及封装中加入上述 `reportSessionInvalid`。需要该恢复功能时检查方法存在；旧 Core 不支持时明确报告桥接版本能力缺失，不能自行跳父窗口、清理宿主存储或调用宿主刷新接口。
2. 在 API/WS/SSE 的明确 `IAM_CONTEXT_USER_SESSION_INVALID` 分支，捕获产生这次请求的快照；如果仍是当前 snapshot，通知宿主，然后停止受保护请求、清理本插件身份消费和连接。多个请求的相同失效只通知一次。
3. 原有 `IAM_CONTEXT_USER_SESSION_EXPIRED` 继续使用 `requestAuth()` 等待新快照后最多重试一次。INVALID 不当作可续期；403、服务凭据、proof、未知 401/503 不发送登录通知。
4. 初始化 `PowerXHostSession.apply` 的 context 失败保留原结构化 code 并抛出，让 Core 注入消费器报告 auth:rejected；晚到的旧 snapshot 失败不得通知新代际。
5. auth-cleared/新登录仍按既有规范取消请求、Router 操作、WS/SSE，避免旧结果写入新租户状态。用户重新登录后的业务写入不得由通用恢复器自动重放。
6. 同步共享客户端、Nuxt Admin、Skeleton、scaffold/CLI 内嵌模板和可消费包。源码测试通过不等于依赖已发布、已安装或实际插件已对齐。

建议 Framework 回归：当前/旧 snapshot、同一错误并发、服务错误/403/503、错 origin/instance、旧 Core 方法缺失、请求取消、新登录竞态和初始化原错误保留。

## AI Craft 需要对齐

- 消费更新后的 Framework 通用入口，移除复制的“全部 401 直接退出/刷新/跳转父窗口”逻辑。
- 合并失效提示，停止受保护操作；不要求 Core 接收插件自报的成员、租户或返回 URL。
- 草稿由插件自己的业务机制保存，绑定原用户、租户和页面；重新登录后核对身份再恢复。不能把宿主 Token 保存进草稿，也不能自动恢复给另一个用户/租户。
- 使用原宿主深链路返回；草稿保存不能依赖 Core 等待 iframe 卸载 ACK。Core 不读取、不迁移 AI Craft 的业务草稿。

## 验收与部署

受控验收使用临时 SQLite、模拟 HTTP 后端和真实浏览器进程，不修改日常数据库或登录记录。至少覆盖：

- 30 天 AccessTTL 与 14 天会话期限；刷新临近截止、到期和撤销；Root/成员绑定；实际 expires_in 与刷新凭据省略。
- 后端数据库不可用保持 503，前端不清理；服务身份与跨身份查询拒绝。
- 桌面/窄屏旧 Access 仍有效、服务端会话已结束时跳登录并保留 query/hash。
- 插件误报但服务器确认 active 不退出；503/403不退出；Cookie 修复、续期、旧消息和在途新登录回归。

Core 所有者按原方式重新编译并重启后端及前端，再由 Framework/AI Craft 使用正式消费版本完成真实登录联调。本轮不重启日常服务、不延长历史会话、不修改 Key/Profile/授权或数据库 schema。

本轮结果：Go race 回归 256 条（含子场景）、前端单元回归 109 条、Chromium 浏览器回归 13 条通过；包含 1440/390 宽度下的登录恢复。Core 编译、前端隔离构建、指定包 vet 和 capability-check 通过。临时进程已退出，3473/3474 端口已释放。浏览器后端为受控模拟，不是已部署 Core 与真实 AI Craft 的联调验收。
