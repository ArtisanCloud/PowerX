# PowerX Core A2A 多智能体协作设计

本文定义 PowerX 底座自有的 A2A 多智能体协作机制。该机制不依赖 PowerXPlugin、MediaX、AI Craft 或任意插件 executor；插件能力可以后续作为某个子 Agent 绑定的 Skill 进入候选池，但不是本机制的 MVP 验收前提。

## 1. 机制定位

PowerX Core A2A 是 Agent Runtime 内部的多智能体任务编排机制：

```text
用户消息
  -> 主 Agent Session
  -> Intent / Planner
  -> ExecutionPlan
  -> agent_handoff 节点
  -> 子 Agent Runtime
  -> 子任务结果
  -> 主 Agent 汇总回复
```

它解决的问题是：一个复杂业务作业不应由单个 Agent 在一个 prompt 中一次性完成，而应由主 Agent 根据团队配置把任务拆给具备不同职责和绑定能力的子 Agent，并按计划依赖、失败策略、上下文隔离和审计 trace 汇总结果。

## 2. MVP 业务故事：发布准备多智能体作业

首个 Core-only MVP 采用“发布准备报告”场景，用于验证主 Agent、三个子 Agent、Skill 绑定、Team 成员、handoff plan、trace 与最终汇总。

用户输入：

```text
帮我准备 6 月 18 日 PowerX Core v0.9.2 的发布准备报告，
重点检查 Agent Skill Bridge、插件安装、回滚风险，并生成通知计划。
```

主 Agent：

- `release.coordinator`
- 职责：理解发布准备目标，拆分子任务，控制依赖顺序，汇总最终报告。
- 绑定 Skill：`powerx.release.report_synthesis`

子 Agent：

1. `release.knowledge_analyst`
   - 职责：分析发布相关知识、历史风险、变更摘要。
   - 绑定 Skill：`powerx.release.knowledge_analysis`
   - 输出：风险摘要、关注模块、证据引用摘要。
2. `release.workflow_planner`
   - 职责：根据风险分析生成发布流程、验证清单、回滚步骤。
   - 绑定 Skill：`powerx.release.workflow_planning`
   - 输出：发布步骤、验证 checklist、rollback plan。
3. `release.notification_scheduler`
   - 职责：生成发布通知计划和值班提醒安排。
   - 绑定 Skill：`powerx.release.notification_schedule`
   - 输出：通知对象、发送时间、提醒计划、升级路径。

Agent Team：

- `release.readiness.team`（落库字段建议使用 `team_name`）
- TL：`release.coordinator`
- Members：三个子 Agent
- 角色枚举：平台固定协作角色，集中定义为 `planner/retriever/executor/reviewer`；不开放租户或用户自定义维护。
- 默认失败策略：`continue`
- 默认上下文策略：只传主 Agent 显式下发的任务输入、风险摘要引用和 release metadata。

## 3. Seed 数据策略

该 MVP 的 Agent、Skill、Binding、Team 必须由 PowerX Core seed 初始化到数据库，禁止只写在测试内存中，也禁止依赖插件同步。

Seed 必须幂等，重复执行不得报错，语义为 upsert：

```text
make seed
  -> upsert agents
  -> upsert skills_registry_records
  -> upsert agent_skill_bindings
  -> upsert agent_teams
  -> upsert agent_team_members
```

建议 seed 对象：

| 类型 | Key / ID | 说明 |
| --- | --- | --- |
| Agent | `release.coordinator` | 发布准备主 Agent |
| Agent | `release.knowledge_analyst` | 知识与风险分析子 Agent |
| Agent | `release.workflow_planner` | 发布流程规划子 Agent |
| Agent | `release.notification_scheduler` | 通知调度子 Agent |
| Skill | `powerx.release.knowledge_analysis` | 平台内置 demo Skill，知识分析 |
| Skill | `powerx.release.workflow_planning` | 平台内置 demo Skill，流程规划 |
| Skill | `powerx.release.notification_schedule` | 平台内置 demo Skill，通知计划 |
| Skill | `powerx.release.report_synthesis` | 平台内置 demo Skill，主 Agent 汇总 |
| Team | `release.readiness.team` | 发布准备协作团队，落库字段使用 `team_name` |

Team Role 约束：

1. 角色是平台固定枚举，不作为数据库字典表，也不提供用户自定义维护入口。
2. 枚举必须集中定义，禁止在 Service、Handler、前端页面散落字符串判断。
3. `planner` 只用于 TL/主 Agent 职责语义，不允许作为子 Agent team member role。
4. 子 Agent 允许角色固定为 `retriever/executor/reviewer`。
5. Role 只表达团队职责与 Planner prompt 语义；真正可调用能力仍以 `agent_skill_bindings` 为准。

Skill Registry 约束：

1. `source=builtin` 或 `source=demo_builtin`，不得标记为 `plugin`。
2. `status=published`。
3. `is_latest_published=true`。
4. 必须有完整 metadata、description、input_schema、output_schema、intent_examples。
5. 可先使用 deterministic executor 或 test handoff invoker，但治理态记录必须真实落库。

Agent-Skill Binding 约束：

1. 子 Agent 只能默认看到自己绑定的 Skill。
2. 主 Agent 只能默认看到汇总 Skill 与团队 handoff 能力。
3. Runtime 构建候选池时必须按当前 `agent_id` 收敛，不得把全局 system/public Skill 暴露成当前 Agent 能力。

