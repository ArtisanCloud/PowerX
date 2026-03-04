# Quickstart

## 目标
验证通用 WebSocket 消息总线可用，并能实时看到入库任务进度。

## 步骤

1. 启动后端与 web-admin。
2. 登录并进入 AI 设置，选择 embedding 模型并完成测试与保存。
3. 在知识空间页面触发一个入库任务。
4. 保持入库页面打开，观察进度条是否实时更新，无需刷新。
5. 切换租户（若有），确认 WS 自动重连且不接收旧租户消息。
6. 断开网络后恢复，确认回退机制仍能显示最新状态。
7. 打开浏览器 DevTools → Network → WS，确认 `/api/ws` 返回 `101 Switching Protocols` 并已建立连接。
8. 触发一次入库任务，确认进度推送在 2 秒内可见。

## 期望结果

- 任务进度实时变化（≤2 秒延迟）。
- 页面仅 1 条 WS 连接。
- 租户切换后消息隔离正确。

## US2 验证（单连接多主题）

1. 打开 DevTools → Network → WS，确认仅存在一个 `/api/ws` 连接。
2. 在控制台发送订阅：
   - `system.notification`
   - `knowledge.ingestion.job`
3. 确认服务端返回 `ack`，且同一连接内可收到不同 topic 的消息。

## US2 验证（无权限订阅）

1. 发送一个未允许的 topic（例如：`system.secret`）。
2. 确认服务端返回 `error`，且无后续推送。

## US3 验证（宿主发布入口）

1. 宿主模式下，插件后端先调用**注册入口**：
   - `POST <APIPrefix>/internal/ws-bus/grant`（默认 `<APIPrefix>=/api/v1`）
   - payload 示例：
     ```json
     {
       "topics": ["org_sync.progress", "powerx.org_sync.progress.v1"]
     }
     ```
2. 再调用底座发布入口：
   - `POST <APIPrefix>/internal/ws-bus/publish`（默认 `<APIPrefix>=/api/v1`）
   - payload 示例：
     ```json
     {
       "topic": "org_sync.progress",
       "payload": {
         "org_id": "demo",
         "status": "running",
         "progress": 25
       }
     }
     ```
3. 前端连接 `/api/ws` 并订阅 `org_sync.progress`（或 `powerx.org_sync.progress.v1`）。
4. 确认消息能实时抵达前端（无需轮询）。

### 现场验证记录（示例）

```
wscat -c "ws://127.0.0.1:8077/api/ws?authorization=Bearer $USER_TOKEN"
> {"type":"subscribe","topics":["org_sync.progress"]}
< {"type":"ack","payload":{"ok":true,"message":"subscribed","topics":["org_sync.progress"]},"ts":...}
< {"topic":"org_sync.progress","type":"event","payload":{"org_id":"demo","progress":25,"status":"running"},"ts":...}
```
