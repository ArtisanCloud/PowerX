# Event Fabric 能力调试指南

## 场景概述

### 目标

- 让租户 `TENANT_UUID` 通过对外能力 `com.corex.eventfabric.publish` 将订单创建等业务事件推送到 Event Fabric，并由订阅者实时消费。
- 验证成功后应看到：
  1. Publish 请求（gRPC 或 `/tenant/invocations`）返回 200 / OK；
  2. 订阅端收到同一条事件（含 `trace_id`、attributes）；
  3. 管控侧（Topic、ACL、审计表）均记录此次操作。

### 为什么要先建 Topic / ACL

1. Event Fabric 采用“Topic = `<tenant>.<namespace>.<name>`”的多租户隔离模型，没有对应 Topic 就无法发布，第一段必须与 `tenant_uuid` 完全一致，否则会报 `tenant mismatch`。
2. ACL 控制 publish/subscribe 权限，缺少授权会返回 `event_fabric: unauthorized`。本文档中的 HTTP Admin 示例就是为了把这些基础设施准备好，再去调用外部能力。

### 流程产出

```
a) 创建 topic TENANT.orders.created，并授予 principal=plugin.demo 的 publish 权限；
b) 启动订阅端（CLI/grpcurl）监听 TENANT.orders.created；
c) 通过 gRPC Publish 或 /tenant/invocations 发送 payload；
d) 订阅端收到事件，同时审计/事件目录可查到记录。
```

这样就验证了整个链路（租户 → 能力 → Event Fabric → 订阅者），后续可作为自动化测试或回归 checklist 的基线。

PowerX Event Fabric 对外暴露一条平台能力 `com.corex.eventfabric.publish`，允许插件或宿主通过统一 gRPC 接口发布事件、订阅回执并触发回放。该能力归属模块 `event_fabric`，Tool Scope 为 `event.fabric`。

| Capability ID | Intent | Prefer/Fallback | Channels |
| --- | --- | --- | --- |
| `com.corex.eventfabric.publish` | `event.fabric.publish` | Prefer `grpc` | gRPC `powerx.event_fabric.v1.EventDeliveryService/PublishEvent`、`EventSubscriberService/Subscribe` |

> **认证要求**
>
> - `Authorization: Bearer <TENANT_TOKEN>`
> - `x-tenant-uuid: <TENANT_UUID>`
> - 租户需开启 Event Fabric Tool Grant。

## Manifest 自动播种

在插件安装 / 升级期间，CoreX 会自动查找并应用插件包内的 `event_fabric.yaml` manifest 来创建 Topic 与 ACL，无需人工调用 Admin API。CLI 及安装 Hook 会按以下顺序寻找文件：

1. `<plugin_root>/config/event_fabric.yaml`
2. `<plugin_root>/platform_capabilities/event_fabric.yaml`
3. `<plugin_root>/event_fabric.yaml`
4. `Plugin.Paths.ConfigDir/event_fabric.yaml`

Manifest 支持使用 `tenant_uuid`、`plugin_id`、`plugin_version` 以及 `variables.*`（来自插件安装 metadata，如 `scope`、`namespace`、`environment` 等）进行模板替换。示例：

```yaml
version: 1
topics:
  - key: orders.created
    namespace: orders
    name: created
    metadata:
      origin: platform
    acl:
      - principal_type: service
        principal_id: "{{ plugin_id }}-writer"
        actions: [publish]
      - principal_type: service
        principal_id: "svc-{{ tenant_uuid }}-consumer"
        actions: [subscribe, replay]
  - namespace: ops
    name: alert
    acl:
      - principal_type: service
        principal_id: "ops-{{ variables.environment | default \"dev\" }}"
        actions: [publish]
```

> 支持的模板变量：
> - `{{ tenant_uuid }}`：当前租户（小写 UUID）
> - `{{ plugin_id }}` / `{{ plugin_version }}`
> - `{{ variables.<key> }}`：插件安装时写入的任意 metadata（例如 `cluster`、`environment`）

