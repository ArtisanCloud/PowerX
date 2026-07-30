# Workflow Runtime 架构

## 1. 总体架构

```text
Trigger
  -> WorkflowService.StartInstance
  -> WorkflowInstance
  -> WorkflowRunner
  -> NodeAdapterRegistry
  -> StepRecord / Context / Event / Audit
```

Trigger 可以来自：

- Admin 手动启动。
- Agent 计划节点。
- EventBus 事件。
- Scheduler 定时任务。
- Plugin delegated 调用。

## 2. 核心组件

| 组件 | 职责 |
| --- | --- |
| WorkflowDefinition Service | 创建、校验、发布不可变定义版本 |
| WorkflowInstance Service | 创建实例、查询实例、控制实例 |
| WorkflowRunner | 自动推进实例和步骤 |
| NodeAdapterRegistry | 根据 `node_kind` 找到执行适配器 |
| NodeAdapter | 执行单个语义节点 |
| ContextStore | 管理 input/output/runtime variables |
| HumanReview Service | 创建审核任务、确认、驳回、超时 |
| Compensation Service | 逆序补偿和人工补偿 |
| WorkflowEvent Emitter | 发出状态事件和审计投影 |

## 3. Runner 责任

Runner 必须负责：

1. 选择可运行的 queued step。
2. 校验实例状态和租户边界。
3. 构造节点输入上下文。
4. 调用 NodeAdapter。
5. 写入 StepRecord。
6. 根据结果计算 next steps。
7. 处理 waiting、retry、failed、compensating、succeeded 状态。
8. 发出 WorkflowEvent。

Runner 不负责：

1. 绕过 NodeAdapter 直接调用业务服务。
2. 绕过 Capability Registry 调用插件或 Core 能力。
3. 直接把草稿发布到正式知识库。
4. 自动合并个人知识库和部门知识库。

## 4. Node Adapter Registry

Adapter 注册键使用 `node_kind`：

```text
input.capture
skill.invoke
capability.invoke
metadata.classify
knowledge.stage
decision.gateway
parallel.fanout
parallel.join
human.review
knowledge.publish
event.emit
compensation.rollback
```

每个 adapter 必须声明：

- `node_kind`
- `input_schema`
- `output_schema`
- `required_permissions`
- `required_capabilities`
- `idempotency_policy`
- `retry_policy_support`
- `compensation_support`

## 5. 上下文模型

Workflow 运行上下文分为：

| 区域 | 用途 |
| --- | --- |
| `input` | StartInstance 传入的原始参数 |
| `vars` | 节点之间传递的结构化变量 |
| `artifacts` | source asset、draft、bundle、report 等对象引用 |
| `review` | 人工审核任务和结果 |
| `permissions` | 本次运行使用的授权快照 |
| `trace` | trace_id、agent_uuid、operator_user_uuid |

节点只能读取声明的上下文路径，只能写入声明的输出路径。

## 6. 状态推进

WorkflowInstance 状态：

```text
draft
running
waiting
suspended
succeeded
failed
compensating
compensated
canceled
compensation_failed
```

StepRecord 状态：

```text
queued
in_progress
waiting
completed
failed
compensating
compensated
skipped
```

## 7. 与 Core Flow 的边界

Core Flow 可以用于：

- Runner 任务投递。
- 延迟重试。
- 定时触发。
- EventBus 状态广播。

Core Flow 不能替代：

- WorkflowDefinition。
- WorkflowInstance。
- Human Review。
- Knowledge publish 审核链。
- 节点级审计和版本回滚。

## 8. 与 Agent 的关系

Agent 可以：

- 启动 Workflow。
- 作为 `agent.invoke` 或 `skill.invoke` 的上下文来源。
- 汇总 Workflow 结果。

Agent 不应该：

- 自己维护长流程状态。
- 直接发布知识库。
- 绕过 Workflow 的人工审核节点。

## 9. 与 Capability 的关系

Capability 是执行能力，不是 Workflow 本身。

Workflow 节点可以通过 Capability Adapter 调用：

- PowerX Core capability。
- Plugin capability。
- 外部集成 capability。

调用前必须校验：

- capability 已登记。
- tenant 已授权。
- agent/user/role 满足权限。
- method/protocol/input schema 匹配。
