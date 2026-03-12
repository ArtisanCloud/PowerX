# Channel 可落地用例（PowerX 底座）

## 用例 1：多渠道入站统一标准化

- 目标：飞书/企微/TG/Discord 入站都输出统一 `ChannelMessage`。
- 验收标准：
  - 字段齐全：message_id/session_id/tenant_uuid/idempotency_key。
  - topic 统一发布到 `channel.message.received`。

## 用例 2：网关鉴权与租户隔离

- 目标：所有入站请求经过网关并绑定租户上下文。
- 验收标准：
  - 缺失或非法鉴权直接拒绝。
  - 不信任客户端自带 tenant 字段。

## 用例 3：命令派发与回执回推

- 目标：Runtime 派发命令给插件并把结果回推渠道。
- 验收标准：
  - `channel.command.dispatched` -> 插件执行 -> `channel.command.result`。
  - 失败回执含 error_code/error_message。

## 用例 4：幂等与重试治理

- 目标：重复投递不触发重复业务执行。
- 验收标准：
  - 相同 idempotency_key 只处理一次。
  - 重试返回上次结果或受控状态。

## 用例 5：插件切换模式零业务改动

- 目标：SCRM 从 standalone 切 host/proxy 不改编排层。
- 验收标准：
  - 插件代码只替换 provider/runtime 接入层。
  - Contract 与业务结果保持一致。
