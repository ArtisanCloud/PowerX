# 原生智能体规格

## 1. 核心对象

### NativeAgentTemplate

PowerX 智能体来源快照。对象名暂定为 `NativeAgentTemplate`，中文产品术语在规格和审计上下文中使用“智能体模板包”。它不是租户实际运行实例，也不是用户单独维护的产品对象，而是可安装、可复制、可版本化的配置快照。

页面和业务操作的主对象必须是智能体或智能团。`NativeAgentTemplate` 只支撑内置 seed、插件导入、版本发布、克隆预览和来源审计。

建议字段：

- `template_uuid`
- `agent_key`
- `display_name`
- `category`：`role|industry|expert|system`
- `role_code`：如 `ceo|finance|marketing|operations`
- `industry_code`：可空，如 `finance|retail|manufacturing`
- `description`
- `default_skill_pack_refs`
- `default_workflow_pack_refs`
- `default_metadata_template_refs`
- `default_knowledge_space_profile`
- `status`：`draft|published|deprecated|disabled`
- `version`

### AgentSource

AgentSource 表示智能体来源。所有来源最终都必须归一为 AgentInstance，并进入同一套 Skill、Workflow、Knowledge Space、Capability 和权限治理机制。

来源类型：

| 来源 | 含义 | 示例 |
| --- | --- | --- |
| `builtin_template` | PowerX 内置智能体来源 | 营销智能体、财务智能体 |
| `tenant_clone` | 租户克隆智能体 | 某公司营销总监智能体 |
| `tenant_custom` | 租户从零创建 | 创始人个人战略顾问 |
| `plugin_agent` | 插件市场声明并同步 | AI Craft 工艺流程专家 |
| `imported_agent` | 后续从外部包导入 | 第三方顾问智能体包 |

建议字段：

- `source_uuid`
- `source_type`
- `provider_plugin_id`：仅插件来源使用
- `template_uuid`：克隆来源可记录
- `source_version`
- `checksum`
- `sync_status`
- `created_by`
- `created_at`

### AgentInstance

租户实际启用的智能体实例，引用 NativeAgentTemplate 或插件 Agent Source 作为来源快照。

关键绑定：

- Agent -> Skill Registry
- Agent -> Workflow Definition
- Agent -> Knowledge Space
- Agent -> Capability Grants
- Agent -> Metadata Template
- Agent -> IAM/Role/User Access

建议字段：

- `agent_uuid`
- `tenant_uuid`
- `source_uuid`
- `source_type`
- `agent_key`
- `display_name`
- `owner_scope`：`tenant|department|user|project|customer`
- `owner_uuid`
- `persona_snapshot`
- `prompt_snapshot`
- `knowledge_space_refs`
- `skill_pack_refs`
- `workflow_pack_refs`
- `capability_grant_refs`
- `status`
- `version`

规则：

1. 同一租户允许多个同类 AgentInstance 并存。
2. 克隆后 AgentInstance 与来源快照解耦，后续知识库迭代不回写来源快照。
3. 同一职位名称可以重复，但同一 owner scope 内 `agent_key` 必须唯一。
4. 插件 Agent 必须先通过 Plugin Registry 同步为治理态 AgentInstance，不能绕过 PowerX 直接进入运行态。
5. 数字分身类 Agent 默认 owner-scoped，离职交接必须通过知识库共享或转移流程，不自动把旧知识并入新智能体。

### KnowledgeSpaceProfile

知识空间创建模板，用于声明该智能体需要什么类型知识库。

示例：

- `expert_personal_knowledge`
- `department_methodology`
- `marketing_strategy_library`
- `finance_analysis_library`
- `operations_sop_library`

### SkillPack

一组可复用 Skill 的逻辑组合。

示例：

- `knowledge.ingestion.basic`
- `knowledge.curate.methodology`
- `knowledge.review.conflict_detection`
- `marketing.strategy.extract`
- `operations.sop.normalize`

### WorkflowPack

一组可发布 Workflow 模板的逻辑组合。

示例：

- `expert_knowledge_daily_capture`
- `audio_to_knowledge_card`
- `document_to_methodology_update`
- `feedback_to_reprocess`
- `domain_template_publish`

## 2. Agent 分类规格

智能体分类以职位作为主轴：

