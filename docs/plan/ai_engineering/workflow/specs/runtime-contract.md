# Workflow Runtime Contract

## 1. WorkflowDefinition

定义是不可变版本化蓝图。发布后不得原地修改。

必需字段：

- `workflow_definition_uuid`
- `tenant_uuid`
- `name`
- `version`
- `status`
- `input_schema`
- `step_graph`
- `default_retry_policy`
- `compensation_policy`
- `sla_policy`
- `metadata`

## 2. StepDefinition

建议结构：

```json
{
  "id": "parse_source",
  "display_name": "解析来源材料",
  "type": "system",
  "node_kind": "skill.invoke",
  "node_ref": "knowledge.ingestion.parse_source",
  "config": {
    "skill_id": "knowledge.ingestion.parse_source",
    "input_path": "$.artifacts.source",
    "output_path": "$.vars.parsed"
  },
  "depends_on": ["capture_input"],
  "next_step_ids": ["extract_knowledge"],
  "compensatable": false,
  "retry_policy": {
    "max_attempts": 3
  }
}
```

规则：

1. `id` 是定义内稳定 step key。
2. `type` 只能使用底层 StepType。
3. `node_kind` 决定 Node Adapter。
4. `node_ref` 指向 Skill、Capability、模板或内部 adapter 引用。
5. `config` 必须符合对应 node_kind schema。

## 3. WorkflowInstance

实例必须引用不可变 definition version。

必需字段：

- `workflow_instance_uuid`
- `workflow_definition_uuid`
- `definition_version`
- `tenant_uuid`
- `agent_uuid`
- `initiator_user_uuid`
- `state`
- `input_context`
- `runtime_context`
- `output_context`
- `trace_id`
- `correlation_id`

## 4. StepRecord

每次 step attempt 都必须记录。

必需字段：

- `workflow_instance_uuid`
- `step_id`
- `node_kind`
- `node_ref`
- `state`
- `attempt`
- `payload_in`
- `payload_out`
- `error_code`
- `error_message`
- `started_at`
- `finished_at`
- `subject_type`
- `subject_uuid`

## 5. NodeAdapter 接口

逻辑接口：

```text
ValidateDefinition(step, catalog) -> error
PrepareInput(context, step) -> payload
Execute(ctx, payload) -> NodeResult
ResolveNext(step, result) -> next_step_ids
BuildCompensation(step, result) -> compensation_request
```

`NodeResult`：

```text
status: completed|waiting|failed|skipped
output: object
decision: string
selected_branches: string[]
review_task_uuid: string
error_code: string
error_message: string
```

## 6. Human Review Contract

Human Review 必须是 first-class 节点，不是普通备注。

审核任务字段：

- `review_task_uuid`
- `tenant_uuid`
- `workflow_instance_uuid`
- `step_id`
- `review_type`
- `payload`
- `approver_policy`
- `status`
- `reviewer_user_uuid`
- `decision`
- `comment`
- `created_at`
- `completed_at`

动作：

- `approve`
- `reject`
- `request_changes`
- `cancel`

## 7. Error Contract

Workflow Runtime 必须使用结构化错误：

```text
workflow.definition_invalid
workflow.node_kind_unknown
workflow.node_config_invalid
workflow.node_adapter_unavailable
workflow.skill_not_published
workflow.capability_not_registered
workflow.permission_denied
workflow.knowledge_space_missing
workflow.metadata_namespace_missing
workflow.review_required
workflow.retry_exhausted
workflow.compensation_failed
```

不得把节点执行失败吞掉后继续推进。

## 8. Trace Contract

每条链路必须包含：

- `trace_id`
- `tenant_uuid`
- `workflow_definition_uuid`
- `workflow_instance_uuid`
- `step_id`
- `node_kind`
- `node_ref`
- `agent_uuid`
- `skill_id`
- `capability_id`

字段不存在时必须为空，不得用 numeric id 替代业务 UUID。
