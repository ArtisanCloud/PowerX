# 插件菜单导航状态修复

日期：2026-10-11。范围：Core Web Admin 的宿主桥接与 iframe 注入消费器。

## 观察与定位

AI Craft 0.1.76 在宿主重新登录后恢复数据读取。Core 日志已出现真实身份票据 issued/resolved；随后多次菜单切换出现“插件缺少新版页面导航适配，请升级插件”。已安装 JS 实际包含 `createPowerXRouterNavigator` 的 Router 适配逻辑。

旧宿主把每个 `navigation:result ok=false` 都显示为缺少适配器；该提示无法区分 Router 失败、目标路径不一致或真正缺少适配。

## 已复现并修复的 Core 问题

1. 导航尚未完成时切回上次成功页面：旧成功路径被当成当前位置，漏发新导航，旧请求还可能继续执行。
2. 相同目标的待完成导航重复发送：消费器取消前一个导航，可能造成 Router 操作互相取消。
3. 导航失败后切回旧成功路径：缓存导致不重新确认，页面保留旧错误状态。
4. iframe 文档重新 load：旧导航与成功路径状态需要清除，再完成认证及当前目标确认。
5. 失败提示分类：缺少适配器、Router 失败、目标不一致及非法地址分别返回稳定错误码。宿主仍校验 origin/window/插件/实例/当前 requestId；旧结果不修改当前请求状态。
6. 水合后丢失挂载前缀：Nuxt 的 `revive-payload.client` 在水合完成后删除 `window.__NUXT__`，旧消费器随后无法读取 runtime config，导致宿主相对菜单地址不在插件挂载范围。挂载范围现由 iframe 入口确认的插件 ID 确定，不依赖 Nuxt 临时全局对象。全路径、查询及 hash 保留原语义；跨插件、协议相对地址、外部 URL 及越界路径仍明确拒绝。

## 验证与应用

- 修复前四个定向回归场景失败，修复后通过。
- `npx vitest run tests/unit/auth tests/unit/settings-users/tenant-context.spec.ts tests/unit/settings-users/user-store-context.spec.ts`：10 文件、67 测试通过，包含删除 Nuxt 全局对象后的连续导航及地址范围检查。
- 隔离 Chromium fixture 使用真实 Vue Router 与当前 Framework 导航适配器，删除 Nuxt 全局对象后连续导航 12 次成功，跨插件请求明确失败；临时浏览器和子进程均已退出。
- 注入脚本语法检查、四语词条 JSON 校验、重构自检与 `git diff --check` 通过。
- 当前 `http://127.0.0.1:3030/powerx-bridge-client.js?v=2` 返回 200，响应与工作区脚本逐字节一致。
- 刷新宿主整页加载新消费器后，复验“小样 → 大货 → 支付单 → 小样”及快速来回切换。

截图中的 `PLUGIN_NAVIGATION_INVALID` 已定位到 Core 依赖 Nuxt 临时全局对象的挂载路径丢失。上述浏览器 fixture 验证导航合同；用户浏览器里的已安装插件菜单与业务数据仍需刷新新脚本后复验。此次没有修改外部 Framework/AI Craft 仓库，也没有重启用户服务。

正式合同：[会话桥接 v2](../plugin-session-bridge-v2.md)。证据：[Core 导航修复](../evidence/plugin-navigation-core-20261011.json)。
