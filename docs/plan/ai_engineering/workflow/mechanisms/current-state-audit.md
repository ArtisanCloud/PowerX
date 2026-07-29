# Workflow 当前状态审计

## 1. 结论

PowerX 已经有 Workflow 基础设施，但还不是完整 Workflow Runtime。

当前代码能支撑：

- 创建 WorkflowDefinition。
- 发布 WorkflowDefinition。
- 创建 WorkflowInstance。
- 写入初始 StepRecord。
- 查询定义、实例和导出记录。
- 控制实例暂停、恢复、取消、重试、补偿。
- 记录 WorkflowEvent。

当前代码不能完整支撑：

- 自动推进完整 DAG。
- 执行 `skill.invoke` 节点。
- 执行 `capability.invoke` 节点。
- 执行 `metadata.classify` 节点。
- 执行 `knowledge.stage` 和 `knowledge.publish` 节点。
- 生成和处理 Human Review 任务。
- 从 Capability Registry 生成 Workflow Builder 节点目录。
- Web Admin 真实创建、编辑、发布、运行和监控 Workflow。

## 2. 后端已有实现

| 文件 | 状态 |
| --- | --- |
| `backend/internal/service/workflow/service.go` | 定义创建、发布、启动实例基础逻辑 |
| `backend/internal/service/workflow/validator.go` | 校验 step graph、step 类型、依赖和环 |
| `backend/internal/service/workflow/executor_*.go` | 基础 step executor |
| `backend/internal/service/workflow/service_control.go` | 实例控制、重试、补偿入口 |
| `backend/internal/service/workflow/assignment_tracker.go` | Agent 派发记录和超时处理 |
| `backend/internal/transport/http/admin/workflow/*` | Admin HTTP API |
| `backend/internal/transport/grpc/workflow/*` | gRPC API |
| `backend/config/platform_capabilities/workflow.yaml` | Workflow 底座 capability 声明 |

## 3. 后端缺口

### 3.1 Runner 缺失

`StartInstance` 当前会创建实例和起始 StepRecord，但没有完整 Runner 循环负责：

- 拉取 queued step。
- 调用对应 Node Adapter。
- 写入 payload_out。
- 根据执行结果计算 next steps。
- 推进分支和并行。
- 处理等待人工审核。
- 标记 instance succeeded/failed。

### 3.2 节点适配器缺失

现有 executor 主要负责校验和路由，不负责真实业务调用。native-agent 需要明确的 adapter：

- Skill Adapter。
- Capability Adapter。
- Knowledge Adapter。
- Metadata Adapter。
- Event Adapter。
- Human Review Adapter。
- Agent Adapter。

### 3.3 API 与前端不对齐

后端真实路由是：

```text
/api/v1/admin/workflows/definitions
/api/v1/admin/workflows/definitions/:definition_uuid
/api/v1/admin/workflows/definitions/:definition_uuid/publish
/api/v1/admin/workflows/instances
/api/v1/admin/workflows/instances/:instance_uuid
/api/v1/admin/workflows/instances/:instance_uuid/actions
/api/v1/admin/workflows/instances/export
```

当前代码里的 gin 参数名仍是 `definitionId`、`instanceId`，但目标契约必须按 UUID 语义收敛。实现时应改为 `definition_uuid`、`instance_uuid`，并拒绝 numeric id。

前端 `workflowService.ts` 当前使用 `/workflow` 作为 baseUrl，并包含 mock 的 kinds/palette/workflow list。这不是可上线实现。

## 4. 前端已有实现

| 文件 | 状态 |
| --- | --- |
| `web-admin/app/pages/workflow/index.vue` | 工作流列表页面，当前包含 mock 数据 |
| `web-admin/app/pages/workflow/workspace.vue` | 工作流编辑器页面 |
| `web-admin/app/components/workflow/WorkflowEditor.vue` | Vue Flow 画布编辑器 |
| `web-admin/app/composables/api/services/workflowService.ts` | API client 形状存在，但路径和 mock 未对齐 |
| `web-admin/app/types/workflow.ts` | 前端工作流类型存在，但与后端 StepDefinition 不一致 |

## 5. 风险

1. `specs/006-workflow-and-agent/tasks.md` 已全部标记完成，但实现不等于完整 native-agent 所需 runtime。
2. 如果直接开发 native-agent 页面，会绕开缺失的 Runner 和节点适配器。
3. 如果保留前端 mock，会让用户误以为 Workflow Builder 已可用。
4. 如果不建立节点目录，插件能力、Core 能力、Skill 和 Knowledge 操作会在每个业务场景中各写一套。

## 6. 处理原则

1. 以现有 006 基础为底座继续实现，不重写。
2. Runtime 不完整时，依赖 Workflow 的 Agent 不得启用。
3. 前端必须移除 mock，调用真实 Admin API 和节点目录 API。
4. 所有节点必须有明确 adapter、输入输出 schema、权限校验和审计字段。
