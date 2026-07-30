# Workflow Node Catalog

## 1. 目标

Node Catalog 是 Workflow Builder 和 Workflow Runner 共享的节点目录。它把业务语义节点映射到现有底层 StepType，并声明输入输出、权限、能力和审计要求。

底层 StepType 继续复用 006 已有类型：

```text
agent
system
decision
parallel
human_approval
compensation
```

native-agent 和插件使用更细的 `node_kind`。

## 2. 节点总类

| 节点总类 | 典型 node_kind | 底层 StepType |
| --- | --- | --- |
| Input | `input.capture` | `system` |
| Skill | `skill.invoke` | `agent` 或 `system` |
| Capability | `capability.invoke` | `system` |
| Metadata | `metadata.classify` | `system` |
| Knowledge | `knowledge.stage`, `knowledge.publish` | `system` |
| Decision | `decision.gateway` | `decision` |
| Parallel | `parallel.fanout`, `parallel.join` | `parallel` |
| Human | `human.review` | `human_approval` |
| Event | `event.emit` | `system` |
| Compensation | `compensation.rollback` | `compensation` |

## 3. 标准节点

### 3.1 `input.capture`

用途：

- 接收上传、文本、链接、业务对象引用。
- 创建 source asset 或输入引用。
- 初始化 workflow context。

必需配置：

- `input_schema_ref`
- `source_policy`
- `artifact_output_path`

### 3.2 `skill.invoke`

用途：

- 调用 Skill Registry 中已发布 Skill。
- 适合 OCR、转写、抽取、摘要、结构化等单步能力。

必需配置：

- `skill_id`
- `input_path`
- `output_path`
- `agent_uuid` 可选；需要指定执行 Agent 时必须使用 UUID。

执行规则：

1. Skill 必须已发布。
2. Agent 必须绑定该 Skill 或通过 Workflow Pack 显式授权。
3. Skill 内部调用 Capability 时仍走 Capability Registry。

### 3.3 `capability.invoke`

用途：

- 直接调用 PowerX Core 或插件 capability。

必需配置：

- `capability_id`
- `preferred_protocol`
- `input_path`
- `output_path`
- `idempotency_key_path`

执行规则：

1. capability 必须已登记。
2. tenant、agent/user/role 必须有授权。
3. 写操作必须声明幂等键。

### 3.4 `metadata.classify`

用途：

- 对草稿知识对象打分类、标签、字典项和资源类型。

必需配置：

- `taxonomy_namespace`
- `tag_namespace`
- `dictionary_namespace`
- `resource_type_namespace`
- `input_path`
- `output_path`

### 3.5 `knowledge.stage`

用途：

- 把抽取结果写入知识草稿区。
- 生成待审核 draft knowledge objects。

必需配置：

- `knowledge_space_uuid`
- `draft_schema_ref`
- `input_path`
- `output_path`

规则：

1. 只能写入 draft/staging。
2. 不允许直接写正式 Knowledge Space。

### 3.6 `decision.gateway`

用途：

- 根据节点输出、规则表达式或审核结果选择分支。

必需配置：

- `routes`
- `default_route`
- `condition_source_path`

### 3.7 `human.review`

用途：

- 创建人工审核任务。
- 等待审批人确认、驳回或要求修改。

必需配置：

- `review_type`
- `approver_policy`
- `review_payload_path`
- `approved_route`
- `rejected_route`

### 3.8 `knowledge.publish`

用途：

- 把审核通过的草稿发布到正式 Knowledge Space。
- 写入版本、embedding、图谱引用和审计。

必需配置：

- `knowledge_space_uuid`
- `draft_refs_path`
- `review_result_path`
- `publish_policy`

规则：

1. 必须依赖 `human.review` 的通过结果。
2. 必须生成发布版本。
3. 必须记录 source asset 和 reviewer。

### 3.9 `event.emit`

用途：

- 向 EventBus 发布 workflow 结果事件。

必需配置：

- `topic`
- `payload_path`
- `event_schema_ref`

### 3.10 `compensation.rollback`

用途：

- 回滚已发布版本、撤销草稿、释放锁或撤销外部调用。

必需配置：

- `target_step_id`
- `rollback_policy`
- `manual_approval_required`

## 4. Builder 展示规则

Workflow Builder 必须从 Node Catalog 加载节点，不允许硬编码 mock 节点。

节点显示必须使用业务名称：

- 显示 `display_name`。
- capability_id、skill_id、uuid 只能作为调试元数据。
- 节点必须有搜索、分类和权限状态提示。

## 5. 发布校验

WorkflowDefinition 发布前必须校验：

1. 所有 node_kind 已注册。
2. 所有 Skill 已发布。
3. 所有 Capability 已登记并可授权。
4. 所有 Knowledge Space Profile 或具体 Knowledge Space 合法。
5. 所有 Metadata namespace 存在。
6. 所有 Human Review approver policy 合法。
7. DAG 无环，分支有明确收敛或终止。
8. 补偿节点覆盖所有高风险写操作。
