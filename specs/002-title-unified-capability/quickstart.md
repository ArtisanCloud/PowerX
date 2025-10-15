# 快速开始 — 统一能力契约与传输适配器

## 前置条件

- 已配置 PowerX CoreX 环境，数据库连接指向 Postgres。  
- 安装 `buf`、`protoc`、`go` 及 `make proto-gen proto-lint`.  
- 管理员 Token 拥有 `integration.capability.admin` Scope；若通过 Agent 代理，需申请对应 Tool Grant。

## 1. 生成并校验契约

1. 编辑 `specs/002-title-unified-capability/contracts/capability-http-openapi.yaml` 与 `capability-grpc.proto`，根据能力补充字段。  
2. 执行：

```bash
make proto-lint proto-gen
```

确保 gRPC 契约通过 lint 并生成至 `api/grpc/gen/go/powerx/capability/v1`.

3. 编写契约草稿 JSON：

```json
{
  "capability_key": "crm.lead.create",
  "version": "1.0.0",
  "provider_id": "com.powerx.plugin.crmplus",
  "display_name": "创建 CRM 线索",
  "security_scope": "crm.lead.write",
  "tool_grant_required": true,
  "io_schemas": [
    {"direction":"input","format":"json_schema","schema_uri":"s3://schemas/crm/lead_create_input.json"},
    {"direction":"output","format":"json_schema","schema_uri":"s3://schemas/crm/lead_create_output.json"}
  ],
  "transport_profiles": [
    {"transport":"grpc","mode":"prefer","timeout_ms":8000,"retry":{"max_attempts":2,"backoff_ms":200,"idempotent":false},"streaming":false},
    {"transport":"http","mode":"fallback","timeout_ms":12000,"retry":{"max_attempts":1,"backoff_ms":0,"idempotent":false},"streaming":false}
  ]
}
```

4. 调用 REST 草稿创建接口：

```bash
curl -X POST https://corex.powerx.dev/api/admin/capabilities \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d @contract.json
```

## 2. 发布契约

1. 确保 IAM 中存在 `crm.lead.write` Scope，并通过安全模块创建对应 Tool Grant（如需代理调用）。  
2. 触发发布：

```bash
curl -X POST "https://corex.powerx.dev/api/admin/capabilities/crm.lead.create/versions/1.0.0/publish" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"effective_at":"2025-10-15T00:00:00Z"}'
```

3. 发布成功后会自动向 EventBus 广播事件 `integration.capability.published`。

## 3. 查询与调用

### REST

```bash
curl -X GET "https://corex.powerx.dev/api/admin/capabilities/crm.lead.create/versions/1.0.0" \
  -H "Authorization: Bearer $TOKEN"
```

### gRPC

```bash
grpcurl -H "Authorization: Bearer $TOKEN" \
  -d '{"capability_key":"crm.lead.create","version":"1.0.0"}' \
  corex.powerx.dev:7443 powerx.capability.v1.CapabilityRegistryService/GetCapability
```

> 说明：服务端与客户端均复用现有 PowerX gRPC SDK，无需额外生成专用客户端代码，可直接通过 SDK 暴露的 `CapabilityRegistryServiceClient` 发起调用。

Router 将根据传输偏好优先选择 gRPC Adapter，失败时自动降级到 HTTP Adapter。

## 4. 版本治理

1. 更新版本策略：

```bash
curl -X PUT "https://corex.powerx.dev/api/admin/capabilities/crm.lead.create/version-policy" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"capability_key":"crm.lead.create","default_strategy":"latest_minor","allowed_versions":[{"version":"1.0.0","compatible_with":["1.0.*"],"status":"active"}]}'
```

2. 发布补丁版本 `1.1.0` 后，设置兼容关系并通知调用方；旧版本可通过 `/deprecate` 接口设置废弃时间与替代建议。

## 5. 观测与故障处理

- Adapter 将输出 `transport.<protocol>.<capability>` Tracing Span，可在 Jaeger/Tempo 查看调用链。  
- Metrics `integration_capability_latency_ms`、`integration_capability_error_total` 可在 Prometheus 中按协议、租户、错误码聚合。  
- 当调用失败并匹配 `ErrorTaxonomy` 严重级别 ≥ ERROR 时，会写入审计事件 `integration.capability.invocation`，可在安全审计面板检索。