### 自动播种何时触发

- **插件启用/升级**：安装 orchestrator 会在写入 Capability Registry 后调用 `event_fabric.SeedService.ApplyManifest`，若 Topic/ACL 已存在则根据 binding 记录跳过重复授权。
- **Root 复查 / CI**：`go run ./cmd/event_fabric_seed --tenant <uuid> --plugin <id> --dry-run` 可预览计划；不带 `--dry-run` 会真正写入。`scripts/capability_registry/verify.sh` 也会自动执行一次 `event_fabric_seed --dry-run`，确保 manifest 语法与模板变量在 CI 阶段即被验证，可通过 `--skip-event-seed` 跳过。

如果 manifest 缺失，对应插件只会跳过 Topic/ACL 步骤，其余能力验证仍可继续；建议在插件仓库中新增上述文件，并随功能更新保持同步。

## Topic / ACL 准备

在调试能力之前，请确保已经为当前租户创建 Topic，并授予发布/订阅主体。否则会收到 `tenant mismatch` 或 `topic ... not found` 的 502 错误。

```bash
export TENANT_UUID="<tenant-uuid>"
export ADMIN_TOKEN="<admin-jwt>"

# 1) 创建 Topic（Topic 全名 = <tenant_uuid>.<namespace>.<name>）
curl -sS -X POST http://127.0.0.1:8077/api/v1/admin/event-fabric/topics \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_uuid": "'"$TENANT_UUID"'",
    "namespace": "orders",
    "name": "created",
    "payload_format": "json",
    "versioning_mode": "backward",
    "max_retry": 5,
    "ack_timeout_seconds": 30
  }'

# 2) 授权主体（可根据需要添加 publish/subscribe 多个 principal）
curl -sS -X POST http://127.0.0.1:8077/api/v1/admin/event-fabric/acl \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_uuid": "'"$TENANT_UUID"'",
    "topic_full_name": "'"$TENANT_UUID"'.orders.created",
    "grants": [
      {"principal_type":"service","principal_id":"plugin.demo","action":"publish"},
      {"principal_type":"service","principal_id":"svc-demo-consumer","action":"subscribe"}
    ]
  }'
```

> 注意：Topic 名称的第一段必须与 `tenant_uuid` 完全一致；上面的示例会生成 `aeffc79f-...orders.created` 这样的全名，后续示例也会引用该命名。

## gRPC 合同

- Proto：`backend/api/grpc/contracts/powerx/event_fabric/v1/event_fabric.proto`
- 默认地址：`127.0.0.1:9001`

```bash
export GRPC_ADDR="127.0.0.1:9001"
export TENANT_TOKEN="<tenant-jwt>"
export TENANT_UUID="<tenant-uuid>"
```

### 发布事件

```bash
PAYLOAD=$(printf '{"orderId":"ord_123","amount":99.9}' | base64)

grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "x-tenant-uuid: $TENANT_UUID" \
  -d '{
    "tenant_uuid": "'$TENANT_UUID'",
    "topic": "'$TENANT_UUID'.orders.created",
    "event_id": "evt-demo-001",
    "trace_id": "trace-demo",
    "version": "v1",
    "payload_format": "json",
    "payload": "'$PAYLOAD'",
    "attributes": {
      "principal_id": "plugin.demo",
      "source": "plugin.demo"
    }
  }' \
  $GRPC_ADDR powerx.event_fabric.v1.EventDeliveryService/PublishEvent
```

`payload` 字段是 bytes，必须填写 **Base64** 字符串；`payload_format` 可用来标记业务格式。响应中包含 `event_id` 与 `trace_id`，可用于后续追踪。

### 订阅事件回执

