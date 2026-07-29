# Workflow 前端实现调整说明

## 1. 目标

现有 Web Admin workflow 页面是早期演示形态，不能作为 native-agent 可用的 Workflow Runtime 页面。

本调整目标是把它收敛为真实产品页面：

```text
Workflow Definition 列表
  -> Workflow Builder
  -> Publish Validation
  -> Workflow Instance Monitor
  -> Human Review
```

页面必须调用真实 `/api/v1/admin/workflows/*` Admin API，不允许继续使用 `/workflow` 作为后端 API baseUrl，也不允许保留 mock workflow list、mock node palette、mock kind schema。

## 2. 现有文件与调整方向

| 现有文件 | 当前问题 | 调整方向 |
| --- | --- | --- |
| `web-admin/app/composables/api/services/workflowService.ts` | `baseUrl = "/workflow"`，包含 `getKinds()`、`getPalette()` mock，接口语义偏旧 | 改为 `/admin/workflows`，按 Definition、Instance、NodeCatalog、ReviewTask、Pack 分组 |
| `web-admin/app/types/workflow.ts` | 类型偏编辑器 demo，未对齐 `definition_uuid`、`node_kind`、HumanReviewTask | 重建类型为 Admin API DTO + Builder ViewModel 两层 |
| `web-admin/app/pages/workflow/index.vue` | 内置 workflow list，硬编码中文，未接分页/搜索/发布状态 | 接 `GET /definitions`，提供分页、搜索、状态筛选、创建、发布、归档、进入实例 |
| `web-admin/app/pages/workflow/workspace.vue` | 没有稳定 definition_uuid 工作台语义，新建时有 demo workflow | 用 `definition_uuid` 加载/保存 draft；新建必须走 create API，不创建 demo 数据 |
| `web-admin/app/components/workflow/WorkflowEditor.vue` | 左侧 palette 和节点配置没有接 Node Catalog；运行按钮只是 console | 左侧接 `GET /node-catalog`；右侧按 `config_schema` 渲染；运行创建 instance |
| `web-admin/app/components/workflow/nodes/*` | 节点显示偏通用画布组件 | 节点主标签显示 i18n 名称，`node_kind`/`node_ref` 只作为诊断元数据 |
| `web-admin/app/composables/workflow/useWorkflowManager*` | 管理状态可能围绕 demo workflow | 改为 Definition draft/dirty/validation/instance 状态管理 |

## 3. 前端路由

保留业务入口：

```text
/workflow
/workflow/workspace?definition_uuid=<workflow_definition_uuid>
```

新增运行态入口：

```text
/workflow/instances
/workflow/instances/:instance_uuid
/workflow/review-tasks
/workflow/review-tasks/:review_task_uuid
```

规则：

1. URL 参数使用 `definition_uuid`、`instance_uuid`、`review_task_uuid`。
2. 页面主标题显示业务名称，不显示 UUID。
3. UUID 只能在详情页诊断区或复制按钮中出现。

## 4. API Client 目标结构

`workflowService.ts` 建议拆成清晰方法组：

```text
definitions:
  listDefinitions()
  createDefinition()
  getDefinition(definition_uuid)
  updateDefinition(definition_uuid)
  validateDefinition(definition_uuid)
  publishDefinition(definition_uuid)
  archiveDefinition(definition_uuid)

instances:
  listInstances()
  startInstance()
  getInstance(instance_uuid)
  listInstanceSteps(instance_uuid)
  controlInstance(instance_uuid)
  exportInstances()

nodeCatalog:
  listNodeCatalog()
  getNodeCatalogItem(node_kind)

reviewTasks:
  listReviewTasks()
  getReviewTask(review_task_uuid)
  actReviewTask(review_task_uuid)

packs:
  listWorkflowPacks()
  seedWorkflowPacks()
  getWorkflowPack(workflow_key)
```

所有方法使用 `/admin/workflows` 作为 API base，不保留旧 `/workflow` API 兼容分支。

## 5. 类型分层

前端类型分两层：

1. API DTO：严格对应 `specs/006-workflow-and-agent/contracts/http-openapi.yaml`。
2. Builder ViewModel：只用于 Vue Flow 渲染和表单编辑。

