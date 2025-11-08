# Pinia Store 编写规范

> 适用于 PowerX Web Admin 前端工程的状态管理模块，覆盖命名、模块拆分、数据归一化与持久化策略。示例均引用仓库现有 store，例如 `app/stores/user.ts:1`、`app/stores/envStore.ts:1`。

---

## 1. 总体约定

- **命名**：文件名使用 `useXStore.ts` 或短名如 `user.ts`，导出函数统一为 `useXStore`；`defineStore` 的第一个参数保持与文件名一致，如 `defineStore("user", …)`（`app/stores/user.ts: "7`）。"
- **目录职责**：`app/stores/` 仅放 Pinia store；跨组件复用的业务逻辑仍放在 `app/composables/`。  
- **类型定义**：状态、入参、返回值必须显式声明 TypeScript 类型；通用类型存放在 `app/types/**`，避免在 state 中直接写 `any`。  
- **初始化**：涉及持久化的 store 提供 `initialize()` 方法，并在页面或插件层调用（参见 `app/stores/envStore.ts: "146`）。"
- **客户端守卫**：尽管当前 Nuxt 运行于 SPA 模式（`nuxt.config.ts: "8`），仍禁止在 `setup` 阶段直接访问 `window`，统一使用 `process.client` 判断。"

---

## 2. Store 模式选择

| 场景 | 推荐模式 | 参考实现 |
| --- | --- | --- |
| 需要清晰的 state / getters / actions | 对象式：`defineStore(id, { state, getters, actions })` | `app/stores/message.ts:1` |
| 依赖 `ref`/`computed`/组合式 API | Setup 模式：`defineStore(id, () => { … })` | `app/stores/permission.ts:60` |
| 重用工具函数 | 将通用逻辑抽到 composable，再在 store 中调用 | `useApiClient()` 被多个 store 使用 |

> 在对象式 store 中新增方法时，优先放在 `actions` 内；Setup 模式中则显式返回需要暴露的 `ref` 与函数。

---

## 3. 状态结构与归一化

1. **按资源 ID 建立字典**  
   - 聊天消息：`messagesBySession: Record<string, ChatMessage[]>` `app/stores/message.ts:9`。  
   - Agent 会话：`sessionsByAgent: Record<number, ChatSession[]>` `app/stores/agentSession.ts:9`。  
   使用字典避免数组扫描，并便于追加/替换。

2. **辅助状态分离**  
   - Loading、分页游标、错误信息分别记录，如 `loadingBySession`、`hasMoreBySession`、`error`（`app/stores/message.ts:15`）。  
   - 权限模块将列表、目录、租户授权拆分为不同 ref，防止状态互相覆盖（`app/stores/permission.ts:87`）。

3. **派生数据使用 getters/computed**  
   - Setup 模式通过 `computed` 返回树形结构（`catalogTree`、`normalizedList` `app/stores/permission.ts:118`）。  
   - 对象式 store 使用 `(state) =>` getter 提供查询器（`getMessagesBySession` `app/stores/message.ts:23`）。

4. **只在 actions 内修改 state**  
   - 通过浅拷贝生成新对象，保证响应性：`this.messagesBySession = { ...this.messagesBySession, [key]: next }`。  
   - 更新数组时优先复制：`const next = [...current, ...newMessages]`。

---

## 4. 异步行为与错误处理

- **统一 Loading**：进入异步前设置 Loading，`finally` 中恢复（`setLoading()` + `fetchCatalog()`）。  
- **错误处理**：捕获异常后写入 `error` 字段并 `console.error`，同时将错误抛给调用方决定 UI 行为（见 `app/stores/permission.ts:150`）。  
- **幂等更新**：更新单条记录时先查找索引，然后创建浅拷贝（`updateMessage()` `app/stores/message.ts:97`）。  
- **批处理**：追加分页数据时维护 `lastMessageId`，便于后端游标请求（`app/stores/message.ts:119`）。

---

## 5. 持久化策略

| 需求 | 实现方式 | 示例 |
| --- | --- | --- |
| 环境/偏好长期记忆 | 手写 `loadFromStorage` / `saveToStorage` 并在 `process.client` 守卫内操作 localStorage | `app/stores/envStore.ts:126` |
| 会话级缓存 | 仅存内存，提供 `clear()` 清理（路由切换时调用） | `app/stores/agentSession.ts:139` |
| 未来 SSR | 预留 Pinia 插件位于 `app/plugins/`（尚未启用） | — |

实施要点：

1. 统一以 JSON 存储数据；读写都需 try/catch，打印明确日志。  
2. 公开 `initialize()`，在布局或 `app.vue` 中触发一次。  
3. 若需要跨标签同步，可结合 `storage` 事件在 composable 中监听（TODO）。

---

## 6. Store 间协作

- 避免直接互相修改 state；在 action 内调用对方公开 API，或在页面层 orchestrate。  
- 复用 API 调用必须走 `app/composables/api/services/**` 或 `useApiClient()`，不要在多个 store 内重复拼 URL。  
- 事件总线场景（如 Agent 新消息广播）优先通过 store → composable → 组件的单向流，减少循环依赖。

---

## 7. 使用与测试建议

1. **注册 Pinia**：在 `plugins/`（客户端插件）中 `app.use(pinia)`，Nuxt 会自动注入。  
2. **组件消费**：
   ```ts
   const messageStore = useMessageStore();
   const { getMessagesBySession } = storeToRefs(messageStore);
   await messageStore.setMessages(sessionId, initialMessages);
   ```
   - 使用 `storeToRefs` 保持解构后仍具响应性。  
   - 在 `onBeforeUnmount` 中调用清理方法，防止遗留旧数据。

3. **单元测试**：在 Vitest 中调用 `setActivePinia(createPinia())`，然后直接实例化 store 并测试 actions。必要时 mock 掉 `useApiClient()`。

---

## 8. Code Review Checklist

- [ ] store ID、导出名称是否与文件命名一致。  
- [ ] state / actions / getters 是否全部有类型注释。  
- [ ] 是否正确管理 loading / error。  
- [ ] 是否避免对响应式对象的原地删除、直接赋值导致的不可追踪变更。  
- [ ] 持久化逻辑是否包裹 `process.client`。  
- [ ] 是否存在未清理的计时器、订阅。  
- [ ] 新增的 store 是否在文档和 `scripts/check-refactor.sh` 中同步登记（如迁移路径）。

> 若未来引入 Pinia 插件或多端共享 store，需要在本指南中追加相应约定。
