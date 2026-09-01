# Multi-Agent 场景规划

## 1. 规划目标

多 Agent 场景只用于一个人类用户难以靠单个智能体稳定完成的复合任务。它必须满足：

1. 有明确主目标。
2. 可拆成多个职责清晰的子任务。
3. 子任务之间有依赖或复核关系。
4. 最终结果需要主智能体统一汇总。
5. 每个子任务都能通过 Trace 追溯。

不满足以上条件时，优先使用单个 Native Agent 或 Workflow。

## 2. 场景选择规则

| 判断问题 | 适合形态 |
| --- | --- |
| 只是一次问答、摘要、改写 | 单个 Native Agent |
| 需要审核、发布、版本、知识库写入 | Workflow |
| 需要多个角色分工、并行或串行协作、最终汇总 | Multi-Agent |
| 需要多个 Agent 协作后再写知识库 | Multi-Agent + Workflow |

## 3. 场景结构模板

```text
场景名称
  主智能体：负责理解目标、拆分任务、汇总结果
  子智能体 A：负责检索、证据、历史上下文
  子智能体 B：负责执行方案、步骤、产物生成
  子智能体 C：负责复核、风险、遗漏检查
  输出：用户可直接使用的业务结果
  Trace：主任务、子任务、Skill 调用、最终汇总
```

## 4. 当前场景：发布准备协作

### 4.1 业务目标

用户输入一次发布准备请求，系统自动生成发布准备报告，覆盖风险、步骤、验证、回滚和通知计划。

### 4.2 团队组成

| 职责 | 中文名称 | 英文名称 | key | 角色 |
| --- | --- | --- | --- | --- |
| 主智能体 | 发布准备协调员 | Release Readiness Coordinator | `release.coordinator` | `planner` |
| 子智能体 | 发布知识分析员 | Release Knowledge Analyst | `release.knowledge_analyst` | `retriever` |
| 子智能体 | 发布流程规划员 | Release Process Planner | `release.workflow_planner` | `executor` |
| 子智能体 | 发布通知计划员 | Release Notification Planner | `release.notification_scheduler` | `reviewer` |

### 4.3 输入

```text
帮我准备 6 月 18 日 PowerX Core v0.9.2 的发布准备报告，重点检查 Agent Skill Bridge、插件安装、回滚风险，并生成通知计划。
```

### 4.4 输出

1. 发布风险摘要。
2. 发布流程。
3. 验证 checklist。
4. 回滚步骤。
5. 通知和值班升级计划。
6. 部分失败说明。

### 4.5 验收入口

1. 使用手册：`docs/guides/agent/native_agents/release-readiness-demo.md`
2. 验收剧本：`docs/guides/agent/multi_agent/09_a2a_team_collab_progressive.md`
3. 设计对齐：`docs/plan/ai_engineering/skills/multi_agent_a2a.md`

## 5. 后续业务场景候选

| 场景 | 主智能体 | 子智能体分工 | 输出 |
| --- | --- | --- | --- |
| 销售作战协作 | 销售负责人智能体 | 客户分析、方案生成、风险复核、跟进计划 | 客户作战计划 |
| 客服升级协作 | 客服主管智能体 | 问题归类、知识检索、工单建议、质量复核 | 升级处理建议 |
| 招聘评估协作 | 招聘负责人智能体 | JD 分析、候选人匹配、面试问题、风险复核 | 候选人评估报告 |
| 财务经营分析 | 经营分析智能体 | 指标抽取、异常分析、预算建议、管理摘要 | 经营分析报告 |

这些只是规划候选。未完成 seed、Skill、Team、使用文档和验收前，不应出现在最终用户可见入口中。

## 6. 场景落地检查表

| 项目 | 必填 |
| --- | --- |
| 业务目标 | 是 |
| 主智能体中文/英文名称 | 是 |
| 子智能体中文/英文名称 | 是 |
| Team key | 是 |
| 子任务职责 | 是 |
| 绑定 Skill | 是 |
| 输入样例 | 是 |
| 输出结构 | 是 |
| 失败策略 | 是 |
| Trace 字段 | 是 |
| 最终使用方文档 | 是 |
| 回归测试 | 是 |
