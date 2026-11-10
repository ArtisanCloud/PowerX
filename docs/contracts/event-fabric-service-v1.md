# Event Fabric 插件可靠事件合同与 Framework 对齐清单

合同：`event_fabric.service.v1`。本次修改日期：2026-11-04。

2026-11-05 增补：现有 service_publish / service_consume 的 `core_internal` binding 支持只读 `resolve_topics`，按实际凭据租户取得已准备业务 key 映射。装配、请求及 Framework 调整见 [插件安装与租户映射合同](plugin-event-fabric-install.md#framework-对齐按真实租户读取映射)。该操作不创建主题或授权，继续逐次检查实时 grant 与 Topic ACL。

## 1. 范围与交付边界

本合同用于插件业务事件发布、订阅、ACK/NACK，与 TaskQueue v2、WSBus 页面推送分开。
复用 Core 的 `event_envelopes`、`event_delivery_attempts`、`event_dlq_messages` 和 topic ACL。
可靠通道以数据库投递记录为准，不依赖先从 Redis 移除记录再发送；不新增表，但需正式迁移补充 claimed_by（varchar(64)，默认空值）领取主体指纹。旧记录不回填身份、不改变状态。
服务事件使用内部 `efs1:` ID，旧内部总线 worker 不得领取或自动确认这些记录。

源码、测试通过不代表已经发布。部署后还需确认正式目录、租户 registration、服务 grant、topic ACL 和消费者联调。
`powerx.event_fabric.v1` 是 Proto 包名；`event_fabric.service.v1` 是本次应用层合同，均不能代替实际 registration 版本及 catalog/protocol hash。

## 2. 正式能力与授权

| 能力 | 可执行操作 | API Key 精确 grant |
|---|---|---|
| `com.corex.eventfabric.service_publish` | publish | scope=`_scope.event_fabric.service.publish`, action=`publish`, resource_type=`capability`, resource_pattern=`event_fabric_service_publish` |
| `com.corex.eventfabric.service_consume` | subscribe、ack、nack | scope=`_scope.event_fabric.service.consume`, action=`consume`, resource_type=`capability`, resource_pattern=`event_fabric_service_consume` |

两项均要求 `effect=allow`。发布权限不包含消费；消费权限不包含发布、topic 管理或 ACL 管理。
还必须有对应租户、topic、插件主体的 `publish` / `subscribe` ACL。
如果要通过 grant-status 自查，还需原有的 `com.corex.capabilities.grant_status.read` 查询权限，它不代替发布/消费授权。

Core 从已认证凭据读取租户及真实主体，并实时检查 published capability、最新 registration、当前 grant 和插件 owner。
`plugin_id` 是请求选择的插件，但必须属于当前凭据的可信 owner：STS 使用签发记录验证的插件；API Key 使用该 Key 的显式 owner policy。
未绑定 owner 的 Key 不可以自行填写插件 ID 获得身份。不要清除 owner 来绕过；配置精确 owner 和上述服务 grant。

订阅的 `subscriber_id` 必须等于经验证的 `plugin_id`。当前合同是一租户、一插件一个稳定消费身份，可订阅多个 topic；不支持调用方任意创建订阅身份或冒充另一插件。
同一插件的多实例竞争领取，不是每实例独立广播。每个 receipt 还绑定实际领取凭据的稳定主体；同 owner 的另一把 Key 或另一种服务凭据不能确认它，须等待期限后用自己的新 receipt 继续。若业务需要独立订阅组，应先升级正式合同。

用户/Admin JWT 不能调用本服务平面。管理类 gRPC 也拒绝服务凭据，不能绕过上述权限自行授予 topic ACL；插件创建自身主题继续走现有正式 topic_bootstrap。旧 `com.corex.eventfabric.publish` 不再隐含订阅权，也不能代替上述新服务 grant。
无效凭据、授权依赖异常、跨租户、owner 不匹配均明确失败，不降级到 WSBus、Admin 或本地队列。

## 3. Framework 首选 Host 通道：固定 core_internal

沿用现有 Gateway 客户端及凭据提供器：

- 普通调用：`POST {api_prefix}/tenant/invocations`。
- 订阅流：`POST {api_prefix}/tenant/invocations/stream`，读取 SSE。
- `preferred_protocol=core_internal`。
- 正式绑定：`core://event-fabric/service`，method=`INVOKE`。
- Payload 只允许 `body`，body 按本合同 DTO 校验；不得传 method、endpoint、headers、URL 或 raw proxy。
- API Key 用现有 `Authorization: ApiKey <secret>`；STS 用 `Authorization: Bearer <token>`。不得发送 Admin Bearer。
- 使用配置的 API prefix，不写死生产域名。禁止跨源重定向及向其他来源泄露凭据。

发布请求示例：

```json
{
  "capability_id": "com.corex.eventfabric.service_publish",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "contract_version": "event_fabric.service.v1",
      "operation": "publish",
      "tenant_uuid": "<当前凭据租户UUID>",
      "plugin_id": "com.powerx.plugins.scrm",
      "event": {
        "event_id": "<outbox稳定事件ID>",
        "idempotency_key": "<outbox稳定幂等键>",
        "topic": "<当前租户正式topic全名>",
        "version": "v1",
        "payload": {"endpoint_uuid": "<业务对象UUID>"},
        "attributes": {"source": "messaging.outbox"}
      }
    }
  }
}
```

成功结果包含 `accepted=true`、原 `event_id`、Core `trace_id`。它只证明信封及投递接收者已经在同一事务提交，不证明消费者业务完成。
事件 ID 与幂等键各 1–128 字节，允许不同；重试必须保持两者及原 payload/version/topic/attributes 不变。
同一租户、发布插件和幂等键的不同内容返回 `EVENT_FABRIC_IDEMPOTENCY_CONFLICT`；不覆盖原事件。
首次发布按当时有效的订阅 ACL 创建投递；之后新增订阅 ACL 不会自动补发历史事件，历史回放另走明确授权流程。
保留字段 `principal_id`、`plugin_id`、`tenant_uuid`、`published_by`、`core_*` 不能放入 attributes。

## 4. 订阅及回执示例

订阅使用 consume 能力，发送到流接口：

```json
{
  "capability_id": "com.corex.eventfabric.service_consume",
  "preferred_protocol": "core_internal",
  "payload": {"body": {
    "contract_version": "event_fabric.service.v1",
    "operation": "subscribe",
    "tenant_uuid": "<凭据租户UUID>",
    "plugin_id": "com.powerx.plugins.ai",
    "subscriber_id": "com.powerx.plugins.ai",
    "topics": ["<获授权的topic全名>"],
    "batch_size": 10,
    "compatibility_mode": "strict",
    "supported_versions": ["v1"]
  }}
}
```

Core 发出 `ready`、`delivery`、`reconnect` 或 `error` 事件。`delivery` 字段见 OpenAPI 的 Message。
流最长 30 秒，之后 `reconnect` 提示以原 tenant/plugin/subscriber/topics 重连；EOF 也可按有上限退避重连。
`event_id` 用于业务去重；`delivery_id` 是本次尝试的回执 UUID，不是游标。
不要把 `delivery_id` 用作业务幂等键；每次超时或 NACK 后重投会换新 UUID，事件 ID 不变。

ACK/NACK 使用普通调用接口及 consume 能力：

```json
{
  "capability_id": "com.corex.eventfabric.service_consume",
  "preferred_protocol": "core_internal",
  "payload": {"body": {
    "contract_version": "event_fabric.service.v1",
    "operation": "ack",
    "tenant_uuid": "<凭据租户UUID>",
    "plugin_id": "com.powerx.plugins.ai",
    "subscriber_id": "com.powerx.plugins.ai",
    "delivery_id": "<当前delivery_id>"
  }}
}
```

NACK 成功返回 `outcome=retry_scheduled` 或 `dead_lettered`，以及 `remaining_attempts`、`next_delay_seconds`。确认 NACK 已登记不代表业务成功。
NACK 将 operation 改为 `nack`，可带短 reason（不放消息正文、凭据或用户隐私）。
ACK 成功后重复相同 ACK 幂等；已超时、已 NACK、已更换尝试的旧回执拒绝，不会确认下一次投递。

## 5. 可靠性、撤权和取消

Core 在事务中锁定投递，设置 ACK deadline；网络发送成功不会修改为 succeeded。
断线、取消、发送失败或 Core 重启后，未确认记录仍在数据库；期限届满后，同一订阅身份重新领取时生成新尝试，并绑定当次实际凭据主体。
消费者离线期间不要求维持连接；恢复在后续领取时执行。达到 topic 的 MaxRetry 后原子进入死信；不无限重投。
旧后台回放通道不能创建或覆盖 `efs1:` 服务信封；服务死信的人工重放不属于本合同，应另行走明确授权的服务回放合同，不能用 Admin 冒充原发布者。
NACK 持久化下一次尝试，默认 1 秒后可领取。当前合同不提供 renew；业务必须在 ACK deadline 内提交，或通过幂等机制处理超时重投。

每次操作、每次轮询领取及实际发送前重新校验 live grant/registration/topic ACL。撤权生效后不会再开始新的发送；已经写入网络或已交给业务的内容不能追回。
ACK/NACK 同样受实时授权限制。撤权后 ACK 被拒绝时，消费者必须保留已提交业务的幂等记录。
客户端关闭必须取消请求 context、关闭响应体及重连任务，不得遗留 goroutine 或继续消费旧身份。

## 6. Framework 类型与装配要求

增加独立的 Event Fabric 客户端，不把现有 TaskQueue、TaskBus/WSBus 发布器改名当作可靠事件客户端：

```go
type EventFabric interface {
    Publish(context.Context, PublishRequest) (PublishReceipt, error)
    Subscribe(context.Context, SubscribeRequest) (Subscription, error)
    Ack(context.Context, DeliveryReceipt) error
    Nack(context.Context, DeliveryReceipt, string) error
}
type Subscription interface {
    Recv(context.Context) (Delivery, error)
    Close() error
}
```

建议映射到现有 Framework `runtime/taskbus` 邻近的独立 eventfabric 包；最终包名由 Framework 维护者确定。
必须交付实现和运行装配，不能只有 interface、sample 或 mock：

1. DTO 包含 contract、tenant、plugin、subscriber、topic、payload、event ID、幂等键、delivery ID、attempt、deadline 和 trace。
2. Host 使用上面的 typed core_internal + SSE；运行配置在启动时确定通道，缺少依赖直接启动失败。
3. 保留现有 Gateway 的凭据刷新。仅连接恢复可以重连；明确的 401/403、error 授权码应停止并上报，不能无限重连或换通道。
4. SSE 解析必须处理多行 data、分块、错误帧、正常 reconnect 和非正常 EOF；校验 contract/tenant/subscriber/topic/receipt UUID/deadline，再交给业务。
5. 发布的 ID/幂等键来自 outbox，重试不能重新生成。请求超时可能已提交，要沿用原键重试。
6. 业务 handler 和去重登记应在业务事务中一起提交，提交后才 ACK。处理失败再 NACK。
7. ACK 请求超时/断线属于确认不确定：不要回滚已提交业务，不要重新生成事件或主动重复业务；保留 receipt 和业务去重状态，允许重投触发去重后再 ACK。
8. `EVENT_FABRIC_RECEIPT_STALE` 不可用旧 receipt 循环重试，等待新尝试；重复业务由 SCRM 幂等层阻止。
9. 同一订阅身份重连，不追加随机 instance ID。消费者并发必须有上限，与 batch_size、业务耗时和 ACK deadline 匹配。
10. 同步 Framework 合同、Host 实现、runtime 装配、共享样例、模板及发布版本；模板检查通过后提供可消费版本。

### 6.1 本次源码核对得到的具体调整点

| Framework 位置 | 必须调整 |
|---|---|
| `framework/backend/go/runtime/taskbus/provider.go` 的 `HostProvider.NewEmitter` | 当前构造 `wsbus.NewHostClient`，不能作为可靠事件通道。为业务 outbox 显式装配 Event Fabric 适配器；页面 WSBus 保留自己的入口。 |
| `framework/backend/go/event/models.go` | 当前 Event/Meta 没有稳定 event ID、幂等键或回执。增加 ReliablePublish/Delivery/Receipt 类型，明确承载这些字段；不能临时用 RequestID 或新 UUID 替代 outbox 事件 ID。 |
| `eventbridge`、runtime provider 装配及生命周期 | 实际注入可靠发布器和订阅器，启动时固定通道、启动消费循环，关闭时取消并等待退出。不能只新增可调用 client 后仍让 SCRM 使用原 WSBus emitter。 |
| Gateway 客户端及 stream 解码 | 使用正式能力 ID、固定 core_internal、现有凭据提供器；支持 ACK/NACK 独立请求、SSE error/reconnect/EOF、context 取消和同身份重连。 |
| shared examples、插件模板及依赖版本 | 展示 outbox 稳定 ID → 发布回执 → 业务事务 → ACK 的完整代码；发布可消费版本并让 SCRM 更新依赖。 |

SCRM 的 `DecodeMinimalEvent` 会严格核对 Framework Event.Meta 与内部版本化 payload，适配器必须完整保留原信封：

| Core 字段 | Framework / SCRM 映射 |
|---|---|
| `event.event_id` | outbox 中的 `event_uuid` |
| `event.idempotency_key` | outbox 中的 `dedupe_key`；SCRM 当前要求等于 event_uuid |
| `event.version` | 原 `Meta.PayloadVersion`；SCRM 当前值是字符串 `1`，不得自动变成 `v1` |
| `event.topic`、订阅 topics | 由正式 topic 创建/同步回执取得的物理 FullTopic；通过显式映射关联业务 topic，不随意补删前缀 |
| `event.payload` | 完整、原样序列化的 Framework Event：`{topic, meta, payload}`；消费时解包，不只转发最内层 payload |
| Delivery 的 `tenant_uuid` / `publisher_plugin_id` | 可信传输身份；解包后校验与原 Meta.TenantUUID / Meta.SourcePlugin 一致，再交给业务 |
| Delivery 的 `trace_id` | Core 传输 trace，单独记录；不得用它覆盖原 Meta.TraceID 或嵌入 payload 中的业务 trace |
| Delivery 的 `delivery_id` / `ack_deadline` | 传输回执及确认期限，保留在 DeliveryReceipt，不写入业务幂等键 |

Core 通用示例中的裸 payload 用于说明传输 DTO，并非 SCRM 的完整消息信封示例。
业务 `topic=powerx.messaging.message.received.v1`、Meta.RequestID、OccurredAt、PayloadVersion 及嵌入 payload 都应保持 outbox 原值。
Framework 不需要解析客服字段；它只负责保留信封、核对可信传输范围和管理回执。SCRM 继续做 MinimalEvent、endpoint_uuid、会话及业务权限校验。

## 7. gRPC 对齐

已生成的 Proto：`backend/api/grpc/gen/go/powerx/event_fabric/v1/event_fabric.pb.go`。

- PublishEvent 新增 `contract_version`、`plugin_id`。
- Subscribe 新增 `contract_version`、`plugin_id`，其余必须填写 tenant/subscriber/topics/batch/version 策略。
- AckDelivery/NackDelivery 新增 `tenant_uuid`、`plugin_id`、`contract_version`。
- DeliveryMessage 新增可信 tenant、contract、published_by、publisher_plugin_id。
- 包名和 RPC 名不变，但缺少应用层 contract 的旧请求明确失败，禁止无声沿用宽松旧语义。
- gRPC 使用 Core 签发并验证签发记录的 STS Bearer；Host API Key 使用 core_internal 通道，不直接把 Key 当作 gRPC Bearer。
- 流结束后按同一身份重建；不依赖 Proto 包名推导 registration 版本。

## 8. 错误处理

| 错误码 | HTTP / gRPC | Framework 行为 |
|---|---|---|
| EVENT_FABRIC_SERVICE_REQUIRED | 401 / Unauthenticated | 检查正式服务凭据，停止当前订阅 |
| EVENT_FABRIC_CONTRACT_REQUIRED、INVALID_ARGUMENT、RESERVED_ATTRIBUTE | 400 / InvalidArgument | 修 DTO，不重试业务 |
| EVENT_FABRIC_TENANT_FORBIDDEN、OWNER_FORBIDDEN、SUBSCRIBER_FORBIDDEN、TOPIC_FORBIDDEN、DELIVERY_FORBIDDEN、OPERATION_FORBIDDEN | 403 / PermissionDenied | 停止，不切换身份或通道 |
| EVENT_FABRIC_GRANT_REQUIRED | 401 或 403 | grant/registration/凭据已失效，停止并明确反馈 |
| EVENT_FABRIC_SCHEMA_NOT_READY | 503 / Unavailable | 执行正式迁移后重启 Core，不重试业务绕过 |
| EVENT_FABRIC_AUTHORIZATION_UNAVAILABLE、UNAVAILABLE | 503 / Unavailable | 保持不确定状态，可按有界退避恢复 |
| EVENT_FABRIC_IDEMPOTENCY_CONFLICT | 409 / FailedPrecondition | 同一键内容冲突，需人工/业务修正 |
| EVENT_FABRIC_RECEIPT_STALE | 409 / FailedPrecondition | 丢弃旧回执，等待重投，不重复业务 |
| EVENT_FABRIC_VERSION_INCOMPATIBLE | 409 / FailedPrecondition | 修版本合同，不能把 any 当自动降级 |

流建立后错误由 `event: error` 的 `data.code` 表达；HTTP 200 不能代替读取错误帧。

## 9. Core 部署与验收

不手工 SQL、不批量扩展共享 Profile、不自动赋予默认角色。正式部署步骤：

1. 完成受控测试、race、vet、编译及 `make capability-check`。
2. 先执行正式结构迁移。Mac dev/test 的限定命令为 `cd backend && go run ./cmd/database migrate -scope event-fabric-service-identity`（默认现有配置；必要时用 `-config` 指定私有配置）。已安装正式环境使用完整 upgrade plan/apply，不使用 dev/test scope。结构未就绪返回 `EVENT_FABRIC_SCHEMA_NOT_READY`，不在请求中自动补列。
3. 经环境所有者安排，使用 scoped `make capability-seed` 发布这两项能力；保留 installed state。核对每项的 published 状态、registration 版本、provider_version、catalog/protocol hash。
4. 为需要发布/消费的真实服务分别配置所需 grant 和插件 owner，并配置 topic ACL。查询真实 Key 的 grant-status；不能用 Admin JWT 代验。
5. 用户按日常方式重启正确 Core 进程。既有业务事件不自动重放；旧 gRPC 客户端必须同步升级合同。
6. Framework 发布可消费版本后，由 SCRM 连接现有 outbox 和消费事务，完成下列端到端矩阵。

| 场景 | 预期 |
|---|---|
| 已授权发布→订阅→业务提交→ACK | event ID/trace 连贯，业务执行一次，投递 succeeded |
| 仅发布 grant 尝试订阅，或仅消费 grant 尝试发布 | 拒绝 |
| 跨租户、假 plugin/subscriber、Admin Bearer | 拒绝且不泄露正文 |
| 在排队后撤销 grant、registration、topic ACL | 后续发送停止，明确错误 |
| 发送失败、取消、断线、Core 重启、ACK 丢失 | deadline 后新 receipt 重投，业务去重 |
| 同一 subscriber 两个消费者 | 不同时持有同一有效 receipt；旧回执不可确认新投递 |
| 同一幂等键重复相同/不同内容 | 分别幂等接收 / 409，不覆盖已存事件 |
| 达到最大尝试 | 单次死信，不继续无限投递 |

验收日志记录 tenant/plugin/subscriber/event/delivery/attempt/trace 及机器错误码，不记录 Key、ticket 或消息正文。

## 10. SCRM 边界与回滚

SCRM 继续复用已有 outbox、ApplyEventOnce 及业务授权入口，在业务消费者执行前校验 endpoint_uuid、入口与会话范围。Framework 不解释客服业务权限。
投递语义是至少一次；不能宣称端到端 exactly once。业务去重登记和业务事务的正确性仍由 SCRM 验收。

如联调失败，停止新消费者，保留 Core 投递记录、SCRM outbox 和去重记录。不要回退到 Admin、WSBus 或自动确认旧通道；不要清表或重新生成幂等键。
回滚代码前先停止新通道，保留 `efs1:` 记录待恢复；旧 worker 必须保留对这些记录的领取及 ACK/NACK 隔离保护。

## 11. 代码映射

- `backend/internal/service/event_fabric/service_channel/`：可信身份、typed DTO、授权、发布、领取、回执、SSE。
- `backend/pkg/corex/db/database/migration.go`、`backend/cmd/database/migrate.go`：集中迁移、dev/test 限定入口及结构预检。
- `backend/pkg/corex/db/persistence/model/event_fabric/delivery_attempt.go`：领取主体指纹，运行请求不自动改表。
- `backend/pkg/corex/db/persistence/repository/event_fabric/service_delivery_repository.go`：事务、行锁、持久化恢复。
- `backend/internal/transport/grpc/event_fabric/{publisher_server,subscriber_server,tenant_scope}.go`：正式 gRPC 入口和错误映射。
- `backend/internal/app/shared/deps.go`：Core invoker 运行装配。
- `backend/config/platform_capabilities/event_fabric.yaml`：正式发布/消费权限合同。
- `backend/internal/service/event_fabric/delivery/service.go`：旧 worker 与服务记录隔离。
- `docs/contracts/event-fabric-service-v1.openapi.yaml`：Framework DTO/schema。