`Subscribe` 为双向流式调用，可用 `grpcurl -d` 发送初始订阅指令，并保持连接：

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "x-tenant-uuid: $TENANT_UUID" \
  -d '{
    "tenant_uuid": "'$TENANT_UUID'",
    "subscriber_id": "demo-plugin",
    "topics": ["'$TENANT_UUID'.orders.created"],
    "batch_size": 10,
    "compatibility_mode": "VERSION_COMPATIBILITY_MODE_BACKWARD",
    "supported_versions": ["v1"]
  }' \
  $GRPC_ADDR powerx.event_fabric.v1.EventSubscriberService/Subscribe
```

> 提示：开发阶段通常在另一个终端运行 `PublishEvent`，即可观察 Subscribe 流里实时输出的事件。生产场景应使用 SDK 或长连容器来维护订阅。

## 通过 `/tenant/invocations` 触发

若需要通过统一 Selector 访问该能力，可以向 `/api/v1/tenant/invocations` 发送调用请求：

```bash
PAYLOAD=$(printf '{"orderId":"ord_123","amount":99.9}' | base64)

curl -sS -X POST "http://127.0.0.1:8077/api/v1/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "X-Tenant-UUID: $TENANT_UUID" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.corex.eventfabric.publish",
    "preferred_protocol": "grpc",
    "payload": {
      "endpoint": "powerx.event_fabric.v1.EventDeliveryService",
      "rpc": "PublishEvent",
      "body": {
        "tenant_uuid": "'$TENANT_UUID'",
        "topic": "{{ _['x-tenant-uuid'] }}.orders.created",
        "event_id": "evt-demo-001",
        "trace_id": "trace-demo",
        "version": "v1",
        "payload_format": "json",
        "payload": "'$PAYLOAD'",
        "attributes": {
          "principal_id": "plugin.demo",
          "source": "plugin.demo"
        }
      }
    }
  }'
```

- `payload.endpoint`：可选，若缺省则由 Registry 协议元数据自动填写；显式传入可覆盖默认 Service。
- `payload.body`：字段需与 `PublishEventRequest` 完全一致，其中 `payload` 为 **Base64** 字符串。
- 若切换为 REST 通道，只需把 `preferred_protocol` 改为 `rest`，并在 payload 中提供 `method` + `endpoint` + `headers/body`（具体路由参见文档）。

响应示例（只截取 Data 部分）：

```json
{
  "data": {
    "payload": {
      "event_id": "evt_123",
      "trace_id": "7ecb9a7c-1b9e-4e88-a4e9-5a7a31f7050a"
    },
    "trace_id": "fd7fb6ce-d0e9-4367-9b3f-0c73c0b71626",
    "protocol_used": "grpc",
    "fallback_used": false
  }
}
```

`data.payload` 中的结构与 gRPC `PublishEvent` 响应一模一样，因此插件只要解析这一层即可获取业务数据；`data.trace_id` 则用于继续串联日志/观测。

## Topic / ACL 自动播种 CLI

当需要为多个租户重新应用插件自带的 `event_fabric.yaml`（例如上线后补齐 Topic/ACL）时，可使用内置 CLI：

```bash
cd backend
# 预览：仅渲染 manifest，不真正写入
go run ./cmd/event_fabric_seed --tenant aeffc79f-e72a-4fd9-b908-5c150bce3741 --plugin plugin.demo --dry-run

# 实际播种：遍历租户-插件绑定，逐个创建/授权
go run ./cmd/event_fabric_seed \
  --tenant aeffc79f-e72a-4fd9-b908-5c150bce3741 \
  --plugin plugin.demo
```

- `--tenant` 与 `--plugin` 支持重复指定；若省略则遍历所有已启用插件。
- CLI 会读取插件安装目录下的 `config/event_fabric.yaml`（或 `platform_capabilities/event_fabric.yaml`），并复用安装阶段同样的模板变量（tenant_uuid、plugin_id 等）。
- `--manifest` 可覆盖默认路径（需要配合单一 `--plugin` 使用），便于直接播种自定义 manifest。
- 非 `--dry-run` 模式下，CLI 会输出每个 Topic 的创建/ACL 数量，失败会返回非 0 exit code，方便 CI 脚本串联。
