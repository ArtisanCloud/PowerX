# 本地 Mock 与数据夹具指南

> 目标：解释 PowerX Web Admin 在缺少后端时如何通过内置 Mock 和测试页面维持可视化与交互调试，并给出扩展新夹具的最佳实践。

---

## 目录
- [Mock 策略总览](#mock-策略总览)
- [API 层内置 Mock 数据](#api-层内置-mock-数据)
- [组件级静态夹具](#组件级静态夹具)
- [测试页面与手动验证](#测试页面与手动验证)
- [如何新增 Mock](#如何新增-mock)
- [常见问题排查](#常见问题排查)

---

## Mock 策略总览

目前项目尚未引入统一的 Mock 平台（例如 MSW 或 Mirage），主要依靠以下方式覆盖本地演示需求：

1. **服务层内联 Mock**：部分 API composable 在真实接口上线前返回静态数组/对象，模拟网络延迟与筛选逻辑。  
2. **组件内静态夹具**：局部组件使用常量数组作为临时数据源，保障 UI 骨架与交互。  
3. **测试路由**：在 `app/pages/test/**` 下创建的演示页，用于验证连接状态、主题、语言切换等功能。  
4. **反向代理断言**：通过 `POWERX_BACKEND` 与 `POWERX_BACKEND` 环境变量把请求导向可用的后端或 Mock 服务。

在开发迭代中，建议保持“**真实接口优先，Mock 仅做过渡**”的原则：当后端能力就绪，应及时替换为 `$fetch` 或 `useApiClient` 调用，并清理不再需要的静态数据。

---

## API 层内置 Mock 数据

| 模块 | 位置 | 说明 |
| --- | --- | --- |
| 全局搜索 | `app/composables/api/services/searchService.ts:10` | `mockSearchData` 与 `mockSuggestions` 提供完整的搜索结果、建议与分面统计，`search()` 和 `getSuggestions()` 会模拟 300ms 延迟并执行筛选。 |
| 工作流编排 | `app/composables/api/services/workflowService.ts:70` | `getKinds()`、`getPalette()` 在 `try` 失败分支返回 `mockKinds`/`mockPalette`，包含常见节点类型与工具箱元素，保证 Builder UI 可渲染。 |
| Agent 双通道连接 | `app/composables/agent/useDualChannelConnection.ts:130` | 代码中保留“`// const url = buildHttpUrl("/agents/stream/mock")`”注释，供接入本地流式 Mock 时启用；默认仍指向实际后端。 |

**使用建议**

- 默认流程会先尝试真实接口，仅当请求抛错（如本地无后端）才退回 Mock，避免开发过程中忽视真实服务异常。  
- 在替换为真实 API 时，务必更新对应文档并删除冗余静态数据，参考“如何新增 Mock”章节的开关模式。

---

## 组件级静态夹具

| 组件 | 位置 | Mock 内容 | 目的 |
| --- | --- | --- | --- |
| 通知中心 | `app/composables/useNotifications.ts:12` | `mockNotifications` 列表包含多类型通知、重要标记与关联动作；`fetchNotifications()` 会按过滤器分页。 | 支撑 Notification 面板与角标统计在无服务端时也可演示。 |
| 租户成员表 | `app/components/settings/users/UsersTenantMember.vue:86` | `mockData` 以租户 ID 为键提供成员行。 | 配合设置页面的表格与权限展示。 |
| 搜索演示页 | `app/pages/test/search-showcase.vue:62` | `mockResults` 与 `mockSuggestions` 复用了搜索类型，方便在 test 路由直接调试 UI。 | 手动验证搜索组件优化。 |

组件夹具多通过 `ref` 包装常量，便于未来无缝切换到真实接口。引入新夹具时，请在代码顶部注明用途与替换计划。

---

## 测试页面与手动验证

`app/pages/test/` 目录承载多项前端实验与手动用例：

| 页面 | 用途 | 关键依赖 |
| --- | --- | --- |
| `connection.vue` | 观测 SSE / WebSocket 探活与消息流; 依赖 `useDualChannelConnection` | 访问地址 `/test/connection`，需浏览器本地存储 token。 |
| `layout-demo.vue` | 布局占位与导航验证 | 适合新组件在主布局内预览。 |
| `search-showcase.vue` | 搜索组件调试，复用 `mockResults` | 支持修改查询词、查看分页。 |
| `test-lang.vue` / `test-theme.vue` | i18n 与主题切换回归 | 读取 `.env` 中的语言/主题配置。 |
| `test-loading.vue` / `test-nuxtui.vue` | 全局 Loading、Nuxt UI 元素的演示 | 便于 QA 快速检查基础交互。 |

> 提示：测试路由默认在开发环境开放，若需要在生产环境保留，请加访问守卫或通过 `runtimeConfig.public.debugMode` 控制显示。

---

## 如何新增 Mock

1. **定义开关**：优先考虑通过环境变量或函数参数在 Mock 与真实接口间切换，比如读取 `config.public.debugMode`。  
2. **隔离数据**：将静态数据放置于 `app/composables/api/mocks/`（待创建）或单独文件，避免主服务文件臃肿。  
3. **模拟真实延迟**：使用 `await new Promise((r) => setTimeout(r, X))` 模拟网络耗时，帮助发现并发/Loading 状态问题。  
4. **保持类型一致**：复用 `~/types/**` 中的接口类型，确保未来替换真实接口时无需额外重构。  
5. **文档同步**：更新本文件与 `docs/environment/Dev_Environment_Setup.md`，标明 Mock 使用范围；在 PR 中注明“Mock Update”。  
6. **逐步淘汰**：后端上线后删除 Mock 数据并在提交记录中说明，以防残留导致误判。

---

## 常见问题排查

- **忘记清理 Mock**：若功能已连通真实后端，但仍看到旧数据，请检查组件内是否残留 `mock*` 数组或提前返回。  
- **测试页对外暴露**：部署前确认未授权的测试路由是否需要屏蔽，可在 `definePageMeta` 中加权限守卫或利用中间件拦截。  
- **接口 404**：`POWERX_BACKEND` 未指向有效后端时，`$fetch` 会直接抛错并触发 Mock 分支；请留意控制台警告，避免将 Mock 结果误认为真实数据。  
- **WebSocket Mock**：项目暂未实现本地 WS Mock，如需调试，可在 `useDualChannelConnection` 中创建 `ws://localhost:xxxx` 的本地服务或启用注释中的 `/agents/stream/mock` 备用接口。  
- **数据漂移**：多人并行调试时，请约定 Mock 数据的版本控制，避免 PR 互相覆盖；推荐把大型夹具拆分为 JSON 并加注释。

---

## 后续规划

- 引入 `msw` + `@vitest/browser` 方案统一管理 REST/WS Mock，同时服务于单元测试与 Storybook。  
- 在 `scripts/` 目录添加 Fake API Server（Fastify / Express），与前端共享类型定义，提升联调效率。  
- 为测试页面增加访问守卫与“仅在 `debugMode` 下显示”的逻辑，避免生产环境暴露内部工具。
