# Agent Template Package 规格

## 1. 定义

`Agent Template Package` 中文统一称为“智能体模板包”。它是智能体的可克隆配置快照，不是运行态智能体，也不是单个 prompt。它是一组可版本化、可发布、可审计的默认配置集合，用于内置 seed、插件导入、外部包导入、克隆预览和审计追溯。

产品界面上不单独管理“智能体模板包”。用户管理的是智能体和智能团；模板包只在智能体创建、克隆、发布、来源追溯、版本差异和调试审计中出现。

智能体模板包作为来源快照只提供起点：

- 不保存租户私有知识内容。
- 不直接授予租户运行权限。
- 不代表某个用户或部门。
- 不绕过 Skill、Workflow、Capability、Knowledge Space 和 IAM 的治理链路。
- 不作为独立用户管理页面或独立业务入口。

## 2. 包结构

建议逻辑结构：

```text
AgentTemplatePackage
  manifest
  persona
  prompt_seed
  skill_pack_refs
  workflow_pack_refs
  knowledge_space_profile_refs
  metadata_template_refs
  capability_grant_preset_refs
  page_ux_preset_refs
  validation
```

## 3. Manifest

`manifest` 描述智能体来源快照身份和发布状态。

建议字段：

- `template_uuid`
- `template_key`
- `display_name`
- `description`
- `category`：`role|industry|expert|system|plugin_vertical`
- `role_code`
- `industry_code`
- `provider_type`：`powerx_builtin|tenant|plugin|external`
- `provider_plugin_id`
- `version`
- `status`：`draft|published|deprecated|disabled`
- `checksum`
- `created_by_user_uuid`
- `created_at`
- `updated_at`

规则：

1. `template_uuid` 是外部引用身份。
2. `template_key` 只能作为稳定机器标识，不作为页面主标签。
3. 插件提供的智能体来源快照必须记录 `provider_plugin_id`。
4. 智能体来源快照发布后同版本内容不可原地修改，必须发布新版本。
5. 页面主标签必须显示智能体业务名称，`template_uuid`、`template_key` 只能用于 API、审计、调试和来源详情。

## 4. Persona

`persona` 定义智能体职责和边界。

建议字段：

- `role_title`
- `business_scope`
- `responsibility_summary`
- `decision_boundary`
- `risk_boundary`
- `handoff_policy`
- `recommended_owner_scope`

示例：

```text
营销总监智能体
  business_scope: 品牌、增长、渠道、活动、复盘
  decision_boundary: 生成建议、方案、复盘草稿；不直接审批预算
  recommended_owner_scope: user|department
```

## 5. Prompt Seed

`prompt_seed` 是初始提示配置，不是最终运行时不可变合同。

建议字段：

- `system_prompt_ref`
- `developer_prompt_ref`
- `response_policy_ref`
- `safety_policy_ref`
- `locale_policy_ref`

规则：

1. prompt 文本应进入可版本化资源，不应散落在代码里。
2. 租户克隆后可以调整 prompt snapshot。
3. AgentInstance 运行时使用自己的 prompt snapshot，不回写来源快照。

## 6. Skill Pack

`skill_pack_refs` 引用一组 Skill Pack。

Skill Pack 用于组织可复用任务能力，例如：

- `knowledge.ingestion.basic`
- `knowledge.curate.methodology`
- `knowledge.review.conflict_detection`
- `marketing.strategy.extract`

规则：

1. Skill Pack 只声明推荐绑定，不等于运行授权。
2. AgentInstance 启用时必须校验 Skill 是否已发布。
3. Skill 调用 Capability 时仍然走 Capability Registry 和 Agent 权限。

## 7. Workflow Pack

`workflow_pack_refs` 引用一组 Workflow Pack。

Workflow Pack 用于长流程，例如：

- 每日专家知识采集。
- 音频转知识卡片。
- 文档转方法论更新。
- 冲突检测和人工审核。
- 发布和回滚。

规则：

1. Workflow 负责流程状态、分支、人工审核、补偿和版本。
2. Skill 负责单步任务能力。
3. Workflow 可以调用 Skill，Skill 不应反向承担长流程状态。
4. Workflow runtime 未启用时，依赖 Workflow 的智能体来源快照不得发布为可克隆/可启用状态，只能显示依赖缺失。

## 8. Knowledge Space Profile

`knowledge_space_profile_refs` 声明智能体来源快照建议创建或绑定的知识库类型。

常见 Profile：

- `expert_personal_knowledge`
- `department_methodology`
- `marketing_strategy_library`
- `finance_analysis_library`
- `operations_sop_library`

规则：

1. Profile 只定义结构和策略，不包含租户私有内容。
2. 克隆 Agent 时必须显式选择知识库策略：新建、绑定、fork、暂不绑定。
3. 个人数字分身默认使用 `owner_scope=user` 的 Knowledge Space。

## 9. Metadata Template

`metadata_template_refs` 声明分类、标签、字典和资源类型模板。

示例：

```text
marketing_methodology
  taxonomy: strategy / tactic / execution / review
  tags: channel, lifecycle_stage, customer_segment
  dictionaries: campaign_type, content_format, budget_level
  resource_types: playbook, case_study, checklist, decision_record
```

规则：

1. Metadata Template 定义知识结构，不保存知识内容。
2. 不同行业可以叠加自己的 Metadata Template。
3. 同一 AgentInstance 可以绑定多个 Metadata Template，但发布前必须校验分类和字典冲突。

## 10. Capability Grant Preset

`capability_grant_preset_refs` 声明智能体来源快照建议需要的能力授权。

规则：

1. Preset 不是最终授权。
2. 安装或克隆时必须转化为租户内可审核的 Agent Capability Grants。
3. 高风险能力必须二次确认或走审批。
4. 插件能力和 PowerX 底座能力使用同一授权模型。

## 11. Page/UX Preset

`page_ux_preset_refs` 是可选配置，用于提示后台页面如何组织该类 Agent 的配置入口。

示例：

- 知识库策展工作台。
- 营销复盘工作台。
- 财务分析工作台。
- 插件垂类业务控制台。

规则：

1. Page/UX Preset 不能绕过权限。
2. 页面展示必须显示业务名称，不以 UUID 或 capability id 作为主标签。
3. 插件提供页面时也必须挂入 PowerX 统一菜单、权限和审计。

## 12. 校验规则

发布智能体来源快照前必须校验：

1. 所有引用对象使用 UUID 或稳定 key，外部合同不使用数字 ID。
2. Skill Pack 引用的 Skill 已存在且已发布。
3. Workflow Pack 引用的 WorkflowDefinition 已存在，且 Workflow runtime 支持其全部必需节点。
4. Metadata Template 无命名冲突。
5. Capability Grant Preset 中的 capability 已在 Capability Registry 登记。
6. 知识库 Profile 的 owner scope 合法。
7. 插件来源快照必须能追溯到插件版本和校验和。

校验失败必须阻止发布，不做隐式降级或兼容兜底。
