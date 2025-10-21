## Quickstart: Integration Gateway & MCP Server

### 前置条件
- 已完成 Capability Registry、Router、Tool Grants 与 EventBus 部署（参考相关特性 Quickstart）。
- Postgres/Redis 配置在 `config/config.yaml` 中补充 `integration_gateway` 节点：
  ```yaml
  integration_gateway:
    rate_limit_prefix: "integration_gateway:rl"
    event_topics:
      created: "integration.gateway.route.created"
      updated: "integration.gateway.route.updated"
      invocation_succeeded: "integration.gateway.invocation.succeeded"
      invocation_failed: "integration.gateway.invocation.failed"
  ```
- 管理员具备对应 RBAC 权限（`integration_gateway.admin.*`），租户调用方具备 Tool Grant。
- MCP Server 已启用（`make dev` 默认会启动），确保 `internal/server/mcp` 配置加载成功。

### 步骤 1：启动服务并确认健康
1. 运行 `make dev` 或相应二进制，确认 HTTP Admin、Tenant API 以及 MCP Server 均启动。
2. 调用 `GET /api/health`，响应示例：
   ```json
   {"code":0,"message":"ok","data":{"services":["integration_gateway"]},"trace_id":"trc-123"}
   ```
3. gRPC 使用 `grpcurl -authority powerx localhost:8443 list powerx.integration.gateway.v1.IntegrationGatewayAdminService` 验证服务注册。

### 步骤 2：创建集成入口（管理员）
1. 管理端调用：
   ```bash
   curl -X POST "$POWERX_BASE_URL/admin/integration/routes" \
     -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "tenant_id":"tenant-001",
       "route_slug":"crm-sync",
       "capability_id":"capabilities.crm.sync.v1",
       "tool_grant_ids":["grant-crm-sync"],
       "channels":["http","mcp"],
       "rate_limit":{"limit":120,"burst":120,"window_seconds":60},
       "event_topics":{
         "invocation_succeeded":"integration.gateway.invocation.succeeded",
         "invocation_failed":"integration.gateway.invocation.failed"
       }
     }'
   ```
2. 响应体包含 `route_id`、`current_version` 与 `trace_id`，后续更新需携带 `If-Match: W/"<version>"` Header。
3. 若未提供 `rate_limit` 字段，系统会使用默认策略：每分钟基准速率 + 2 倍突发。

### 步骤 3：租户调用统一 API
1. 发起调用：
   ```bash
   curl -X POST "$POWERX_BASE_URL/tenant/integration/routes/crm-sync:invoke" \
     -H "Authorization: Bearer $TENANT_TOKEN" \
     -H "X-PowerX-Tenant: tenant-001" \
     -H "Content-Type: application/json" \
     -d '{"payload":{"customer_id":"C123","operation":"sync"}}'
   ```
2. 成功响应：
   ```json
   {"code":0,"message":"ok","data":{"result":"queued"},"trace_id":"trc-456"}
   ```
3. 超限时返回：
   ```json
   {
     "code":42901,
     "message":"rate limit exceeded",
     "data":{"retry_after":"15s","quota_scope":"tenant"},
     "trace_id":"trc-789"
   }
   ```
4. 可在事件总线订阅 `integration.gateway.invocation.*` 主题获取执行结果。

### 步骤 4：使用 MCP 工具
1. MCP 客户端握手后执行 `mcp call integration.route.list`，可选参数 `tenant_id`。
2. 返回的工具 schema 包含可调用的 `route_slug`、`capability_id`、输入/输出说明。
3. 调用工具：
   ```json
   {
     "tool":"integration.route.invoke",
     "arguments":{
       "tenant_id":"tenant-001",
       "route_slug":"crm-sync",
       "payload":{"customer_id":"C123","operation":"sync"}
     }
   }
   ```
4. 调用失败会在结果中附带 `trace_id` 与错误码，同时事件总线上发布 `invocation_failed`。

### 步骤 5：监控与排障
- Prometheus 指标路径：`/metrics` 中新增 `integration_gateway_invocations_total`, `integration_gateway_rate_limit_hits_total` 等。
- 追踪：在日志中检索 `trace_id`（通过 HTTP header `X-Trace-Id` 或响应取得）。
- 补偿：如事件发布失败，可调用 `POST /admin/integration/routes/{route_id}/events:replay` 触发重试（后续阶段实现）。
