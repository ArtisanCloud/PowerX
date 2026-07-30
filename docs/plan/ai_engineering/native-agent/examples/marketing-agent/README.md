# 营销智能体案例推演

## 1. 目标

本案例用“营销智能体”说明 native-agent 如何把通用知识工程骨架、内置智能体、行业扩展、租户自建 AgentInstance、Workflow、Skill、Knowledge Space 和 Metadata 组织起来。

这个案例不是要把所有营销岗位一次性固定死，而是给出一套可扩展拆法：PowerX 提供少量内置智能体和基础知识库结构，租户可以克隆、自建、重复创建、绑定不同知识库。模板包只是内置智能体背后的来源快照。

## 2. 营销智能体不是一个 Agent

营销域至少会出现三层对象：

```text
PowerX 内置职位智能体
  -> 租户营销 AgentInstance
  -> 个人/部门/项目/客户 owner-scoped 知识库
```

因此“营销智能体”在产品里不应只有一个固定实例，而应拆成几类常见 AgentInstance。

## 3. 建议内置智能体

PowerX 内置不需要覆盖所有营销职位，建议提供 4 个营销相关智能体：

| 内置智能体 | 类型 | 主要用途 | 默认 owner scope |
| --- | --- | --- | --- |
| 营销负责人智能体 | 职位智能体 | 战略、预算、渠道、复盘、跨团队协作 | department 或 user |
| 内容营销智能体 | 职位智能体 | 内容选题、内容资产、素材复用、内容复盘 | department |
| 增长运营智能体 | 职位智能体 | 增长实验、转化漏斗、活动策略、AB test 复盘 | project 或 department |
| 专家知识库策展智能体 | 横向基础智能体 | 把专家输入转化为结构化知识库 | user 或 department |

不建议内置过细职位，例如 CMO、市场总监、品牌总监、投放负责人、策划经理、短视频运营各自都做成固定内置智能体。它们应优先通过克隆、自建或行业扩展产生。

## 4. 租户里的实际 AgentInstance 示例

同一家公司可以同时存在多个营销相关 AgentInstance：

| AgentInstance | 来源 | owner scope | 绑定知识库 | 说明 |
| --- | --- | --- | --- | --- |
| 公司营销总监智能体 | 从“营销负责人智能体”克隆 | user | 张三个人营销方法论库 | 张三的个人数字分身 |
| 新任营销总监智能体 | 从同一内置智能体重新克隆 | user | 李四个人营销方法论库 | 张三离职后新建，不自动继承张三私有知识 |
| 市场部方法论智能体 | 从“营销负责人智能体”克隆 | department | 市场部方法论库 | 面向部门共享 |
| 内容营销智能体 | 从“内容营销智能体”克隆 | department | 内容资产库 | 管理内容选题、素材、复盘 |
| 618 活动增长智能体 | 从“增长运营智能体”克隆 | project | 618 Campaign 知识库 | 项目型，活动结束后可 retired |
| 零售门店营销智能体 | 营销负责人智能体 + 零售行业扩展 | department | 零售营销知识库 | 行业语义叠加 |
| 插件导入广告投放智能体 | plugin_agent | department | 投放实验知识库 | 由广告插件声明并同步 |

这说明“营销智能体会有几种”不应该由 PowerX 固定死。PowerX 内置的是智能体起点和推荐组合，租户实际运行的是 AgentInstance。

## 5. 默认对象绑定

以“营销负责人智能体”为例：

| 绑定对象 | 示例 |
| --- | --- |
| Skill Pack | `knowledge.ingestion.basic`、`knowledge.curate.methodology`、`marketing.strategy.extract`、`marketing.review.summarize` |
| Workflow Pack | `marketing_knowledge_capture`、`campaign_review_to_methodology` |
| Knowledge Space Profile | `marketing_strategy_library`、`department_methodology`、`expert_personal_knowledge` |
| Metadata Template | `marketing_methodology` |
| Capability Grant Preset | 文件解析、媒体转写、知识库写入、标签绑定、字典查询、Agent 审计查询 |
| Permission Preset | 营销管理员、市场部成员、个人 owner |

## 6. Workflow 引用

营销案例使用的 Workflow Pack 来自通用 Workflow seed 示例，不在本案例目录里单独维护。

参考：

```text
docs/plan/ai_engineering/native-agent/examples/workflow/README.md
```

营销案例默认引用：

