# 插件页面会话桥接 v2：Core 交付与 Framework 接入合同

> 日期：2026-10-10；状态：Core 前端代码已实施，Framework／插件接入及真实浏览器验收待完成。不能单独发布 Core 后宣称现有插件已经对齐。

## 1. 本次 Core 改动

1. `web-admin/app/utils/auth-session.ts` 使用 `powerx.session` 保存版本为 1 的完整宿主会话快照：sessionId、revision、active/signed-out、access／refresh、expiry、scope、tenant。
2. 普通 `access_token` 等字段是消费镜像，宿主不从它们或 Cookie 恢复会话。旧零散存储不自动迁移；部署后关闭旧相关页面并重新登录一次。
3. 登录／切租户新建 sessionId；续期增加 revision；退出立即建立新 signed-out 代际。API、路由守卫、WS 和 context 在途结果受会话检查约束。
4. 同源宿主标签页通过 Web Locks `powerx-auth-refresh` 串行续期；不支持 Web Locks 的环境明确失败，不能退回无约束并发刷新。生产需要 HTTPS，本机 localhost／127.0.0.1 使用浏览器支持的安全上下文。
5. 插件代理中的 login／refresh／logout 写入返回 HTTP 409、`error_code=AUTH_SESSION_HOST_MANAGED`，不调用上游、不写 Set-Cookie。宿主的 `/api/v1/admin/user/auth/*` 正常入口保留；代理 context 读取保留。
6. 桥接固定 protocolVersion=2，校验已注册 iframe window、精确 origin、pluginId、instanceId；认证消息以 sequence 排序。不再广播 refreshToken。
7. 首次入口捕获当前深链路，根页与子页统一由 catch-all 页面承载。正常子导航不更改 iframe src。
8. iframe load 仅为文档生命周期事件；页面可用需要当前认证适配器成功确认与导航确认。认证错误／协议超时不触发文档重载。未就绪 503 保留 12 次自动重载预算，超限停止；卸载清理 timer、监听与绑定。

这些是浏览器可靠性与信任来源改进。当前宿主会话仍含 JavaScript 可读凭证，同源 trusted 插件仍能读取宿主存储；停止广播 refreshToken 不等于完成第三方凭证隔离。后端 STS、plugin request token、capability／grant 和数据库合同没有改变。

## 2. 来源与消息合同

类型定义：`web-admin/app/types/pluginBridge.ts`；宿主实现：`app/utils/plugin-bridge-host.ts`；iframe 消费器：`public/powerx-bridge-client.js`。

所有消息必须携带 `protocolVersion: 2`、`pluginId`、`instanceId`。宿主消息 source=`powerx`；插件消息 source=`plugin` 或 `powerx-plugin`。hello 在学习宿主 instanceId 前允许空 instanceId；其余消息必须匹配注册实例。

| 方向／type | 字段 | 语义 |
| --- | --- | --- |
| 插件 → 宿主 `bridge:hello` | envelope | 发起受来源校验的初始化 |
| 宿主 → 插件 `sync` | locale、theme、hostOrigin | 学习当前 instance，同步外观；hostOrigin 不作为接收方自报信任来源 |
| 插件 → 宿主 `bridge:ready` | navigationReady | 注入客户端／导航适配器状态；不代表认证完成 |
| 宿主 → 插件 `auth-token` | sequence、sessionId、revision、accessToken、tokenType、expiresAt、expiresIn、scope、tenantUuid | 当前宿主访问会话；没有 refreshToken |
| 宿主 → 插件 `auth-cleared` | sequence、可选 sessionId／revision | 清除插件消费状态与在途操作，不恢复旧缓存 |
| 插件 → 宿主 `auth:applied` | sequence、sessionId、revision | 当前用户 context 验证成功后确认；注入消费器负责发送 |
| 插件 → 宿主 `auth:rejected` | sequence | 当前认证适配器失败；注入消费器负责发送 |
| 插件 → 宿主 `auth-token:request` | envelope | 获取当前会话；过期时由宿主统一续期 |
| 插件 → 宿主 `request-sync` | envelope | 显式重新同步；相同 Token 也会重新发送 |
| 宿主 → 插件 `navigate` | requestId、path | 当前插件挂载点内的目标路由 |
| 插件 → 宿主 `navigation:result` | requestId、ok、可选 code | 实际路由结果；旧 request ID 不改变当前状态；失败 code 仅使用约定的导航错误码 |

旧协议不参与会话更新。不能用 `'*'`、旧别名或无版本回写静默补齐；缺少适配会显示桥接／认证／导航未就绪提示。

## 3. Framework 与插件必须实施的适配

Core 消费器位于 **iframe 自己的 window**，不是宿主 window。公共接口：

```typescript
interface PowerXFrameBridgeClient {
  protocolVersion: 2;
  onAuthToken(handler: (message: AuthToken | AuthCleared) => void | Promise<void>): () => void;
  setNavigator(handler: (path: string, request: {
    signal: AbortSignal; requestId: string; instanceId: string;
  }) => void | Promise<void>): () => void;
  requestAuth(): void;
  requestSync(): void;
  getLastSyncAt(): number;
}
```

