# 路由设计与导航指南

> 适用角色：前端工程师 / 架构师 / QA  
> 代码来源：`nuxt.config.ts:7`, `nuxt.config.ts:59`, `app/app.vue:10`, `app/layouts/default.vue:13`, `app/components/layout/Sidebar.vue:31`, `app/components/layout/Header.vue:9`, `app/pages/home/index.vue:5`, `app/pages/dashboard/index.vue:30`, `app/pages/agent/index.vue:12`, `app/pages/plugins/market.vue:141`, `app/pages/plugins/[id].vue:249`, `app/pages/settings/index.vue:200`, `app/pages/settings/users/index.vue:9`, `app/pages/workflow/workspace.vue:18`, `app/pages/setup/index.vue:4`, `app/middleware/auth.ts:2`, `app/pages/plugins/index.vue:1`

## 摘要
- Nuxt 以 SPA 模式运行并关闭严格路径检测，所有顶层页面从 `app/pages/**` 自动注册，别名与布局由 `definePageMeta` 控制（`nuxt.config.ts:7`, `app/pages/home/index.vue:5`）。  
- 默认布局提供侧边栏、头部与自适应内容卡片；专用工作流编辑页改用 `workflow` 布局，营销页面与初始化向导禁用布局以获取全屏体验（`app/layouts/default.vue:13`, `app/pages/workflow/workspace.vue:18`, `app/pages/setup/index.vue:4`）。  
- 导航数据通过菜单服务动态拉取，Sidebar 会识别系统路由、插件来源及 `//_p/` 外链，并结合全局认证中间件实现登陆守卫与旧路径重定向（`app/components/layout/Sidebar.vue:31`, `app/middleware/auth.ts:2`, `app/pages/plugins/index.vue:1`）。

## 代码来源
- 路由全局配置：`nuxt.config.ts:7`、`nuxt.config.ts:59`  
- 应用壳层与布局：`app/app.vue:10`、`app/layouts/default.vue:13`、`app/pages/workflow/workspace.vue:18`  
- 导航组件：`app/components/layout/Sidebar.vue:31`、`app/components/layout/Header.vue:9`  
- 代表性页面：`app/pages/home/index.vue:5`、`app/pages/dashboard/index.vue:30`、`app/pages/agent/index.vue:12`、`app/pages/plugins/market.vue:141`、`app/pages/plugins/[id].vue:249`、`app/pages/settings/users/index.vue:9`  
- 守卫与重定向：`app/middleware/auth.ts:2`、`app/pages/plugins/index.vue:1`

## 路由总体结构
- `srcDir` 指向 `app/`，Nuxt 以 SPA 形式运行且关闭严格模式，使 `/path` 与 `/path/` 等价，便于后端生成菜单路径（`nuxt.config.ts:7`）。  
- Nitro 在预渲染时忽略 `/_p/**`，为后续插件侧代理提供占位路径；运行时通过 WebSocket 实验特性支持代理实时通道（`nuxt.config.ts:59`）。  
- 全局应用壳在挂载时展示品牌 Loading，并始终包裹 `NuxtLayout`/`NuxtPage`，使布局切换成为导航主要策略（`app/app.vue:10`）。  
- 默认布局根据路由前缀隐藏 Footer，并共享侧边栏折叠状态及移动端遮罩，保障所有业务页的一致可用性（`app/layouts/default.vue:13`）。

## 主要页面路由
| 路径 | 源文件 | 布局 | 说明 |
| --- | --- | --- | --- |
| `/home` (`/` 别名) | `app/pages/home/index.vue:5` | `layout: false` | 着陆/开机页，跳过默认框架，兼容公共访问 |
| `/dashboard` | `app/pages/dashboard/index.vue:30` | `default` | 数据可视化总览 |
| `/agent` | `app/pages/agent/index.vue:12` | `default` | Agent 会话工作区，绑定双通道聊天 |
| `/plugins/market` | `app/pages/plugins/market.vue:141` | `default` | 插件市场，分页筛选 |
| `/plugins/:id` | `app/pages/plugins/[id].vue:249` | `default` | 动态详情，路由参数 `id` |
| `/settings` | `app/pages/settings/index.vue:200` | `default` | 系统设置目录卡片，链接各子路由 |
| `/settings/users` | `app/pages/settings/users/index.vue:9` | `default` | 用于管理部门/成员/权限，配合菜单 `order` 元信息 |
| `/workflow/workspace` | `app/pages/workflow/workspace.vue:18` | `workflow` | 编辑器视图，通过查询参数 `id` 载入工作流 |
| `/setup` | `app/pages/setup/index.vue:4` | `layout: false` | 首次安装向导，多步骤 Stepper |
| `/users/login` | `app/pages/users/login.vue:4` | `layout: false` | 登录页，登录后根据 `redirect` 参数导航 |

> 说明：`/plugins` 根路径在客户端立即跳转至 `/plugins/market`，用于兼容历史书签（`app/pages/plugins/index.vue:1`）。

