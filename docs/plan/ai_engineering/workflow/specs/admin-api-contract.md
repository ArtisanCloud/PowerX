# Workflow Admin API Contract

## 1. 目标

Workflow Admin API 面向 PowerX Admin 和插件 Admin 页面，用于管理 WorkflowDefinition、WorkflowInstance、Node Catalog、Human Review 和 Workflow Pack seed。

所有 API 路径位于：

```text
/api/v1/admin/workflows
```

路径参数、请求体、事件、审计引用必须使用 UUID。不得暴露 numeric id 作为业务对象引用。

## 2. Definition API

```text
GET    /api/v1/admin/workflows/definitions
POST   /api/v1/admin/workflows/definitions
GET    /api/v1/admin/workflows/definitions/:definition_uuid
PATCH  /api/v1/admin/workflows/definitions/:definition_uuid
POST   /api/v1/admin/workflows/definitions/:definition_uuid/publish
POST   /api/v1/admin/workflows/definitions/:definition_uuid/archive
```

查询参数：

- `keyword`
- `status`
- `category`
- `source_type`
- `agent_uuid`
- `page`
- `page_size`

发布必须执行完整依赖校验。校验失败返回结构化错误：

```json
{
  "code": "workflow.definition_invalid",
  "details": [
    {
      "step_id": "publish_knowledge",
      "node_kind": "knowledge.publish",
      "reason": "workflow.knowledge_space_missing"
    }
  ]
}
```

## 3. Instance API

```text
GET    /api/v1/admin/workflows/instances
POST   /api/v1/admin/workflows/instances
GET    /api/v1/admin/workflows/instances/:instance_uuid
POST   /api/v1/admin/workflows/instances/:instance_uuid/actions
GET    /api/v1/admin/workflows/instances/:instance_uuid/steps
GET    /api/v1/admin/workflows/instances/:instance_uuid/events
GET    /api/v1/admin/workflows/instances/export
```

实例动作：

- `suspend`
- `resume`
- `cancel`
- `retry_step`
- `start_compensation`

启动实例必须引用 published definition：

```json
{
  "workflow_definition_uuid": "00000000-0000-0000-0000-000000000000",
  "definition_version": 1,
  "agent_uuid": "00000000-0000-0000-0000-000000000000",
  "input_context": {}
}
```

## 4. Node Catalog API

```text
GET /api/v1/admin/workflows/node-catalog
GET /api/v1/admin/workflows/node-catalog/:node_kind
POST /api/v1/admin/workflows/definitions/:definition_uuid/validate
```

Node Catalog 返回：

- node_kind
- display_name_i18n_key
- description_i18n_key
- category
- step_type
- input_schema
- output_schema
- config_schema
- required_permissions
- required_capabilities
- idempotency_required
- compensation_supported
- source_status

`source_status` 用于 Builder 展示依赖状态，例如 Skill 未发布、Capability 未授权、Knowledge Space 不存在。页面主标签必须使用 i18n 显示名，不能用 node_kind 或 capability_id 作为主标签。

## 5. Human Review API

```text
GET  /api/v1/admin/workflows/review-tasks
GET  /api/v1/admin/workflows/review-tasks/:review_task_uuid
POST /api/v1/admin/workflows/review-tasks/:review_task_uuid/actions
```

审核动作：

- `approve`
- `reject`
- `request_changes`
- `cancel`

请求：

```json
{
  "action": "approve",
  "comment": "review.comment.i18n.or.operator_note",
  "decision_payload": {}
}
```

规则：

1. 只有合法审批人可以操作。
2. 审核通过后由 Runner 唤醒等待 step。
3. 审核拒绝不得发布正式知识。
4. 审核动作必须写 WorkflowEvent 和 Audit。

## 6. Workflow Pack API

```text
GET  /api/v1/admin/workflows/packs
POST /api/v1/admin/workflows/packs/seed
GET  /api/v1/admin/workflows/packs/:workflow_key
```

`packs/seed` 仅用于管理员或部署流程，必须幂等并写 checksum。缺依赖时失败，不自动降级。

## 7. 权限

| 操作 | 权限 |
| --- | --- |
| 管理定义 | `workflow.builder:manage` |
| 发布定义 | `workflow.definition:publish` |
| 启动实例 | `workflow.instance:invoke` |
| 控制实例 | `workflow.instance:control` |
| 查看实例 | `workflow.instance:read` |
| 审核任务 | `workflow.review:manage` |
| seed pack | `workflow.pack:seed` |

这些权限必须进入 platform capability 和 RBAC，不得只在页面隐藏按钮。
