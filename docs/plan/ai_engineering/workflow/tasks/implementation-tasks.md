# Workflow Runtime 实现任务

## 1. 后端任务

### 1.1 API 对齐

- [ ] 对齐 HTTP OpenAPI 与当前 `/admin/workflows/*` 路由。
- [ ] 把 HTTP 路径参数从 `definitionId`、`instanceId` 收敛为 `definition_uuid`、`instance_uuid`，并拒绝 numeric id。
- [ ] 补齐 Node Catalog API。
- [ ] 补齐 Human Review API。
- [ ] 补齐 Workflow Pack seed API 或 CLI。
- [ ] 确认 capability registry 中 workflow capability 与正式接口一致。

### 1.2 数据模型

- [ ] 在 StepDefinition 中增加 `node_kind`、`node_ref`、`retry_policy`、`input_mapping`、`output_mapping`。
- [ ] 补齐 HumanReviewTask 模型。
- [ ] 补齐 WorkflowPack/WorkflowSeed 版本记录。
- [ ] 补齐 StepRecord 的 `node_kind`、`node_ref`、结构化错误字段。

### 1.3 Runner

- [ ] 实现 WorkflowRunner。
- [ ] 支持 queued step 拉取和 lease。
- [ ] 支持 system step 自动执行。
- [ ] 支持 agent step dispatch 和完成回调。
- [ ] 支持 human_review waiting。
- [ ] 支持 decision route。
- [ ] 支持 parallel fanout/join。
- [ ] 支持 instance succeeded/failed 判断。
- [ ] 支持 retry、compensation 和 cancel。

### 1.4 Node Adapter

- [ ] 实现 NodeAdapterRegistry。
- [ ] 实现 `input.capture` adapter。
- [ ] 实现 `skill.invoke` adapter。
- [ ] 实现 `capability.invoke` adapter。
- [ ] 实现 `metadata.classify` adapter。
- [ ] 实现 `knowledge.stage` adapter。
- [ ] 实现 `knowledge.publish` adapter。
- [ ] 实现 `event.emit` adapter。
- [ ] 实现 `compensation.rollback` adapter。

### 1.5 Human Review

- [ ] 创建审核任务。
- [ ] 支持 approve/reject/request_changes。
- [ ] 支持审核超时。
- [ ] 审核完成后唤醒 WorkflowRunner。
- [ ] 审核事件写入 WorkflowEvent。

### 1.6 Seed

- [ ] 增加 `backend/config/workflow_packs`。
- [ ] 增加 `expert_knowledge_capture` seed。
- [ ] 增加 `marketing_knowledge_capture` seed。
- [ ] 增加 `campaign_review_to_methodology` seed。
- [ ] 把 workflow seed 纳入 `make seed` 或专门 `make workflow-seed`，并由 `make seed` 调用。

## 2. 前端任务

### 2.1 API Client

- [ ] 把 workflow API baseUrl 从 `/workflow` 改为 `/admin/workflows`。
- [ ] 移除 `getKinds()` mock。
- [ ] 移除 `getPalette()` mock。
- [ ] 移除 workflow list mock。
- [ ] DTO 对齐后端 `WorkflowDefinition`、`WorkflowInstance`、`StepRecord`。
- [ ] API client 按 definitions、instances、nodeCatalog、reviewTasks、packs 分组。
- [ ] 前端 DTO 使用 `definition_uuid`、`instance_uuid`、`review_task_uuid`，不继续使用业务对象 `id`。

### 2.2 Builder

- [ ] 左侧节点目录改为 Node Catalog API。
- [ ] 节点属性面板按 schema 渲染。
- [ ] Skill 选择器接 Skill Registry。
- [ ] Capability 选择器接 Capability Registry。
- [ ] Knowledge Space 选择器接 Knowledge API。
- [ ] Metadata namespace 选择器接 Metadata API。
- [ ] 发布前显示依赖校验结果。
- [ ] 保存 draft 时只写 WorkflowDefinition，不创建 demo workflow。
- [ ] 运行按钮改为 `POST /instances`，不再只写 console。

### 2.3 Instance Monitor

- [ ] 实例列表。
- [ ] 实例详情。
- [ ] Step timeline。
- [ ] Retry / compensation 记录。
- [ ] Human Review 待办。
- [ ] 审计导出入口。
- [ ] 审核通过后显示 Runner 唤醒结果。
- [ ] 审核拒绝后显示终止或补偿路径。
- [ ] 新增 `/workflow/instances`、`/workflow/instances/:instance_uuid`。
- [ ] 新增 `/workflow/review-tasks`、`/workflow/review-tasks/:review_task_uuid`。

### 2.4 i18n

- [ ] 移除工作流页面硬编码中文。
- [ ] 补齐 zh/en locale。
- [ ] 节点名称、按钮、错误、状态都走 i18n。

## 3. 测试任务

- [ ] Service 单测：NodeAdapterRegistry。
- [ ] Service 单测：Runner route。
- [ ] Service 单测：Human Review。
- [ ] Service 单测：Knowledge publish 依赖 review。
- [ ] 集成测试：`expert_knowledge_capture` 完整链路。
- [ ] 集成测试：`marketing_knowledge_capture` 完整链路。
- [ ] HTTP 合同测试：definitions/instances/review/node-catalog。
- [ ] 前端测试：Workflow Builder 无 mock 数据。
- [ ] capability-check：Workflow routes 和 gRPC 能力已登记。

## 4. 验收命令

```bash
cd backend && go test ./internal/service/workflow ./tests/workflow/... -count=1
make capability-check
cd web-admin && npm run typecheck
cd web-admin && npm run test
```

## 5. 完成定义

只有同时满足以下条件，Workflow Runtime 才算可支撑 native-agent：

1. `marketing_knowledge_capture` 可以真实启动并推进到 Human Review。
2. Human Review approve 后可以发布到 Knowledge Space。
3. reject 后不会发布正式知识。
4. 所有节点都有 StepRecord、WorkflowEvent 和 trace_id。
5. Web Admin 不再使用 mock workflow 数据。
6. native-agent 启用时能校验 Workflow Pack 是否可执行。
