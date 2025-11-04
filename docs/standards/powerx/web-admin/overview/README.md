# PowerX Web Admin 项目概览

> 适用角色：产品负责人 / 前端与全栈工程师 / QA / 解决方案架构师  
> 阅读前提：熟悉 Nuxt 4、Pinia 状态管理与基于 REST/WebSocket 的前端集成；了解多租户 RBAC 模型与插件市场常见业务场景。

## 摘要
- PowerX Web Admin 以 Nuxt 4 构建为 SPA，统一从 `runtimeConfig.public` 读取 API/WebSocket 入口并启用暗色优先的 UI 外观（`nuxt.config.ts:7-52`）。  
- 框架通过默认布局组合侧边栏、头部、欢迎向导与全局告警，页面内容插槽统一包裹在玻璃拟态卡片中（`app/layouts/default.vue:1-115`）。  
- 核心业务涵盖 Agent 对话/会话协同、插件市场与启停、仪表盘可视化等模块，均以独立页面路由承载（`app/pages/agent/index.vue:1-200`，`app/pages/plugins/market.vue:1-131`，`app/pages/dashboard/index.vue:1-200`）。

## 目录
- [项目定位](#项目定位)
- [目标用户](#目标用户)
- [术语与缩写](#术语与缩写)
- [目录导航](#目录导航)
- [TODO](#todo)

## 项目定位
PowerX Web Admin 是 PowerX 体系的运营与集成控制台，提供多租户 Agent 管理、插件生态运营与工作流配置等能力。应用注册为 Nuxt 4 单页模式（`nuxt.config.ts:7-74`）并将品牌名称/描述写入 `<head>` 元信息及开机过渡动画（`app/app.vue:4-25`）。  

全局布局由 `default` 布局组合 Sidebar、Header、Footer 与欢迎引导，针对 `/agent`、`/workflow` 等长页自动收起底部栏以最大化工作区（`app/layouts/default.vue:24-83`）。侧边菜单并非写死，而是通过 `useMenuService` 动态拉取分类、标题翻译与权限元数据，再按分类/插件来源重组展示（`app/components/layout/Sidebar.vue:13-200`，`app/composables/api/services/menuService.ts:1-198`）。  

在交互上，控制台预置全局 Loading、告警 Teleport 与欢迎向导，通过 `useGlobalLoading` 与 `GlobalAlertNotification` 实现统一体验（`app/app.vue:10-43`，`app/components/GlobalAlertNotification.vue:1-28`）。各业务面向 API 服务目录 `app/composables/api/services/*` 提供统一封装，涵盖插件、Agent、权限、租户等资源（`app/composables/api/services/index.ts:1-25` 及同级文件）。

## 目标用户
- **平台 Root 管理员**：具备跨租户与系统操作权限，可在插件市场执行安装、启停、重启与凭证轮换（`app/stores/user.ts:24-58`，`app/pages/plugins/market.vue:12-84`，`app/composables/api/services/adminPluginsService.ts:16-73`）。  
- **租户管理员与成员**：可在 Agent 工作区发起双通道会话、维护租户偏好并在仪表盘关注运营指标（`app/stores/user.ts:27-92`，`app/pages/agent/index.vue:20-200`，`app/pages/dashboard/index.vue:39-192`）。  
- **权限与合规专员**：通过权限商店与角色配置监控 RBAC 模型、API 关联及启用状态（`app/stores/permission.ts:1-195`）。  
- **多语言运营团队**：依赖 `@nuxtjs/i18n` 提供的 `langDir` 与多语种导航词条维护统一界面（`nuxt.config.ts:84-102`，`i18n/locales/en.json:1-80`）。

## 术语与缩写
| 术语 | 说明 | 源码定位 |
| --- | --- | --- |
| Root | 平台级超级用户，可越权访问各租户与插件操作。 | `app/stores/user.ts:27-58` |
| Tenant | 业务租户，上下文包含租户成员、当前租户 ID 与切换动作。 | `app/stores/user.ts:30-149` |
| Agent | PowerX 智能体实体，含蓝图引用、工具白名单及对话状态。 | `app/types/agent.ts:1-67` |
| Dual Channel | SSE + WebSocket 双通道聊天连接，负责消息流、重连与缓存。 | `app/composables/agent/useDualChannelConnection.ts:1-200` |
| Plugin Marketplace | 插件市场页面，提供分类筛选、分页与安装对话框。 | `app/pages/plugins/market.vue:1-131` |
| RBAC Catalog | 权限目录与 API 元数据，用于角色授权与可见性控制。 | `app/stores/permission.ts:45-195` |
| Runtime Config | 前端运行态配置（API、WS、主题、语言、功能开关）。 | `nuxt.config.ts:21-52` |

## 目录导航
### overview/
- [Architecture_for_Frontend.md](Architecture_for_Frontend.md) —— 计划描绘页面层、Pinia 与实时通道之间的协作视图（TODO）。

### environment/
- [Dev_Environment_Setup.md](../environment/Dev_Environment_Setup.md) —— 待补充 Node 版本、依赖安装与常见坑。  
- [Env_Variables_Schema.md](../environment/Env_Variables_Schema.md) —— 需整理 `runtimeConfig` 与 `.env` 变量。  
- [Local_Mocks_and_Fixtures.md](../environment/Local_Mocks_and_Fixtures.md) —— 预留本地假数据与 MSW 引导。

### routing-and-layouts/
- [Route_Design_and_Navigation.md](../routing-and-layouts/Route_Design_and_Navigation.md) —— 将基于 `app/pages/**` 路由映射撰写。  
- [Layouts_and_Slots.md](../routing-and-layouts/Layouts_and_Slots.md) —— 对应 `app/layouts` 与布局组件。  
- [Middleware_Permissions.md](../routing-and-layouts/Middleware_Permissions.md) —— 需提炼 `app/middleware/**` 与 `definePageMeta` 中的守卫。

### 其它关键卷
- `state-and-data/`：聚焦 Pinia、API SDK 与数据提取策略。  
- `features/`：按功能（如 Agent 管理、Router Policies、Workflow Builder）沉淀业务说明。  
- `appendix/API_Route_Map.md`：将收集「页面 → API → 权限 → 空态」的权威清单（目前为空，敬请关注）。

> 提示：以上 Markdown 文件已创建但内容待补全，可在相对路径 `docs/<目录>/<文件>.md` 中查看空白模板。

## TODO
- TODO: 梳理 `docs/` 目录中所有空白文档的预期提纲与负责人；建议在 `docs/overview/Architecture_for_Frontend.md` 起草结构图（参考 `app/pages/agent/index.vue` 与 `app/composables/agent/useDualChannelConnection.ts`）。  
- TODO: 在 `docs/appendix/API_Route_Map.md` 引入页面-API-权限映射表；可从 `app/composables/api/services` 目录提取接口。  
- TODO: 补充针对 i18n 与主题策略的终端行为描述，参考 `nuxt.config.ts:84-102` 与 `tailwind.config.ts:1-18`。
