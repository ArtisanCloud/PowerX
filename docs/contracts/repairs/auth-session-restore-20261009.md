# 恢复库切换后的本地登录会话回滚排查

修复分工、实施顺序与验收见[插件会话与首次页面刷新修复计划](../../guides/features/plugin-session-repair/guide.md)。用户补充刷新现象主要发生在首次刷新插件页面时；当前没有对应 Document 时间线，不能把它直接归因于 Token 回滚。

2026-10-10 更新：Core 已实施完整快照和桥接 v2，本文下面的 Cookie／iat 保护属于 10 月 9 日的历史尝试，不代表当前完整实现。当前交付与 Framework 接入要求以[桥接 v2 合同](../plugin-session-bridge-v2.md)为准；真实插件登录／首次刷新仍待验收。

## 当前证据

- Core 本地运行于 8077，当前 DSN 指向本机 Docker `127.0.0.1:55432/powerx_backup_verify`。
- 恢复库租户为 `1edd4132-1644-412d-abb4-d5f1e9487052`；旧本地库租户为 `6b5d0240-9920-46da-b707-88200e0f51ea`。
- 2026-10-09 16:49:59 登录成功，trace `743b31b5-e631-40e3-b043-a374ba86ade1`。
- 同时段新租户 me/context 返回 200，trace `84bc10cd-cc75-4e69-9731-a9e5acd85d38`；随后旧租户再次返回 403，例如 trace `ad87d45f-88ac-4d53-b3cf-16718ddeb6f7`。
- 这说明重新登录确实成功。仅据这些日志，不能认定用户未退出；后续请求继续携带旧 JWT 的具体写入来源还需浏览器当前 storage/Cookie 租户值确认。

## 已复现的代码风险与修复

`web-admin/public/powerx-bridge-client.js` 原先启动即用 Cookie 覆盖 localStorage，也无条件接受认证消息。两个回归测试均在原实现失败：新租户 token 被旧租户 token 替换。

修复：

1. iframe 启动时优先保留宿主已保存且未过期的 localStorage 会话，并同步 Cookie、JWT 到期时间和 JWT 中的租户。
2. 首次没有有效本地会话时仍可从有效 Cookie 初始化。
3. 迟到消息的 JWT 签发时间早于当前会话时，拒绝回滚。
4. 租户从 JWT claims 推导，不使用消息中不一致的 tenant 字段。
5. `auth-cookie.ts` 登录/退出时同步更新浏览器 Cookie，缩小 Nuxt watcher 执行前 iframe 读取旧值的窗口。

## 验证

- 原始两项桥接测试：均失败，成功复现覆盖。
- 修复后 auth-cookie、bridge、tenant-context、user-store-context：13 项通过。
- `npm run build`：通过。
- 运行中 3030 的 `/powerx-bridge-client.js` 已包含新保护逻辑。
- 尚未取得当前用户浏览器的 Cookie/localStorage 输出，也尚未完成修复后用户重新登录的真实浏览器复验；浏览器控制连接不可用。

## 用户复验

用户已反馈硬刷新后仍失败，不能把前述 Core 桥接修复视为真实链路已修复。

## 继续排查：已安装插件自己的认证写回

- 19:49:24 登录再次成功，trace `0012979d-8f68-4ac1-9bf2-79a06f27cd89`。
- 19:49:24.251 的 me/context 返回 200，trace `8b2fb7d9-9519-4980-9a71-8d7fd867dc8c`；19:49:24.507 返回 403，trace `e1c01e7e-576f-4a34-afc6-bdd0d4e92c93`。
- 本机已安装 AI Craft 0.1.75 的 `web-admin/.output/public/assets/D1WEYIpV.js` 包含独立 `useAuth` 实现，SHA256 为 `057c145561a5c9e5ddeec74a89a824ab47eb8d32fc3517325719f6f0dc708205`。
- 从该实际安装包提取认证函数，在 jsdom 中保留它的实际 storage 监听与写回逻辑，模拟宿主登录保存新会话后触发 storage 事件。结果：Token 的租户从 `1edd4132-1644-412d-abb4-d5f1e9487052` 回滚至 `6b5d0240-9920-46da-b707-88200e0f51ea`。复现没有调用登录 API、写业务数据库或修改插件包。

对应 AI Craft 源码 `web-admin/app/composables/useAuth.ts`：

1. `ensureStorageConsistency()` 仅在 `!insidePowerX` 时接受 localStorage 中改变的 Token；嵌入模式反而用 `token.value`／`RUNTIME_TOKEN_CACHE` 写回宿主共享的 `access_token`。
2. `syncFromStorage()` 在已有内存 Token 时仍保留 `token.value`，随后调用上述写回函数。
3. storage 监听调用 `syncFromStorage()`，所以宿主换会话会触发旧插件内存会话覆盖宿主。直接修 Core 注入脚本不能覆盖这个独立监听。

插件修复合同：嵌入模式以宿主当前会话为准；storage 事件应更新插件内存会话，不能把旧内存 Token 写回共享存储；宿主退出后不能从 `__px_access_token`、Cookie 或运行时缓存复活旧会话；异步刷新回包不得覆盖已切换的宿主会话。独立模式也需保留现有登录／退出语义。

实际浏览器回归应包含：插件页面已打开时退出再登录，以及存在另一个旧插件标签页时重新登录；检查新会话随后持续调用 me/context 返回 200，且宿主与插件 Token 租户均保持恢复库租户。

浏览器普通控制连接返回 `nodeRepl.fetch request failed`，IAB 不可用；Chrome DevTools 仅连接到独立空白浏览器。用户的 PowerX root 密码尚未提供，不能声称已经替用户完成真实登录。本轮只读取插件源码／安装包，未修改另一仓库。

Core、前端日常服务由用户启动，本轮未重启这些服务；没有修改数据库业务数据。
