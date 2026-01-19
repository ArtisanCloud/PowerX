# PowerX 通用 WebSocket 消息总线方案（单连接/多主题）

> 目标：所有异步任务与系统通知通过**一个 WebSocket 连接**推送，前端按主题订阅消费，避免多模块多连接。

## 1. 设计目标

- **单连接多主题**：全站仅建立一条 WS 连接，支持多个 topic 订阅。
- **统一消息结构**：业务模块不自造格式，统一 envelope。
- **可扩展**：支持新模块直接新增 topic。
- **可回退**：断线/不支持 WS 时，前端可退回轮询。
- **低侵入**：尽量复用现有鉴权与租户上下文。

## 2. 连接与鉴权

- **连接地址**：`wss://<host>/api/ws`
- **鉴权方式**：
  - 方案 A（推荐）：URL Query 带 token（`?access_token=...`）
  - 方案 B：连接后第一条消息 `auth` 指令
- **租户上下文**：
  - token 中携带 `tenant_uuid` 或 `tid`，服务端解析并绑定到 WS session
  - 必须支持切换租户：前端切租户时重连 WS

## 3. 消息格式（统一 envelope）

### 3.1 客户端 → 服务端

```json
{
  "action": "subscribe",
  "topics": ["knowledge.ingestion.*", "system.notice"],
  "meta": {
    "client": "web-admin",
    "version": "1.0"
  }
}
```

```json
{
  "action": "unsubscribe",
  "topics": ["knowledge.ingestion.*"]
}
```

```json
{
  "action": "ping",
  "ts": 1730000000000
}
```

### 3.2 服务端 → 客户端

```json
{
  "topic": "knowledge.ingestion.job",
  "type": "snapshot",
  "trace_id": "...",
  "ts": 1730000000000,
  "payload": {
    "space_id": "...",
    "job_id": "...",
    "status": "running",
    "progress": 42,
    "chunk_total": 108,
    "embedding_pct": 30,
    "masking_pct": 50
  }
}
```

```json
{
  "topic": "system.notice",
  "type": "event",
  "payload": {
    "level": "info",
    "message": "维护完成"
  }
}
```

### 3.3 约定字段

- `topic`：主题名，支持 `*` 通配订阅
- `type`：
  - `snapshot`：全量快照
  - `delta`：增量更新
  - `event`：事件类通知
- `payload`：业务数据，模块自定义

## 4. Topic 规划（第一期）

### 4.1 入库任务

- `knowledge.ingestion.job`：任务状态/进度
- `knowledge.ingestion.progress`：细粒度进度（可合并到 job）

Payload 示例：

```json
{
  "space_id": "...",
  "job_id": "...",
  "status": "running|completed|failed|blocked|retrying",
  "progress": 0-100,
  "stage": "extract|chunk|embed|mask|persist",
  "chunk_total": 108,
  "embedding_pct": 20,
  "masking_pct": 50,
  "updated_at": "2026-01-19T12:34:56Z"
}
```

### 4.2 系统通知（预留）

- `system.notice`
- `system.metrics`

## 5. 后端实现要点

### 5.1 WS Router

- 新增 `backend/internal/transport/websocket/routes.go` 下通用 `/api/ws`
- 复用 gin + gorilla websocket
- 每个连接创建 session：
  - 保存 tenant UUID
  - 保存订阅 topics

### 5.2 消息分发

- 建立一个全局 Hub（广播 + topic 路由）
- 业务服务通过 `hub.Publish(topic, payload)` 发送

### 5.3 入库进度推送

- 在 `IngestionService` pipeline 中更新进度
- 每个阶段写入进度并 push：
  - 0%：任务创建
  - 20%：抽取完成
  - 50%：切块完成
  - 80%：embedding 完成
  - 100%：落库完成

### 5.4 认证/鉴权

- 解析 JWT token → tenant UUID → 绑定 session
- 只允许订阅本租户数据

## 6. 前端实现要点

### 6.1 单连接管理

- 新增全局 WS 客户端（Pinia store / composable）
- 页面层只订阅/取消订阅 topics
- 连接断开自动重连（指数退避）

### 6.2 消息消费

- 由 `topic` 派发给对应模块
- `knowledge.ingestion.job` 更新列表进度条
- 提供 fallback：若 WS 不可用，继续轮询

## 7. 兼容与迁移

- 旧轮询逻辑保留，WS 作为优先通道
- 逐步将所有异步任务接入该消息总线

## 8. 开发步骤（建议）

1. 后端：实现 WS Hub + 通用 `/api/ws`
2. 后端：入库 pipeline 加 `Publish` 进度
3. 前端：全局 WS 客户端 + 订阅机制
4. 前端：入库页改为 WS 驱动（轮询兜底）
5. 压测/验证：并发连接、消息投递

## 9. 验收标准

- 同一浏览器仅 1 条 WS 连接
- 入库进度实时更新，无需手动刷新
- 断线后自动重连并恢复订阅
- 多租户隔离正确

