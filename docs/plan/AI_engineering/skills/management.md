# PowerX Skills 开发总览与管理规范

本文是 `docs/plan/AI_engineering/skills/` 的总索引与总规范，定义 Skill 在 PowerX 中的目标、边界和实施路线。

## 1. 文档定位

本目录采用“主文 + 分文档”的结构：

- `management.md`：总览、术语、边界、实施阶段与导航
- `skill_standard_definition.md`：Skill 标准定义、使用原理、外部出处
- `standard_mapping.md`：`SKILL.md` 与 PowerX SkillManifest 映射
- `runtime_architecture.md`：运行时架构与调用链
- `api_contracts.md`：Admin/Tenant/Plugin 接口契约
- `data_model_and_registry.md`：数据模型、状态机与注册治理
- `security_and_governance.md`：安全与合规约束
- `plugin_third_party_integration.md`：插件与第三方接入机制
- `testing_and_rollout.md`：测试策略与上线计划
- `test_use_cases/README.md`：从简单到复杂的开发验收用例
- `examples/skill_manifest_example.md`：示例清单

推荐阅读顺序：

1. `skill_standard_definition.md`
2. `standard_mapping.md`
3. `runtime_architecture.md`
4. `api_contracts.md`
5. `security_and_governance.md`
6. `testing_and_rollout.md`

## 2. 目标与边界

### 2.1 目标

1. 让 Skill 成为 Agent 一等能力（与 tool calling / MCP 并列）。
2. 支持两条调用路径：
   - Agent 内调用 Skill
   - 通过 Capability Selector/Invocation 统一调用 Skill
3. 对插件与第三方开放 Skill 注册、发布、调用与治理能力。

### 2.2 非目标（首版）

1. 不直接执行未托管、未校验的远程脚本。
2. 不引入“无限制本地命令执行”能力。
3. 不在首版做多标准并行解析（以 `SKILL.md` 为主，其他标准后续适配）。

## 3. 核心术语

- Skill：以 `SKILL.md` 为入口、可执行的任务能力单元。
- SkillManifest：PowerX 内部归一化后的 Skill 元数据结构。
- Skill Bundle：Skill 资产包（文档、脚本、模板、引用等）。
- SkillRunner：执行 Skill 的运行时组件（Agent 内）。
- SkillAdapter：Capability Router 下 `protocol=skill` 的适配层。

## 4. 与现有能力的关系

1. Tool calling：偏向“函数调用粒度”。
2. MCP：偏向“标准化外部工具协议”。
3. Skill：偏向“可复用任务工作流/操作手册能力”。
4. Capability Registry：Skill 可被注册为 capability，并通过 Selector 统一鉴权与路由。

## 5. 实施原则

1. 标准优先：优先兼容公开 `SKILL.md` 规范。
2. 双路径一致：Agent 内调用与 Gateway 调用在结果模型、错误模型、审计模型上保持一致。
3. 安全默认收敛：默认最小权限，显式授权放开。
4. 可观测先行：每次 Skill 调用必须带 trace 与审计字段。

## 6. 分阶段落地

1. Phase 1：标准映射 + 数据模型
2. Phase 2：注册与管理 API
3. Phase 3：运行时双路径接入
4. Phase 4：插件/第三方开放
5. Phase 5：治理、压测、灰度、回滚

详细拆解见 `testing_and_rollout.md`。

## 7. 决策快照

1. 标准基线：`SKILL.md` 兼容层。
2. 分发方式：托管仓库 + 元数据注册。
3. 调用模式：支持独立 Skill 调用与 Agent+Skill 混合调用。
4. 文档组织：总览 1 份 + 标准定义 1 份 + 开发分文档 7 份 + 示例 1 份。

## 8. 参考

- Anthropic Claude Code Skills: https://docs.anthropic.com/en/docs/claude-code/skills
- OpenAI Skills Repository: https://github.com/openai/skills