上面 AuthToken／AuthCleared 是第二节消息类型的简称；最终 TypeScript 类型以 Core 文件为准。`window.PowerXBridgeClient` 只在宿主注入消费器后存在；独立模式使用插件自己的登录合同，不能假设该对象存在。

认证适配器要求：

1. 通过共享 Framework 路径订阅当前快照，更新插件自己的受控内存会话；不得写 `powerx.session` 或用旧缓存回写宿主镜像。
2. `auth-token` 回调必须返回 Promise，等待当前 Token 的 context 读取、租户／成员验证完成。消息到达后立刻保存必要字段不等于认证验证完成。
3. 每次异步操作捕获 sessionId／revision／sequence；过期结果不能覆盖新状态。旧认证失败不能清空新会话。
4. `auth-cleared` 清理插件认证状态、用户／租户业务缓存、WS 和在途请求；不能删除宿主或其他插件的存储。
5. access 到期／缺失时调用 requestAuth，有限等待新快照后重试一次；不访问 scoped refresh、不自行恢复备用 Token、不增加周期轮询。
6. 迟注册的适配器可以得到注入消费器缓存的最新未过期快照。Framework 的重复 Token 去重不能阻止本地状态恢复。

导航适配器要求：

1. 用插件实际 Router 注册 setNavigator；不得直接整页 location 跳转或通过 iframe src 代替普通导航。
2. 尊重 request.signal。新的导航、实例切换与退出会取消旧请求；异步守卫／数据加载返回后再次确认未取消。
3. 等待 Router 完成；Core 消费器比较实际 pathname／query／hash 后发结果。目标仅限当前插件挂载点，不接受另一插件或外部 URL。
4. 初始目标已是当前深链路时，消费器可直接确认同一地址；这不意味着未接入 Router 的插件已经支持后续导航。
5. 组件卸载时取消两个订阅／适配器；Framework、Skeleton、scaffold 和 CLI 内嵌模板必须同一次更新。

### 导航请求与失败分类（2026-10-11 修复）

- 一次待完成导航绑定自己的 requestId 和 path；相同待完成目标不重复发送，避免取消同一个 Router 操作。
- 开始新导航后，旧成功路径不能证明 iframe 当前位置。切回旧页面必须发送新的确认请求；过期结果不能修改新请求状态。
- iframe 文档 load 清除旧导航及成功路径状态，重新完成当前实例的认证与导航确认。
- 导航挂载范围由 iframe 入口确认的 pluginId 固定为 `/_p/{pluginId}/admin`；不读取水合后可能被 Nuxt 删除的 `window.__NUXT__`。已带命名空间的地址仍须通过当前插件范围校验，不能把跨插件或外部 URL 重新包装成合法地址。
- `PLUGIN_NAVIGATION_UNSUPPORTED` 仅用于缺少导航适配器；适配器拒绝或抛错返回 `PLUGIN_NAVIGATION_FAILED`，结束后实际路径不同返回 `PLUGIN_NAVIGATION_ROUTE_MISMATCH`，非法或跨插件地址返回 `PLUGIN_NAVIGATION_INVALID`。
- `code` 是 protocol v2 的兼容新增字段。旧消费器仅返回 `ok=false` 时，宿主依据该实例已报告的 `navigationReady` 区分缺少适配器与普通导航失败，不把所有失败都提示成“升级插件”。
- 核对注入消费器的加载版本后复验“小样 → 大货 → 支付单 → 小样”及快速来回切换；Core 回归测试不能替代已安装插件的真实页面验收。

AI Craft 与 CRM 等仓库中的独立 useAuth 也需改为共享适配器。仅升级依赖包不能替换复制代码。当前已安装 AI Craft 0.1.75 尚未据本合同完成接入。

## 4. 部署与验收

1. Framework 实现并发布支持本合同的实际版本，记录版本号；本文不虚构已发布的包版本。
2. AI Craft 接入、重新构建包；核对安装包的 JS 和依赖版本。其他受影响插件按审计结果逐个验证。
3. Core 与新插件版本组合先在本地／开发环境验收。关闭旧相关标签页，前端新代码加载后重新登录一次。无需为本次前端合同执行 migrate。
4. 使用[会话矩阵](../guides/features/plugin-session-repair/usecase-us1-session-consistency.md)及[首次刷新矩阵](../guides/features/plugin-session-repair/usecase-us2-first-refresh.md)。尤其验证首次刷新、身份确认、正常续期、WS、图片／下载和根页↔子页。
5. 老插件显示升级／适配未就绪属于合同拒绝，不能用隐藏 fallback 绕过。发布和回滚使用配套版本组合。

Core 定向验证命令（在 web-admin）：

```bash
npx vitest run tests/unit/auth tests/unit/settings-users/tenant-context.spec.ts tests/unit/settings-users/user-store-context.spec.ts
npm run build
bash scripts/check-refactor.sh
```

2026-10-10：定向测试 54 项通过；构建与自检已通过。[脱敏验证记录](evidence/plugin-session-core-20261010.json)包含命令、运行脚本 SHA256 与验收限制。用户日常 Core 与 Nuxt 未由 Agent 重启，没有新增运行服务，没有修改数据库业务数据。真实浏览器连接仍返回 `nodeRepl.fetch request failed`，账号密码未提供；本次不能声称实际登录与首次刷新已完成验收。匿名健康／页面入口 200 仅证明入口可访问。
