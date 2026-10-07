# 营销 AgentInstance 拆分示例

## 1. 拆分原则

营销域不要只有一个“营销智能体”。实际使用时应按职责、owner、知识边界和任务模式拆分 AgentInstance。

判断是否需要拆成不同 AgentInstance：

1. owner 不同：个人、部门、项目、客户。
2. 知识库不同：个人经验、部门方法论、Campaign 项目库、行业知识库。
3. 任务边界不同：战略分析、内容生产、增长实验、知识策展。
4. 权限不同：谁能看、谁能写、谁能发布。
5. 生命周期不同：长期岗位智能体、短期项目智能体、离职只读智能体。

## 2. 推荐内置智能体

| 内置智能体 | 默认用途 | 默认 Skill Pack |
| --- | --- | --- |
| 营销负责人智能体 | 战略、预算、渠道、复盘 | marketing.strategy.extract、marketing.review.summarize |
| 内容营销智能体 | 选题、内容资产、脚本、复用 | content.brief.generate、content.asset.classify |
| 增长运营智能体 | 漏斗、实验、转化、活动 | growth.experiment.summarize、campaign.metric.analyze |
| 专家知识库策展智能体 | 专家输入转结构化知识 | knowledge.ingestion.basic、knowledge.curate.methodology |

## 3. 租户实例拆分

| 实例 | 推荐来源 | owner | 生命周期 |
| --- | --- | --- | --- |
| 市场部方法论智能体 | 营销负责人智能体 | department | 长期 active |
| CMO 数字分身 | 营销负责人智能体 | user | 随人员生命周期变化 |
| 营销总监数字分身 | 营销负责人智能体 | user | 可与 CMO 并存 |
| 策划总监智能体 | 内容营销智能体 | user 或 department | 长期 active |
| 短视频内容智能体 | 内容营销智能体 | department | 长期 active |
| 私域增长智能体 | 增长运营智能体 | project 或 department | 按项目 active/retired |
| Campaign 复盘智能体 | 增长运营智能体 | project | 活动结束后 retired |

## 4. 重名处理

同一租户允许多个“营销总监智能体”，但必须通过 owner 和描述区分。

页面显示建议：

```text
营销总监智能体
张三 · 个人数字分身 · active

营销总监智能体
李四 · 个人数字分身 · draft

营销总监智能体
市场部 · 部门方法论 · active
```

API 和审计必须使用 `agent_uuid`，页面主标签不能显示 UUID。

## 5. 交接示例

张三离职，李四接任营销总监：

1. 张三的 AgentInstance 改为 `read_only`。
2. 张三个人知识库保持 user owner，不自动转移。
3. 部门管理员查看张三已发布知识，选择部分 fork/import 到市场部方法论库。
4. 李四从营销负责人智能体克隆新 AgentInstance。
5. 李四绑定自己的个人知识库，也可绑定市场部方法论库作为共享知识。

## 6. 插件导入示例

广告投放插件可以声明：

```text
广告投放优化智能体
  -> Skill: ad.keyword.analyze
  -> Skill: ad.budget.recommend
  -> Capability: com.powerx.plugins.ads.campaign.analyze
  -> Metadata Template: ads_campaign
```

安装插件后，PowerX 不直接运行插件原始 Agent，而是同步为 `provider_type=plugin` 的智能体来源快照，再由租户创建 AgentInstance。
