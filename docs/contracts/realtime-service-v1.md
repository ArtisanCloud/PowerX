# 插件服务实时事件回送 v1

## 边界与入口

本合同是有界的实时通知回送，不是任务队列、业务确认或业务重投。发布成功只证明 Core 缓冲已接受事件，不证明浏览器已收到或业务完成。客户端应重新读取已持久化业务状态，不能根据事件自行确认付款、生产或已读。

- `com.corex.runtime.realtime.service_publish`：`publish`。
- `com.corex.runtime.realtime.stream`：`cursor` 与 `subscribe`。
- 固定 binding：`core_internal`、`core://runtime/realtime`、`INVOKE`、`service_actor`、`api_key_or_sts`。
- 普通调用经配置 API prefix 下的 `/tenant/invocations`，订阅经 `/tenant/invocations/stream`。请求 `preferred_protocol=core_internal`，`payload` 只含 `body`；禁止调用方指定 URL、method、headers、context 或 query。
- `body` 严格采用 `realtime.service.v1`，未知字段、旧格式和错误作用域均拒绝。Schema：`realtime-service-v1.openapi.yaml`。

租户以 Core 已认证上下文为准；`tenant_key` 仅作相等断言。只有有效 API Key 或 STS 服务身份可用，用户 Bearer／root 不能代替插件服务凭证。每次发布和读取均核验发布记录、租户注册及当前授权；订阅每次读取前、交付前重新检查。普通 HTTP API Key 鉴权和 STS 签发另由现有 Gateway 执行。

## 隔离与多副本

Redis key 的作用域为 Core 环境 namespace、租户、当前服务主体、调用 namespace。API Key 主体与 STS 主体独立。多个插件副本必须使用相同服务主体及 namespace；不同 Key、不同 STS client、不同环境不会共享事件。更换主体／凭证的发布切换必须另行安排，不能假定旧主体历史自动迁移。

每个副本独立保存读游标，使用 XREAD 广播读取，不使用抢占式消费组。Core 接受租户内服务事件，插件仍须在浏览器订阅及逐条发送时执行用户／成员／topic 权限校验。该合同不授予浏览器访问整个租户流的权限。

## 数据与恢复

事件包含 canonical UUID `event_id`、`tenant_uuid`、`topic`、可选 UUID `member_uuid`、`trace_id` 和 `payload_base64`。Payload 是最多 64 KiB 的合法 JSON 原始字节，使用 canonical Base64，避免 Gateway 通用 JSON 解码损失大整数。不得记录业务 payload 或凭证到审计日志。

- 每个作用域精确保留最多 2048 项；流在最后一次写入后 1 小时失效。不是每项独立保留整整 1 小时。
- `cursor` 返回当前尾部或 `0-0`，只读且不创建业务数据。
- `subscribe` 首帧 `ready` 回显作用域／游标，后续 `event` 带 Redis cursor；空闲时发送 `heartbeat`。
- 每条连接约 30 秒后发送 `reconnect` 及最后 cursor，然后关闭；接收方从该 cursor 继续，不改用其他传输。
- 游标已因裁剪／过期丢失，返回 `REALTIME_CURSOR_EXPIRED` 并停止，禁止悄悄跳到最新值。初始空流 `0-0` 仅用于建立新订阅。
- 临时不可用为 `REALTIME_UNAVAILABLE`；撤权为 `REALTIME_GRANT_REQUIRED`。无隐式内存分发、其他 Redis、普通轮询或业务重提。
- 连接取消／读取关闭时取消 Redis 阻塞读取和后台 goroutine。服务端不储存每个浏览器的游标。

Framework RemoteHub 对同一读流按 cursor 抑制重复／乱序，按 event_id 有界去重；其作用仅是传输去重，不保证业务 exactly-once。业务事件本身的 operation/event ID 和持久化幂等继续由业务合同负责。

## 发布与验收

须先发布两项能力并完成租户注册，再显式配置现有服务凭证的授权；`capability-seed` 只同步目录、注册和显式权限定义，不应授予任何既有 Key。实际部署缺能力或授权时必须显式失败。无需业务表迁移。

开发验收覆盖 typed DTO、API Key／STS 主体隔离、环境／租户隔离、实时撤权、Base64 大整数、缓冲上限／过期及独立副本游标。受控鉴权桥不代表真实凭证签发或安装验收；实际发布、grants、插件代理及重启后的环境验收应另记录。
