# Quickstart — Tool Grants & Security Policy

## 前提条件

- Go 1.24、buf CLI、Docker（PostgreSQL + Redis）、Kafka（复用 SOAR 队列）
- PowerX 仓库已执行 `make deps-tidy`

## 启动步骤

1. 启动依赖服务：
   ```bash
   docker compose -f deploy/compose/dev-authz.yaml up -d
   ```
   - 包含 PostgreSQL、Redis、Kafka、ClickHouse。

2. 生成并验证 gRPC/HTTP 合同：
   ```bash
   make proto-gen proto-lint
   ```

3. 运行 CoreX 应用（含 Admin API 与 gRPC Server）：
   ```bash
   make dev
   ```

4. 预置能力与模板（示例）：
   ```bash
   curl -X POST http://localhost:8077/api/v1/admin/event-fabric/capabilities \
     -H 'Content-Type: application/json' \
     -d '{
       "namespace": "event_fabric",
       "action": "publish",
       "description": "发布事件",
       "risk_level": "medium"
     }'
   ```

5. 创建 Grant 并触发 Challenge：
   ```bash
   curl -X POST http://localhost:8077/api/v1/admin/event-fabric/grants \
     -H 'Content-Type: application/json' \
     -d '{
       "tenant_id": "00000000-0000-0000-0000-000000000001",
       "subject_type": "agent",
       "subject_id": "00000000-0000-0000-0000-000000000101",
       "capabilities": ["event_fabric.publish"],
       "conditions": {
         "time_window": {"start": "2025-10-20T00:00:00Z", "end": "2025-10-21T00:00:00Z"}
       },
       "ttl_seconds": 7200
     }'
   ```

6. 模拟授权评估（gRPC）：
   ```bash
   grpcurl -plaintext -d '{
     "tenant_id": "00000000-0000-0000-0000-000000000001",
     "subject": {"type": "agent", "id": "00000000-0000-0000-0000-000000000101"},
     "capability": "event_fabric.publish",
     "context_tags": ["prod"],
     "resource": "topic://payments"
   }' localhost:19090 powerx.event_fabric.v1.AuthorizationService/Evaluate
   ```

7. 查看审计记录（ClickHouse SQL）：
   ```bash
   clickhouse client --query="
     SELECT event_type, subject_id, decision_reason
     FROM audit.authorization
     WHERE tenant_id='00000000-0000-0000-0000-000000000001'
     ORDER BY timestamp DESC LIMIT 20"
   ```

## 验证指标

- 查看 Prometheus 指标 `event_fabric_authorization_latency_ms` 是否满足目标。
- Kafka 主题 `secops.challenge` 是否收到 Challenge 事件。
- Redis 键 `grant:{tenant_id}:{subject_id}:*` 在 Grant 失效后立即移除。
