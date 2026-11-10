# Runtime TaskQueue v2

本合同为 Framework 的可靠异步队列提供正式 Core 服务入口，复用现有 Stream TaskBus、共享 TaskHistory、租约及重领机制。旧 `/admin/runtime/task-queue/*` 入口不是本合同的服务授权入口，v2 客户端不会回退到旧入口。

## 授权与命名空间

| 平面 | 能力／入口 | 授权 |
|---|---|---|
| 服务入队 | `com.corex.runtime.taskqueue.service_enqueue` | 发布的 capability、当前租户注册和 live API Key／STS grant |
| 服务消费 | `com.corex.runtime.taskqueue.service_consume` | 独立的消费 grant，覆盖 dequeue／renew／ack／nack |
| Admin | `POST /api/v1/admin/runtime/task-queue/v2/invoke` | 用户 JWT 和实时 RBAC；API Key／STS 不可访问 |

服务调用使用 `/tenant/capabilities/invoke`，固定 `preferred_protocol=core_internal`、`core://runtime/taskqueue`、`INVOKE` 和 typed `payload.body`。方法、地址、headers、query 和任意 proxy payload 不可由调用者选择。正式声明位于 `config/platform_capabilities/runtime_taskqueue.yaml`，字段合同见 [OpenAPI](runtime-taskqueue-v2.openapi.yaml)。

队列由服务端环境标识、受信租户、live credential subject 和逻辑 subscriber 共同隔离。API Key subject 使用实际 Key UUID；STS subject 使用经实时配置核验的插件和 client identity。共享同一凭据意味着共享服务身份，不能把 metadata.plugin_id 当作身份来源。Admin 使用当前用户自己的命名空间，不可选择其他服务主体。

变更 API Key、STS client 或环境标识属于显式运行态切换。v2 不在首次请求、读取或启动时搬运其他主体、其他环境或旧 processing 中的任务。入队保留插件 drain guard；STS 的插件声明必须与已认证插件一致。

## 投递合同

所有请求携带 `contract_version=taskqueue.v2`、`operation` 和与身份上下文一致的 `tenant_key`。未知字段、不属于当前操作的字段和不满足限制的值均拒绝。

- `enqueue`：必须提供消息 ID、幂等键、subscriber、topic 和严格 Base64 载荷。两种身份均约束同一不可变信封；ID／key／载荷冲突返回 409。确认返回 `accepted=true` 和相同 ID／key，不能用 HTTP 2xx 或空响应代替确认。TaskHistory 持久保存初始信封；原生历史的新增幂等键为 NULL，不伪造回填。
- `dequeue`：每次实际领取生成独立 opaque receipt UUID、递增 fence 和到期时间。服务器保留完整权威投递，不接受调用者传入 StreamID、owner 或原始 lease token。
- `renew`：只允许有效 receipt 续约；失去租约后不能复活。
- `ack`：必须携带当前 receipt 和 `outcome=completed|failed`。先持久化共享历史终态，再确认 Stream。确认不代表任意外部副作用具有 exactly-once 保证。
- `nack`：携带当前 receipt、机器原因码和可选的 `retry_at`；复用 TaskBus 的有界次数和 DLQ。延后投递由正式消费入口推进。

过期 receipt、跨租户／主体的 receipt 以及已重领任务的旧确认返回 `TASK_QUEUE_RECEIPT_STALE`。合法终态的重复 ACK 可重复确认，不重新执行任务。缺少环境配置明确返回 `TASK_QUEUE_ENVIRONMENT_REQUIRED`。

## 发布与验收

先做 Core schema 升级、capability-check／增量 seed 和目标凭据的 grant-status，再发布 Framework 并更新调用方。不能给 API Key 附加 Admin REST 权限替代服务 grant，也不能把隔离 fixture 中的 granted 当作开发／生产 Key 已授权。

本批源码测试包含真实 Redis／PostgreSQL 的入队去重、Nack 日期、续约与确认，API Key 实时撤权／禁用注册、旧 receipt，以及 Framework／AI Craft 的受控 HTTP 联调。完整 Core 安装态、实际发布依赖和历史队列切换另行验收。
