# Event Fabric 能力调试指南

PowerX Event Fabric 对外暴露一条平台能力 `com.corex.eventfabric.publish`，允许插件或宿主通过统一 gRPC 接口发布事件、订阅回执并触发回放。该能力归属模块 `event_fabric`，Tool Scope 为 `event.fabric`。

| Capability ID | Intent | Prefer/Fallback | Channels |
| --- | --- | --- | --- |
| `com.corex.eventfabric.publish` | `event.fabric.publish` | Prefer `grpc` | gRPC `powerx.event_fabric.v1.EventDeliveryService/PublishEvent`、`EventSubscriberService/Subscribe` |

> **认证要求**
>
> - `Authorization: Bearer <TENANT_TOKEN>`
> - `x-tenant-uuid: <TENANT_UUID>`
> - 租户需开启 Event Fabric Tool Grant。

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
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "x-tenant-uuid: $TENANT_UUID" \
  -d '{
    "channel": "orders.created",
    "payload": {
      "type_url": "type.googleapis.com/google.protobuf.Struct",
      "value": "{\"orderId\":\"ord_123\",\"amount\":99.9}"
    },
    "attributes": {
      "tenant": "'$TENANT_UUID'",
      "source": "plugin.demo"
    }
  }' \
  $GRPC_ADDR powerx.event_fabric.v1.EventDeliveryService/PublishEvent
```

响应中包含 `event_id` 与 `trace_id`，可用于后续追踪。

### 订阅事件回执

`Subscribe` 为双向流式调用，可用 `grpcurl -d` 发送初始订阅指令，并保持连接：

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "x-tenant-uuid: $TENANT_UUID" \
  -d '{
    "channel": "orders.created",
    "group": "demo-plugin",
    "cursor": "LATEST"
  }' \
  $GRPC_ADDR powerx.event_fabric.v1.EventSubscriberService/Subscribe
```

> 提示：开发阶段通常在另一个终端运行 `PublishEvent`，即可观察 Subscribe 流里实时输出的事件。生产场景应使用 SDK 或长连容器来维护订阅。

## 通过 `/tenant/invocations` 触发

若需要通过统一 Selector 访问该能力，可以向 `/api/v1/tenant/invocations` 发送调用请求：

```bash
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
        "channel": "orders.created",
        "payload": {
          "type_url": "type.googleapis.com/google.protobuf.Struct",
          "value": "{\"orderId\":\"ord_123\"}"
        }
      }
    }
  }'
```

- `payload.endpoint`：可选，若缺省则由 Registry 协议元数据自动填写；显式传入可覆盖默认 Service。
- `payload.body`：与 gRPC JSON 映射一致，Gateway 会负责将其转换为 protobuf 请求。
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