## 4. ExecutionPlan MVP

MVP 阶段可以先显式构造 `ExecutionPlan`，不要求自然语言自动稳定拆分为三个子 Agent。这样可以先验证底座执行语义，再推进 Team-aware Planner。

建议计划：

```json
{
  "plan_id": "release_readiness_multi_agent_mvp",
  "tasks": [
    {
      "task_id": "knowledge_analysis",
      "node_kind": "agent_handoff",
      "node_ref": "release.knowledge_analyst",
      "params": {
        "team_name": "release.readiness.team",
        "child_agent_key": "release.knowledge_analyst",
        "message": "分析 PowerX Core v0.9.2 发布相关知识、历史风险和 Agent Skill Bridge / 插件安装风险。",
        "failure_policy": "continue"
      }
    },
    {
      "task_id": "workflow_planning",
      "node_kind": "agent_handoff",
      "node_ref": "release.workflow_planner",
      "depends_on": ["knowledge_analysis"],
      "params": {
        "team_name": "release.readiness.team",
        "child_agent_key": "release.workflow_planner",
        "message": "基于风险分析生成发布流程、验证清单和回滚步骤。",
        "failure_policy": "continue"
      }
    },
    {
      "task_id": "notification_schedule",
      "node_kind": "agent_handoff",
      "node_ref": "release.notification_scheduler",
      "depends_on": ["workflow_planning"],
      "params": {
        "team_name": "release.readiness.team",
        "child_agent_key": "release.notification_scheduler",
        "message": "根据发布流程生成通知计划、提醒计划和值班升级路径。",
        "failure_policy": "continue"
      }
    },
    {
      "task_id": "report_synthesis",
      "node_kind": "skill",
      "node_ref": "powerx.release.report_synthesis",
      "depends_on": ["knowledge_analysis", "workflow_planning", "notification_schedule"],
      "params": {
        "failure_policy": "fail-fast"
      }
    }
  ]
}
```

## 5. 上下文隔离

主 Agent 分发给子 Agent 的输入必须是结构化上下文切片，不得默认复制完整会话历史。

允许传递：

1. `release_name`
2. `release_date`
3. `focus_areas`
4. `parent_message_id`
5. `context_refs[]`
6. 上游子任务的输出摘要

禁止默认传递：

1. 完整 session messages。
2. 未脱敏 prompt。
3. 其他子 Agent 私有执行 payload。
4. 未授权 Skill/Tooling 候选清单。

## 6. Trace 与报告

A2A 每个 handoff 节点必须写入 Agent Run Trace，并能与 SkillExecutionTrace / Capability InvocationTrace 关联。

必填字段：

```text
tenant_uuid
session_id
message_id
run_id
trace_id
plan_id
node_id
node_kind=agent_handoff
team_id
team_name
parent_agent_id
parent_agent_key
child_agent_id
child_agent_key
handoff_task_id
failure_policy
status
duration_ms
```

root 下载 Message 报告时必须能看到：

1. 主 Agent 输入。
2. A2A plan。
3. 三个子 Agent 的 handoff 输入摘要。
4. 三个子 Agent 的输出摘要或失败原因。
5. 主 Agent 最终发布准备报告。

## 7. 测试策略

Core-only MVP 测试必须满足：

1. 不启动 PowerXPlugin。
2. 不安装 AI Craft / MediaX。
3. 不调用插件 executor。
4. 从 seed 数据读取 Agent、Skill、Team。
5. 显式构造包含三个 `agent_handoff` 节点的 plan。
6. 注入 deterministic handoff invoker 或使用内置 demo executor，保证测试结果稳定。

建议测试文件：

```text
backend/tests/integration/skills/skill_agent_a2a_release_readiness_mvp_test.go
```

断言：

1. Seed 后四个 Agent 存在。
2. 四个 Skill 均为 latest published。
3. Team 成员关系正确。
4. 三个 handoff 节点均被调用。
5. 每个子 Agent 只收到自己的上下文切片。
6. 最终汇总包含风险摘要、发布流程、回滚步骤、通知计划。
7. `continue` 策略下单个子 Agent 失败时，最终报告标注部分失败而不是假装全成功。
8. Trace 中可按 `team_id/team_name/handoff_task_id/child_agent_id` 检索。

建议回归命令：

```bash
cd backend && go test ./tests/integration/skills \
  -run 'TestSkillAgentA2AReleaseReadinessSeed|TestSkillAgentA2AReleaseReadinessMVP|TestSkillAgentA2AReleaseReadinessPartialFailure' \
  -count=1
```

## 8. 后续产品化

MVP 通过后再推进以下能力：

1. Bootstrap 正式接线 `AgentHandoffInvoker`。
2. Team-aware Planner：Planner prompt 中注入当前主 Agent 的 team members、角色、可用 Skill 摘要。
3. 自然语言自动拆分：从用户发布请求自动生成 A2A plan。
4. Web Admin Team 调试页显示 handoff 过程。
5. Agent Trace 页面增加 A2A 过滤器和跳转。
6. 节点级模型策略扩展到子 Agent：主 Agent 可用大模型规划，子 Agent 可用小模型抽取或执行。
