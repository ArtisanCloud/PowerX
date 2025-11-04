# SSE / WebSocket 客户端指南

> 指导如何在 PowerX Web Admin 中使用双通道实时连接（SSE + WebSocket），涵盖初始化、断线重连、主题订阅和背压处理，帮助前端快速集成实时能力。

---

## 1. 架构总览

- **双通道封装**：`useDualChannelConnection()`（`app/composables/agent/useDualChannelConnection.ts:27`）同时管理 SSE 与 WebSocket，提供统一的 `sendMessage`、`reconnectSSE`、`reconnectWS`、`messages` 等接口。  
- **数据路径**：前端请求 `/api/agents/stream/sse` 和 `/api/agents/stream/ws`（相对路径），由 Nitro `devProxy` 将请求转发至后端 `UPSTREAM`（`nuxt.config.ts:55`）。  
- **鉴权**：请求头携带 `Authorization: <token_type> <access_token>`，token 来源于 `localStorage`（`getAuthToken()`）。  
- **测试入口**：`/test/connection` 页面使用该 composable 验证探活、消息收发与日志记录（`app/pages/test/connection.vue:1`）。

---

## 2. 初始化与依赖

```ts
const connection = useDualChannelConnection(
  ref(currentAgentId),
  ref(activeSessionId)
);

await connection.reconnectSSE();
await connection.reconnectWS();
```

- **Agent / Session**：传入 `Ref`，自动缓存消息到 `messageStore`（`app/stores/message.ts:23`），切换会话时恢复历史记录。  
- **环境选择**：`getEnv()` 从 `localStorage: env-store` 读取当前环境，用于请求参数 `env`。  
- **Token 管理**：`getTokenType()`、`getAuthToken()` 读取 `localStorage`，若无 token 会禁用 WebSocket 连接。

---

## 3. SSE 流程

### 3.1 请求发起

- 调用 `sendSSEMessage(message, flowId, meta)`：  
  - 生成 `requestId` 并写入 `currentRequestId`。  
  - 构造查询参数：`q`、`env`、`flow_id`、`agent_id`、`session_id`。  
  - GET `/agents/stream/sse`，请求头 `Accept: text/event-stream`。

### 3.2 事件处理

- 使用 `TextDecoder` 将流拆分为 `event:`/`data:` 行，兼容 `[DONE]` 结束标记。  
- 各事件类型（`ACK`、`START`、`TOKEN`、`CHUNK`、`FINAL` 等）通过 `useStreamingThinkParser()` 解析思维链，填充 `messages`。  
- `applyMainContent()`、`dedupeThinkBlocks()` 避免重复内容并保持快照一致性。  
- 超时机制：若 10 秒未收到数据则标记为 `isError`，提示“连接超时：服务器未响应确认包。”。

### 3.3 背压控制

- SSE 不支持双向流控，因此在收到大段消息时先写入内存，再在空闲时同步至 `messageStore`（`syncMessagesToCache()`）。  
- 如需进一步控制，可在后端实现分块/节流，并在前端对 `messages` 加限长或分页。

---

## 4. WebSocket 流程

### 4.1 探活与重连

- `reconnectWS()` 建立短连验证：  
  - 构造协议 `bearer.<base64(token)>`。  
  - 连接 `ws(s)://<host><apiBase>/agents/stream/ws?probe=1&authorization=Bearer xxx`。  
  - `onopen/onmessage` 标记成功，`onerror/onclose` 标记失败。  
  - 超时时间 5 秒，失败后保持 `wsActive=false`。

### 4.2 正式连接（TODO）

- 当前 `sendMessage` 仍使用 SSE。若需要使用 WS 进行指令交互，可扩展：  
  ```ts
  const ws = new WebSocket(buildWSUrl("/agents/stream/ws"));
  ws.send(JSON.stringify({ type: "message", payload }));
  ```
- 与后端约定消息格式（如 `type`、`topic`、`payload`），并在前端处理 ACK/ERROR。

### 4.3 主题订阅

- 未来可按 `topic` 订阅不同流：  
  - 调用 `ws.send({ type: "subscribe", topic: "agent:<id>" })`。  
  - 收到 `topic` 字段后路由至对应 store。  
- 订阅信息应缓存，断线重连后重新发送。

---

## 5. 错误处理与通知

- `onErrorCallback` 可由业务层注入，统一处理网络异常、权限问题（401/403）。  
- 出现错误后组件可提示用户点击“重试 SSE/WS”按钮（见测试页）。  
- 对于无法恢复的错误，记录日志，并引导用户提交工单。

---

## 6. 背压与资源释放

- **消息长度限制**：在 `appendMessages()` 时可检测数组长度，超过阈值（比如 500 条）时移除最早记录或归档。  
- **取消请求**：`cancel()` 应调用 `AbortController`（未来待实现）终止 SSE 读取，避免长时间挂起。  
- **清理**：在组件卸载时执行 `disconnect()`，关闭 WS 并清空事件监听；消息保存在 `messageStore` 以便重新进入页面恢复。

---

## 7. 调试与测试

- `/test/connection` 提供：  
  - SSE/WS 探活按钮，展示 `connection.sseActive/wsActive`。  
  - 消息发送、取消、日志查看。  
  - 预期事件说明（ACK → token 流 → END）。  
- 浏览器 DevTools：  
  - Network → EventStream 查看 SSE。  
  - Network → WS 查看帧。  
  - Console 查看 `useDualChannelConnection` 打印的错误信息。

---

## 8. 上线检查表

- [ ] `UPSTREAM` / `WS_UPSTREAM` 指向正确，HTTPS 环境下确认证书信任。  
- [ ] WebSocket 代理开启 `ws: true`（`nuxt.config.ts:55`），生产 Nginx/Ingress 同步配置。  
- [ ] 断线重连逻辑在网络抖动、刷新 Token 后仍可恢复。  
- [ ] SSE 流在长时间会话中不会累积未释放的 reader。  
- [ ] 消息列表在租户/会话切换时正确缓存与恢复。  
- [ ] 测试页 `/test/connection` 通过 QA 验收，并在生产环境加访问守卫。

> 后续如需支持多路并发、队列优先级或扩展至 WebRTC，可在此文档继续沉淀协商协议与数据结构。
