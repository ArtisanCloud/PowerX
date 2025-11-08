# Quickstart: Agent Lifecycle & Observability

**目标**：在本地环境快速验证代理注册、生命周期控制与健康观测链路。

## 1. 启动依赖

```bash
make deps-up             # 启动 Postgres、Redis、EventBus（nats/kafka）等依赖
make dev                 # 运行 CoreX 服务（默认端口 8077）
```

> 确保 `.env` 中配置企业 IM Webhook（`AGENT_IM_WEBHOOK`）以及默认租户/管理员凭证。

## 2. 注册并激活代理

```bash
curl -X POST http://localhost:8077/admin/agents \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "alias": "content-writer",
    "tenantId": "tenant-001",
    "toolGrants": [{"name": "summarize", "version": "v1"}],
    "telemetryContractVersion": "otel-agent-v1",
    "defaultCapacityInstances": 3,
    "notificationChannel": "https://im.example/hooks/agent"
  }'

curl -X POST http://localhost:8077/admin/agents/{agentId}/activate \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reason":"initial rollout","traceId":"trace-123"}'
```

成功后可在数据库 `agent_profiles` 表中看到 `status=active`，EventBus 会发布 `agent.lifecycle.activate` 事件。

## 3. 生命周期控制

```bash
# 暂停
curl -X POST http://localhost:8077/openapi/agents/{agentId}/pause \
  -H "Authorization: Bearer $OPS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reason":"scheduled maintenance"}'

# 扩容到 5 个实例
curl -X POST http://localhost:8077/openapi/agents/{agentId}/scale \
  -H "Authorization: Bearer $OPS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"targetCapacityInstances":5,"reason":"q4 peak"}'
```

验证扩缩容：查看 `agent_lifecycle_events` 日志，确认 `scale_up` 事件写入并附带 `requested_capacity=5`。

## 4. 健康观测

```bash
curl -X GET http://localhost:8077/openapi/agents/{agentId}/health/summary \
  -H "Authorization: Bearer $SRE_TOKEN"
```

示例响应包含 `healthScore`、`status`、推荐动作。要模拟异常，可在本地触发高错误率或延时，并检查企业 IM 群消息是否 30 秒内收到“退化”告警。

> ⚠️ OpenAPI 路由位于 `/api/openapi/agents/...`，与管理员路由 `/api/admin/agent/lifecycle/...` 相互独立，便于面向 SRE/运维暴露只读健康视图。

## 5. 配置可观测性订阅

管理员可按租户/代理维度配置健康告警过滤条件（守护指标 + 关注状态），保存后自动写入代理 Metadata 并推送到缓存：

```bash
curl -X PUT http://localhost:8077/api/admin/agent/lifecycle/agents/{agentId}/subscription \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "tenant_id": "tenant-001",
        "metrics_filter": ["error_rate", "p95_latency_ms"],
        "health_statuses": ["degraded", "unavailable"],
        "requested_by": "sre.oncall"
      }'

curl -X GET http://localhost:8077/api/admin/agent/lifecycle/agents/{agentId}/subscription \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

同样可以通过 gRPC RPC `UpdateSubscription` / `GetSubscription` 完成配置，配置项会被自动校验（非法状态会返回 `ErrInvalidSubscription`）。

```bash
grpcurl -plaintext -d '{
  "agentId":"{agentId}",
  "tenantId":"tenant-001",
  "config":{
    "metricsFilter":["error_rate","p95_latency_ms"],
    "healthStatuses":["degraded","unavailable"]
  }
}' localhost:9090 powerx.agent.v1.AgentLifecycleService.UpdateSubscription

grpcurl -plaintext -d '{"agentId":"{agentId}"}' \
  localhost:9090 powerx.agent.v1.AgentLifecycleService.GetSubscription
```

## 6. 触发健康退化并验证告警

若无法在本地引入真实遥测，可借助单元/合同测试快速验证链路：

```bash
# 运行健康相关单元测试，模拟事件发布 + 告警节流
go test ./tests/unit/agent_lifecycle -run TestRecordHealthSnapshotPublishesEventAndThrottlesAlerts -v

# 运行 HTTP/GRPC 合同测试，验证健康与订阅端点契约
go test ./tests/contract/agent_lifecycle -run 'Health|Subscription' -v
```

测试日志示例：

```text
发布事件 agent.health.degraded，通知 1 个订阅者
告警 IM Payload: {"title":"代理 content-writer 健康告警","severity":"critical","metadata":{"status":"degraded"}}
# 冷却期内重复信号不会再次触发告警
```

若接入真实 IM Webhook，可在 `AGENT_IM_WEBHOOK` 中配置值班群地址，等待 30 秒内收到“健康退化”通知；同时可以在事件总线上看到 `agent.health.degraded` 主题的推送。

## 7. gRPC 控制面验证

```bash
grpcurl -plaintext -d '{"agentId":"{agentId}"}' \
  localhost:9090 powerx.agent.v1.AgentLifecycleService.GetHealthSummary
```

确保 gRPC Server 已在 `internal/server/grpc/server.go` 注册新 Service，并通过 buf 生成客户端。

## 8. 快速回归 / 清理

```bash
# 执行订阅 + 告警相关单元测试
go test ./tests/unit/agent_lifecycle/...

# 停止本地依赖
make deps-down

# 如需恢复临时配置
git checkout -- .env
```

> 附加建议：在提交前执行 `go test ./tests/contract/agent_lifecycle/...`，确保 HTTP/gRPC 契约在工程级别保持一致；若启用了 OpenAPI UI，可通过浏览器访问 `/openapi.min.json` 进行快速联调。

```bash
make deps-down           # 关闭本地依赖
git checkout -- .env     # 如需恢复 Webhook 等临时配置
```

> 日志与指标存储保留 13 个月；若仅为本地测试，可手动清理 Redis/Postgres 数据表。确保 EventBus 中相关主题退订以免残留测试事件。
