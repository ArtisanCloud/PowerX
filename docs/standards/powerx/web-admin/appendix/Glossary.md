# 术语表（Glossary）

> 收录 PowerX Web Admin 中常见的业务与技术术语，便于团队成员快速对齐概念。

---

| 术语 | 定义 | 参考位置 |
| --- | --- | --- |
| **Agent** | PowerX 智能体实体，具备模型配置、工具白名单、会话上下文。 | `app/types/agent.ts: "1` |"
| **Agent Session** | 用户与 Agent 的一次会话，包括消息历史与状态。 | `app/stores/agentSession.ts: "1` |"
| **Dual Channel** | SSE + WebSocket 双通道通讯，用于流式消息与指令。 | `app/composables/agent/useDualChannelConnection.ts: "1` |"
| **Plugin Marketplace** | 插件市场浏览与安装界面，支持筛选、安装、启停。 | `app/pages/plugins/market.vue: "1` |"
| **Tenant** | 平台中的业务租户（企业/组织），包含成员、角色、偏好。 | `app/stores/user.ts: "27` |"
| **Root 用户** | 平台超级管理员，拥有跨租户和系统级操作权限。 | `app/stores/user.ts: "39` |"
| **RBAC Catalog** | 权限目录，列举资源/操作与可授权角色。 | `app/stores/permission.ts: "62` |"
| **Workflow** | 基于 Vue Flow 的可视化编排流程，包含节点和连线。 | `app/components/workflow/WorkflowEditor.vue: "1` |"
| **Palette** | Workflow 节点面板，列举可拖拽的节点模板。 | `app/components/workflow/WorkflowEditor.vue: "48` |"
| **Runtime Config** | Nuxt 运行时配置，包含公共与私有变量。 | `nuxt.config.ts: "21` |"
| **Global Loading** | 全局 Loading 状态管理与 Overlay。 | `app/app.vue: "10`, `app/plugins/gl-overlay.client.ts:1` |"
| **OneShot Alert** | 一次性全局提示（UAlert）。 | `app/components/GlobalAlertNotification.vue: "1` |"
| **Menu Service** | 动态加载侧边菜单的 API 服务与处理逻辑。 | `app/composables/api/services/menuService.ts: "1` |"
| **Env Store** | 客户端环境选择器（Dev/Staging/Prod 等）。 | `app/stores/envStore.ts: "1` |"
| **UPSTREAM** | 后端 API 基础地址环境变量。 | `nuxt.config.ts: "3` |"
| **WS_UPSTREAM** | WebSocket 服务基础地址环境变量。 | `nuxt.config.ts: "24` |"
| **Config Panel** | Agent 配置侧栏，编辑模型参数与工具。 | `app/components/agent/ConfigPanel.vue` |
| **Install Dialog** | 插件安装弹窗，处理安装表单与反馈。 | `app/components/plugins/InstallDialog.vue` |
| **GlobalAlertNotification** | 负责渲染 `useOneShotAlert` 触发的全局通知组件。 | `app/components/GlobalAlertNotification.vue: "1` |"
| **Permission Store** | Pinia Store，管理权限目录、租户授权。 | `app/stores/permission.ts: "1` |"
| **Agent Manager** | 组合式函数，封装 Agent CRUD API。 | `app/composables/agent/useAgentManager.ts: "1` |"
| **Admin Plugins Service** | 管理插件市场、安装、状态的 API 封装。 | `app/composables/api/services/adminPluginsService.ts: "1` |"
| **Global Error Page** | Nuxt 默认错误页面，提供错误详情复制与返回操作。 | `app/error.vue: "1` |"
| **SDP** | Service Delivery Platform（若出现）—— 指代后端服务平台。 | — |
| **A2A** | Agent-to-Agent 调用授权开关。 | `docs/features/Agent_Manager_UI.md` |
| **Topic ACL** | 实时主题权限控制，决定读写能力。 | `docs/realtime/Topic_ACL_in_UI.md` |
| **Env Bootstrap** | 启动所需的最小配置集合（数据库连接、初始 token 等）。 | `docs/environment/Dev_Environment_Setup.md` |

> 术语持续更新，新增概念时请同步补充并链接至相关文档或代码。
