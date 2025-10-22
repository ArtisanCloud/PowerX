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

## 5. gRPC 控制面验证

```bash
grpcurl -plaintext -d '{"agentId":"{agentId}"}' \
  localhost:9090 powerx.agent.v1.AgentLifecycleService.GetHealthSummary
```

确保 gRPC Server 已在 `internal/server/grpc/server.go` 注册新 Service，并通过 buf 生成客户端。

## 6. 清理

```bash
make deps-down           # 关闭本地依赖
git checkout -- .env     # 如需恢复 Webhook 等临时配置
```

> 日志与指标存储保留 13 个月；若仅为本地测试，可手动清理 Redis/Postgres 数据表。确保 EventBus 中相关主题退订以免残留测试事件。