必需 DTO：

- `WorkflowDefinition`
- `WorkflowStepDefinition`
- `WorkflowInstance`
- `WorkflowStepSummary`
- `NodeCatalogItem`
- `HumanReviewTask`
- `WorkflowPack`
- `WorkflowValidationIssue`

字段规则：

1. API DTO 使用 snake_case，避免在 client 层偷偷改名导致合同不清。
2. ViewModel 可以转成 camelCase，但转换函数必须集中定义。
3. 业务引用统一使用 UUID 字段，不使用 `id` 表示业务对象。

## 6. 列表页实现

`/workflow` 必须包含：

- 搜索：`keyword`
- 状态筛选：draft、published、archived
- 来源筛选：manual、builtin_pack、plugin_pack、imported
- 分页
- 创建定义
- 发布/归档
- 进入 Builder
- 进入实例列表

空状态必须来自真实 API 返回空集，不能渲染 demo 数据。

## 7. Builder 实现

Builder 左侧：

- 调用 `GET /node-catalog`
- 按 category 分组
- 支持搜索
- 显示依赖状态：available、missing_dependency、permission_denied、disabled

Builder 中间：

- Vue Flow 画布
- 节点连线
- decision/parallel/human.review 必须有明确分支和终止状态

Builder 右侧：

- 根据 `NodeCatalogItem.config_schema` 渲染表单
- Skill 选择器接 Skill Registry
- Capability 选择器接 Capability Registry
- Knowledge Space 选择器接 Knowledge API
- Metadata namespace 选择器接 Metadata API
- 不显示 UUID 作为主标签

发布前：

- 调用 `POST /definitions/:definition_uuid/validate`
- validation 有 error 时禁用 publish
- publish 后 definition version 固定，不允许原地编辑 published version

## 8. Instance Monitor 实现

实例列表：

- 调用 `GET /instances`
- 支持 workflow、状态、Agent、时间范围筛选
- 列表显示工作流名称、状态、当前步骤、触发来源、Agent 名称、耗时

实例详情：

- 调用 `GET /instances/:instance_uuid`
- 调用 `GET /instances/:instance_uuid/steps`
- 展示 Step timeline、payload、error_code、error_message、attempt、trace_id
- 支持 suspend、resume、cancel、retry_step、start_compensation

高风险 payload 默认折叠，展开需要权限。

## 9. Human Review 实现

审核列表：

- 调用 `GET /review-tasks`
- 支持待我审核、状态、review_type、Agent、Knowledge Space 筛选

审核详情：

- 调用 `GET /review-tasks/:review_task_uuid`
- 展示知识草稿、来源材料、分类标签、冲突检测、发布版本预览
- 操作调用 `POST /review-tasks/:review_task_uuid/actions`

操作规则：

1. approve 后显示 Runner 唤醒结果。
2. reject 后不得显示已发布。
3. request_changes 必须返回指定修订节点。
4. Runner 唤醒失败必须显示失败状态，不伪装成成功。

## 10. i18n 与 UI 规则

必须补齐 zh/en locale：

- 菜单名称
- 页面标题
- 筛选项
- 表格列
- 节点名称
- 状态
- 按钮
- 错误
- 空状态
- 审核动作

禁止：

1. 在 Vue template/script 中硬编码用户可见中文。
2. 把 `node_kind`、`capability_id`、`skill_id`、UUID 当成主标签。
3. 使用 demo workflow、mock palette、mock kinds 作为空状态。
4. 在 API client 中保留 `/workflow` 旧 baseUrl。

## 11. 验收

1. `/workflow` 首屏数据来自 `GET /api/v1/admin/workflows/definitions`。
2. Builder 左侧节点来自 `GET /api/v1/admin/workflows/node-catalog`。
3. 发布前会调用 validate，并展示依赖缺失。
4. 启动实例后可以在 `/workflow/instances/:instance_uuid` 看到 Step timeline。
5. `human.review` 会在 `/workflow/review-tasks` 出现待办。
6. 前端搜索 `mockKinds`、`mockPalette`、`demo-workflow` 没有命中。
7. `npm run typecheck` 通过。
