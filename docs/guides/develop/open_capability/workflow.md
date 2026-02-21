# Workflow & Scheduler 能力调试指南

Workflow 模块包含两类底座能力：

| Capability ID | Intent | 描述 | Prefer/Fallback | 主要 Channel |
| --- | --- | --- | --- | --- |
| `com.corex.scheduler.jobs` (`workflow.scheduler.invoke`) | 通过 Scheduler 触发/控制实例 | Prefer `grpc` | gRPC `WorkflowService/StartInstance`、`ControlInstance`、`ListInstances` |
| `com.corex.workflow.builder` (`workflow.builder.manage`) | Workflow Builder 模板的创建/发布 | Prefer `grpc` | gRPC `WorkflowService/CreateDefinition`、`PublishDefinition`、`ListDefinitions` |

> **认证要求**：`Authorization: Bearer <TENANT_TOKEN>`、`x-tenant-uuid: <TENANT_UUID>`，并为目标租户授予 `workflow.scheduler` 和/或 `workflow.builder` Tool Grant。

## gRPC 接口

- Proto：`backend/api/grpc/contracts/powerx/workflow/v1/workflow.proto`
- 服务端：`powerx.workflow.v1.WorkflowService`
- 地址：`127.0.0.1:9001`（按 `grpc.port` 配置）

```bash
export GRPC_ADDR="127.0.0.1:9001"
export TENANT_TOKEN="<tenant-jwt>"
export TENANT_UUID="<tenant-uuid>"
```

### 启动 Workflow 实例（Scheduler 能力）

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "x-tenant-uuid: $TENANT_UUID" \
  -d '{
    "definition_id": "wf.definitions.demo",
    "input": {
      "type_url": "type.googleapis.com/google.protobuf.Struct",
      "value": "{\"customerId\":\"cus_789\",\"total\":199}"
    }
  }' \
  $GRPC_ADDR powerx.workflow.v1.WorkflowService/StartInstance
```

控制实例（暂停/恢复/取消）：

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "x-tenant-uuid: $TENANT_UUID" \
  -d '{ "instance_id": "wf-inst-123", "action": "PAUSE" }' \
  $GRPC_ADDR powerx.workflow.v1.WorkflowService/ControlInstance
```

### 管理模板（Builder 能力）

创建定义：

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "x-tenant-uuid: $TENANT_UUID" \
  -d '{
    "definition_id": "wf.definitions.demo",
    "display_name": "Demo Flow",
    "graph": {
      "type_url": "type.googleapis.com/google.protobuf.Struct",
      "value": "{\"nodes\":[],\"edges\":[]}"
    }
  }' \
  $GRPC_ADDR powerx.workflow.v1.WorkflowService/CreateDefinition
```

发布定义：

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "x-tenant-uuid: $TENANT_UUID" \
  -d '{ "definition_id": "wf.definitions.demo", "version_note": "v1.0" }' \
  $GRPC_ADDR powerx.workflow.v1.WorkflowService/PublishDefinition
```

## 通过 Selector 调度

如果希望由 Integration Gateway 统一路由（选择 gRPC 或其他协议），可使用 `/api/v1/tenant/invocations`：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "capabilityId": "com.corex.scheduler.jobs",
    "protocol": "grpc",
    "payload": {
      "rpc": "StartInstance",
      "body": {
        "definition_id": "wf.definitions.demo",
        "input": {
          "customerId": "cus_789",
          "total": 199
        }
      }
    }
  }'
```

Selector 会读取 Registry 中的 `policy.prefer` 与 `protocols`，必要时自动处理 fallback（例如工作流服务 gRPC 不可用时走预设的 REST/MCP 通道）。
