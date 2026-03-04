# Knowledge Space 能力调试指南

PowerX 知识空间模块对外暴露能力 `com.corex.knowledge.space`，用于配置知识库、触发增量任务。该能力属于模块 `knowledge_space`，Tool Scope `knowledge.space`。

| Capability ID | Intent | Prefer/Fallback | Channels |
| --- | --- | --- | --- |
| `com.corex.knowledge.space` | `knowledge.space.manage` | Prefer `grpc` | gRPC `powerx.knowledge.v1.KnowledgeSpaceAdminService`（Create/Update/TriggerIngestion） |

> **认证要求**
>
> - `Authorization: Bearer <TENANT_TOKEN>`
> - `tenant_uuid: <TENANT_UUID>`
> - 租户需被授权 `knowledge.space` Tool Grant。

## gRPC 接口

- Proto：`backend/api/grpc/contracts/powerx/knowledge/v1/knowledge_space.proto`
- 服务：`powerx.knowledge.v1.KnowledgeSpaceAdminService`
- 端口：`127.0.0.1:9001`

```bash
export GRPC_ADDR="127.0.0.1:9001"
export TENANT_TOKEN="<tenant-jwt>"
export TENANT_UUID="<tenant-uuid>"
```

### 创建知识空间

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "tenant_uuid: $TENANT_UUID" \
  -d '{
    "space_id": "ks.demo",
    "display_name": "Demo Knowledge Space",
    "max_document_count": 2000,
    "embedding_model": "text-embedding-3-large"
  }' \
  $GRPC_ADDR powerx.knowledge.v1.KnowledgeSpaceAdminService/CreateKnowledgeSpace
```

### 调整策略

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "tenant_uuid: $TENANT_UUID" \
  -d '{
    "space_id": "ks.demo",
    "retention_days": 30,
    "max_document_count": 5000
  }' \
  $GRPC_ADDR powerx.knowledge.v1.KnowledgeSpaceAdminService/UpdateKnowledgeSpace
```

### 触发增量任务

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $TENANT_TOKEN" \
  -H "tenant_uuid: $TENANT_UUID" \
  -d '{ "space_id": "ks.demo", "job_type": "INGEST_FROM_DATASET" }' \
  $GRPC_ADDR powerx.knowledge.v1.KnowledgeSpaceAdminService/TriggerIngestion
```

## 通过平台能力入口查询

使用 `/admin/platform-capabilities/knowledge_space` 可随时确认该能力的协议矩阵、最新哈希以及参考文档链接。若需要让插件调用，可在 Admin > 租户 > 能力 开启 `knowledge.space` 并将 tenant token 配置到插件运行时。
