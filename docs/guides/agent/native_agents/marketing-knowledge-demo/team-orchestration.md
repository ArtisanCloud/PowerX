# 团队编排配置与验收

本页说明如何让一个团队真正可以执行。团队名称、成员显示名和固有智能体 Key 都不是运行时路由条件；唯一权威条件是团队保存的编排图、成员角色和实际 Skill 绑定。

团队身份分为三层：`uuid` 用于跨域引用，`team_key` 用于稳定机器配置，`display_name_i18n` 用于界面显示。名称必须提供 `zh-CN`、`en-US`、`ja`、`ko` 四种语言；缺少当前语言名称时界面明确提示配置缺失，绝不把 Key 当作名称显示。

## 升级既有数据库

Schema migration 与 Seed 是两个独立操作，不能互相隐式触发。升级到本契约时必须按顺序执行：

```bash
make db-migrate
make seed
```

第一步将旧列 `team_name` 重命名为 `team_key` 并新增 `display_name_i18n`；第二步只负责更新固有团队和其他种子数据。若跳过迁移直接运行 Seed，数据库会因缺少 `team_key` 明确失败。

## 配置顺序

```text
选择主智能体
  -> 添加并启用子智能体成员
  -> 为每个成员绑定可调用 Skill
  -> 保存团队编排图
  -> 启用团队
  -> 在团队任务页发送业务材料
```

团队主智能体承担 `planner` 责任，子智能体只能承担 `retriever`、`executor` 或 `reviewer`。角色不是能力授权；每个任务还必须明确 `skill_id`，并由运行时验证该智能体确实绑定了这个 Skill。

每个被编排的 `skill_id` 还必须有一个已发布的 Skill Manifest，并声明两段正式 Capability 合同：`executor.prepare_capability` 和 `executor.capability`。前者只校验并准备输入，后者才执行业务 Skill；Capability ID 由 Manifest 明确声明（营销 Demo 为 `com.corex.marketing.*`），不能由团队或 Skill ID 拼接推导。二者都必须在正式 Capability 目录中登记且可由该智能体授权调用。团队运行时不会因为 Manifest 缺失、未发布或 Capability 缺失而改由主智能体/模型直答，而是将该节点和整轮 `fail-fast` 运行明确标为失败。

## 编排图

当前格式为 `powerx.agent.team-orchestration/v1`。它描述“谁用哪个技能、依赖谁的输出、失败如何处理”，不描述业务文案。

```json
{
  "schema": "powerx.agent.team-orchestration/v1",
  "tasks": [
    {
      "task_id": "source_analysis",
      "node_kind": "agent_handoff",
      "assignee_role": "retriever",
      "skill_id": "marketing.audio_or_document_parse",
      "stage": 1,
      "failure_policy": "fail-fast"
    },
    {
      "task_id": "campaign_analysis",
      "node_kind": "agent_handoff",
      "assignee_role": "executor",
      "skill_id": "marketing.metric_extract",
      "stage": 1,
      "failure_policy": "fail-fast"
    },
    {
      "task_id": "knowledge_curation",
      "node_kind": "agent_handoff",
      "assignee_role": "reviewer",
      "skill_id": "marketing.extract_methodology",
      "stage": 2,
      "depends_on": ["source_analysis", "campaign_analysis"],
      "failure_policy": "fail-fast"
    },
    {
      "task_id": "campaign_review_synthesis",
      "node_kind": "skill",
      "assignee_role": "planner",
      "skill_id": "marketing.review_summarize",
      "stage": 3,
      "depends_on": ["source_analysis", "campaign_analysis", "knowledge_curation"],
      "failure_policy": "fail-fast"
    }
  ]
}
```

`depends_on` 的上游业务结果会以 `upstream_<task_id>` 传给下游任务，对应引用为 `{{task.<task_id>.output.result}}`。它不会把其他智能体的完整会话或未授权候选技能传过去。

## 营销活动复盘 Demo

Seed 的“营销活动复盘协作团队”使用上面的四步：内容营销解析原始材料，活动复盘分析员计算目标和漏斗指标，知识策展员基于这两项结果提炼事实、假设与行动，最后由负责人汇总。最终汇总严格要求三个 `upstream_*` 产物；缺一个即失败，绝不降级成原文摘要。输入应给出真实或模拟的活动材料，例如：

```text
活动：夏季新品预热；目标：收集 SQL；周期：6 月 1–14 日。
渠道数据：曝光 120000、点击 4800、表单 260、SQL 31、成交 4。
已知变化：第 2 周落地页将表单从 5 项改为 8 项；投放素材没有 A/B 记录。
请输出：关键漏斗问题、证据与假设、下周可验证动作、负责人和优先级。
```

验收不是“每个节点变绿”而是同时满足：

1. Trace 显示四个配置任务及依赖关系，不出现 `base_flow` 单模型直答。
2. 每个 handoff 的 `child_skill_id` 与该成员的绑定一致。
3. 汇总任务收到三个 `upstream_*` 输出，并在回复中明确区分事实、假设和待验证项；至少输出一个由输入数据计算出的指标或明确的数据缺口。
4. 删除成员、解绑 Skill、改错依赖或停用团队后，运行明确失败；不得静默改由团队负责人直接回答。

## 排障

| 现象 | 原因 | 操作 |
| --- | --- | --- |
| 团队不能启用 | 没有编排图或编排图无效 | 补齐 `schema`、任务、技能、依赖；消除重复或循环任务 ID。 |
| Trace 在响应规划后失败 | 当前团队成员/技能已变更，和保存的图不匹配 | 恢复成员角色与 Skill 绑定，或更新图后重新执行。 |
| `missing executor.prepare_capability` | Skill 只有目录或绑定，没有完整执行 Manifest | 补齐并发布该 Skill 的 `executor.prepare_capability` 与 execute Capability；重新执行 `make seed`。 |
| `manifest is not published or empty` | 当前 Skill 没有可选的 published 版本 | 发布完整 Manifest 后重新 seed；不得用未登记的模型调用替代该 Skill。 |
| 看到了 `base_flow` | 不是团队入口，或部署仍是旧版本 | 确认团队任务页选中的是已启用团队；重启后端并查看该条消息 Trace。 |
| 结果没有引用上游结论 | 技能执行器没有消费 `upstream_*` | 修复该业务 Skill 的输入契约；这不是 Runtime 的回退条件。 |

底层字段、接口和严格校验见 [A2A 规格的数据模型](../../../../../specs/024-ai-engineering-skills/data-model.md#agentteamorchestration)。
