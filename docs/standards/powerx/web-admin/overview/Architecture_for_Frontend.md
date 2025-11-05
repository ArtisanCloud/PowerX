# Nuxt 前端架构总览

> 适用角色：前端与架构工程师 / 解决方案顾问 / QA  
> 代码来源：`nuxt.config.ts:7-107`、`app/app.vue:1-44`、`app/layouts/default.vue:1-115`、`app/pages/agent/index.vue:1-200`、`app/composables/api/index.ts:1-198`、`app/composables/agent/useDualChannelConnection.ts:1-200`、`app/middleware/auth.ts:1-55`

## 摘要
- 应用以 Nuxt 4 SPA 形式运行，`runtimeConfig.public` 统一注入 REST 与 WS 入口，并通过插件装配全局拦截、主题及加载态。  
- 页面层由 `app/layouts/default.vue` 和 `workflow.vue` 提供两套布局，结合 `app/middleware/auth.ts` 做登录守卫与公共路由豁免。  
- 业务模块通过 Pinia（`~/stores/**`）、组合式 API 服务（`~/composables/api/services/*`）与 SSE/WS 双通道聊天能力协作。

## 代码来源
- 配置入口：`nuxt.config.ts:7-107`（SSR 关闭、模块声明、`runtimeConfig`、Nitro 代理）  
- 应用壳：`app/app.vue:1-44`，`app/layouts/default.vue:1-115`，`app/layouts/workflow.vue:1-200`  
- 路由与守卫：`app/pages/**`，`app/middleware/auth.ts:1-55`，`app/plugins/redirect-p-to-plugins.client.ts:1-11`（占位）  
- 数据与服务：`app/composables/api/index.ts:1-198`，`app/plugins/api.ts:1-74`，`app/stores/*.ts`  
- 实时通道：`app/composables/agent/useDualChannelConnection.ts:1-200`，`app/stores/message.ts:1-200`

## 关键事实
### 应用壳与布局
- Nuxt 以 `srcDir: "app"` 和 `ssr: false` 运行，默认暗色模式并允许 localStorage 首选项（`nuxt.config.ts:7-52`）。  
- `app/app.vue:10-43` 注册全局 Loading、LoadingIndicator 及告警 Teleport，确保每个页面共享相同壳层体验。  
- `app/layouts/default.vue:24-83` 控制 Sidebar/Header/Footer 组合，移动端通过 `useWindowSize` 自动折叠；`route.path` 为 `/agent` 与 `/workflow` 时隐藏底栏。  
- `app/layouts/workflow.vue:1-200` 提供另一路径的画布布局，含工具栏、属性面板与全屏切换；通过 `useColorMode` 绑定暗色主题。

### 路由映射与中间件
- 页面源位于 `app/pages/`，如 `home/index.vue`（欢迎页）、`dashboard/index.vue`（指标仪表盘）、`agent/index.vue`（主聊天工作区）、`plugins/market.vue`（插件市场）等。  
- `app/middleware/auth.ts:1-55` 在客户端拦截所有非公共路径（`/home`、`/users/login` 等），缺 token 时清理本地凭证并重定向登录。  
- 多语言策略使用 `@nuxtjs/i18n`，采用 `strategy: "no_prefix"` 并设置 `langDir: "locales"`（`nuxt.config.ts:84-102`），侧边栏标题通过 `useI18n` 动态解析（`app/components/layout/Sidebar.vue:56-200`）。  
- 旧的 `/plugins` 根路由通过 `app/pages/plugins/index.vue:1-9` 在客户端跳转至 `/plugins/market`。

### 状态管理与数据流
- Pinia 由 `@pinia/nuxt` 自动注入；示例：`app/stores/user.ts:1-171` 处理用户上下文、租户切换及 Root 判定，`app/stores/permission.ts:1-195` 聚合权限目录与角色勾选缓存。  
- API 客户端通过 `setApiConfig` 暴露拦截器链，附带全局 Loading 计数（`app/composables/api/index.ts:34-118`）；插件 `app/plugins/api.ts:6-74` 根据登录态设置 Authorization 并在 401 时清理状态。  
- 菜单与业务服务置于 `app/composables/api/services/*`，例如 `menuService.ts:1-198` 规范化后端菜单结构、排序及翻译；`adminPluginsService.ts:16-73` 管理插件安装启停。  
- 共享 UI 状态（如 Loading、Alert、Theme）由 `app/plugins/global-loading.ts:1-59`、`app/components/GlobalAlertNotification.vue:1-28`、`app/plugins/theme.ts:18-53` 协调。

### 实时通道与会话协作
- `app/composables/agent/useDualChannelConnection.ts:1-200` 同时维护 SSE 与 WebSocket：`reconnectSSE`/`reconnectWS` 探测可用性、`sendMessage` 统一 REST 发送、`messages` ref 同步到 `useMessageStore`。  
- 会话管理落在 `app/composables/agent/useChatSessions.ts:77-198`（REST 拉取历史、缓存分页）与 `app/stores/agentSession.ts`（缓存当前 Agent 与会话列表）。  
- `app/components/agent/ChatInterface.vue:1-200` 负责消息滚动、输入框行为和未读条数，配合父级页面 `app/pages/agent/index.vue:18-200` 进行 Agent 切换、会话 CRUD 与错误反馈。

