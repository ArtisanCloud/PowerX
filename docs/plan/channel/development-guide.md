# PowerX Channel Runtime 开发文档（完整）

## 1. 文档目的

本文件是 PowerX 底座侧的 Channel Runtime 完整开发文档，用于直接指导平台开发与插件协同。

核心目标：

- 统一接入飞书、企业微信、Telegram、Discord。
- 对所有插件提供统一消息/会话/命令能力。
- 在网关层统一实现鉴权、限流、审计、幂等、租户隔离。

## 2. 平台职责（必须遵守）

底座必须提供：

1. Channel Runtime（连接器、Webhook 接入、消息路由、会话桥）。
2. Runtime 级治理（鉴权、审计、限流、重试、幂等）。
3. 统一协议与事件输出（对插件暴露稳定 Contract）。

底座不做：

- 业务域编排（如 SCRM 线索策略、员工作业语义）。

## 3. 与 Framework、插件的边界

- PowerXPlugin framework：
  - 定义 Contract 类型与 client/provider 接口。
  - 提供插件侧 SDK 与 mock。
- 插件（如 SCRM）：
  - 消费 `channel.*` 事件并执行业务编排。
  - 发布 `channel.command.result` 返回业务执行结果。

## 4. 统一数据契约

### 4.1 ChannelSession

```json
{
  "session_id": "sess_xxx",
  "tenant_uuid": "7561d35a-a35d-4a8e-87b6-c78b842b1f87",
  "channel": "wecom",
  "actor_type": "employee",
  "actor_id": "emp_1001",
  "opened_at": "2026-03-10T10:00:00Z"
}
```

### 4.2 ChannelMessage

```json
{
  "message_id": "msg_xxx",
  "session_id": "sess_xxx",
  "direction": "inbound",
  "content": {"type": "text", "text": "创建一个新线索"},
  "timestamp": "2026-03-10T10:00:03Z",
  "idempotency_key": "wecom:chat123:seq9981"
}
```

### 4.3 ChannelCommand

```json
{
  "command_id": "cmd_xxx",
  "session_id": "sess_xxx",
  "intent": "scrm.lead.create",
  "payload": {"name": "张三", "mobile": "13800000000"},
  "operator": {"type": "employee", "id": "emp_1001"}
}
```

### 4.4 ChannelResult

```json
{
  "command_id": "cmd_xxx",
  "status": "succeeded",
  "error_code": "",
  "error_message": "",
  "result": {"lead_id": "lead_001"},
  "timestamp": "2026-03-10T10:00:05Z"
}
```

## 5. 事件主题

- `channel.session.opened`
- `channel.message.received`
- `channel.command.dispatched`
- `channel.command.result`

Runtime 处理规则：

1. 渠道入站消息标准化为 `ChannelMessage`。
2. 发布 `channel.message.received` 给插件。
3. 接收插件命令或业务调用，发布 `channel.command.dispatched`。
4. 收到插件结果后发布 `channel.command.result` 并回推渠道。

## 6. 网关绑定要求

1. 入站请求必须先过网关。
2. Runtime -> 插件调用必须走网关能力链路。
3. 不信任客户端 `tenant_uuid`，以网关上下文为准。

## 7. 幂等与重试

1. 每条入站消息必须带 `idempotency_key`。
2. 重试时 `message_id` 与 `idempotency_key` 不变。
3. `command_id + status` 可重复上报但必须可去重。

## 8. 审计要求

每次关键操作必须记录：

- tenant_uuid
- request_id
- session_id
- message_id
- command_id
- operator
- channel
- action
- result

## 9. 模式兼容

- Host/Proxy：平台标准模式，插件通过 runtime 接入。
- Standalone：插件本地可自带 provider，但必须遵守同一 Contract。

平台必须保证：

- Contract 不破坏兼容。
- 插件从 standalone 切到 host/proxy 时无需改业务编排层。

## 10. 开发实现建议（底座）

- `internal/channel/runtime`：runtime 主流程。
- `internal/channel/adapters`：各平台 adapter。
- `internal/channel/contracts`：协议与 schema 校验。
- `internal/channel/router`：事件路由与插件分发。
- `internal/channel/governance`：鉴权、限流、幂等、重试。
- `internal/channel/audit`：审计落库与检索。

## 11. 联调与验收

### 11.1 必测

1. 渠道入站 -> 事件发布 -> 插件处理 -> 回执回推全链路成功。
2. 异常重试不重复执行业务动作。
3. 多租户并发不串租户数据。
4. 插件掉线/超时时可观测、可恢复。

### 11.2 验收标准

- 四类渠道至少两类真实联调通过（建议企微+飞书先行）。
- SCRM 在 standalone 与 host/proxy 下行为一致。
- 全链路审计可按 request_id 追踪。
