# SSE / WebSocket 客户端指南（当前实现）

> 本文仅描述当前已落地的实时链路：**业务流走 SSE，事件总线走 ws-bus WebSocket**。

## 1. 实时链路分工

1. SSE（Agent/推理流）
- 用途：LLM 对话与流式文本输出。
- 典型路径：`/api/agents/stream/sse`。

2. ws-bus WebSocket（事件流）
- 用途：进度、通知、异步任务事件（例如 Shopify 同步进度）。
- 典型路径：`/api/ws`（同源反代后由宿主处理）。

## 2. ws-bus 协议要点（必须对齐）

1. 连接建立后，客户端发送订阅命令：
```json
{ "type": "subscribe", "topics": ["ai_craft.shopify.sync.progress.<tenant_uuid>"] }
```

2. 服务端授权失败时返回：
```json
{
  "type": "error",
  "payload": {
    "code": "permission_denied",
    "message": "subscription rejected",
    "detail": "topic not allowed"
  }
}
```

3. 注意：当前协议是 `topics[]`，不是单字段 `topic`。

## 3. 前端时序（推荐）

1. 先调用 `grant`（插件 runtime grant 入口）。
2. 确认 `grant.data.topics` 命中目标 topic（非空）。
3. 再发送 ws `subscribe`。
4. 收到事件后更新 UI。

若 `grant` 仅返回 fallback（`topics: []`），后续很可能被 `topic not allowed` 拒绝。

## 4. 调试最短步骤

1. 看 grant 响应
- 目标：`data.topics` 包含完整 topic。

2. 看浏览器 WS 帧
- 必须看到：`{"type":"subscribe","topics":[...]}`
- 不应出现：`permission_denied / topic not allowed`

3. 看宿主日志
```bash
sudo journalctl -u powerx-backend --since "10 min ago" --no-pager -l | \
grep -E 'transport.wsbus|stage":"subscribed"|stage":"emit"|topic not allowed'
```

## 5. 已知边界

1. 当前 topic 注册是 `FindByComposite(tenant, namespace, name)` 精确命中。
2. 占位符/前缀模板匹配不是默认行为。
3. 需要在插件启用阶段完成 topic registry upsert（见 `Topic_Registry_and_Grant_SOP.md`）。
