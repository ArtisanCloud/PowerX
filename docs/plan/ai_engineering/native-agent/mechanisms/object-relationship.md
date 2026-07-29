# 原生智能体对象关系

## 1. 总览

用户心智中的主对象是“智能体”和“智能团”。“智能体模板包”在 native-agent 语境中指 `Agent Template Package`，不是用户单独维护的产品对象，也不是单个 prompt。它更接近“智能体的可克隆发布快照”，用来 seed、导入、版本化、审计和创建租户实际运行的 `AgentInstance`。

```text
Agent / Agent Team
  -> source snapshot: Agent Template Package
  -> clone/import/create
  -> AgentInstance
      -> SkillPack -> Skill
      -> WorkflowPack -> WorkflowDefinition
      -> KnowledgeSpaceProfile -> KnowledgeSpace
      -> MetadataTemplate -> taxonomy/tag/dictionary/resource type
      -> CapabilityGrantPreset -> Capability Grants
      -> IAM Access -> user/role/department
```

## 2. Agent Template Package

Agent Template Package 是创建智能体的来源快照，中文统一称为“智能体模板包”。

包含：

- Agent Persona：职责、边界、岗位/行业语义。
- Prompt Seed：默认系统提示、回答风格、任务边界。
- Skill Pack：推荐绑定的一组 Skill。
- Workflow Pack：推荐发布或启用的一组 Workflow 模板。
- Knowledge Space Profile：建议创建或绑定的知识库类型。
- Metadata Template：分类、标签、字典、资源类型模板。
- Capability Grant Preset：默认需要的底座或插件能力。
- Page/UX Preset：可选，决定后台如何呈现配置入口。

智能体模板包不保存租户私有知识，也不是运行时 Agent。它不应作为独立管理页面暴露给用户，只应在智能体管理的来源、克隆、版本、审计和调试信息中出现。

## 3. AgentInstance

AgentInstance 是租户真正运行的智能体。

来源可以是：

- 从 PowerX 内置智能体克隆。
- 租户从零创建。
- 插件市场声明并同步。
- 后续从外部智能体包导入。

AgentInstance 可以修改 persona、prompt、Skill、Workflow、知识库和权限。克隆后它与来源快照解耦，后续知识沉淀不会回写智能体模板包。

## 4. Skill

Skill 是可复用任务能力包。

Skill 负责单项任务，例如：

- 转写录音。
- OCR 图片。
- 抽取方法论。
- 生成知识卡片。
- 检测知识冲突。
- 生成营销 Campaign 复盘。

Skill 可以被 Agent 直接调用，也可以被 Workflow 节点调用。Skill 不负责长流程状态和人工审核流转。

## 5. Workflow

Workflow 是业务流程模板和运行实例。

Workflow 负责：

- 多步骤编排。
- 分支判断。
- 人工审核。
- 重试和补偿。
- SLA 和运行状态。
- 发布与回滚。

专家知识库迭代应优先建模为 Workflow，而不是把多个步骤塞进单个 Skill。

## 6. Knowledge Space

Knowledge Space 是知识资产承载层。

它保存：

- 原始材料引用。
- chunk 和结构化内容。
- embedding 和图谱索引。
- staging 草稿。
- 发布版本。
- 来源和审计。

同一个 AgentInstance 可以绑定多个 Knowledge Space。多个 AgentInstance 也可以共享部门知识库，但个人数字分身默认绑定个人 owner-scoped 知识库。

## 7. Metadata Template

Metadata Template 给知识库提供结构化语义。

例如营销 Metadata Template 可以包含：

- 战略层
- 策略层
- 执行层
- 复盘层

它不直接存知识内容，只定义分类、标签、字典和资源类型。

## 8. Capability

Capability 是真实执行能力。

Skill 和 Workflow 最终可以通过 Capability 调用：

- PowerX 底座能力。
- 插件能力。
- 外部集成能力。

所有 Capability 调用都必须经过 Capability Registry、租户授权、Agent 权限和审计。

## 9. 生命周期关系

典型创建路径：

```text
选择 PowerX 内置营销智能体
  -> 叠加零售行业扩展
  -> 克隆为“某公司营销总监智能体”
  -> 新建 owner_scope=user 的知识库
  -> 绑定营销 Skill Pack
  -> 绑定专家知识库迭代 Workflow Pack
  -> 配置用户/角色访问
  -> 开始上传材料并审核入库
```

人员更替路径：

```text
旧营销总监智能体 -> retired/read_only
新营销总监 -> 从同一内置智能体或已发布来源快照重新克隆 AgentInstance
新 AgentInstance -> 绑定新个人知识库
部门知识库 -> 可经审批共享给新 AgentInstance
旧个人知识库 -> 不自动合并
```
