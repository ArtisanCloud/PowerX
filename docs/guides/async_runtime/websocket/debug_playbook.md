# WebSocket 联调与排障手册（Host / 插件 / Framework）

> 入口：`docs/guides/async_runtime/websocket/README.md`

## 1. 先做的三件事（避免无效排查）

1. 明确当前模式：Host 还是插件 standalone。
2. 明确 WS 最终地址是怎么计算出来的（来自 runtime contract，而不是手写端口）。
3. 明确 token 类型：用户 token、delegated token、tool token 分工不同。

## 2. 地址与鉴权硬规则

1. WS 连接地址
- 必须走：`NUXT_PUBLIC_WS_ORIGIN + NUXT_PUBLIC_WS_PATH`（通常 `/api/ws`）。
- 禁止直接拼前端端口（例如 `127.0.0.1:3030`）去当后端地址。

2. 宿主 ws-bus 接口鉴权
- `POST /api/v1/admin/runtime/ws-bus/grant`
- `POST /api/v1/admin/runtime/ws-bus/publish`
- 插件后端调用这两个接口时，必须优先用 `PX_PLUGIN_TOOL_TOKEN`。
- 禁止透传 plugin delegated bearer 去调用上述接口，否则常见报错是 `token has invalid audience`。

## 3. 五段状态验收（前端）

必须依次可观测：

1. `connected`
2. `welcome_received`
3. `subscribe_sent`
4. `ack_received`
5. `event_received`

说明：
- 前四步成功但第五步失败，通常是 topic 不匹配、未真正 publish、或 ACL 未生效。

## 4. 统一协议（当前实现）

1. 客户端命令：`subscribe` / `unsubscribe` / `ping`
2. 服务端消息：`welcome` / `ack` / `error` / `event`
3. 前端业务消费只看 `type=event`

## 5. 最小联调命令

1. `grant`
```bash
curl -X POST "http://127.0.0.1:8077/api/v1/admin/runtime/ws-bus/grant" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"topics":["_topic.system.notification"]}'
```

2. `publish`
```bash
curl -X POST "http://127.0.0.1:8077/api/v1/admin/runtime/ws-bus/publish" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"topic":"_topic.system.notification","payload":{"msg":"ping"}}'
```

3. 页面/客户端必须已完成 `subscribe` 且收到 `ack`。

## 6. 日志判读模板（按优先级）

1. 宿主网关
- 看 `[GATE-DENY]` / `[GATE-ALLOW]`
- 看 `[PROXY-BACKEND-ERR]` 是否有上游 4xx/5xx

2. 插件后端
- 若出现 `grant rejected with status 401` 且伴随 `invalid audience`：
- 直接定位为“插件调用宿主 ws-bus 接口时 token 选错”。

3. WS 服务端
- 有 `subscribe ack` 无 `event`：
  - 先查 topic 是否一致
  - 再查 tenant 是否一致
  - 再查是否确实执行了 publish

## 7. 本轮典型故障与修复对照

1. 403 `no permission rule for this route`
- 原因：宿主网关路径归一化与策略坐标不一致。
- 修复：统一到 `/api/v1/...` 坐标并补显式 route 映射。

2. 404 `/api/v1/api/v1/...`
- 原因：反代路径重复拼接 basePath。
- 修复：上游路径构造去重。

3. 401 `invalid audience` on `/admin/runtime/ws-bus/grant`
- 原因：插件透传 delegated token。
- 修复：插件调用宿主 ws-bus 接口改用 `PX_PLUGIN_TOOL_TOKEN`。

## 8. Framework/插件对齐清单（上线前）

1. 前端只读 contract 构造 WS URL，不猜端口。
2. 后端 internal 调用 token 源正确（tool token 优先）。
2. 后端 ws-bus 调用 token 源正确（tool token 优先）。
3. 页面具备五段状态可视化。
4. topic 命名与 tenant 前缀全链路一致。
5. `grant -> subscribe -> publish -> event` 端到端演练通过。