| workflow_key | 用途 |
| --- | --- |
| `marketing_knowledge_capture` | 营销素材、访谈、录音、文档和经验沉淀为方法论知识。 |
| `campaign_review_to_methodology` | Campaign 复盘、指标解释、失败原因和优化动作沉淀为方法论。 |

## 7. 一次知识入库推演

场景：营销总监张三上传一段语音：“这次新品发布失败，不是渠道不行，而是前期没有定义清楚高意向客户的触发信号……”

正式 Workflow 流程：

```text
WorkflowInstance: marketing_knowledge_capture
  node: input.capture
  node: skill.invoke.audio_transcribe
  node: skill.invoke.marketing_extract
  node: metadata.classify
  node: knowledge.stage
  node: decision.gateway.conflict_detect
  node: human.review
  node: knowledge.publish
  node: event.emit
```

对应 Workflow Pack：

```text
backend/config/workflow_packs/marketing/marketing_knowledge_capture.yaml
```

关键节点：

| 节点 | node_kind | 说明 |
| --- | --- | --- |
| `capture_input` | `input.capture` | 接收文本、媒体、链接等来源材料。 |
| `parse_source` | `skill.invoke` | 调用 `marketing.audio_or_document_parse` 解析音频或文档。 |
| `extract_marketing` | `skill.invoke` | 调用 `marketing.extract_methodology` 抽取营销方法论。 |
| `classify_metadata` | `metadata.classify` | 使用 `corex.marketing.methodology`、`corex.marketing` 等命名空间分类。 |
| `stage_knowledge` | `knowledge.stage` | 将结果暂存为知识草稿。 |
| `conflict_check` | `decision.gateway` | 决策是否进入审核。当前默认进入审核。 |
| `review_knowledge` | `human.review` | 由 `knowledge_reviewer` 审核。 |
| `publish_knowledge` | `knowledge.publish` | 审核通过后发布到目标知识库。 |
| `emit_published` / `emit_rejected` | `event.emit` | 发送发布或拒绝事件。 |

## 8. 产出的知识对象

同一段录音不应只变成普通 chunk，而应拆成可治理知识对象：

| 知识对象 | 示例内容 |
| --- | --- |
| Observation | 新品发布失败的主要问题不是渠道覆盖，而是高意向客户识别不足 |
| Principle | 高客单价新品发布前必须先定义客户触发信号 |
| Method | 高意向客户触发信号拆解法 |
| Decision Rule | 如果用户连续查看定价页、案例页和售后保障页，进入高意向客户池 |
| SOP | 新品发布前 2 周完成高意向客户信号校准 |
| Case | 2026 Q3 新品发布复盘 |
| Evidence | 原始录音片段和转写文本引用 |

## 9. 分类和标签示例

营销 Metadata Template：

```text
taxonomy
  strategy
    positioning
    market_segment
    competitive_analysis
  tactic
    channel_strategy
    content_strategy
    conversion_strategy
    budget_strategy
  execution
    campaign_sop
    creative_asset
    launch_checklist
  review
    metric_interpretation
    failure_case
    optimization_action
```

标签：

| 标签组 | 示例标签 |
| --- | --- |
| channel | private_domain、paid_ads、seo、offline_event |
| funnel_stage | awareness、interest、consideration、conversion、retention |
| customer_segment | enterprise、smb、high_intent、existing_customer |
| asset_type | brief、sop、case、checklist、script |
| risk_level | low、medium、high |

## 10. 人员离职和交接

张三离职时：

1. `公司营销总监智能体` 进入 `read_only` 或 `retired`。
2. 张三个人知识库不自动转给李四。
3. 已发布到“市场部方法论库”的内容继续归部门使用。
4. 管理员可从张三知识库中选择部分内容 fork/import 到部门知识库。
5. 李四创建新的 `新任营销总监智能体`，绑定自己的个人知识库。

这保证“同一岗位”不会错误合并不同人的管理理念、经验和方法论。

## 11. 这个案例验证的设计点

1. 内置智能体只提供起点，其背后的模板包快照不固定企业真实岗位全集。
2. 同类营销智能体可以重复存在。
3. 个人数字分身和部门知识库必须隔离。
4. 知识库不是普通文件夹，而是结构化知识对象集合。
5. Workflow 是知识库迭代唯一主干，Skill 只能作为 Workflow 节点能力。
6. 插件垂类智能体必须进入同一套 AgentInstance 治理模型。
