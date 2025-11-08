# API 路由映射表

> 映射前端页面/模块与后端 API 端点的关系，便于前后端对齐与权限校验。若权限键尚未确定，可在表中补充。

---

## 1. 总览

| 页面 / 功能 | 代码位置 | Composable / Store | 请求 | API 端点 | 权限 / 备注 |
| --- | --- | --- | --- | --- | --- |
| 登录 | `app/pages/users/login.vue`（TODO） | `useAuthService().login` | `POST` | `/admin/user/auth/login` | `auth.login` |
| 用户上下文 | `app/stores/user.ts` | `useMe().getUserContext` | `GET` | `/admin/user/me/context`（假定） | `user.context.read` |
| Agent 列表 | `/agent` | `useAgentManager().fetchAgents` | `GET` | `/admin/agents` | `agent.list` |
| Agent 详情 | `/agent` | `useAgentManager().fetchAgentDetail` | `GET` | `/admin/agents/{id}` | `agent.read` |
| Agent 会话列表 | `/agent` | `useChatSessions().listSessions` | `GET` | `/admin/agents/{id}/sessions`（待确认） | `agent.session.list` |
| Agent 聊天流 | `/agent` | `useDualChannelConnection().sendMessage` | `GET` SSE / `WS` | `/agents/stream/sse` / `/agents/stream/ws` | `agent.chat` |
| 插件市场 | `/plugins/market` | `useAdminPluginsService().getMarketplaceV2` | `GET` | `/admin/plugins/marketplace/plugins_v2` | `plugin.market.view` |
| 插件启停 | `/plugins/installed`（TODO） | `useAdminPluginsService().enable/disable` | `POST` | `/admin/plugins/{id}/enable|disable` | `plugin.manage` |
| 插件安装 | `/plugins/market` | `useAdminPluginsService().installFromUrl` | `POST` | `/admin/plugins/install/url` | `plugin.install` |
| 插件卸载 | `/plugins/installed` | `useAdminPluginsService().uninstall` | `POST` | `/admin/plugins/{id}/uninstall` | `plugin.uninstall` |
| 权限目录 | `/settings/users` | `usePermissionStore().fetchCatalog` | `GET` | `/admin/iam/permissions/catalog` | `permission.catalog.read` |
| 权限列表 | `/settings/users` | `usePermissionStore().fetchList` | `GET` | `/admin/iam/permissions` | `permission.list` |
| 角色权限 | `/settings/users` | `usePermissionStore().fetchRolePermissions`（TODO） | `GET` | `/admin/iam/roles/{id}/permissions` | `role.permission.read` |
| Workflow 节点规格 | `/workflow/workspace` | `useWorkflowService().getKinds` | `GET` | `/workflow/kinds` | `workflow.read`（mock 回退） |
| Workflow Palette | `/workflow/workspace` | `useWorkflowService().getPalette` | `GET` | `/workflow/palette` | `workflow.read` |
| Workflow 保存 | `/workflow/workspace` | `useWorkflowService().saveWorkflow`（未来） | `POST` | `/workflow` | `workflow.write` |
| Dashboard 概览 | `/dashboard` | `useDashboardService()`（TODO） | `GET` | `/admin/dashboard/overview` | `dashboard.view` |
| 通知列表 | `useNotifications`（Mock） | 待后端接入 | `GET` | `/notifications` | `notification.list` |
| 菜单 | 全局 | `menuService.getUserMenus` | `GET` | `/admin/user/menu` | `menu.read` |
| 登出 | 全局 | `useAuthService().logout` | `POST` | `/admin/user/auth/logout` | `auth.logout` |
| Token 刷新 | 全局 | `useAuthService().refreshToken` | `POST` | `/admin/user/auth/refresh` | `auth.refresh` |

> 若实际接口不同，请在集成时更新表格，确保前后端使用统一路径。

---

## 2. 约定

- 所有管理端接口以 `/admin` 开头；用户侧/公开接口以 `/` 或 `/tenant` 区分。  
- RESTful 规范：列表 `GET /resource`，详情 `GET /resource/{id}`，新增 `POST`，更新 `PUT/PATCH`，删除 `DELETE`。  
- WebSocket/SSE 路由放在 `/agents/stream/ws|sse`，由 Nuxt `devProxy` 与 Nginx 代理。

---

## 3. 权限映射

| 权限键 | 说明 | 前端位置 |
| --- | --- | --- |
| `agent.list` | 查看 Agent 列表 | `AgentSidebar` |
| `agent.chat` | 与 Agent 对话 | `ChatInterface` |
| `plugin.install` | 安装/卸载插件 | `InstallDialog` |
| `permission.catalog.read` | 查看权限目录 | `PermissionManager` |
| `workflow.write` | 保存工作流 | `WorkflowEditor` |
| `dashboard.view` | 查看仪表盘 | `/dashboard` |

权限键需与后端 RBAC 系统一致，并在 `Permission_Guards_and_RBAC.md` 中描述 UI 控制逻辑。

---

## 4. TODO

- [ ] 补充缺失的 API 服务（Dashboard、通知、Workflow 保存）。  
- [ ] 验证租户上下文相关接口路径与参数。  
- [ ] 将权限键与后端 YAML/数据库配置同步存档。  
- [ ] 为插件生态提供动态路由映射表（按插件 ID 生成）。
