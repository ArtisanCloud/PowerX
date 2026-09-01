# L9 - 营销活动复盘协作验收用例

## 文档目的

验证安装后的“营销活动复盘协作团队”能以团队配置的任务图调度成员，并只执行已发布的声明式 Skill Revision。它不是发布准备场景，也不是 Core 内置业务分支。

完整对话话术、输入样本和验收结果见 [营销活动复盘 Demo](../native_agents/marketing-knowledge-demo/README.md)。

## 当前可测对象

| 类型 | 名称 | 机器标识 |
| --- | --- | --- |
| 团队 | 营销活动复盘协作团队 | `marketing.campaign_review` |
| 团队负责人 | 营销负责人智能体 | `marketing.director_advisor` |
| 成员 | 内容营销、活动复盘、知识策展智能体 | 配置在 Team Member 中 |
| 可执行 Skill | 素材事实、指标分析、方法论、复盘汇总 | 声明式 Published Revision |

## 前置条件

1. 先执行 `make migrate`，再执行 `make seed`；Seed 不迁移数据库。
2. S3/MinIO 已可用，`skill_package_sources` 和 Revision 的对象 URI 都存在。
3. AI 设置中存在可用模型 Profile。
4. 团队、成员绑定和四个营销 Skill Revision 都为 published/active。

## 页面验证

1. 打开 `/agent/team-tasks`，选择“营销活动复盘协作团队”。
2. 粘贴 `team-conversation-playbook.md` 的首轮样本，发送。
3. 确认执行过程按任务图出现素材解析、指标分析、方法论和最终汇总。
4. 最终答复为 Markdown，包含事实、结论、待验证假设、行动和验收标准。
5. 点击该条消息的“追踪本轮”，核对 Trace 的 `session_id/message_id/run_id` 与消息一致。

## 失败判定

- 未发布 Revision、无对象 URI/checksum、缺调用 locale 或任务图成员未绑定 Skill，必须整轮明确失败。
- 子任务失败后不得伪造最终成功；Trace 必须定位失败节点及机器错误码。
- 不允许依团队 key、Agent key 或 Skill ID 在 Core 中选择业务 executor。

## 本地回归

```bash
cd backend
go test ./internal/service/skills ./cmd/database/seed ./internal/server/agent ./internal/server/agent/bootstrap
```

通过说明：测试覆盖定义生命周期、对象包、通用 executor 与 Team handoff；业务场景的实际模型输出仍需在页面按对话手册验收。
