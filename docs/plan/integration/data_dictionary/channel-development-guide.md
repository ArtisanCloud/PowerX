# Channel 开发与集成主文档（PowerX 底座口径）

## 1. 文档目标

定义 PowerX 底座与插件在 Channel 能力上的职责边界与集成规范，确保：

- 插件业务层不感知运行模式差异；
- standalone 与 host/proxy 行为一致；
- 多插件复用统一运行时能力（鉴权、租户、审计、幂等、限流）。

## 2. 分层职责

- PowerX Core（底座）
  - Channel Runtime（连接管理、网关接入、路由分发）
  - 安全与治理（鉴权、RBAC、审计、租户隔离、限流）
  - 统一运行时接口（`/api/v1/admin/runtime/*`）
- PowerXPlugin framework
  - 统一 contract、client/provider 抽象
  - Provider 模式切换（`POWERX_PROVIDER_MODE`）与宿主链路开关（`POWERX_PROXY`）
  - skeleton 模板能力沉淀
- 业务插件（如 SCRM）
  - 业务编排（线索、作业、回执）
  - 不直接耦合第三方渠道 SDK

## 3. Provider 与运行时链路语义

- `POWERX_PROVIDER_MODE=local`：插件业务 provider 使用本地 service / DB。
- `POWERX_PROVIDER_MODE=delegated`：插件业务 provider 委派到 PowerX Core 能力。
- `POWERX_PROXY=0`：不连接 PowerX 宿主代理、网关、WS、Scheduler 等运行时链路。
- `POWERX_PROXY=1`：连接 PowerX 宿主代理、网关、WS、Scheduler 等运行时链路。

`POWERX_PROXY` 不得推导或覆盖 `POWERX_PROVIDER_MODE`。例如 `POWERX_PROVIDER_MODE=local` 且 `POWERX_PROXY=1` 表示业务数据仍走插件本地 provider，但运行时链路连接宿主。

要求：同一业务接口在两种模式下返回语义一致。

## 4. Contract 与事件基线

建议统一对象：

- `ChannelSession`
- `ChannelMessage`
- `ChannelCommand`
- `ChannelResult`

建议统一事件主题：

- `channel.session.opened`
- `channel.message.received`
- `channel.command.dispatched`
- `channel.command.result`

必要字段：`tenant_uuid`、`session_id`、`message_id`、`idempotency_key`、`request_id`。

## 5. 与 SCRM 的关系

- SCRM 是 Channel 能力的重点业务插件之一；
- 但 Channel Runtime 与“系统级数据字典”等基础设施属于底座能力，不应绑定单插件。

即：

- 业务模型在 SCRM；
- 运行时与治理能力在 PowerX Core + framework。

## 6. 对框架与插件的约束

1. 插件业务层禁止直接判断宿主/本地并拼接不同 API。
2. 模式差异只能在 framework/service 适配层处理。
3. 统一透传 `X-Request-ID`，统一审计字段。
4. 与可枚举配置相关场景，统一使用系统数据字典能力。

## 7. 验收项

1. local/delegated 业务语义一致。
2. 幂等重放不造成重复写入。
3. 多租户隔离有效。
4. 链路可观测（trace/request/audit 可追踪）。
5. 插件不再维护平行运行时基础设施（如私有字典系统）。
