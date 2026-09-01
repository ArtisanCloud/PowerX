# Multi-Agent 工程化文档索引

## 定位

Multi-Agent 是 PowerX Agent Runtime 的协作运行方式，不是一个独立业务模块。它把多个 Native Agent 组织成 Agent Team，由主智能体规划任务、子智能体执行分工、主智能体汇总结果。

当前首个已落地业务场景是“发布准备协作团队”：

```text
发布准备协调员
  -> 发布知识分析员
  -> 发布流程规划员
  -> 发布通知计划员
  -> 发布准备报告汇总
```

## 文档分层

| 文档 | 面向对象 | 内容 |
| --- | --- | --- |
| [scenario-catalog.md](./scenario-catalog.md) | 产品、方案、研发 | 多 Agent 场景规划规则、当前场景和后续扩展模板。 |
| [implementation-map.md](./implementation-map.md) | 研发、QA | 当前实现映射：模型、seed、接口、页面、Trace、测试。 |
| [../skills/multi_agent_a2a.md](../skills/multi_agent_a2a.md) | 研发 | A2A 执行机制、上下文隔离、handoff plan 和测试策略。 |
| [../../../guides/agent/native_agents/release-readiness-demo.md](../../../guides/agent/native_agents/release-readiness-demo.md) | 最终使用方、实施、QA | 发布准备协作团队使用手册。 |
| [../../../guides/agent/multi_agent/09_a2a_team_collab_progressive.md](../../../guides/agent/multi_agent/09_a2a_team_collab_progressive.md) | QA、实施 | 发布准备多智能体验收剧本。 |

## 当前权威规则

1. Agent Team 是 Multi-Agent 的配置载体。
2. Team 必须有且只有一个主智能体，主智能体承担 `planner` 职责。
3. 子智能体角色固定为 `retriever/executor/reviewer`，不从业务数据字典动态扩展。
4. 真正可调用能力由 Agent-Skill Binding 和授权决定，Team role 只表达协作职责。
5. 子智能体不能默认继承完整会话，只能接收主智能体显式下发的上下文切片。
6. 失败策略只允许 `fail-fast`、`continue`、`retry-once`。
7. 没有可执行团队、模型、Skill、权限或上下文时必须显式失败，不做隐式降级。

## 新增业务场景时必须补齐

1. Native Agent seed：主智能体和子智能体的中文/英文名称、职责、分类、保护字段。
2. Skill seed 或已发布 Skill：每个子智能体可调用能力必须真实登记。
3. Agent Team seed：主智能体、成员、角色、失败策略。
4. 最终使用方文档：`docs/guides/agent/native_agents/<scenario>.md`。
5. QA 验收剧本：输入、预期输出、Trace、失败判定。
6. 实现映射：模型、接口、页面、测试命令。
