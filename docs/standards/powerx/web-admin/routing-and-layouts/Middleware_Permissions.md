# 中间件与权限策略

> 适用角色：前端工程师 / 架构师 / 安全审核 / QA  
> 代码来源：`app/middleware/auth.ts:1-55`, `app/middleware/00-debug-router.client.ts:1-8`, `app/plugins/api.ts:18-69`, `app/plugins/auth-init.client.ts:1-13`, `app/composables/useAuth.ts:15-152`, `app/stores/user.ts:24-149`, `app/pages/plugins/market.vue:13-174`, `app/pages/plugins/installed.vue:13-175`, `app/components/layout/Sidebar.vue:31-200`, `app/composables/api/services/menuService.ts:21-148`

## 摘要
- 客户端全局中间件会在访问根路径时跳转到 `/home`，并校验本地 Token 是否过期；未登录访问受限路由时强制跳转登录并清理本地缓存（`app/middleware/auth.ts:1-55`）。  
- API 插件自动附加 `Authorization` 头并在 401 时触发登出流程；认证插件负责在应用挂载后恢复持久化 Token（`app/plugins/api.ts:18-69`，`app/plugins/auth-init.client.ts:1-13`）。  
- 权限态保存在 `useUserStore`，页面通过 `isRoot` 与 `isCurrentTenantAdmin` 控制按钮与功能入口，同时菜单结构可携带 `permissions` 元数据供后续细粒度守卫（`app/stores/user.ts:24-149`，`app/components/layout/Sidebar.vue:31-200`）。

## 全局中间件
- `app/middleware/auth.ts:1-55` 为默认路由守卫：  
  - `/` 强制重定向到 `/home`。  
  - 服务器端跳过鉴权逻辑，只在客户端读取 `localStorage`。  
  - 白名单正则匹配 `/home`、`/intro`、`/users/login`、`/users/register`（支持语言前缀）。  
  - 非白名单路由要求存在且未过期的 `access_token`，否则清理 `access_token`/`refresh_token` 等字段并携带原路径跳转 `/users/login?redirect=...`。  
- `app/middleware/00-debug-router.client.ts:1-8` 作为调试插件拦截 `router.beforeEach` / `afterEach` 输出日志，可在生产构建中禁用以避免泄露路由信息。

## 认证初始化与刷新
- `app/composables/useAuth.ts:15-152` 定义 `setAuth`、`clearAuth`、`getToken`、`isTokenExpired` 等方法，Token 及过期时间全存储于 `localStorage`。`logout` 会调用后端 API，并清理 `sessionStorage`、常见 Cookie 与带有 `auth`/`px_` 前缀的 Key。  
- `app/plugins/auth-init.client.ts:1-13` 通过 `app:mounted` 钩子调用 `initAuth()`，确保在页面渲染前完成 Token 恢复；使用 `useState('auth.__booted')` 防止重复初始化。  
- API 插件 `app/plugins/api.ts:18-69`：  
  - 请求拦截器在未设置 `skipAuth` 时附加 `Authorization: Bearer {{token}}`。  
  - 响应拦截器遇到 401 且请求未显式跳过认证时，清理认证信息并 `router.push('/users/login')`。  
  - 错误日志统一输出 `error.response.data.message` 方便排查。  
- 结合全局中间件，前端即便刷新页面也会在首个请求失败后回收本地凭据，避免反复触发 401。

## 权限状态与角色判定
- `useUserStore` 提供以下核心 Getter：`isRoot`、`isCurrentTenantAdmin`、`memberTenants`、`getTenantRole` 等（`app/stores/user.ts:24-93`）。  
- `fetchUserContext` 会缓存 5 分钟并在切换租户后刷新上下文（`app/stores/user.ts:96-149`）；Header 在应用加载时主动调用该方法并填充通知中心（`app/components/layout/Header.vue:21-44`）。  
- 页面使用角色信息控制 UI 能力：  
  - 插件市场按钮仅 Root 可见（`app/pages/plugins/market.vue:13-174`）。  
  - 插件已安装列表根据 `isRoot` 与 `isCurrentTenantAdmin` 控制系统级启停和租户级启停（`app/pages/plugins/installed.vue:13-175`）。  
  - 设置页面 Tabs 只有 Root 或租户管理员才展示权限管理页签（`app/pages/settings/users/index.vue:24-45`）。  
- 这类判断位于渲染层，若需更强约束可在 API 层追加 `X-Require-Auth` 头配合后端校验。

## 菜单与权限元数据
- 菜单服务 `app/composables/api/services/menuService.ts:21-148` 定义 `MenuItem.permissions?: string[]`，可由后端返回权限编码供前端自定义过滤。  
- 侧边栏在 `processMenuItems` 时仅做可见性与排序控制，目前未对 `permissions` 字段进行过滤；若要实现菜单级权限守卫，可在该函数中结合 `usePermissionStore` 或 `useUserStore` 进行判定（`app/components/layout/Sidebar.vue:151-164`）。  
- 权限目录与角色授权信息集中在 `app/stores/permission.ts:45-195`，为后续实现菜单与按钮级别的细粒度控制提供数据源。

## 示例代码
```ts
// 插件市场仅允许 Root 用户触发安装操作
const userStore = useUserStore()
const isRoot = computed(() => userStore.isRoot) // app/pages/plugins/market.vue:171-174

<UButton v-if="isRoot" size="sm" icon="i-heroicons-arrow-down-tray" @click="openInstallGeneric">
  安装
</UButton>
```

## 最佳实践与陷阱
- **保持白名单同步**：新增公共页面时需同时更新 `PUBLIC_RULES`；否则中间件会误判并跳转登录（`app/middleware/auth.ts:20-29`）。  
- **使用 `skipAuth` 控制匿名请求**：调用登录、注册、健康检查等接口时设置 `skipAuth: true`，避免请求拦截器附带失效 Token 导致重复重定向。  
- **Token 过期策略**：前端判断基于 `expires_at` 时间戳，若后端调整 Token 生命周期，需确保登录响应包含正确 `expires_in`。  
- **最小权限渲染**：视图级判定（`isRoot`、`isCurrentTenantAdmin`）只能隐藏按钮，不能阻止恶意调用 API；必须确保后端做二次校验。  
- **调试插件处理**：`00-debug-router.client.ts` 会打印所有路由跳转，发布前应移除或受 `process.dev` 控制以防日志泄露。

## 相关链接
- [Route_Design_and_Navigation.md](Route_Design_and_Navigation.md)  
- [Layouts_and_Slots.md](Layouts_and_Slots.md)  
- [../state-and-data/API_Client_and_Typed_SDK.md](../state-and-data/API_Client_and_Typed_SDK.md)

## TODO
- TODO: 在 `Sidebar` 处理 `MenuItem.permissions`，结合 `usePermissionStore` 自动隐藏无访问权的菜单项（建议修改 `processMenuItems`）。  
- TODO: 为敏感 API 封装添加 `X-Require-Auth` 头部，避免被错误标记为 `skipAuth` 时泄露数据（目标位置：`app/composables/api/index.ts` 请求封装）。  
- TODO: 统一调试输出开关，将 `00-debug-router.client.ts` 挂在 `process.dev` 判断下，防止生产环境日志噪音。
