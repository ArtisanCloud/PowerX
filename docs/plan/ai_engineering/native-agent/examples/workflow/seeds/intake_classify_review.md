# 采集分类审核（intake_classify_review）

## 页面对应关系

| 项目 | 值 |
| --- | --- |
| Web Admin 显示名 | 采集分类审核 |
| workflow_key | `intake_classify_review` |
| i18n key | `workflow.pack.intakeClassifyReview.name` |
| seed 文件 | `backend/config/workflow_packs/common/intake_classify_review.yaml` |
| 页面入口 | `/workflow` |
| 页面卡片标识 | 内置包 |

## 1. 这个 seed 解决什么问题

`intake_classify_review` 用来做“资料进入系统前的元数据治理”。

它不负责发布知识，也不负责调用业务能力。它只做三件事：

1. 收集一份业务材料。
2. 用 Taxonomy、Dictionary、Tag、Resource Type 做分类和标签治理。
3. 让人审核分类结果，然后发事件给下游。

## 2. seed 文件

```text
backend/config/workflow_packs/common/intake_classify_review.yaml
```

## 3. 谁会用它

| 使用方 | 场景 |
| --- | --- |
| 元数据治理智能体 | 帮管理员给资料分类、打标签、归入资源类型。 |
| 知识库策展智能体 | 正式入库前先做分类审核。 |
| 插件导入流程 | 插件上传业务对象后，先交给 Core Metadata 治理。 |
| 运营人员 | 批量整理材料时，让系统先分类再人工确认。 |

## 4. 前置对象

必须准备：

| 对象 | 示例 | 说明 |
| --- | --- | --- |
| Taxonomy namespace | `corex.marketing.methodology` | 分类树命名空间。 |
| Tag namespace | `corex.marketing` | 标签命名空间。 |
| Dictionary namespace | `corex.marketing` | 字典命名空间。 |
| Resource Type namespace | `corex.knowledge` | 资源类型命名空间。 |
| 审核角色 | `metadata_reviewer` | 审核分类结果的人。 |

依赖能力：

```text
com.corex.metadata.taxonomy.read
com.corex.metadata.dictionary.read
com.corex.metadata.tag.manage
com.corex.metadata.resource_type.read
```

## 5. 节点一步步做什么

```text
capture_intake
  -> classify_intake
  -> review_classification
  -> emit_approved
```

拒绝分支：

```text
review_classification
  -> emit_rejected
```

| 步骤 | node_kind | 做什么 | 输入 | 输出 |
| --- | --- | --- | --- | --- |
| `capture_intake` | `input.capture` | 接收文本、媒体、链接材料。 | 用户输入 | `$.artifacts.intake` |
| `classify_intake` | `metadata.classify` | 按配置命名空间做分类、标签、字典匹配。 | `$.artifacts.intake` | `$.vars.metadata` |
| `review_classification` | `human.review` | 让 `metadata_reviewer` 审核分类结果。 | `$.vars.metadata` | `$.review` |
| `emit_approved` | `event.emit` | 审核通过后发送分类通过事件。 | `$.vars.metadata` | `workflow.metadata_classification.approved` |
| `emit_rejected` | `event.emit` | 审核拒绝后发送拒绝事件。 | `$.review` | `workflow.metadata_classification.rejected` |

## 6. 需要填哪些占位值

| 占位符 | 必填 | 示例 |
| --- | --- | --- |
| `${taxonomy_namespace}` | 是 | `corex.marketing.methodology` |
| `${tag_namespace}` | 是 | `corex.marketing` |
| `${dictionary_namespace}` | 是 | `corex.marketing` |
| `${resource_type_namespace}` | 是 | `corex.knowledge` |

## 7. 怎么 seed

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":["intake_classify_review"]}'
```

## 8. 怎么验证

页面：

```text
/workflow
```

API：

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/definitions?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

预期：

```json
{
  "workflow_pack_key": "intake_classify_review",
  "status": "published"
}
```

## 9. Agent 怎么启动它

典型输入：

```json
{
  "taxonomy_namespace": "corex.marketing.methodology",
  "tag_namespace": "corex.marketing",
  "dictionary_namespace": "corex.marketing",
  "resource_type_namespace": "corex.knowledge",
  "intake": {
    "text": "这份活动复盘需要归类到转化策略和失败案例。",
    "source": "meeting_note"
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
      "taxonomy_namespace": "corex.marketing.methodology",
      "tag_namespace": "corex.marketing",
      "dictionary_namespace": "corex.marketing",
      "resource_type_namespace": "corex.knowledge",
      "intake": {
        "text": "活动复盘材料"
      }
    }
  }'
```

## 10. 常见失败点

| 失败 | 原因 | 处理 |
| --- | --- | --- |
| 分类节点失败 | 命名空间不存在。 | 先在 Metadata Governance seed 或页面中创建命名空间。 |
| 审核任务为空 | Workflow 没推进到 `human.review`。 | 查实例步骤记录。 |
| 事件没发出 | Event adapter 未配置。 | 查 `workflow.metadata_classification.*` 日志。 |

## 11. 适合不适合

适合：

- 只想做分类和审核，不想立刻发布知识。
- Metadata 治理智能体。
- 插件上传材料后的统一治理入口。

不适合：

- 需要写入知识库的完整流程，应使用 `expert_knowledge_capture` 或领域知识 Workflow。
