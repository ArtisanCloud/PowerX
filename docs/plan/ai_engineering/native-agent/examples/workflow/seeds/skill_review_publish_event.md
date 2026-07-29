# 技能执行审核发布（skill_review_publish_event）

## 页面对应关系

| 项目 | 值 |
| --- | --- |
| Web Admin 显示名 | 技能执行审核发布 |
| workflow_key | `skill_review_publish_event` |
| i18n key | `workflow.pack.skillReviewPublishEvent.name` |
| seed 文件 | `backend/config/workflow_packs/common/skill_review_publish_event.yaml` |
| 页面入口 | `/workflow` |
| 页面卡片标识 | 内置包 |

## 1. 这个 seed 解决什么问题

`skill_review_publish_event` 用来表达“调用一个 Skill，结果先人工审核，再发布事件”。

它适合把任意 Skill 包装成可治理流程。Skill 只负责生成结果，Workflow 负责输入采集、审核、事件发布和审计。

## 2. seed 文件

```text
backend/config/workflow_packs/common/skill_review_publish_event.yaml
```

## 3. 谁会用它

| 使用方 | 场景 |
| --- | --- |
| 通用 Skill 审核发布智能体 | Skill 生成报告、文案、分析结果后需要人审。 |
| 插件智能体 | 插件 Skill 输出结果后，通过事件通知插件业务系统。 |
| 内容团队 Agent | 生成内容后先审核再发布。 |
| 运营自动化 | 生成通知、总结、报告后审核发送。 |

## 4. 前置对象

必须准备：

| 对象 | 说明 |
| --- | --- |
| Skill | 真实存在的 `${skill_id}`。 |
| 审核角色 | 默认 `workflow_reviewer`。 |
| 输入材料 | 文本、媒体、链接均可。 |

## 5. 节点一步步做什么

```text
capture_input
  -> invoke_skill
  -> review_result
  -> emit_published
```

拒绝分支：

```text
review_result
  -> emit_rejected
```

| 步骤 | node_kind | 做什么 | 输入 | 输出 |
| --- | --- | --- | --- | --- |
| `capture_input` | `input.capture` | 收集用户输入或上传材料。 | 用户输入 | `$.artifacts.input` |
| `invoke_skill` | `skill.invoke` | 调用 `${skill_id}`。 | `$.artifacts.input` | `$.vars.skill_result` |
| `review_result` | `human.review` | 人工审核 Skill 结果。 | `$.vars.skill_result` | `$.review` |
| `emit_published` | `event.emit` | 审核通过，发布结果事件。 | `$.vars.skill_result` | `workflow.skill_result.published` |
| `emit_rejected` | `event.emit` | 审核拒绝，发布拒绝事件。 | `$.review` | `workflow.skill_result.rejected` |

## 6. 需要填哪些占位值

| 占位符 | 必填 | 示例 |
| --- | --- | --- |
| `${skill_id}` | 是 | `marketing.review_summarize` |

## 7. 怎么 seed

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":["skill_review_publish_event"]}'
```

## 8. 怎么验证

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/definitions?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

预期：

```json
{
  "workflow_pack_key": "skill_review_publish_event",
  "status": "published"
}
```

## 9. Agent 怎么启动它

示例输入：

```json
{
  "skill_id": "marketing.review_summarize",
  "input": {
    "text": "请总结这份活动复盘。"
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
      "skill_id": "marketing.review_summarize",
      "input": {
        "text": "活动复盘原文"
      }
    }
  }'
```

## 10. 常见失败点

| 失败 | 原因 | 处理 |
| --- | --- | --- |
| Skill 找不到 | `${skill_id}` 不存在或未授权。 | 查 Skill Registry 和 Agent 绑定。 |
| Skill 结果为空 | Skill 自身执行失败。 | 查 Agent/Skill trace。 |
| 卡在审核 | 正常进入 `human.review`。 | 在 `/workflow/review-tasks` 审核。 |
| 下游没收到 | event.emit 失败或订阅方未配置。 | 查 `workflow.skill_result.*` 事件。 |

## 11. 适合不适合

适合：

- 任何“Skill 输出必须人审后发布”的场景。

不适合：

- 多节点复杂知识入库，应该使用知识类 Workflow。
- 高风险 Capability 执行，应该使用 `approval_guarded_capability`。
