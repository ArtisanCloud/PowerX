# Workflow 实例监控与人工审核页面

## 1. 目标

Workflow Builder 解决定义创建和发布；实例监控与人工审核页面解决运行态可观测、人工确认、失败处理和审计。

知识库增量迭代必须通过该页面完成草稿审核、发布确认和失败排查。

## 2. 页面入口

建议入口：

```text
/workflow/instances
/workflow/instances/:instance_uuid
/workflow/review-tasks
/workflow/review-tasks/:review_task_uuid
```

前端 API 使用：

```text
/api/v1/admin/workflows/instances
/api/v1/admin/workflows/review-tasks
```

## 3. 实例列表

筛选：

- 工作流名称。
- 状态。
- 触发来源。
- Agent。
- 创建人。
- 时间范围。

列：

- 工作流名称。
- 实例状态。
- 当前步骤业务名称。
- 触发来源。
- Agent 名称。
- 创建时间。
- 耗时。
- 操作。

UUID 只作为详情页诊断信息，不作为列表主列。

## 4. 实例详情

区块：

1. 基础信息：definition、version、agent、initiator、trace_id、correlation_id。
2. Step timeline：queued、in_progress、waiting、completed、failed、compensating。
3. 当前上下文：input、vars、artifacts、review、output。
4. 错误与重试：error_code、error_message、attempt、next_retry_at。
5. 补偿记录：target step、rollback policy、状态、人工确认。
6. 审计事件：WorkflowEvent、Audit、Trace。

高风险 payload 默认折叠，只在有权限时展开。

## 5. Human Review 列表

筛选：

- 待我审核。
- 状态。
- review_type。
- Agent。
- Knowledge Space。
- 超时状态。

列：

- 审核事项名称。
- 来源工作流。
- 关联知识库。
- 提交人。
- 截止时间。
- 状态。
- 操作。

## 6. Human Review 详情

必须展示：

- 待发布知识草稿。
- 来源材料引用。
- 抽取摘要。
- 分类、标签、字典、资源类型。
- 冲突检测结果。
- 影响的 Knowledge Space。
- 发布后的版本变化预览。

操作：

- approve。
- reject。
- request_changes。
- cancel。

操作完成后页面必须显示 Runner 唤醒结果。如果 Runner 唤醒失败，审核状态不得伪装成发布成功。

## 7. 交互规则

1. 缺权限时显示明确不可操作状态。
2. 审核通过前不得出现“已入库”状态。
3. request_changes 必须回到指定修订节点。
4. reject 必须终止发布路径或进入补偿路径。
5. 所有按钮、状态、错误文案走 i18n。
