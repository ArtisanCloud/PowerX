# Quickstart — Workflow & Agent Orchestration

## 前提条件
- 已启动 CoreX 后端 (`make dev`) 并连接默认 PostgreSQL、Redis、EventBus
- 已运行 `make proto-gen proto-lint` 生成最新 gRPC 代码
- 拥有管理员 Token，可访问 `/api/v1/admin/workflows`

## 步骤

1. **创建工作流定义**
   ```bash
   curl -X POST "http://localhost:8077/api/v1/admin/workflows/definitions" \
     -H 'Authorization: Bearer <TOKEN>' \
     -H 'Content-Type: application/json' \
     -d '{
       "tenant_id": "00000000-0000-0000-0000-000000000001",
       "name": "demo-approval",
       "retry_policy": {"initial_delay_ms": 30000, "max_retries": 5, "backoff_factor": 2},
       "steps": [
         {"id": "prepare", "type": "system", "config": {"task": "prefetch"}},
         {"id": "agent_review", "type": "agent", "config": {"capability": "event_fabric.publish"}},
         {"id": "finalize", "type": "system", "config": {"task": "notify"}}
       ]
     }'
   ```

2. **发布定义**
   ```bash
   curl -X POST "http://localhost:8077/api/v1/admin/workflows/definitions/<DEFINITION_ID>/publish" \
     -H 'Authorization: Bearer <TOKEN>'
   ```

3. **启动实例**
   ```bash
   curl -X POST "http://localhost:8077/api/v1/admin/workflows/instances" \
     -H 'Authorization: Bearer <TOKEN>' \
     -H 'Content-Type: application/json' \
     -d '{
       "definition_id": "<DEFINITION_ID>",
       "input": {"request_id": "demo-001", "tenant": "T1"}
     }'
   ```

4. **查询实例状态**
   ```bash
   curl -X GET "http://localhost:8077/api/v1/admin/workflows/instances/<INSTANCE_ID>" \
     -H 'Authorization: Bearer <TOKEN>'
   ```

5. **发生告警时导出审计**
   ```bash
   curl -X GET "http://localhost:8077/api/v1/admin/workflows/instances/export?tenant_id=00000000-0000-0000-0000-000000000001&format=csv" \
     -H 'Authorization: Bearer <TOKEN>' -o workflow_export.csv
   ```

## 验证点
- 发布的定义在 `GET /definitions` 中列出且版本号递增。
- 工作流实例在 30 秒内进入 `running/ waiting / succeeded` 状态之一。
- 导出文件包含步骤、Agent、重试次数等字段，可用于审计复核。