- `role.ceo`
- `role.finance`
- `role.marketing`
- `role.operations`
- `role.sales`
- `role.customer_success`
- `role.hr`
- `role.product`
- `role.legal`
- `role.expert_curator`

行业不直接生成第二套智能体，而是作为行业扩展叠加：

- `industry.finance`
- `industry.retail`
- `industry.manufacturing`
- `industry.education`
- `industry.healthcare`
- `industry.professional_services`

组合示例：

```text
Marketing Agent source snapshot + industry.retail extension = 零售营销智能体
Finance Agent source snapshot + industry.manufacturing extension = 制造业财务智能体
Expert Curator Agent source snapshot + industry.finance extension = 财经专家知识库智能体
```

## 2.1 克隆与自建规则

内置智能体只作为创建起点。租户实际使用时有三种路径：

1. 克隆智能体：复制 persona、默认 Skill Pack、Workflow Pack、Metadata Template 和 Knowledge Space Profile。
2. 从零创建：用户自行选择 persona、知识库、Skill、Workflow 和权限。
3. 从插件安装：插件通过 registry 声明 Agent、Skill、Capability，经 PowerX 同步、校验、发布后成为 AgentInstance。

克隆语义：

- 克隆产生新的 `agent_uuid`。
- 克隆时可选择新建 Knowledge Space 或绑定已有 Knowledge Space。
- 克隆时默认复制绑定关系，不复制来源快照或原智能体的私有知识内容。
- 若要复制知识内容，必须走显式 Knowledge Space fork/import 流程，并保留来源审计。

人员数字分身语义：

- 某个用户的数字分身应绑定 `owner_scope=user` 和该用户自己的 Knowledge Space。
- 用户离职后，旧智能体可进入 `retired` 或 `read_only` 状态。
- 新任岗位负责人应创建新的 AgentInstance，可以从同一岗位内置智能体或来源快照克隆，也可以显式继承经审批的部门知识库。
- 不允许因为岗位名称相同而自动合并两个人的个人知识库。

## 3. 知识库写入状态

专家知识库内容必须经历以下状态：

| 状态 | 含义 |
| --- | --- |
| `captured` | 已采集原始材料 |
| `parsed` | 已完成转写/OCR/解析 |
| `structured` | 已抽取为结构化知识草稿 |
| `review_pending` | 等待专家或管理员审核 |
| `approved` | 审核通过，等待发布 |
| `published` | 已写入正式知识库 |
| `rejected` | 审核拒绝 |
| `superseded` | 被新版本替代 |
| `retired` | 退役保留 |

## 4. 最小 Workflow 节点类型

为了支撑原生智能体，Workflow 至少需要表达：

- `input.capture`
- `skill.invoke`
- `capability.invoke`
- `decision.gateway`
- `human.review`
- `knowledge.stage`
- `knowledge.publish`
- `event.emit`
- `compensation.rollback`

上述节点是知识库迭代 Workflow 的最低可执行集合。缺少任一必需节点时，依赖知识库迭代的 Agent 不得启用。

## 5. 权限与隔离

1. Agent 可见 Skill 必须来自已发布 Skill Registry。
2. Agent 调用 Capability 必须受 agent grant 和用户/角色权限共同约束。
3. Agent 访问 Knowledge Space 必须有明确绑定，不允许按名称模糊查找。
4. 专家个人知识库默认 owner-scoped。
5. 部门知识库默认 department-scoped。
6. 行业扩展只提供结构，不自动授予数据访问权。
7. 插件市场智能体必须使用同一套 AgentInstance 权限模型，不拥有独立旁路权限。
8. 克隆智能体不会自动继承被克隆实例的用户级私有权限。

## 6. 审计字段

每次知识库迭代至少记录：

- `tenant_uuid`
- `agent_uuid`
- `workflow_instance_uuid`
- `skill_id`
- `knowledge_space_uuid`
- `source_asset_uuid`
- `draft_uuid`
- `reviewer_user_uuid`
- `published_version`
- `trace_id`
- `created_at`

每次 Agent 创建、克隆、导入还必须记录：

- `source_type`
- `source_uuid`
- `template_uuid`
- `provider_plugin_id`
- `created_by_user_uuid`
- `clone_options`
- `knowledge_space_strategy`
