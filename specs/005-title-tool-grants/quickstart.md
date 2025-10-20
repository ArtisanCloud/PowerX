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

4. 一键体验脚本（推荐）：
   ```bash
   # 输出结果位于 reports/authorization_audit.{json,csv}
   bash scripts/demo/event_fabric_quickstart.sh
   ```

5. 手动创建能力：
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

6. 手动创建 Grant：
   ```bash
   curl -X POST http://localhost:8077/api/v1/admin/event-fabric/grants \
     -H 'Content-Type: application/json' \
     -d '{
       "tenant_id": "00000000-0000-0000-0000-000000000001",
       "subject": {
         "type": "agent",
         "id": "00000000-0000-0000-0000-000000000101"
       },
       "capabilities": ["event_fabric.publish"],
       "conditions": {
         "resources": ["topic://demo"],
         "context_tags": ["prod"]
       },
       "ttl_seconds": 7200
     }'
   ```

7. 查询授权审计（JSON）：
   ```bash
   curl "http://localhost:8077/api/v1/admin/event-fabric/audit/authorization?tenantId=00000000-0000-0000-0000-000000000001&from=1970-01-01T00:00:00Z&to=$(date -u +'%Y-%m-%dT%H:%M:%SZ')&page=1&pageSize=20"
   ```

8. 导出授权审计（CSV）：
   ```bash
   curl -o authorization_audit.csv "http://localhost:8077/api/v1/admin/event-fabric/audit/authorization?tenantId=00000000-0000-0000-0000-000000000001&from=1970-01-01T00:00:00Z&to=$(date -u +'%Y-%m-%dT%H:%M:%SZ')&format=csv"
   ```

## 验证指标

- 查看 Prometheus 指标 `event_fabric_authorization_latency_ms` 是否满足目标。
- Kafka 主题 `secops.challenge` 是否收到 Challenge 事件。
- Redis 键 `grant:{tenant_id}:{subject_id}:*` 在 Grant 失效后立即移除。
