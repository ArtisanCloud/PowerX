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
| `classify_metadata` | `metadata.classify` | 按所选分类策略、分类体系、标签、字典和资源类型补齐元数据。 | `$.vars.metadata` |
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

先准备当前租户的内置知识库。`make seed` 已为 `system` 租户准备 active 的 `插件联调知识空间`；新租户在后台创建或 SaaS 注册成功后会自动生成自己的 active 内置知识库。已有租户如果列表为空，可以在 `/knowledge-spaces` 点击“初始化固有知识库”，或调用：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/knowledge-spaces/builtin/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}'
```

再启用当前租户的 Workflow Pack：

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

## 10. Web Admin 怎么调试输入

入口：

```text
/workflow -> 营销知识采集 -> 进入编排器 -> 运行测试
```

`marketing_knowledge_capture` 在 Web Admin 中使用业务表单输入，不要求运营人员手写 JSON。

表单字段：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| 目标知识库 | 是 | 下拉选择已启用的 Knowledge Space，界面显示知识库名称，提交时使用 `knowledge_space_uuid`。如果列表为空，先在 `/knowledge-spaces` 初始化固有知识库。 |
| 素材类型 | 是 | `文本`、`音频`、`文档`、`链接`。 |
| 营销材料文本 | 文本类型必填 | 粘贴访谈、活动复盘、渠道策略讨论或内容脚本。 |
| 素材链接 | 链接类型必填 | 输入要解析的公开或内部资料链接。 |
| 素材资产引用 | 音频/文档类型必填 | 当前阶段先粘贴媒体库或文档库资产引用；后续接入选择器。 |
| 业务场景 | 否 | 例如“营销总监访谈”“活动复盘”“渠道策略讨论”。 |
| 内容语言 | 否 | `zh`、`en`、`ja`、`ko`。 |
| 运行备注 | 否 | 仅用于本次调试记录。 |

表单会构造成如下结构提交给 `POST /api/v1/admin/workflows/instances`：

```json
{
  "knowledge_space_uuid": "<KNOWLEDGE_SPACE_UUID>",
  "source": {
    "type": "text",
    "content": "新品发布复盘：高意向客户识别不足导致转化延迟...",
    "context": "活动复盘",
    "language": "zh"
  },
  "note": "验证营销方法论抽取效果"
}
```

当前边界：

- Web Admin 已将运行测试输入表单化，避免用户直接面对 `$.vars.extracted` 这类引擎变量路径。
- 变量路径仍保留在节点高级参数中，供研发和排障使用。
- 后端 `WorkflowService` 已接入真实 `SkillInvoker`、`MetadataClassifier` 和 `KnowledgeOperator`。正式测试前必须确认目标 Skill 已发布、元数据治理对象已启用、目标 Knowledge Space 为 active，并且 `knowledge_chunks` 表已完成迁移。

## 11. Web Admin 怎么配置 Skill 节点

入口：

```text
/workflow -> 营销知识采集 -> 进入编排器 -> 点击 parse_source 或 extract_marketing 节点
```

`skill.invoke` 节点现在提供业务化配置，不要求运营或配置人员直接理解 `$.vars.parsed`、`$.vars.extracted`。

可配置项：

| 字段 | 必填 | 写入节点配置 | 说明 |
| --- | --- | --- | --- |
| 执行技能 | 是 | `skill_id`、`skill_version`、`skill_source`、`skill_status` | 从 `/api/v1/admin/skills?status=published` 加载已发布 Skill。界面显示 Skill 名称、来源和版本。 |
| 模型模态 | 否 | `model_override.modality` | 可选 `llm`、`vlm`、`audio_asr`、`document_parse`、`embedding`。 |
| 模型 Profile | 否 | `model_override.profile_uuid`、`provider`、`model`、`profile_label` | 从 `/api/v1/admin/agents/settings/profiles` 加载 PowerX AI Settings 中已保存的模型 Profile。 |

节点配置示例：

```json
{
  "skill_id": "marketing.extract_methodology",
  "skill_version": "1.0.0",
  "skill_source": "builtin",
  "skill_status": "published",
  "model_override": {
    "modality": "llm",
    "profile_uuid": "<AI_MODEL_PROFILE_UUID>",
    "profile_label": "Marketing extraction model · openai/gpt-4o-mini",
    "provider": "openai",
    "model": "gpt-4o-mini"
  },
  "input_path": "$.vars.parsed",
  "output_path": "$.vars.extracted"
}
```

交互边界：

- 普通配置人员只需要选择“执行技能”和可选“模型 Profile”。
- `input_path` / `output_path` 仍在“高级参数”里展示，用于研发排障和合同校验。
- 后端通过 Skill Registry 查找已发布 Skill，并将节点级 `model_override` 透传给 Skill 执行上下文。Skill 未发布、版本不存在或执行器不支持时会明确失败。

## 12. Web Admin 怎么配置元数据分类节点

入口：

```text
/workflow -> 营销知识采集 -> 进入编排器 -> 点击 classify_metadata 节点
```

可配置项：

| 字段 | 必填 | 写入节点配置 | 说明 |
| --- | --- | --- | --- |
| 分类策略 | 是 | `classification_strategy` | `rule_based`、`llm_assisted`、`hybrid`，当前 seed 默认 `rule_based`。 |
| 分类体系 | 是 | `taxonomy_namespace` | 从元数据治理 Taxonomy 中选择已启用对象。 |
| 标签命名空间 | 是 | `tag_namespace` | 从已启用标签聚合出的 namespace 中选择。 |
| 数据字典 | 是 | `dictionary_namespace` | 从元数据治理 Dictionary Namespace 中选择已启用对象。 |
| 资源类型 | 是 | `resource_type_namespace` | 从元数据治理 Resource Type 中选择已启用对象。 |

节点配置示例：

```json
{
  "classification_strategy": "rule_based",
  "taxonomy_namespace": "corex.marketing.methodology",
  "tag_namespace": "corex.marketing",
  "dictionary_namespace": "corex.marketing",
  "resource_type_namespace": "corex.knowledge",
  "input_path": "$.vars.extracted",
  "output_path": "$.vars.metadata"
}
```

交互边界：

- 普通配置人员选择“分类策略”和治理对象，不需要手写 namespace。
- UI 保存的仍是工作流引擎需要的 namespace 合同，便于后端 Adapter 严格校验。
- 后端会校验 Taxonomy、Tag、Dictionary 和 Resource Type 都属于当前租户且处于 enabled 状态；缺任一治理对象都会明确失败，不做静默跳过。

## 13. 人工审核什么时候介入

人工只在流程推进到 `review_knowledge` 节点后介入。前面的输入采集、Skill 解析/抽取、元数据分类、知识暂存和冲突判断都是自动节点。

入口：

```text
/workflow -> 人工审核
```

审核员看到 pending 任务后：

1. 点击“通过”或“拒绝”。
2. 在确认弹窗中核对审核类型、实例和节点。
3. 填写可选审核意见。
4. 确认提交。

动作结果：

| 动作 | 后续路线 | 结果 |
| --- | --- | --- |
| 通过 | `publish_knowledge` -> `emit_published` | 候选营销知识发布到目标 Knowledge Space。 |
| 拒绝 | `emit_rejected` | 候选知识不发布，只记录拒绝事件和审核意见。 |

## 14. Agent 怎么启动它

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

## 15. 常见失败点

| 失败 | 原因 | 处理 |
| --- | --- | --- |
| 运行测试无法启动 | 未选择目标知识库，或素材类型对应的内容为空。 | 在运行测试表单中补齐必填项。 |
| Skill 下拉为空 | 没有已发布 Skill，或 `/api/v1/admin/skills` 不可访问。 | 到 AI 设置 > Skills 导入并发布 `marketing.audio_or_document_parse` 与 `marketing.extract_methodology`。 |
| 模型 Profile 下拉为空 | AI Settings 未保存对应模态的模型 Profile。 | 到 AI 设置 > 模型配置保存 LLM、VLM、ASR 或 Embedding Profile。 |
| 元数据治理选项不完整 | 缺分类体系、标签、数据字典或资源类型。 | 到设置 > 元数据治理启用营销相关治理对象。 |
| 解析失败 | 输入缺少结构化 `source` 对象，或 `source.type` 对应必填字段缺失。 | 文本补 `source.content`，链接补 `source.url`，音频/文档补 `source.asset_uuid`。 |
| 抽取结果太泛 | `marketing.extract_methodology` prompt 或 schema 不完整。 | 优化 Skill。 |
| 分类失败 | 营销 metadata namespace 没有 seed。 | 先 seed metadata governance。 |
| 没有待审核任务 | 流程还没走到 `review_knowledge`，或前置节点已失败。 | 在实例详情查看当前步骤和 `trace_id`。 |
| 暂存失败 | `knowledge_space_uuid` 缺失、知识库不是 active，或 `knowledge_chunks` 表未迁移。 | 检查知识库状态和知识 chunk store 迁移；租户没有可用库时先执行固有知识库初始化。 |
| 发布失败 | 审核通过前没有生成 `draft_refs`，或草稿 chunk 已不存在。 | 检查 `stage_knowledge` 输出和实例 `trace_id`。 |

## 16. 适合不适合

适合：

- 营销经验沉淀。
- 市场部方法论库。
- 内容和增长团队知识复用。

不适合：

- 单纯 Campaign 指标复盘，应使用 `campaign_review_to_methodology`。
- 通用专家知识库，应使用 `expert_knowledge_capture`。
