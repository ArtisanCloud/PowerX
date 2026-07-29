# Workflow Seed 示例索引

## 1. 先看结论

当前 PowerX 会 seed 6 个 Workflow Pack。这里按“一个 seed 一个文件”说明，不再把所有内容塞进一页。

Web Admin 页面显示的是中文名，seed 文件和 API 使用的是 `workflow_key`。两者必须同时写清楚，否则无法从页面卡片对应到文档。

页面入口：

```text
Web Admin -> /workflow
```

页面中文名来自：

```text
web-admin/i18n/locales/zh.json
```

机器标识来自：

```text
backend/config/workflow_packs/**/*.yaml -> workflow_key
```

运行时 seed 配置在：

```text
backend/config/workflow_packs/
```

说明文档在：

```text
docs/plan/ai_engineering/native-agent/examples/workflow/seeds/
```

## 2. 先按页面中文名找文档

| Web Admin 显示名 | workflow_key | 说明文档 | seed 配置 | 当前是否适合单独测试 |
| --- | --- | --- | --- | --- |
| 技能执行审核发布 | `skill_review_publish_event` | [技能执行审核发布（skill_review_publish_event）](seeds/skill_review_publish_event.md) | `backend/config/workflow_packs/common/skill_review_publish_event.yaml` | 部分适合。需要已有 Skill，否则只能测创建、查看、人审节点。 |
| 营销知识采集 | `marketing_knowledge_capture` | [营销知识采集（marketing_knowledge_capture）](seeds/marketing_knowledge_capture.md) | `backend/config/workflow_packs/marketing/marketing_knowledge_capture.yaml` | 不适合完整单测。依赖 Knowledge、Skill、Metadata。 |
| 采集分类审核 | `intake_classify_review` | [采集分类审核（intake_classify_review）](seeds/intake_classify_review.md) | `backend/config/workflow_packs/common/intake_classify_review.yaml` | 适合先测。主要验证输入、元数据分类、人审和事件。 |
| 专家知识采集 | `expert_knowledge_capture` | [专家知识采集（expert_knowledge_capture）](seeds/expert_knowledge_capture.md) | `backend/config/workflow_packs/knowledge/expert_knowledge_capture.yaml` | 不适合完整单测。依赖 Knowledge、Skill、Metadata。 |
| 活动复盘沉淀 | `campaign_review_to_methodology` | [活动复盘沉淀（campaign_review_to_methodology）](seeds/campaign_review_to_methodology.md) | `backend/config/workflow_packs/marketing/campaign_review_to_methodology.yaml` | 不适合完整单测。依赖营销抽取 Skill 和 Knowledge。 |
| 审批后执行能力 | `approval_guarded_capability` | [审批后执行能力（approval_guarded_capability）](seeds/approval_guarded_capability.md) | `backend/config/workflow_packs/common/approval_guarded_capability.yaml` | 最适合先测。可以用一个低风险 Capability 验证审批后执行。 |

阅读顺序建议：

1. 先在 `/workflow` 页面找到中文卡片名。
2. 到上表找到对应的 `workflow_key`。
3. 打开对应 seed 文档。
4. 按文档里的“怎么单独调试”跑一个 WorkflowInstance。

## 3. 怎么 seed

本地开发：

```bash
make migrate
make seed
```

只触发 Workflow Pack seed：

```bash
export POWERX_BASE_URL="http://127.0.0.1:8077"
export ADMIN_TOKEN="<TOKEN>"

curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":[]}'
```

只 seed 一个：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":["expert_knowledge_capture"]}'
```

## 4. 怎么验证

页面：

```text
Web Admin -> /workflow
```

页面上的卡片显示中文名。进入工作区后，顶部标题也应该显示同一个中文名，例如：

```text
/workflow -> 审批后执行能力 -> 工作区顶部标题：审批后执行能力
```

如果工作区顶部显示 `approval_guarded_capability` 这类机器 ID，说明前端显示逻辑没有和 seed/i18n 对齐。

### 4.1 在页面里运行调试

适合先用页面调试的 seed：

| 页面中文名 | 调试建议 |
| --- | --- |
| 审批后执行能力 | 最适合。页面会带默认测试 input，使用低风险能力 `com.corex.metadata.dictionary.read`，目标是先跑到人工审核。 |
| 采集分类审核 | 适合验证输入、分类、人工审核、事件节点。页面会带默认 Metadata 命名空间，但要求这些命名空间已经 seed。 |
| 技能执行审核发布 | 需要已有可用 Skill。页面默认传 `debug.echo`，如果系统没有这个 Skill，会明确失败。 |

页面操作：

1. 打开 `Web Admin -> /workflow`。
2. 找到中文卡片，例如“审批后执行能力”。
3. 点击卡片，进入工作区。
4. 确认顶部标题是中文名，不是 `workflow_key`。
5. 点击右上角“运行测试”。
6. 看底部“运行记录”：
   - “尚未运行”：还没有点过运行测试。
   - “运行中/排队中”：WorkflowInstance 已创建，Runner 正在推进。
   - “等待中”：通常表示进入 `human.review`，去 `/workflow/review-tasks` 处理人工审核。
   - “已成功”：当前流程实例已完成。
   - “已失败”：看底部错误和 Trace，再查后端日志。

注意：页面“运行测试”必须调用真实接口创建 WorkflowInstance，不能用假成功状态。调试失败时，底部必须显示真实错误。

当前页面默认调试 input：

| 页面中文名 | 默认 input 重点 |
| --- | --- |
| 审批后执行能力 | `capability_id=com.corex.metadata.dictionary.read`，`request.payload.dry_run=true` |
| 采集分类审核 | `taxonomy_namespace=corex.marketing.methodology`，`tag_namespace=corex.marketing`，`dictionary_namespace=corex.marketing`，`resource_type_namespace=corex.knowledge` |
| 技能执行审核发布 | `skill_id=debug.echo` |

如果默认 input 不符合当前环境，先用 API 调试方式传入正式业务参数；页面后续应补一个“调试输入”面板，而不是让用户改 seed YAML。

API：

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/packs?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"

curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/definitions?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

预期：

- `/workflow` 里能看到内置包。
- `definitions` 返回中包含 `workflow_pack_key`。
- seed 出来的定义状态是 `published`。

## 5. Agent 怎么用

关系是：

```text
Workflow Pack YAML
  -> seed 为 published WorkflowDefinition
  -> Agent Template / AgentInstance 绑定 WorkflowDefinition UUID
  -> Agent 运行时启动 WorkflowInstance
  -> Workflow 节点调用 Skill / Capability / Knowledge / Metadata / Event
```

Agent 不直接运行 YAML。YAML 只是 seed 来源。
