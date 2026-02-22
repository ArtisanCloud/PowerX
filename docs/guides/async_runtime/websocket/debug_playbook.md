# WebSocket 联调手册（Host / Standalone / Proxy）

> 入口：`docs/guides/async_runtime/websocket/README.md`

## 1. 目标

用同一套 WS 协议验证三种模式：

1. Host（直连 PowerX）
2. Standalone（直连插件本地 WS）
3. Standalone Proxy（连插件 `/api/ws`，由插件转发到 PowerX）

并验证总线能力：**单连接复用多 topic 信道**（来自 `specs/023-websocket-notify/spec.md`）。

## 2. 协议与消息结构（当前实现）

客户端命令（`type`）：

1. `subscribe`
2. `unsubscribe`
3. `ping`

服务端消息（`type`）：

1. `welcome`
2. `ack`
3. `error`
4. `event`

统一 envelope：

```json
{
  "type": "ack|error|event|welcome",
  "topic": "_topic.system.notification",
  "payload": {},
  "ts": 1770000000000,
  "trace_id": "..."
}
```

## 3. 鉴权方式（WS）

当前支持两种：

1. Query：`?authorization=Bearer <JWT>`
2. 子协议：`Sec-WebSocket-Protocol: bearer.<b64url(jwt)>`

## 4. 连接地址矩阵

1. **PowerX 宿主（推荐）**  
   - `ws://127.0.0.1:8077/api/ws?authorization=Bearer $USER_TOKEN`
2. **插件 standalone（本地 WS）**  
   - `ws://127.0.0.1:8078/api/ws?authorization=Bearer $USER_TOKEN`
3. **插件 standalone proxy（转发到宿主）**  
   - `ws://127.0.0.1:8078/api/ws?authorization=Bearer $USER_TOKEN`

> 你之前使用的 `wscat -c "ws://127.0.0.1:8078/api/ws?authorization=Bearer $USER_TOKEN"`  
> 属于第 3 类（standalone proxy）调试方式。

## 5. `wscat` 快速调试

安装（如未安装）：

```bash
npm i -g wscat
```

连接（示例：standalone proxy）：

```bash
wscat -c "ws://127.0.0.1:8078/api/ws?authorization=Bearer $USER_TOKEN"
```

连接成功后应先收到 `welcome`。

### 5.0 多 topic 复用（同一连接）

在同一 `wscat` 连接里发送：

```json
{"type":"subscribe","topics":["_topic.system.notification","_topic.knowledge.space.feedback.reprocess"],"req_id":"sub-multi-1"}
```

期望：

1. 只保持当前这一条 WS 连接，不新建第二条连接。
2. 收到 `ack`，`payload.topics` 含两个 topic（被授权者）。
3. 后续来自两个 topic 的事件都从这条连接返回。

### 5.1 订阅

发送：

```json
{"type":"subscribe","topics":["_topic.system.notification"],"req_id":"sub-1"}
```

期望：

```json
{"type":"ack","payload":{"req_id":"sub-1","ok":true,"message":"subscribed","topics":["_topic.system.notification"]}}
```

### 5.2 心跳

发送：

```json
{"type":"ping","req_id":"ping-1"}
```

期望：

```json
{"type":"ack","payload":{"req_id":"ping-1","ok":true,"message":"pong"}}
```

### 5.3 取消订阅

发送：

```json
{"type":"unsubscribe","topics":["_topic.system.notification"],"req_id":"unsub-1"}
```

期望：

```json
{"type":"ack","payload":{"req_id":"unsub-1","ok":true,"message":"unsubscribed","topics":["_topic.system.notification"]}}
```

## 6. 用 Event Fabric 触发一条 WS 事件

在另一个终端执行：

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "http://127.0.0.1:8077/api/v1/admin/event-fabric/pipeline/tasks" \
  -d '{"title":"WS联调","content":"from websocket playbook","type":"system","category":"system"}' | jq
```

如果已订阅 `_topic.system.notification`，`wscat` 应收到 `type=event` 推送。

## 7. Host / Standalone 一致性验收

同一 `subscribe/ping/unsubscribe` 命令在三种模式都应满足：

1. 都收到 `welcome`
2. 都能 `ack subscribed`
3. 都能 `ack pong`
4. 都能收到相同语义的 `event` 推送
5. 都支持“单连接多 topic 复用”能力

## 8. 常见失败定位

1. `permission_denied`：ACL 未授权对应 topic 的 subscribe
2. `bad_request topics required`：subscribe/unsubscribe 未传 `topics` 或 `topic`
3. 连接即断开：JWT 无效、tenant/member 上下文不满足
4. 有 ack 无 event：订阅 topic 不对，或触发链路未产生该 topic 事件