### 主题、UI 与生态
- Nuxt UI（`@nuxt/ui`）和 Tailwind v4 通过 `tailwind.config.ts:1-18`、`app/assets/css/main.css` 等文件提供主题令牌；颜色模式首选项写入 `localStorage` 并以 `data-theme` 反映（`app/plugins/theme.ts:24-32`）。  
- `app/components/layout/Header.vue:1-200` 聚合搜索、通知、用户菜单；搜索调用 `useSearch()` 获取建议，通知调用 `useNotifications()`（需后续文档补齐）。  
- i18n 词条按模块划分，如 `i18n/locales/en.json:1-80`，与 `Sidebar` 中的 `titleI18n.key`/`title` 映射保持一致。

## 示例代码
```ts
// 双通道聊天入口（摘自 app/composables/agent/useDualChannelConnection.ts:36-118）
export function useDualChannelConnection(agentId?: Ref<number|null>, sessionId?: Ref<string|null>) {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBase;
  const sseActive = ref(false);
  const wsActive = ref(false);

  const buildWSUrl = (path: string, params?: Record<string, any>) => {
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    const host = location.host;
    let url = `${protocol}//${host}${apiBase}${path}`;
    if (params) {
      const qs = Object.entries(params)
        .filter(([, v]) => v != null)
        .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(typeof v === "string" ? v : JSON.stringify(v))}`)
        .join("&");
      if (qs) url += (url.includes("?") ? "&" : "?") + qs;
    }
    return url;
  };
  // ... 省略 SSE/WS 连接恢复与消息同步逻辑
}
```

## 表格
| 配置项 (`runtimeConfig`) | 默认值 | 功能 | 源码定位 |
| --- | --- | --- | --- |
| `public.apiBase` | `/api/v1` | REST 前缀，与 Nitro 代理 `/api/` 联动 | `nuxt.config.ts:21-70` |
| `public.wsUpstream` | `ws://127.0.0.1:8077/api` | 后端 WS 代理根 | `nuxt.config.ts:25-29` |
| `public.wsUrl` | `/ws` | 同域 WebSocket 路径（可被反向代理覆盖） | `nuxt.config.ts:29-30` |
| `public.defaultLanguage` | `zh` | i18n 默认语言 | `nuxt.config.ts:31-36` |
| `public.defaultTheme` | `auto` | 颜色模式首选项，与主题插件同步 | `nuxt.config.ts:37-41` |

## 最佳实践与陷阱
- **统一鉴权入口**：任何新服务都应复用 `useApiClient` 并避免绕过 `setApiConfig` 的拦截器（`app/composables/api/index.ts:34-118`）；直接调用 `$fetch` 将无法自动附加 token 与 Loading。  
- **路由守卫双端差异**：`app/middleware/auth.ts:8-55` 在客户端依赖 `localStorage`，若要支持 SSR 需改为服务端可访问的 token 源并防止 `navigateTo` 循环。  
- **实时缓存同步**：在 Agent 页切换会话前应调用 `useDualChannelConnection().clearMessages()`，否则缓存与 `useMessageStore` 状态会错位（`app/pages/agent/index.vue:184-189`）。  
- **主题同步**：自定义组件需要监听 `window` 的 `theme-changed` 事件以匹配 `app/plugins/theme.ts:34-52` 的广播，避免 DOM dataset 与内部状态不一致。  
- **插件路由**：`app/pages/plugins/index.vue:1-9` 仅做客户端跳转，若后端通过服务器渲染提供 `/plugins` 入口需在 Nitro 层配合重定向。

## 相关链接
- [overview/README.md](README.md) —— 项目定位与术语  
- [../routing-and-layouts/Route_Design_and_Navigation.md](../routing-and-layouts/Route_Design_and_Navigation.md) —— 即将梳理完整路由映射  
- [../state-and-data/API_Client_and_Typed_SDK.md](../state-and-data/API_Client_and_Typed_SDK.md) —— 计划详细记录 API 层  
- [../realtime/SSE_WS_Client_Guide.md](../realtime/SSE_WS_Client_Guide.md) —— 后续详述双通道协议与限流策略

## TODO
- TODO: 补充实时通道的背压与重放策略细节；建议在 `app/composables/agent/useDualChannelConnection.ts` 附近搜集更多 `SSE_EVENT_TYPES` 处理路径，并在 `docs/realtime/SSE_WS_Client_Guide.md` 展开。  
- TODO: 绘制页面-布局-状态交互架构图，可基于 `app/pages/agent/index.vue` 与 `app/stores/*` 生成 Mermaid 或 PNG（建议提交至 `docs/overview/assets/`）。  
- TODO: 补完通知与搜索组合式函数文档；当前示例依赖 `useNotifications()`、`useSearch()`，需在 `docs/state-and-data/Data_Fetching_and_Caching.md` 指明来源。
