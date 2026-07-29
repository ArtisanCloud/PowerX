# Workflow Runtime 实现落点

## 1. 目标

本文件把 Workflow Runtime 计划映射到现有 PowerX 代码结构，避免重新发明一套并行实现。

结论：继续使用 `specs/006-workflow-and-agent` 和现有 `backend/internal/service/workflow` 作为底座，在其上补 Runner、Node Adapter、Human Review、Node Catalog、Workflow Pack seed 和真实前端接入。

## 2. 后端落点

| 能力 | 现有文件 | 需要调整 |
| --- | --- | --- |
| Definition / Instance Service | `backend/internal/service/workflow/service.go` | 保留定义创建、发布、启动实例入口；增加 Runner 调用、Node Catalog 校验、Workflow Pack 来源字段 |
| Step graph validator | `backend/internal/service/workflow/validator.go` | 从 StepType 校验扩展为 StepType + node_kind + adapter schema 校验 |
| Control / retry / compensation | `backend/internal/service/workflow/service_control.go`, `compensation.go` | 接入 Runner 状态推进；失败、重试、补偿必须写 StepRecord 和 WorkflowEvent |
| Assignment | `backend/internal/service/workflow/assignment_tracker.go` | 继续用于 Agent/Skill 人工或异步派发，不作为 Workflow 替代执行链 |
| HTTP routes | `backend/internal/transport/http/admin/workflow/routes.go` | 参数名收敛到 `definition_uuid`、`instance_uuid`；新增 node catalog、review、pack seed 路由 |
| gRPC service | `backend/internal/transport/grpc/workflow/server.go` | 与 HTTP 保持 definition、instance、review、node catalog 能力对齐 |
| Models | `pkg/corex/db/persistence/model/workflow/*` | 补 node_kind/node_ref/input_mapping/output_mapping、HumanReviewTask、WorkflowPackSeedRecord |
| Repositories | `pkg/corex/db/persistence/repository/workflow/*` | 补 queued step lease、review task、pack seed checksum 查询 |
| Capability | `backend/config/platform_capabilities/workflow.yaml` | 补新路由 capability，并运行 `make capability-check` |

## 3. 新增后端包建议

```text
backend/internal/service/workflow/runner.go
backend/internal/service/workflow/node_adapter.go
backend/internal/service/workflow/node_catalog.go
backend/internal/service/workflow/human_review.go
backend/internal/service/workflow/workflow_pack_seed.go
backend/internal/service/workflow/adapter_skill.go
backend/internal/service/workflow/adapter_capability.go
backend/internal/service/workflow/adapter_metadata.go
backend/internal/service/workflow/adapter_knowledge.go
backend/internal/service/workflow/adapter_event.go
backend/internal/transport/http/admin/workflow/node_catalog_handler.go
backend/internal/transport/http/admin/workflow/review_handler.go
```

规则：

1. Runner 只调 NodeAdapter，不直接调 Skill、Capability、Knowledge、Metadata 业务服务。
2. Adapter 必须显式声明输入输出 schema、权限、幂等和补偿能力。
3. Node Catalog 必须从 adapter registry、Skill Registry、Capability Registry、Knowledge、Metadata 汇总，不允许前端硬编码。
4. 缺 adapter、缺授权、缺 namespace、缺 knowledge space 时发布失败。

## 4. 前端落点

| 能力 | 现有文件 | 需要调整 |
| --- | --- | --- |
| Workflow 列表 | `web-admin/app/pages/workflow/index.vue` | 移除内置列表数据，接 `GET /api/v1/admin/workflows/definitions` |
| Workflow 工作台 | `web-admin/app/pages/workflow/workspace.vue` | 接真实 definition detail、保存 draft、发布校验 |
| Vue Flow 编辑器 | `web-admin/app/components/workflow/WorkflowEditor.vue` | 左侧 palette 改为 Node Catalog API；属性表单按 schema 渲染 |
| API client | `web-admin/app/composables/api/services/workflowService.ts` | baseUrl 改为 `/admin/workflows`，移除 `getKinds/getPalette` mock |
| 类型 | `web-admin/app/types/workflow.ts` | 对齐 StepDefinition、WorkflowDefinition、WorkflowInstance、StepRecord、HumanReviewTask |

页面文案必须走 i18n，业务对象下拉必须显示名称，不把 UUID 作为主标签。

## 5. Seed 与命令

新增目录：

```text
backend/config/workflow_packs/
```

新增命令建议：

```text
make workflow-seed
make seed
```

`make seed` 必须顺序执行基础 seed、capability seed、workflow seed。不得把 workflow seed 做成只能手工运行的隐藏步骤。

## 6. 与 native-agent 的依赖

native-agent 创建、克隆、启用和知识库策展入口必须检查：

1. 绑定的 Workflow Pack 已 seed。
2. WorkflowDefinition 已发布。
3. 所有 node_kind adapter 已注册。
4. 所有 Skill 已发布并可见。
5. 所有 Capability 已登记并授权。
6. Knowledge Space 和 Metadata namespace 存在。

任一条件不满足，必须预检失败并给出明确修复项。