## 布局与插槽
- `default` 布局提供侧边栏、头部、内容卡片与底部栏；在 `/agent*`、`/workflow*` 下自动隐藏 Footer 以扩大画布（`app/layouts/default.vue:13`）。  
- `workflow` 布局使用全屏工作区，内置工具栏、小地图与属性面板切换，更适合 DAG 编辑等重交互页面（`app/pages/workflow/workspace.vue:18`）。  
- `layout: false` 场景包括首页、Intro、登陆与安装向导，使页面可以自定义背景/排版或嵌入 Stepper（`app/pages/home/index.vue:5`，`app/pages/setup/index.vue:4`）。  
- 所有布局均由 `app/app.vue` 包裹，自动注入 `NuxtLoadingIndicator` 与 `GlobalAlertNotification` Teleport，保证路由切换时的统一体验（`app/app.vue:30`）。

## 导航组件
- `Sidebar` 使用菜单服务 `getUserMenus()` 动态生成分类、图标与顺序；支持 `MenuCategory` 与 `MenuItem` 递归排序，并识别 `//_p/` 前缀跳转到代理插件资源（`app/components/layout/Sidebar.vue:31`、`app/composables/api/services/menuService.ts:118`）。  
- `Sidebar` 通过共享的 `sidebar-collapsed` state 与 `useWindowSize` 自动响应移动端折叠，同时根据当前路由片段高亮父子节点（`app/layouts/default.vue:19`、`app/components/layout/Sidebar.vue:49`）。  
- `Header` 组合通知、搜索与用户菜单，在挂载时按 token 拉取用户上下文与通知列表，搜索操作会导航到 `/search?q=...` 并展示建议（`app/components/layout/Header.vue:9`、`app/components/layout/Header.vue:149`）。  
- Loading 状态与路由切换由 `app/plugins/global-loading.ts` 监测 `page` 与 `$fetch` 钩子，提升导航反馈的一致性。

## 中间件与守卫
- 全局路由中间件会将根路径重定向到 `/home`，并在客户端检查访问令牌；公共路由白名单包含 `/home`、`/intro` 与用户认证路径，其他路径要求有效 token，否则跳转登录（`app/middleware/auth.ts:2`）。  
- 登录页、注册页等禁用默认布局，以避免中间件在无令牌时渲染重复框架（`app/pages/users/login.vue:4`）。  
- 页面级重定向通过组合式逻辑处理：例如 `/plugins` 运行时替换为 `/plugins/market`，保持历史链接可访问（`app/pages/plugins/index.vue:1`）。

## 动态与插件路由
- 动态插件详情页通过 `[id]` 段解析路径，页面内部再调用管理服务加载元数据，并展示启停、凭证等操作（`app/pages/plugins/[id].vue:249`）。  
- Sidebar 会把 `//_p/<plugin>` 形式的链接原样返回，用于 PowerX Bridge 启动嵌入式插件；Nitro 预渲染忽略该前缀防止构建失败（`app/components/layout/Sidebar.vue:31`、`nuxt.config.ts:59`）。  
- `definePageMeta.alias` 用于合并路由，例如首页在 `pages/home/index.vue` 同时注册 `/home` 与 `/`，避免出现重复文件（`app/pages/home/index.vue:5`）。  
- `definePageMeta.order`、`icon` 等元信息被菜单接口读取，保持前端渲染顺序与后端配置一致（`app/pages/settings/users/index.vue:9`）。

## 示例代码
```ts
// 首页为公共着陆页，并映射根路径
definePageMeta({
  alias: ["/"],     // 让 /home 同时响应 /
  layout: false,    // 关闭默认布局，使用自定义落地页
});
```
参考：`app/pages/home/index.vue:5`

## 最佳实践与陷阱
- **保持授权一致性**：新增页面若需登录，应使用默认布局并信任全局中间件；公共页必须显式设置 `layout: false` 或追加到白名单，避免被误判为需要认证（`app/middleware/auth.ts:20`）。  
- **使用菜单服务驱动导航**：不要手写固定菜单结构，统一通过 `useMenuService()` 拉取后端配置并按 `order` 排序，以免造成系统与插件菜单不一致（`app/components/layout/Sidebar.vue:151`）。  
- **动态路由参数校验**：`/plugins/:id` 等页面需在加载失败时处理 404 或回退，当前实现仅记录错误，后续可以结合 `navigateTo` 防止空白状态（`app/pages/plugins/[id].vue:317`）。  
- **工作流视图声明布局**：在非默认布局的页面，务必通过 `definePageMeta({ layout: 'workflow' })` 指定，否则会落入默认布局导致 UI 错位（`app/pages/workflow/workspace.vue:18`）。  
- **测试页面隔离**：`/test/*` 路由保留调试组件，发布前如需隐藏，应在菜单接口或中间件层面过滤；当前代码未提供自动屏蔽。

## 相关链接
- [overview/Architecture_for_Frontend.md](../overview/Architecture_for_Frontend.md)  
- [Layouts_and_Slots.md](Layouts_and_Slots.md)  
- [../state-and-data/API_Client_and_Typed_SDK.md](../state-and-data/API_Client_and_Typed_SDK.md)

## TODO
- TODO: 梳理正式的面包屑策略；目前未发现集中式配置，建议在 `app/components/layout/Header.vue` 或独立 composable 中提供（推荐位置：`app/composables/useBreadcrumbs.ts`）。  
- TODO: 为插件动态路由补充 404/回退处理，避免 `svc.status` 等接口异常时残留空白视图（建议在 `app/pages/plugins/[id].vue` 添加）。  
- TODO: 明确定义 `/test/*` 路由的可见性策略，可通过构建环境变量在 `app/middleware/auth.ts` 或菜单服务中做条件过滤。
