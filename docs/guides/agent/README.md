# Agent 使用指南

本目录存放面向最终使用方、实施顾问和 QA 的 Agent 操作手册。设计文档在 `docs/plan/ai_engineering`，这里的文档必须能用于 PowerX 安装后的实际业务验收。

## 文档导航

| 目录 | 内容 | 适用对象 |
| --- | --- | --- |
| [native_agents](./native_agents/README.md) | 固有智能体业务场景使用指南，当前主示例为营销活动复盘协作。 | 企业用户、实施顾问、QA |
| [multi_agent](./multi_agent/README.md) | 多智能体 A2A 协作验收剧本。 | QA、研发、实施顾问 |
| [skills](./skills/README.md) | Skills 管理、导入、调用一致性、审计与隔离。 | 管理员、研发、QA |
| [ai_setting.md](./ai_setting.md) | AI 模型与 Provider 设置。 | 管理员、实施顾问 |
| [provider-operations.md](./provider-operations.md) | Provider 运维操作。 | 管理员、运维 |

## 当前固有智能体业务场景

| 场景 | 使用文档 | 用户输入 | 核心输出 |
| --- | --- | --- | --- |
| 营销活动复盘协作 | [native_agents/marketing-knowledge-demo/](./native_agents/marketing-knowledge-demo/README.md) | 活动复盘文本、漏斗数据、素材与业务约束。 | 可审核方法论草稿、团队 Trace；知识发布另走 Workflow。 |

新增固有智能体时，需要同步补齐：

1. 对应 seed 数据。
2. 对应数据字典或分类配置。
3. 对应 `docs/guides/agent/native_agents/<scenario>.md` 使用手册。
4. 对应验收输入、预期输出、Trace 和回滚说明。
