# 营销知识结构示例

## 1. 分层结构

营销知识库使用通用知识工程骨架，并叠加营销专业结构。

```text
Source
  -> Observation
  -> Concept / Principle / Method
  -> SOP / Decision Rule
  -> Template / Case / FAQ
  -> Evidence
```

营销专业层级：

```text
战略层
  -> 定位
  -> 客群
  -> 市场机会
  -> 竞争格局

策略层
  -> 渠道策略
  -> 内容策略
  -> 转化策略
  -> 预算策略

执行层
  -> Campaign SOP
  -> 素材模板
  -> 投放检查清单
  -> 发布节奏

复盘层
  -> 指标解释
  -> 失败案例
  -> 优化动作
  -> 决策记录
```

## 2. Knowledge Space 拆分

推荐按 owner scope 拆，而不是按“营销”一个大库全部混在一起。

| Knowledge Space | owner scope | 用途 |
| --- | --- | --- |
| 张三个人营销方法论库 | user | 个人数字分身，保存张三个人经验 |
| 市场部方法论库 | department | 部门共识、SOP、复盘和标准 |
| 品牌资产库 | department | 品牌定位、话术、视觉规范、案例 |
| Campaign 项目库 | project | 单次活动过程材料、实验和复盘 |
| 零售营销行业库 | tenant 或 department | 行业扩展知识和术语 |

## 3. 知识对象字段建议

每条结构化知识至少包含：

- `knowledge_uuid`
- `tenant_uuid`
- `knowledge_space_uuid`
- `object_type`
- `title`
- `summary`
- `body`
- `taxonomy_node_uuid`
- `tags`
- `source_asset_uuid`
- `evidence_refs`
- `confidence`
- `review_status`
- `version`
- `created_by_agent_uuid`
- `approved_by_user_uuid`

## 4. 示例：新品发布复盘

原始输入：

```text
这次新品发布失败，不是渠道不行，而是前期没有定义清楚高意向客户的触发信号。
我们把预算平均铺到所有流量，但真正会购买的人其实已经在定价页、案例页、售后页反复停留。
下一次发布前，要先把这些信号定义出来，再让销售提前跟进。
```

结构化输出：

| 类型 | 标题 | 分类 |
| --- | --- | --- |
| Observation | 新品发布失败原因：高意向客户识别不足 | review.failure_case |
| Principle | 高客单价新品发布先定义客户触发信号 | strategy.market_segment |
| Method | 高意向客户触发信号拆解法 | tactic.conversion_strategy |
| Decision Rule | 连续访问定价页、案例页、售后页时进入高意向池 | tactic.conversion_strategy |
| SOP | 新品发布前 2 周完成客户触发信号校准 | execution.launch_checklist |

## 5. 审核规则

1. Observation 可以由 Agent 自动生成，但发布前必须保留 Evidence。
2. Principle、Method、Decision Rule 必须人工审核。
3. SOP 修改会影响执行流程，必须由 owner 或部门管理员审批。
4. Case 可以先发布到项目库，再经复盘后沉淀到部门方法论库。
5. 个人知识库内容不能自动发布到部门知识库。
