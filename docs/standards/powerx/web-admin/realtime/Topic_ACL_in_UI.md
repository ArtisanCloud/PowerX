# Topic ACL 前端策略

> 本指南描述 PowerX Web Admin 在实时 Topic（事件流/会话流）上的访问控制表现，包括只读提示、禁用操作与与 RBAC 的协同策略。

---

## 1. ACL 数据来源

| 数据 | 来源 | 说明 |
| --- | --- | --- |
| Topic 列表 | 后端返回的 Agent/Workflow 订阅配置（待提供 API） | 包含 `topic`、`capabilities`（read/write/manage）等字段。 |
| 用户权限 | `usePermissionStore()` 与 `useMe().hasPermission()` | 结合 RBAC 判断用户是否拥有 topic 对应能力。 |
| 会话上下文 | `useDualChannelConnection()` 入参 `agentId`、`sessionId` | 根据当前资源决定订阅/发布范围。 |

当前代码尚未显式返回 Topic ACL，需要在后端 API 中补充 `topics` 数组或扩展已有权限接口；前端则根据以下约定作 UI 呈现。

---

## 2. 可视化原则

| ACL 情况 | UI 表现 | 说明 |
| --- | --- | --- |
| `read` + `write` | 正常发送消息/操作 | 按默认体验展示。 |
| `read` only | 禁用输入框、按钮，显示只读提示 | 在输入区下方显示 `UAlert`：“当前会话为只读”。 |
| 无访问 | 隐藏该 Topic 或展示 403 空态 | 对话列表仅展示标题和说明，禁止进入详情。 |
| 管理权限缺失 | 隐藏管理按钮（如订阅、踢出） | Tooltip 提示“需要 manage 权限”。 |

同一页面可能存在多个 Topic（如实时日志、Agent 对话），应逐一计算权限并决定 UI 状态。

---

## 3. 接入步骤

1. **获取 ACL**：在进入实时页面时请求 `GET /agents/{id}/topics`（举例），或复用 `permissionStore.fetchCatalog()` 返回的 `meta.type === 'topic'` 数据。  
2. **组合能力**：将 Topic ACL 与 RBAC 权限合并，例如 `topic=agent:chat`, 权限为 `agent.chat.send`/`agent.chat.view`。  
3. **更新连接参数**：在 `useDualChannelConnection` 中根据 ACL 决定是否发送订阅命令、是否允许 `sendMessage`。  
   ```ts
   const allowWrite = acl[topic]?.includes("write");
   if (!allowWrite) disableComposer();
   ```
4. **界面反馈**：  
   - 只读提示使用 `role="status"`，便于辅助技术朗读。  
   - 若用户尝试执行无权操作，调用 `normalizeApiError` 并显示“需要权限”提示。  
5. **日志记录**：将 ACL 决策记录到 Analytics/监控中，便于排查用户反馈。

---

## 4. QA Checklist

- [ ] 切换租户/角色后 Topic 列表及时更新。  
- [ ] 只读状态下，输入区/按钮不可点且有辅助说明。  
- [ ] 无权访问 Topic 时展示统一 403 空态或直接隐藏。  
- [ ] 当权限变更（例如管理员赋权）后，刷新页面或通过事件通知恢复操作能力。  
- [ ] 后端返回 403/401 时前端不重复发送请求，及时断开连接。

---

## 5. 后续计划

- 后端提供 Topic/权限枚举接口，并在登陆时一并返回，减少额外请求。  
- 在 `/test/connection` 页面增加 ACL 模拟参数，便于 QA 验证。  
- 与权限申请流程联动，用户无权操作时可直接提交申请。  
- 将 Topic ACL 与通知中心集成，提示用户订阅状态变化。
