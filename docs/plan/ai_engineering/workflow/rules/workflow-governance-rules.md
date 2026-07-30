# Workflow Governance Rules

## 1. 发布规则

WorkflowDefinition 发布前必须校验：

1. DAG 无环。
2. 所有 step id 唯一。
3. 所有 node_kind 已注册。
4. 所有 node config 符合 schema。
5. 所有 Skill 已发布。
6. 所有 Capability 已登记。
7. 所有 Knowledge Space 或 Profile 合法。
8. 所有 Metadata namespace 存在。
9. 所有 Human Review approver policy 合法。
10. 高风险写操作有补偿策略。

校验失败必须阻止发布。

## 2. 运行规则

1. WorkflowInstance 只能引用 published definition。
2. WorkflowInstance 必须引用固定 definition version。
3. Runner 必须逐步写 StepRecord。
4. 节点失败不得静默跳过。
5. 重试耗尽后必须进入 failed 或 compensating。
6. 等待 Human Review 时实例必须进入 waiting。
7. 取消实例必须阻止新 step 调度。

## 3. 权限规则

1. 管理定义需要 `workflow.builder:manage`。
2. 启动实例需要 `workflow.scheduler:invoke` 或绑定 Agent 授权。
3. Skill 节点必须校验 Agent/Workflow 对 Skill 的可见性。
4. Capability 节点必须校验 Capability Registry 和 grant。
5. Knowledge publish 必须校验 Knowledge Space 写入权限。
6. Human Review 必须校验 reviewer 是合法审批人。

## 4. 审计规则

必须审计：

- Definition created/published/archived。
- Instance started/suspended/resumed/canceled/succeeded/failed。
- Step queued/in_progress/waiting/completed/failed。
- Human review created/approved/rejected。
- Retry scheduled。
- Compensation started/completed/failed。
- Knowledge staged/published/rolled back。

## 5. 命名规则

1. API、事件、审计和跨表引用必须使用 UUID。
2. 页面主标签必须显示业务名称。
3. `workflow_key`、`node_kind`、`capability_id` 可以作为机器标识，但不能作为默认用户主标签。

## 6. 禁止行为

1. 不允许用 Skill + Task Queue 替代 Workflow。
2. 不允许 Builder 使用 mock 节点目录。
3. 不允许 Workflow 直接调用未登记插件接口。
4. 不允许未审核草稿直接发布到正式知识库。
5. 不允许失败节点无记录地继续推进。
