# Quickstart — Workflow & Agent Orchestration

## 前提条件
- 已启动 CoreX 后端 (`make dev`) 并连接默认 PostgreSQL、Redis、EventBus
- 已运行 `make proto-gen proto-lint` 生成最新 gRPC 代码
- 拥有管理员 Token，可访问 `/api/v1/admin/workflows`

## 定义模板 & 校验提示

- **步骤 ID 唯一**：`steps[*].id` 必须唯一，允许引用后续步骤。
- **支持类型**：`agent/system/decision/parallel/human_approval/compensation`
- **入度检查**：至少存在一个 `depends_on` 为空的起始节点。
- **无环拓扑**：`next_step_ids` 与 `depends_on` 形成的图必须可拓扑排序。
- **示例 JSON**：

  ```json
  [
    {"id":"prepare","type":"agent","next_step_ids":["execute"],"config":{"capability":"crm.prefetch"}},
    {"id":"execute","type":"system","depends_on":["prepare"],"config":{"task":"backend.run"}}
  ]
  ```

## 步骤

1. **创建工作流定义**
   ```bash
   curl -X POST "http://localhost:8077/api/v1/admin/workflows/definitions" \
     -H 'Authorization: Bearer <TOKEN>' \
     -H 'Content-Type: application/json' \
     -d '{
       "tenant_id": 1001,
       "name": "demo-approval",
       "retry_policy": {"initial_delay_ms": 30000, "max_retries": 5, "backoff_factor": 2},
       "steps": [
         {"id": "prepare", "type": "system", "config": {"task": "prefetch"}, "next_step_ids":["agent_review"]},
         {"id": "agent_review", "type": "agent", "config": {"capability": "event_fabric.publish"}, "next_step_ids":["finalize"]},
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

5. **控制运行中的实例**
  ```bash
  # 暂停实例
  curl -X POST "http://localhost:8077/api/v1/admin/workflows/instances/<INSTANCE_ID>/actions" \
    -H 'Authorization: Bearer <TOKEN>' -H 'Content-Type: application/json' \
    -d '{"tenant_id":1001,"action":"pause","reason":"planned-maintenance"}'

  # 恢复实例
  curl -X POST "http://localhost:8077/api/v1/admin/workflows/instances/<INSTANCE_ID>/actions" \
    -H 'Authorization: Bearer <TOKEN>' -H 'Content-Type: application/json' \
    -d '{"tenant_id":1001,"action":"resume"}'

  # 手动重试失败步骤
  curl -X POST "http://localhost:8077/api/v1/admin/workflows/instances/<INSTANCE_ID>/actions" \
    -H 'Authorization: Bearer <TOKEN>' -H 'Content-Type: application/json' \
    -d '{"tenant_id":1001,"action":"retry_step","step_id":"agent_review"}'
  ```

6. **导出审计记录**
  ```bash
  # JSON
  curl -X GET "http://localhost:8077/api/v1/admin/workflows/instances/export?tenant_id=1001&format=json" \
    -H 'Authorization: Bearer <TOKEN>'

  # CSV
  curl -X GET "http://localhost:8077/api/v1/admin/workflows/instances/export?tenant_id=1001&format=csv" \
    -H 'Authorization: Bearer <TOKEN>' -o workflow_export.csv
  ```

> gRPC 客户端可调用 `workflow.powerx.WorkflowService/ExportInstances`，传入 `include_step_details=true` 获取与 HTTP 相同的字段。

## 验证点
- 发布的定义在 `GET /definitions` 中列出且版本号递增。
- 工作流实例在 30 秒内进入 `running/ waiting / succeeded` 状态之一。
- 导出数据（HTTP/CSV/gRPC）包含步骤、Agent、重试次数等字段，可用于审计复核。
