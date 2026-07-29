# 营销知识采集（marketing_knowledge_capture）

## 页面对应关系

| 项目 | 值 |
| --- | --- |
| Web Admin 显示名 | 营销知识采集 |
| workflow_key | `marketing_knowledge_capture` |
| i18n key | `workflow.pack.marketingKnowledgeCapture.name` |
| seed 文件 | `backend/config/workflow_packs/marketing/marketing_knowledge_capture.yaml` |
| 页面入口 | `/workflow` |
| 页面卡片标识 | 内置包 |

## 1. 这个 seed 解决什么问题

`marketing_knowledge_capture` 是营销领域的知识库迭代流程。

它处理营销负责人、内容团队、增长团队上传的录音、访谈、活动材料、文档、链接和复盘，将其中的营销经验沉淀为结构化方法论。

## 2. seed 文件

```text
backend/config/workflow_packs/marketing/marketing_knowledge_capture.yaml
```

## 3. 谁会用它

| 使用方 | 场景 |
| --- | --- |
| 营销负责人智能体 | 沉淀战略、定位、渠道、预算、增长经验。 |
| 内容营销智能体 | 沉淀选题、内容资产、脚本、复盘。 |
| 增长运营智能体 | 沉淀实验、漏斗、转化策略。 |
| 市场部方法论智能体 | 将个人经验审核后发布到部门库。 |

## 4. 前置对象

| 对象 | 示例 |
| --- | --- |
| Knowledge Space | `${knowledge_space_uuid}` |
| Skill | `marketing.audio_or_document_parse` |
| Skill | `marketing.extract_methodology` |
| Taxonomy namespace | `corex.marketing.methodology` |
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
  -> parse_source
  -> extract_marketing
  -> classify_metadata
  -> stage_knowledge
  -> conflict_check
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
| `capture_input` | `input.capture` | 接收营销材料。 | `$.artifacts.source` |
| `parse_source` | `skill.invoke` | 调用 `marketing.audio_or_document_parse`，解析录音、文档或链接。 | `$.vars.parsed` |
| `extract_marketing` | `skill.invoke` | 调用 `marketing.extract_methodology`，抽取营销方法论。 | `$.vars.extracted` |
| `classify_metadata` | `metadata.classify` | 按营销方法论分类、标签和字典治理。 | `$.vars.metadata` |
| `stage_knowledge` | `knowledge.stage` | 写入知识草稿。 | `$.vars.drafts` |
| `conflict_check` | `decision.gateway` | 判断是否需要审核，当前默认进审核。 | `review_knowledge` |
| `review_knowledge` | `human.review` | 审核营销知识草稿。 | `$.review` |
| `publish_knowledge` | `knowledge.publish` | 审核通过后发布到知识库。 | `$.vars.published` |
| `emit_published` | `event.emit` | 发布成功事件。 | `workflow.knowledge.published` |
| `emit_rejected` | `event.emit` | 审核拒绝事件。 | `workflow.knowledge.rejected` |

## 6. 需要填哪些占位值

| 占位符 | 必填 | 示例 |
| --- | --- | --- |
| `${knowledge_space_uuid}` | 是 | 市场部方法论知识库 UUID |

## 7. 产出知识对象示例

| 类型 | 示例 |
| --- | --- |
| Observation | 新品发布失败不是渠道问题，而是高意向客户识别不足。 |
| Principle | 高客单价产品发布前必须定义客户触发信号。 |
| Method | 高意向客户触发信号拆解法。 |
| SOP | 新品发布前 2 周完成高意向客户信号校准。 |
| Case | 2026 Q3 新品发布复盘。 |
| Evidence | 原始访谈录音和转写文本。 |

## 8. 怎么 seed

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":["marketing_knowledge_capture"]}'
```

## 9. 怎么验证

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/definitions?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

预期：

```json
{
  "workflow_pack_key": "marketing_knowledge_capture",
  "status": "published"
}
```

页面：

```text
/workflow -> 营销知识采集
```

## 10. Agent 怎么启动它

示例输入：

```json
{
  "knowledge_space_uuid": "<KNOWLEDGE_SPACE_UUID>",
  "source": {
    "type": "audio",
    "asset_uuid": "<MEDIA_ASSET_UUID>",
    "context": "营销总监访谈"
  }
}
```

## 11. 常见失败点

| 失败 | 原因 | 处理 |
| --- | --- | --- |
| 解析失败 | 音频或文档解析 Skill 未实现或未授权。 | 检查 `marketing.audio_or_document_parse`。 |
| 抽取结果太泛 | `marketing.extract_methodology` prompt 或 schema 不完整。 | 优化 Skill。 |
| 分类失败 | 营销 metadata namespace 没有 seed。 | 先 seed metadata governance。 |
| 发布失败 | `knowledge_space_uuid` 缺失或知识库权限不足。 | 检查知识库和权限。 |

## 12. 适合不适合

适合：

- 营销经验沉淀。
- 市场部方法论库。
- 内容和增长团队知识复用。

不适合：

- 单纯 Campaign 指标复盘，应使用 `campaign_review_to_methodology`。
- 通用专家知识库，应使用 `expert_knowledge_capture`。
