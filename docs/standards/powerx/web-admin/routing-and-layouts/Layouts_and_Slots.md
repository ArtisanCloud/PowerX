# 布局与插槽指南

> 适用角色：前端工程师 / 架构师 / 设计系统维护者  
> 代码来源：`app/app.vue:10-34`, `app/layouts/default.vue:1-115`, `app/layouts/workflow.vue:1-200`, `app/components/layout/Header.vue:9-158`, `app/components/layout/Sidebar.vue:31-200`, `app/components/layout/FooterBar.vue:1-28`, `app/plugins/global-loading.ts:1-59`

## 摘要
- PowerX Web Admin 通过 `app/app.vue` 统一加载 `NuxtLayout`、全局 Loading 与 Alert Teleport，保证所有路由共享同一应用壳层。  
- 默认布局 `default` 负责主体验证、侧边导航与自适应内容卡片，融合欢迎引导与测试入口；`workflow` 布局提供全屏画布与工具栏。  
- 页面可用 `definePageMeta({ layout: ... })` 或 `layout: false` 切换布局，常用于登录、介绍页与初始化向导。

## 布局总览
- 全局壳层位于 `app/app.vue`，在 `NuxtLayout` 外包裹 `UApp` 与 `NuxtLoadingIndicator`，并在挂载时触发品牌开屏动画（`app/app.vue:10-34`）。  
- `app/plugins/global-loading.ts:1-59` 监听 `page` 与 `$fetch` 钩子，驱动覆盖式 Loading Overlay，与布局插槽无关但保证切换过程平滑。  
- 布局文件存放在 `app/layouts/`，目前包括 `default.vue` 与 `workflow.vue`；页面若未指定布局则默认使用 `default`。

## 默认布局结构
- 框架组件：顶部 `Header`、左侧 `Sidebar`、可隐藏的 `FooterBar` 与内容插槽（`app/layouts/default.vue:13-103`）。  
- Sidebar 折叠状态存入 `useState('sidebar-collapsed')`，移动端根据 `useWindowSize` 自动折叠并提供遮罩点击关闭（`app/layouts/default.vue:19-48`）。  
- Footer 根据当前路由前缀自动隐藏 `/agent`、`/workflow` 相关页面，避免干扰工作区（`app/layouts/default.vue:13-17`）。  
- 附带的 `WelcomeGuide` 和 `GuideButton` 通过 Teleport/Overlay 展示引导内容或调试入口（`app/layouts/default.vue:7-108`）。  
- Slot 区域外层使用玻璃拟态容器（`bg-white/95` 等）保障背景一致，页面内容通过 `<slot />` 注入（`app/layouts/default.vue:93-96`）。

### 关联组件
- `Header` 处理通知、搜索与用户菜单，挂载时依据 token 拉取用户上下文并在搜索框跳转 `/search?q=...`（`app/components/layout/Header.vue:9-158`）。  
- `Sidebar` 依赖菜单服务生成分组菜单、翻译标题并根据路由高亮当前项；插件路径 `//_p/` 原样输出供代理使用（`app/components/layout/Sidebar.vue:31-200`）。  
- `FooterBar` 与语言/主题切换器整合，并提供法律/关于链接（`app/components/layout/FooterBar.vue:1-28`）。

## Workflow 布局
- `app/layouts/workflow.vue:1-200` 面向 DAG 编辑场景，内含顶部工具栏、小地图与属性面板等 UI，默认开启全屏高度。  
- 工具栏允许切换最小化、打开属性面板或全屏；布局本身通过 `useColorMode` 在 `light/dark` 间切换背景（`app/layouts/workflow.vue:2-36`）。  
- 主内容区的 `<NuxtPage />` 提供画布插槽，右侧属性面板在 `showProperties` 控制下显示（`app/layouts/workflow.vue:145-175`）。  
- 页面通过 `definePageMeta({ layout: 'workflow' })` 使用此布局，例如工作流编辑器（`app/pages/workflow/workspace.vue:18-55`）。

## 无布局页面
- 着陆页、Intro、登录、Setup 等使用 `layout: false` 以获得全屏控制：`home/index.vue`、`home/intro.vue`、`users/login.vue`、`setup/index.vue`（`app/pages/home/index.vue:5`, `app/pages/users/login.vue:4`, `app/pages/setup/index.vue:4`）。  
- 这些页面需自行引入 Header/Footer，如首页组件内手动挂载 `FooterBar` 并管理用户下拉菜单（`app/pages/home/index.vue:1-190`）。

## 插槽与内容约定
- 布局未定义命名插槽，所有页面内容经默认插槽渲染。若页面需自定义边界，可在内容组件中删除或替换卡片容器。  
- Sidebar 与 Header 利用具名事件（`@toggle-sidebar` 等）与共享 state 调度，同时透出 props 供子组件决定密度或可视状态（`app/layouts/default.vue:23`, `app/components/layout/Sidebar.vue:21-28`）。  
- 欢迎引导、测试按钮、全局 Alert 通过 Teleport 接入 `body`，不会污染页面 DOM 层级（`app/app.vue:37-43`, `app/layouts/default.vue:105-109`）。

## 最佳实践与陷阱
- **选择正确布局**：高交互的工作流或画布场景使用 `workflow` 布局；营销、认证页面应设置 `layout: false` 并自行负责导航元素。  
- **折叠状态共享**：依赖 `useState('sidebar-collapsed')` 维护折叠状态，若自定义布局需沿用同样 key，避免 Sidebar 与 Header 状态不一致（`app/layouts/default.vue:19-37`）。  
- **移动端支持**：默认布局提供遮罩按钮关闭 Sidebar，自定义布局在移动端需要一致体验时应复用 `useWindowSize` 逻辑。  
- **全局 Loading**：通过 `$fetch.create` 包裹请求生成 Loading 叠层，如绕过该机制需自行管理 `useGlobalLoading` 状态以维持 UX 一致（`app/plugins/global-loading.ts:21-58`）。  
- **欢迎引导可选**：若特定布局不需要 `WelcomeGuide`，可新建布局移除相关组件，避免在专用工作区造成视图覆盖。

## 相关链接
- [Route_Design_and_Navigation.md](Route_Design_and_Navigation.md)  
- [../overview/Architecture_for_Frontend.md](../overview/Architecture_for_Frontend.md)  
- [../state-and-data/Data_Fetching_and_Caching.md](../state-and-data/Data_Fetching_and_Caching.md)

## TODO
- TODO: 提供命名插槽示例（如顶部工具栏扩展点），建议在 `app/layouts/default.vue` 增设 `<slot name="header-extra" />` 以满足复杂需求。  
- TODO: 评估在 `layout: false` 页面中复用全局 Footer 的最佳方式，可考虑抽出一个 Minimal 布局（位置：`app/layouts/minimal.vue`）。  
- TODO: 为 `workflow` 布局添加退出/保存快捷键提示条，便于 QA 和运营识别当前模式。
