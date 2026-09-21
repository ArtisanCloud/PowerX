# 活动复盘沉淀（campaign_review_to_methodology）

## 页面对应关系

| 项目 | 值 |
| --- | --- |
| Web Admin 显示名 | 活动复盘沉淀 |
| workflow_key | `campaign_review_to_methodology` |
| i18n key | `workflow.pack.campaignReviewToMethodology.name` |
| seed 文件 | `backend/config/workflow_packs/marketing/campaign_review_to_methodology.yaml` |
| 页面入口 | `/workflow` |
| 页面卡片标识 | 内置包 |

## 1. 这个 seed 解决什么问题

`campaign_review_to_methodology` 用来把活动复盘转成可复用方法论。

活动结束后，团队通常有投放数据、会议纪要、复盘文档、素材表现、渠道表现。这个 Workflow 先抽取指标，再总结复盘，再分类、审核、发布到知识库。

## 2. seed 文件

```text
backend/config/workflow_packs/marketing/campaign_review_to_methodology.yaml
```

## 3. 谁会用它

| 使用方 | 场景 |
| --- | --- |
| 增长运营智能体 | 复盘增长实验、活动转化、漏斗问题。 |
| 营销负责人智能体 | 将活动经验沉淀为团队方法论。 |
| 项目型活动智能体 | 例如 618、双 11、新品发布活动复盘。 |
| 内容营销智能体 | 分析素材表现和内容策略。 |

## 4. 前置对象

| 对象 | 示例 |
| --- | --- |
| Knowledge Space | `${knowledge_space_uuid}` |
| Skill | `marketing.metric_extract` |
| Skill | `marketing.review_summarize` |
| Taxonomy namespace | `corex.marketing.campaign_review` |
| Tag namespace | `corex.marketing` |
| Dictionary namespace | `corex.marketing` |
| Resource Type namespace | `corex.knowledge` |
| 审核角色 | `knowledge_reviewer` |

依赖能力：

```text
com.corex.knowledge.space
com.corex.metadata.taxonomy.read
com.corex.metadata.dictionary.read
com.corex.metadata.tag.manage
com.corex.metadata.resource_type.read
```

## 5. 节点一步步做什么

```text
capture_input
  -> extract_metrics
  -> summarize_review
  -> classify_metadata
  -> stage_knowledge
  -> review_knowledge
  -> publish_knowledge
  -> emit_published
```

拒绝分支：

```text
review_knowledge
  -> emit_rejected
```

| 步骤 | node_kind | 做什么 | 输出 |
| --- | --- | --- | --- |
| `capture_input` | `input.capture` | 接收活动复盘材料、报表、会议纪要。 | `$.artifacts.source` |
| `extract_metrics` | `skill.invoke` | 调用 `marketing.metric_extract`，抽取曝光、点击、转化、成本等指标。 | `$.vars.metrics` |
| `summarize_review` | `skill.invoke` | 调用 `marketing.review_summarize`，总结问题、结论和优化动作。 | `$.vars.extracted` |
| `classify_metadata` | `metadata.classify` | 按活动复盘 taxonomy 和 tag 分类。 | `$.vars.metadata` |
| `stage_knowledge` | `knowledge.stage` | 将复盘结论写成知识草稿。 | `$.vars.drafts` |
| `review_knowledge` | `human.review` | 让审核人确认复盘是否能发布为方法论。 | `$.review` |
| `publish_knowledge` | `knowledge.publish` | 发布到目标知识库。 | `$.vars.published` |
| `emit_published` | `event.emit` | 发布成功事件。 | `workflow.knowledge.published` |
| `emit_rejected` | `event.emit` | 审核拒绝事件。 | `workflow.knowledge.rejected` |

## 6. 需要填哪些占位值

| 占位符 | 必填 | 示例 |
| --- | --- | --- |
| `${knowledge_space_uuid}` | 是 | 618 活动知识库 UUID |

## 7. 产出知识对象示例

| 类型 | 示例 |
| --- | --- |
| 指标解释 | CTR 高但转化低，说明落地页承接弱。 |
| 失败案例 | 首屏没有明确高意向客户触发信号。 |
| 优化动作 | 下一轮活动前完成分层落地页和高意向信号校准。 |
| 方法论 | 活动复盘必须拆成目标、渠道、素材、转化、复购五层。 |
| SOP | 活动结束 48 小时内完成指标归因和素材复盘。 |

## 8. 怎么 seed

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":["campaign_review_to_methodology"]}'
```

## 9. 怎么验证

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/definitions?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

预期：

```json
{
  "workflow_pack_key": "campaign_review_to_methodology",
  "status": "published"
}
```

页面：

```text
/workflow -> 活动复盘沉淀
```

## 10. Agent 怎么启动它

示例输入：

```json
{
  "knowledge_space_uuid": "<KNOWLEDGE_SPACE_UUID>",
  "campaign": {
    "name": "2026 618 活动",
    "report_asset_uuid": "<REPORT_ASSET_UUID>",
    "meeting_note_asset_uuid": "<NOTE_ASSET_UUID>"
  }
}
```

## 11. 常见失败点

| 失败 | 原因 | 处理 |
| --- | --- | --- |
| 指标抽取失败 | 报表结构不支持或 Skill 未实现。 | 检查 `marketing.metric_extract`。 |
| 复盘总结空泛 | 输入缺上下文或 Skill schema 不完整。 | 补充活动目标、渠道、预算、素材信息。 |
| 分类失败 | `corex.marketing.campaign_review` namespace 缺失。 | 先 seed metadata。 |
| 发布失败 | 知识库 UUID 错误或权限不足。 | 检查 Knowledge Space 和 RBAC。 |

## 12. 适合不适合

适合：

- Campaign 复盘。
- 增长实验复盘。
- 投放和内容素材效果总结。

不适合：

- 泛营销经验日常沉淀，应使用 `marketing_knowledge_capture`。
- 通用专家方法论沉淀，应使用 `expert_knowledge_capture`。
