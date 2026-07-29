# PowerX Workflow Runtime 开发计划

## 1. 定位

Workflow Runtime 是 PowerX 原生智能体、知识库增量迭代、插件复合能力和人工审核流程的业务编排主干。

它不是 EventBus、Task Queue、Scheduler 这类底座 Flow 的替代品。底座 Flow 负责可靠投递、重试队列和系统事件；Workflow Runtime 负责租户业务流程的定义、运行、人工检查点、补偿、发布、审计和回滚。

native-agent 的知识库增量迭代必须依赖 Workflow Runtime，不提供 Skill 编排或 Task Queue 的替代执行链路。

## 2. 已有基础

仓库已有 `specs/006-workflow-and-agent` 和部分代码实现：

| 能力 | 当前状态 |
| --- | --- |
| WorkflowDefinition / WorkflowInstance 模型 | 已有 |
| StepRecord / Compensation / AgentAssignment / WorkflowEvent | 已有 |
| HTTP 管理端路由 `/admin/workflows/*` | 已有 |
| gRPC `powerx.workflow.v1.WorkflowService` | 已有 |
| 基础 StepType | 已有 `agent/system/decision/parallel/human_approval/compensation` |
| 定义创建、发布、实例启动 | 已有基础实现 |
| 重试、补偿、Assignment 跟踪 | 有基础实现 |
| Web Admin `/workflow` 页面 | 有早期页面和 Vue Flow 编辑器 |

## 3. 关键缺口

| 缺口 | 影响 |
| --- | --- |
| 缺少真实 Workflow Runner | 实例启动后不能完整自动推进 DAG |
| 缺少语义节点目录 | native-agent 需要的 `skill.invoke`、`knowledge.stage` 等无法标准化 |
| 缺少节点适配器 | Skill、Capability、Knowledge、Metadata、EventBus 没有统一执行入口 |
| 缺少 Human Review 任务模型 | `human_approval` 不能支撑审核任务、确认、驳回和审计 |
| 缺少 Workflow Pack seed | 没有 `expert_knowledge_capture`、`marketing_knowledge_capture` 等可复用流程 |
| 前端仍有 mock | `/workflow` 页面和 service 未对齐真实 `/admin/workflows` API |
| 节点目录未接 capability registry | Workflow Builder 无法从正式能力目录加载 Core/Plugin 节点 |

## 4. 目标链路

```text
Agent / Admin / Event
  -> Start WorkflowInstance
  -> Workflow Runner
  -> Node Adapter Registry
      -> input.capture
      -> skill.invoke
      -> capability.invoke
      -> metadata.classify
      -> knowledge.stage
      -> decision.gateway
      -> human.review
      -> knowledge.publish
      -> event.emit
      -> compensation.rollback
  -> StepRecord / WorkflowEvent / Audit / Trace
```

## 5. 文档结构

- `mechanisms/current-state-audit.md`：现有代码和页面覆盖度审计。
- `mechanisms/runtime-architecture.md`：Workflow Runtime 目标架构。
- `mechanisms/implementation-map.md`：基于现有 006 代码的后端、前端、seed、能力登记落点。
- `specs/node-catalog.md`：Workflow 语义节点目录和底层 StepType 映射。
- `specs/runtime-contract.md`：Runner、节点适配器、上下文和状态契约。
- `specs/admin-api-contract.md`：Admin HTTP、Node Catalog、Human Review、Workflow Pack 的目标接口。
- `specs/workflow-pack-seed.md`：内置 Workflow Pack seed 规格。
- `pages/workflow-builder.md`：Web Admin Workflow Builder 页面计划。
- `pages/frontend-implementation-adjustment.md`：现有 Web Admin workflow 页面、API client、类型和 i18n 的具体调整落点。
- `pages/human-review-and-monitor.md`：Workflow 实例监控与人工审核页面计划。
- `rules/workflow-governance-rules.md`：发布、权限、审计、失败处理规则。
- `tasks/implementation-tasks.md`：开发任务拆分和验收命令。

## 6. 实现顺序

1. 完成现状修正：让前端和后端 API 命名、DTO、状态、节点类型对齐。
2. 实现 Runner：支持实例自动推进、节点完成回调、分支、并行、等待和失败处理。
3. 实现 Node Adapter Registry：统一调度 Skill、Capability、Knowledge、Metadata、EventBus、Human Review。
4. 实现 Human Review：审核任务、确认、驳回、超时和审计。
5. 实现 Workflow Pack seed：内置专家知识库和营销知识库迭代流程。
6. 改造 Web Admin：移除 mock，接真实 `/admin/workflows` 和节点目录 API。
7. 接入 native-agent：Agent 来源快照绑定 Workflow Pack，启用前做 Workflow 依赖预检。

## 7. 不做的事

1. 不用 Skill + Task Queue 替代 Workflow 编排。
2. 不把 `/workflow` 前端 mock 当成可用产品能力。
3. 不允许缺节点适配器的 Workflow 发布为可执行。
4. 不允许 Workflow Builder 使用未登记、未授权的 Core/Plugin 能力。
5. 不保留旧路径 `/workflow/*` 作为后端 API 兼容别名；前端必须改到真实 Admin API client。
