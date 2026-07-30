# Workflow Pack Seed 规格

## 1. 目标

Workflow Pack 是一组可发布的 WorkflowDefinition 模板来源，用于内置智能体和插件智能体。它不是页面上的独立主对象，而是 Agent 来源快照绑定的流程能力。

关键边界：

- 全局 YAML 目录是内置 Workflow Pack Catalog 的来源。
- `make seed` 只校验 catalog 文件和节点依赖，不给所有租户自动生成 WorkflowDefinition。
- 租户必须通过显式启用/安装动作，才会 materialize 成自己的 `workflow_definitions`。
- 租户删除或禁用内置 Pack 后，系统记录 installation tombstone；后续 seed 不自动补回。
- YAML checksum 变化只表示 Pack 有新版，不能静默覆盖用户已编辑或已删除的租户实例。

## 2. Seed 目录建议

```text
backend/config/workflow_packs/
  common/
    approval_guarded_capability.yaml
    intake_classify_review.yaml
    skill_review_publish_event.yaml
  knowledge/
    expert_knowledge_capture.yaml
  marketing/
    marketing_knowledge_capture.yaml
    campaign_review_to_methodology.yaml
```

## 3. Seed 字段

```yaml
workflow_key: marketing_knowledge_capture
display_name_i18n_key: workflow.marketingKnowledgeCapture.name
description_i18n_key: workflow.marketingKnowledgeCapture.description
category: knowledge_curation
version: 1
owner_scope: tenant
required_node_kinds:
  - input.capture
  - skill.invoke
  - metadata.classify
  - knowledge.stage
  - decision.gateway
  - human.review
  - knowledge.publish
  - event.emit
required_capabilities:
  - com.corex.knowledge.drafts.stage
  - com.corex.knowledge.publish
  - com.corex.metadata.classify
steps:
  - id: capture_input
    type: system
    node_kind: input.capture
```

## 4. 内置 Workflow Pack

当前实际 seed 文件：

| Web Admin 显示名 | workflow_key | 文件 | 类别 | 用途 |
| --- | --- | --- | --- | --- |
| 审批后执行能力 | `approval_guarded_capability` | `common/approval_guarded_capability.yaml` | `governance` | 人审后调用指定 Capability，再发送完成或拒绝事件。 |
| 采集分类审核 | `intake_classify_review` | `common/intake_classify_review.yaml` | `governance` | 采集业务材料，执行 Metadata 分类和人工审核。 |
| 技能执行审核发布 | `skill_review_publish_event` | `common/skill_review_publish_event.yaml` | `automation` | 调用指定 Skill，审核结果后发布事件。 |
| 专家知识采集 | `expert_knowledge_capture` | `knowledge/expert_knowledge_capture.yaml` | `knowledge_curation` | 专家资料解析、抽取、分类、审核和知识发布。 |
| 营销知识采集 | `marketing_knowledge_capture` | `marketing/marketing_knowledge_capture.yaml` | `knowledge_curation` | 营销资料和经验沉淀为方法论知识。 |
| 活动复盘沉淀 | `campaign_review_to_methodology` | `marketing/campaign_review_to_methodology.yaml` | `knowledge_curation` | Campaign 复盘沉淀为可复用方法论。 |

逐个 seed 的详细解释、使用场景、节点步骤、启动示例和排障说明见：

```text
docs/plan/ai_engineering/native-agent/examples/workflow/seeds/
```

### 4.1 `approval_guarded_capability`

用途：

- 对高风险或需人工确认的能力调用加审核。
- 支持插件智能体、运营智能体或后台管理员提交请求。
- 审核通过后调用 `${capability_id}`，拒绝则发送拒绝事件。

必需节点：

```text
input.capture
human.review
capability.invoke
event.emit
```

### 4.2 `intake_classify_review`

用途：

- 采集文本、媒体、链接等业务材料。
- 使用 Taxonomy、Tag、Dictionary、Resource Type 做治理。
- 人工审核分类结果后发送事件，不直接发布知识。

必需节点：

```text
input.capture
metadata.classify
human.review
event.emit
```

### 4.3 `skill_review_publish_event`

用途：

- 将任意 `${skill_id}` 包装为“执行 -> 审核 -> 事件发布”的治理流程。
- 适合插件或智能体生成内容后需要人工确认再对外发布。

必需节点：

```text
input.capture
skill.invoke
human.review
event.emit
```

### 4.4 `expert_knowledge_capture`

用途：

- 专家持续上传材料。
- 解析、抽取、分类、审核。
- 发布到个人或部门知识库。

必需节点：

```text
input.capture
skill.invoke.parse_source
skill.invoke.extract_knowledge
metadata.classify
knowledge.stage
decision.gateway.conflict_check
human.review
knowledge.publish
event.emit
```

### 4.5 `marketing_knowledge_capture`

用途：

- 营销负责人沉淀方法论。
- 抽取 Observation、Principle、Method、SOP、Case、Evidence。
- 支持个人知识库和部门方法论库。

必需节点：

```text
input.capture
skill.invoke.audio_or_document_parse
skill.invoke.marketing_extract
metadata.classify
knowledge.stage
decision.gateway.conflict_check
human.review
knowledge.publish
event.emit
```

### 4.6 `campaign_review_to_methodology`

用途：

- 把 Campaign 复盘转换为可复用方法论。
- 识别指标解释、失败案例、优化动作。

必需节点：

```text
input.capture
skill.invoke.metric_extract
skill.invoke.review_summarize
metadata.classify
knowledge.stage
human.review
knowledge.publish
event.emit
```

## 5. Seed 使用方式

本地开发：

```bash
make migrate
make seed
```

这一步只会校验：

- `backend/config/workflow_packs/**/*.yaml` 可解析。
- `workflow_key/version/required_node_kinds/steps` 完整。
- 节点类型能被当前 Node Adapter Registry 识别。

它不会遍历所有租户，也不会批量写入 `workflow_definitions`。

当前租户显式启用内置 Workflow Pack：

```bash
export POWERX_BASE_URL="http://127.0.0.1:8077"
export ADMIN_TOKEN="<TOKEN>"

curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":[]}'
```

只启用一个：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":["marketing_knowledge_capture"]}'
```

查看 seed 结果：

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/packs?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

`workflow_pack_installations` 是租户级安装状态表，用来判断是否已启用、已禁用、已删除，避免日常 seed 重新生成用户明确删除的内置工作流。

查看被 seed 出来的 WorkflowDefinition：

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/definitions?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

## 6. Seed 规则

1. Seed 只能创建或更新 draft/published WorkflowDefinition 版本。
2. 已发布版本不可原地覆盖。
3. 缺少 node_kind、Skill、Capability、Metadata namespace 时 seed 必须失败。
4. Seed 必须写入版本、checksum 和来源。
5. Agent 来源快照只能绑定已发布 WorkflowDefinition。
