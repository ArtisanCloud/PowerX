# 集成网关运行手册

## 1. 功能概览
- 统一面向租户的能力入口，支持 HTTP、gRPC 与 MCP 三种通道复用同一配置。
- 管理端负责路由生命周期（创建/更新/暂停/退役）、版本快照与事件发布。
- 租户端调用路由时自动执行 Tool Grant 校验、Redis 限流、事件与审计记录。
- MCP 工具暴露 `integration.route.list` / `integration.route.invoke`，方便智能体快速枚举与触发能力。

## 2. 快速操作
### 2.1 管理员创建路由
```bash
curl -X POST "$ADMIN_BASE/api/v1/admin/integration/routes" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id":"tenant-001",
    "route_slug":"crm-sync",
    "capability_id":"cap.crm.sync.v1",
    "tool_grant_ids":["grant-crm-sync"],
    "channels":["http","mcp"],
    "rate_limit":{"limit":120,"burst":120,"window_seconds":60},
    "event_topics":{
      "invocation_succeeded":"integration.gateway.invocation.succeeded",
      "invocation_failed":"integration.gateway.invocation.failed"
    }
  }'
```
- 成功后响应中包含 `route_id`、`current_version` 与 `trace_id`，后续更新需携带 `If-Match`。

### 2.2 租户调用 HTTP
```bash
curl -X POST "$OPEN_BASE/api/v1/tenant/integration/routes/crm-sync/invoke" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "X-PowerX-Tenant: tenant-001" \
  -H "Content-Type: application/json" \
  -d '{"payload":{"customer_id":"C123","operation":"sync"}}'
```
- 超限会返回 `429`，body 中包含 `retry_after`、`quota_scope`，trace id 保持一致。

### 2.3 MCP 工具调用
列举路由：
```json
{
  "tool": "integration.route.list",
  "arguments": { "tenant_id": "tenant-001" }
}
```
执行调用：
```json
{
  "tool": "integration.route.invoke",
  "arguments": {
    "tenant_id": "tenant-001",
    "route_slug": "crm-sync",
    "payload": {"customer_id": "C123"}
  }
}
```
- 结果 JSON 会返回 `status`、`trace_id`、`routed_capability_id` 等字段。

## 3. 监控与告警
| 关注项 | 指标 / 日志 | 阈值建议 | 处置 |
|--------|-------------|----------|------|
| 调用量 & 成功率 | `integration_gateway_invocations_total` 按 channel 分组 | p95 > 200ms 或错误率 > 1% | 检查下游能力可用性、Redis/DB 状态 |
| 限流命中 | `integration_gateway_rate_limit_hits_total` | 突增或单租户持续 > 0.1 QPS | 审核路由限流配置，必要时调大配额或通知客户 |
| 事件补偿 | EventBus `integration.gateway.*` Pending 条数 / `integration_event_publications{status="failed"}` | 连续 5 分钟 > 0 | 查看发布日志，排查下游订阅失败原因 |
| MCP 工具可用性 | MCP 日志 `integration_mcp_tool_error_total{tool="integration.route.invoke"}` | 任意 | 检查路由状态、Tool Grant、下游能力 |

PromQL 示例：
```promql
sum(rate(integration_gateway_invocations_total{channel="mcp"}[5m]))
sum(rate(integration_gateway_rate_limit_hits_total[5m])) by (tenant_id)
integration_event_publications{status!="sent"}
```

## 4. 运行脚本
`scripts/integration_gateway/verify_flow.sh` 提供端到端验证流程（管理员创建路由 → 租户调用 → MCP 调用）。执行前请设置：
- `ADMIN_BASE`、`OPEN_BASE`、`ADMIN_TOKEN`、`TENANT_TOKEN`
- `TENANT_ID`、`ROUTE_SLUG`（默认 `demo-sync`）

脚本会输出每一步的 trace id 及关键响应，方便快速排查。

## 5. 常见故障排查
1. **调用返回 403 / Tool Grant Denied**  
   - 通过管理端查询路由当前 `tool_grant_ids`；  
   - 核对 Event Fabric Grant 是否处于 active；必要时刷新缓存。
2. **限流误触发**  
   - 检查 Redis 中 `integration_gateway:rl:*` 计数；  
   - 确认 route 的 `rate_limit` 是否落盘成功（管理员 GET 接口）；  
   - 若为突发流量，可临时提升 `burst`。
3. **事件未到达订阅方**  
   - 查询 `integration_event_publications` 表 `status` 是否为 `failed`；  
   - 查看 EventBus 订阅服务日志；  
   - 手动重放（后续补偿队列上线前可人工触发）。
4. **MCP 客户端报错**  
   - 核对工具输入是否包含 `tenant_id`、`route_slug`；  
   - 观察返回 JSON 中 `trace_id`，在租户调用日志或事件中继续串联。

## 6. 术语速记
- **Route**：租户对外暴露的能力入口，由 slug 唯一标识。
- **Channel**：路由可用通道，当前支持 `http` / `mcp`。
- **Event Topics**：`integration.gateway.*` 事件统一在 EventBus 广播。
- **Trace ID**：全链路追踪字段，HTTP Header `X-Trace-Id` 可自定义，也可由系统生成。

> 所有示例默认管理端前缀 `/api/v1/admin`，租户前缀 `/api/v1/tenant`，可根据实际部署域名调整。

