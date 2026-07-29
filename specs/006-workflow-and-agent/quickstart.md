# Quickstart — Workflow & Agent Orchestration

## 前提条件
- 已启动 CoreX 后端 (`make dev`) 并连接默认 PostgreSQL、Redis、EventBus
- 已运行 `make proto-gen proto-lint` 生成最新 gRPC 代码
- 拥有管理员 Token，可访问 `/api/v1/admin/workflows`

## 定义模板 & 校验提示

- **步骤 ID 唯一**：`steps[*].id` 必须唯一，允许引用后续步骤。
- **支持类型**：`agent/system/decision/parallel/human_approval/compensation`
- **语义节点**：`steps[*].node_kind` 必须来自 Node Catalog，例如 `skill.invoke`、`human.review`、`knowledge.publish`。
- **入度检查**：至少存在一个 `depends_on` 为空的起始节点。
- **无环拓扑**：`next_step_ids` 与 `depends_on` 形成的图必须可拓扑排序。
- **发布校验**：发布前必须校验 Skill、Capability、Knowledge Space、Metadata namespace 和 Human Review approver policy。
- **示例 JSON**：

  ```json
  [
    {"id":"capture","type":"system","node_kind":"input.capture","next_step_ids":["extract"],"config":{"artifact_output_path":"$.artifacts.source"}},
    {"id":"extract","type":"system","node_kind":"skill.invoke","node_ref":"knowledge.extract.basic","depends_on":["capture"],"next_step_ids":["review"],"config":{"skill_id":"knowledge.extract.basic"}},
    {"id":"review","type":"human_approval","node_kind":"human.review","depends_on":["extract"],"config":{"review_type":"knowledge_publish","approver_policy":{"roles":["knowledge_reviewer"]},"review_payload_path":"$.vars.extracted","approved_route":"publish","rejected_route":"revise"}}
  ]
  ```

## 步骤

1. **创建工作流定义**
   ```bash
   curl -X POST "http://localhost:8077/api/v1/admin/workflows/definitions" \
     -H 'Authorization: Bearer <TOKEN>' \
     -H 'Content-Type: application/json' \
     -d '{
       "name": "marketing-knowledge-capture",
       "default_retry_policy": {"initial_interval_ms": 30000, "max_attempts": 5, "backoff_multiplier": 2},
       "steps": [
         {"id": "capture", "type": "system", "node_kind": "input.capture", "config": {"artifact_output_path": "$.artifacts.source"}, "next_step_ids":["extract"]},
         {"id": "extract", "type": "system", "node_kind": "skill.invoke", "node_ref": "knowledge.extract.marketing", "depends_on":["capture"], "config": {"skill_id": "knowledge.extract.marketing", "output_path": "$.vars.extracted"}, "next_step_ids":["stage"]},
         {"id": "stage", "type": "system", "node_kind": "knowledge.stage", "depends_on":["extract"], "config": {"knowledge_space_uuid": "<KNOWLEDGE_SPACE_UUID>", "input_path": "$.vars.extracted"}, "next_step_ids":["review"]},
         {"id": "review", "type": "human_approval", "node_kind": "human.review", "depends_on":["stage"], "config": {"review_type": "knowledge_publish", "approver_policy": {"roles": ["knowledge_reviewer"]}, "review_payload_path": "$.vars.extracted", "approved_route": "publish", "rejected_route": "revise"}},
         {"id": "publish", "type": "system", "node_kind": "knowledge.publish", "depends_on":["review"], "config": {"knowledge_space_uuid": "<KNOWLEDGE_SPACE_UUID>"}}
       ]
     }'
   ```

2. **发布定义**
   ```bash
   curl -X POST "http://localhost:8077/api/v1/admin/workflows/definitions/<DEFINITION_UUID>/publish" \
     -H 'Authorization: Bearer <TOKEN>'
   ```

3. **启动实例**
   ```bash
   curl -X POST "http://localhost:8077/api/v1/admin/workflows/instances" \
     -H 'Authorization: Bearer <TOKEN>' \
     -H 'Content-Type: application/json' \
     -d '{
       "definition_uuid": "<DEFINITION_UUID>",
       "input": {"source_asset_uuid": "<SOURCE_ASSET_UUID>"}
     }'
   ```

4. **查询实例状态**
   ```bash
   curl -X GET "http://localhost:8077/api/v1/admin/workflows/instances/<INSTANCE_UUID>" \
     -H 'Authorization: Bearer <TOKEN>'
   ```

5. **处理 Human Review**
  ```bash
  curl -X GET "http://localhost:8077/api/v1/admin/workflows/review-tasks?status=pending" \
    -H 'Authorization: Bearer <TOKEN>'

  curl -X POST "http://localhost:8077/api/v1/admin/workflows/review-tasks/<REVIEW_TASK_UUID>/actions" \
    -H 'Authorization: Bearer <TOKEN>' -H 'Content-Type: application/json' \
    -d '{"action":"approve","comment":"approved"}'
  ```

6. **控制运行中的实例**
  ```bash
  # 暂停实例
  curl -X POST "http://localhost:8077/api/v1/admin/workflows/instances/<INSTANCE_UUID>/actions" \
    -H 'Authorization: Bearer <TOKEN>' -H 'Content-Type: application/json' \
    -d '{"action":"pause","reason":"planned-maintenance"}'

  # 恢复实例
  curl -X POST "http://localhost:8077/api/v1/admin/workflows/instances/<INSTANCE_UUID>/actions" \
    -H 'Authorization: Bearer <TOKEN>' -H 'Content-Type: application/json' \
    -d '{"action":"resume"}'

  # 手动重试失败步骤
  curl -X POST "http://localhost:8077/api/v1/admin/workflows/instances/<INSTANCE_UUID>/actions" \
    -H 'Authorization: Bearer <TOKEN>' -H 'Content-Type: application/json' \
    -d '{"action":"retry_step","step_id":"extract"}'
  ```

7. **导出审计记录**
  ```bash
  # JSON
  curl -X GET "http://localhost:8077/api/v1/admin/workflows/instances/export?format=json" \
    -H 'Authorization: Bearer <TOKEN>'

  # CSV
  curl -X GET "http://localhost:8077/api/v1/admin/workflows/instances/export?format=csv" \
    -H 'Authorization: Bearer <TOKEN>' -o workflow_export.csv
  ```

> gRPC 客户端必须使用 `definition_uuid`、`instance_uuid`、`agent_uuid` 这类 UUID 字段；numeric id 只允许作为内部存储细节。

## 验证点
- 发布的定义在 `GET /definitions` 中列出且版本号递增。
- 工作流实例在 30 秒内进入 `running / waiting / succeeded` 状态之一。
- `human.review` 节点会创建 Review Task，approve 后 Runner 继续推进。
- `knowledge.publish` 只能在审核通过后执行。
- 导出数据（HTTP/CSV/gRPC）包含步骤、Agent、重试次数等字段，可用于审计复核。
