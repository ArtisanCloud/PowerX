# 专家知识采集（expert_knowledge_capture）

## 页面对应关系

| 项目 | 值 |
| --- | --- |
| Web Admin 显示名 | 专家知识采集 |
| workflow_key | `expert_knowledge_capture` |
| i18n key | `workflow.pack.expertKnowledgeCapture.name` |
| seed 文件 | `backend/config/workflow_packs/knowledge/expert_knowledge_capture.yaml` |
| 页面入口 | `/workflow` |
| 页面卡片标识 | 内置包 |

## 1. 这个 seed 解决什么问题

`expert_knowledge_capture` 用来做“专家知识库持续迭代”。

典型场景是某个专家、顾问、负责人每天上传录音、文字、图片、链接、会议纪要。系统不能只把这些材料切成普通 chunk，而要把其中的经验、原则、方法、案例和证据抽取出来，分类、审核后发布到某个知识库。

## 2. seed 文件

```text
backend/config/workflow_packs/knowledge/expert_knowledge_capture.yaml
```

## 3. 谁会用它

| 使用方 | 场景 |
| --- | --- |
| 专家知识库策展智能体 | 把专家输入转成结构化知识。 |
| 个人数字分身 Agent | 沉淀某个人自己的方法论和经验。 |
| 部门知识库 Agent | 把多个成员的经验审核后发布到部门库。 |
| 客户归属知识库 Agent | 针对某个客户积累服务经验、诊断结论和方案。 |

## 4. 前置对象

必须准备：

| 对象 | 示例 | 说明 |
| --- | --- | --- |
| Knowledge Space | `${knowledge_space_uuid}` | 目标知识库。必须使用 UUID。 |
| Skill | `knowledge.parse_source` | 解析来源材料。 |
| Skill | `knowledge.extract_expert_methodology` | 抽取专家方法论。 |
| Taxonomy namespace | `corex.knowledge.expert` | 专家知识分类。 |
| Tag namespace | `corex.knowledge.expert` | 专家知识标签。 |
| Dictionary namespace | `corex.knowledge.expert` | 专家知识字典。 |
| Resource Type namespace | `corex.knowledge` | 知识资源类型。 |
| 审核角色 | `knowledge_reviewer` | 审核知识草稿。 |

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
  -> extract_knowledge
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
| `capture_input` | `input.capture` | 接收专家上传的媒体、文本、链接。 | `$.artifacts.source` |
| `parse_source` | `skill.invoke` | 调用 `knowledge.parse_source`，把音频、文档、链接转为可分析内容。 | `$.vars.parsed` |
| `extract_knowledge` | `skill.invoke` | 调用 `knowledge.extract_expert_methodology`，抽取原则、方法、案例、证据。 | `$.vars.extracted` |
| `classify_metadata` | `metadata.classify` | 使用专家知识命名空间分类、打标签、补字典属性。 | `$.vars.metadata` |
| `stage_knowledge` | `knowledge.stage` | 将结果写成知识草稿，不直接发布。 | `$.vars.drafts` |
| `conflict_check` | `decision.gateway` | 检查草稿是否需要审核。当前默认进审核。 | 路由到 `review_knowledge` |
| `review_knowledge` | `human.review` | 知识审核员确认草稿是否可发布。 | `$.review` |
| `publish_knowledge` | `knowledge.publish` | 审核通过后发布到 `${knowledge_space_uuid}`。 | `$.vars.published` |
| `emit_published` | `event.emit` | 发布成功事件。 | `workflow.knowledge.published` |
| `emit_rejected` | `event.emit` | 审核拒绝事件。 | `workflow.knowledge.rejected` |

## 6. 需要填哪些占位值

| 占位符 | 必填 | 示例 |
| --- | --- | --- |
| `${knowledge_space_uuid}` | 是 | `8f3e...` |

## 7. 这条流程最终产出什么

不是只产出 chunk，而是建议产出这些知识对象：

| 对象 | 示例 |
| --- | --- |
| Observation | 专家对某个现象的判断。 |
| Principle | 可复用原则。 |
| Method | 可执行方法。 |
| SOP | 操作步骤。 |
| Case | 案例和复盘。 |
| Evidence | 原始录音、图片、文档引用。 |

## 8. 怎么 seed

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":["expert_knowledge_capture"]}'
```

## 9. 怎么验证

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/definitions?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

预期：

```json
{
  "workflow_pack_key": "expert_knowledge_capture",
  "status": "published"
}
```

页面：

```text
/workflow -> 专家知识采集
```

## 10. Agent 怎么启动它

示例输入：

```json
{
  "knowledge_space_uuid": "<KNOWLEDGE_SPACE_UUID>",
  "source": {
    "type": "audio",
    "asset_uuid": "<MEDIA_ASSET_UUID>",
    "note": "专家访谈录音"
  }
}
```

启动实例：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "definition_uuid": "<DEFINITION_UUID>",
    "input": {
      "knowledge_space_uuid": "<KNOWLEDGE_SPACE_UUID>",
      "source": {
        "type": "text",
        "text": "这里是一段专家方法论原文"
      }
    }
  }'
```

## 11. 常见失败点

| 失败 | 原因 | 处理 |
| --- | --- | --- |
| `knowledge_space_uuid` 缺失 | 没有指定目标知识库。 | 先创建或选择 Knowledge Space。 |
| parse skill 失败 | 文件类型不支持或 Skill 未授权。 | 查 Skill Registry 和媒体解析能力。 |
| metadata 分类失败 | `corex.knowledge.expert` 相关 namespace 没 seed。 | 先执行 metadata seed。 |
| 卡在审核 | 正常等待 `knowledge_reviewer`。 | 打开 `/workflow/review-tasks`。 |
| 发布失败 | 知识库写入能力缺失或草稿格式不合规。 | 查 `knowledge.stage/publish` 步骤错误。 |

## 12. 适合不适合

适合：

- 专家个人知识库。
- 部门方法论库。
- 客户专属经验库。

不适合：

- 只做 metadata 分类，不发布知识，应使用 `intake_classify_review`。
- 纯 skill 结果审核，应使用 `skill_review_publish_event`。
