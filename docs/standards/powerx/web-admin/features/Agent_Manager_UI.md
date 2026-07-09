# Agent 管理控制台

> 本文描述 `/agent` 页面及相关组件的布局、交互与 API，对应实现分布在 `app/pages/agent/index.vue`、`app/components/agent/*` 与 `app/composables/agent/*`。

---

## 1. 页面概览

| 区域 | 组件 | 功能 |
| --- | --- | --- |
| 左侧栏 | `AgentSidebar` | 展示 Agent 列表、切换、创建/删除入口（TODO）。 |
| 主区 | `ChatInterface` | 会话窗口、消息列表、输入框。 |
| 顶部状态 | `ConnectionIndicators` | 显示 SSE/WS 连接状态、消息流进度。 |
| 顶部权限 | `AgentWorkspace` 生效权限弹层 | 展示当前登录用户使用所选 Agent 的最终权限交集。 |
| 右侧配置 | `ConfigPanel` | 展示与编辑 Agent 配置（模型、温度、工具等）。 |

页面初始化时调用 `useAgentManager().fetchAgents()` 并自动选中第一个 Agent（`app/pages/agent/index.vue: "56`）。未加载成功的错误通过 `useOneShotAlert` 提示。"

---

## 2. Agent 列表与详情

- `useAgentManager` 封装 `/admin/agents` CRUD 接口（`app/composables/agent/useAgentManager.ts:7`）。  
- 列表默认按照 `status=active` 拉取，可扩展支持搜索、排序。  
- `fetchAgentDetail(agentId)` 返回单个 Agent 信息，便于在 `ConfigPanel` 中展示并编辑。  
- 创建/更新成功后自动刷新列表；删除 Agent 会重新加载并保持 UI 同步。

> 当前界面尚未提供显式的“新建/编辑” UI，需要在 `ConfigPanel` 或独立弹窗中接入表单。

---

## 3. 会话管理

- 使用 `useChatSessions()` 管理会话列表、分页、置顶、重命名等逻辑（参见 `app/stores/agentSession.ts` 与对应 composable）。  
- 选择 Agent 后触发 `listSessions(agentId)`，并清空消息缓存。  
- 选中会话时加载历史消息并写入 `useDualChannelConnection().messages`。  
- 提供创建、删除、加载更多、重命名等事件处理函数，失败时通过 `notifyOnce` 提示。

---

## 4. 双通道聊天

- `useDualChannelConnection(currentAgentId, currentSessionId)` 管理 SSE/WS 连接，见 `docs/realtime/SSE_WS_Client_Guide.md`。  
- `handleSendMessage` 接收输入框文本，调用 `chat.sendMessage(content, "chat", meta)`，其中 `meta` 包含 `sessionId`、`agentId`。  
- `isConnected`、`isStreaming` 暴露给 UI 控制按钮状态与 Loading。

---

## 5. Agent 配置面板

- `ConfigPanel`（待补充）预期展示：  
  - 基础信息：名称、描述、Avatar。  
  - 模型设置：provider、model、参数（温度、maxTokens 等）。  
  - 工具白名单：可选工具列表与启用状态。  
  - 授权（A2A）：开关 Agent-to-Agent 调用、配额限制。  
- 保存调用 `useAgentManager.updateAgent()`；若后端要求草稿/发布流程，可在 panel 中区分“保存草稿”与“发布”。

### 5.1 工具与权限配置

Agent 管理页的“工具与权限”只配置 Agent 自身最大可用能力边界，不展示或覆盖登录用户 IAM 权限。保存时调用：

| Method | Path | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/admin/agents/grantable-capabilities` | 拉取当前租户可授权 capability |
| `GET` | `/api/v1/admin/agents/{agent_uuid}/grants` | 拉取 Agent 已配置授权 |
| `PUT` | `/api/v1/admin/agents/{agent_uuid}/grants` | 全量替换 Agent 授权 |

页面展示 capability 时应优先显示 `display_name`，`capability_uuid` 仅作为隐藏值或诊断信息，不得作为主标签。

---

## 6. Quota 与 A2A（规划中）

- **配额**：展示每日调用次数、令牌消耗上限，支持重置或申请提升。  
- **Agent-to-Agent 授权**：提供切换开关，选择允许调用本 Agent 的其他 Agent 列表。  
- 数据需由后端接口返回（例如 `/admin/agents/{id}/quota`），前端通过右侧面板或独立模态呈现。

---

## 7. 权限控制

- Root / 平台管理员：可管理所有 Agent，显示启停、删除、配额配置入口。  
- 租户管理员：可管理租户内 Agent；对系统级 Agent 只读。  
- 普通成员：可发起会话但不能更改配置。  
- 建议在 `AgentSidebar` 和 `ConfigPanel` 中根据权限隐藏操作按钮，并在按钮禁用时提供 Tooltip。

### 7.1 当前会话生效权限

会话工作台顶部提供“生效权限”入口，用于展示当前登录用户使用当前 Agent 时的最终权限集合。数据来自：

```text
GET /api/v1/admin/agents/{agent_uuid}/my-effective-permissions
```

展示列包括：

| 列 | 含义 |
| --- | --- |
| 能力名称 | `display_name`，缺失时可显示 `capability_id` |
| 权限码 | `permission_code` |
| 用户权限 | 当前用户 IAM 是否允许 |
| Agent 授权 | Agent grant 是否允许 |
| 租户启用 | 当前租户是否启用 capability |
| 策略允许 | capability policy 是否允许 Agent 使用 |
| 拒绝原因 | `deny_reason` 的本地化说明 |

该面板是诊断和解释视图，不允许在会话页直接提升用户权限或 Agent grant。若某项拒绝，应跳转到对应管理页处理，而不是在前端静默隐藏错误。

---

## 8. 测试与验收

- [ ] 能加载 Agent 列表并切换不同 Agent 会话。  
- [ ] 在有/无历史会话时聊天窗口表现一致。  
- [ ] 连接断开时显示红色指标，并给出重连按钮。  
- [ ] 创建/删除/重命名会话后页面状态正确更新。  
- [ ] 编辑配置后成功保存，更新列表数据。  
- [ ] 在权限不足情况下操作被正确阻止并提示。
- [ ] Agent 管理页能保存 capability grant，刷新后授权状态保持一致。
- [ ] 会话工作台“生效权限”显示用户权限与 Agent 授权的交集；缺任一维度时显示拒绝原因。
- [ ] 页面不把 UUID 作为 Agent、capability、plugin 的主展示标签。

---

## 9. 后续计划

- Agent 复制/版本管理；支持导入导出 JSON 配置。  
- 会话归档、搜索与标签。  
- 对话运行日志、调用链追踪。  
- 将 Agent 管理拆分为 `/agent/list`（管理）与 `/agent/chat`（对话）两套视图，满足不同角色需求。
