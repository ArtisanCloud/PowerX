# Web Admin Workflow Builder 页面计划

## 1. 目标

把现有 `/workflow` mock 页面改造成真实 Workflow Builder。

页面必须支持：

- 工作流定义列表。
- 创建定义。
- 编辑 draft。
- 发布定义。
- 启动实例。
- 查看实例运行状态。
- 查看步骤记录和错误。
- 人工审核任务处理。
- 导出审计。

## 2. 路径

建议保留用户页面路径：

```text
/workflow
/workflow/workspace?id=<workflow_definition_uuid>
```

前端 API 必须调用：

```text
/api/v1/admin/workflows/definitions
/api/v1/admin/workflows/definitions/:definition_uuid
/api/v1/admin/workflows/definitions/:definition_uuid/publish
/api/v1/admin/workflows/instances
/api/v1/admin/workflows/instances/:instance_uuid
/api/v1/admin/workflows/instances/:instance_uuid/actions
/api/v1/admin/workflows/instances/export
```

不得继续使用 `/workflow` 作为后端 API baseUrl。

## 3. 列表页

功能：

- 搜索：名称、描述、来源。
- 筛选：状态、分类、是否被 Agent 使用。
- 分页。
- 创建工作流。
- 查看实例。
- 发布/归档。

数据来源：

- `GET /admin/workflows/definitions`

## 4. 编辑器

左侧 Node Catalog：

- 按节点总类分组。
- 可搜索。
- 显示权限状态。
- 显示依赖对象是否存在。

中间画布：

- Vue Flow。
- 节点连线。
- 分支和并行可视化。
- 必需节点缺失提示。

右侧属性：

- 根据 node_kind schema 渲染表单。
- 对 Skill、Capability、Knowledge Space、Metadata namespace 使用可搜索选择器。
- 不显示 UUID 作为主标签。

## 5. 实例详情页

必须展示：

- WorkflowInstance 状态。
- 当前步骤。
- 每个 StepRecord 的状态、输入、输出、错误。
- Human Review 待处理任务。
- Retry 和 compensation 记录。
- Trace ID 和审计事件。

## 6. 移除 mock

必须移除：

- `workflowService.getKinds()` 内置 mock。
- `workflowService.getPalette()` 内置 mock。
- `pages/workflow/index.vue` 内置 mock workflowList。
- 前端硬编码中文文案。

替换为：

- Node Catalog API。
- Workflow Definition API。
- Workflow Instance API。
- i18n locale key。

## 7. 验收

1. 用户可以创建 draft WorkflowDefinition。
2. 用户可以从 Node Catalog 拖入真实节点。
3. 用户可以发布合法 WorkflowDefinition。
4. 用户可以启动实例。
5. 实例详情能看到 StepRecord。
6. Human Review 节点能创建待办并完成 approve/reject。
7. 发布前缺依赖会阻止发布。
