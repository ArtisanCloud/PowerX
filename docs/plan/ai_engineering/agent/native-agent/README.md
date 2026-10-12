# PowerX 原生智能体体系

## 1. 定位

PowerX 原生智能体不是单个聊天机器人，而是一套面向企业业务运作的智能体团队体系。此前建设的能力注册、Skill 管理、Agent 绑定、权限治理、知识空间、元数据治理、插件框架和 Event Fabric，都是为了让这套智能体可以稳定参与真实业务。

本目录描述 PowerX 原生智能体的产品架构、知识工程机制、页面规划和内置智能体目录。知识库策展是其中的关键能力：专家、部门或行业团队可以持续上传文字、图片、录音、文件和业务记录，由原生智能体通过 Skill/Workflow 转化为可审核、可版本化、可检索的知识资产。

产品心智必须保持简单：用户管理的是“智能体”和“智能团”。“智能体模板包”只是智能体的可克隆配置快照，用于内置 seed、版本发布、插件导入、审计和克隆预览；它可以作为数据库实体或包规格存在，但不应成为独立的用户管理页面。

## 2. 设计目标

1. 让 PowerX 开箱即用地提供企业核心岗位智能体。
2. 让智能体能够绑定知识库、Skill、Workflow、Capability 和权限策略，形成可治理的业务执行单元。
3. 让专业人士的隐性知识可以持续沉淀为结构化知识库。
4. 让不同行业、岗位、部门可以复用同一套知识工程骨架，并通过智能体克隆和来源快照扩展专业结构。
5. 让 Agent 运行过程可追踪、可审核、可回滚，而不是黑盒自动化。

## 3. 核心分层

| 层级 | 作用 | PowerX 对象 |
| --- | --- | --- |
| Agent | 业务入口、职责边界、对话体验、任务调度入口 | Agent Profile、Agent Team、Agent Permission |
| Workflow | 长流程编排、状态管理、人工审核、分支、补偿 | WorkflowDefinition、WorkflowInstance |
| Skill | 可复用任务能力包，负责单类知识工程或业务操作 | Skill Package、Skill Registry |
| Capability | 底座或插件的真实执行能力 | Capability Registry、Tenant Invocation |
| Knowledge Space | 知识资产、chunk、向量、图谱、版本和策略 | KnowledgeSpace、IngestionJob、ArtifactBundle |
| Metadata | 分类、标签、字典、资源类型和领域模板 | Metadata Governance |

## 4. 当前实现边界

native-agent 不是从零重做 Agent、Skill、Knowledge、Capability 或 Metadata。PowerX 已经具备这些基础设施，native-agent 的重点是把它们组织成可复制、可克隆、可交接、可治理的原生智能体体系。

已具备基础：

| 能力域 | 当前基础 | native-agent 中的使用方式 |
| --- | --- | --- |
| Agent | 智能体管理、Agent 绑定、运行态调用、权限配置、effective permissions、trace | 作为 AgentInstance 的运行底座 |
| Skill | Skill 管理、Skill Registry、插件 Skill 同步、Agent 绑定 Skill、Skill invoke/pending/确认链路 | 作为智能体单步任务能力和 Workflow 节点能力 |
| Knowledge | Knowledge Space、上传、ingestion、向量索引、知识库管理基础 | 作为专家知识、部门方法论、行业知识资产承载层 |
| Capability | capability-gen/audit/check、platform_capabilities、Capability Registry、Tenant Invocation、STS | 作为 Skill/Workflow 最终执行能力和授权单元 |
| Metadata | 数据字典、分类体系、标签、资源类型、seed、页面和 API 基础 | 作为知识结构、标签、分类、资源引用的治理层 |
| Plugin Bridge | 插件 local/delegated、插件能力、插件页面、底座 gateway 调用规范 | 允许插件垂类智能体进入同一套 AgentInstance 治理态 |

主要缺口：

1. Workflow runtime / Workflow Pack 还未完整实现，包括可视化编排、运行实例、人工审核节点、补偿、版本发布；这是知识库增量迭代的硬前置依赖。
2. Agent Template Package / NativeAgentTemplate 需要正式模型、API 和 seed；它不需要独立管理页，应嵌入智能体管理的克隆、发布、预览和审计流程。
3. AgentInstance 需要在现有 Agent 基础上进一步固化来源、克隆快照、行业扩展快照、生命周期、知识绑定和交接规则。
4. 专家知识库策展必须由 Workflow 表达完整的采集、结构化、审核、发布、回滚链路；不得用编排型 Skill 或 Task Queue 替代 Workflow。

详细说明见 `mechanisms/current-foundation-and-gap.md`。Workflow Runtime 的开发计划入口见 `../workflow/README.md`。

## 5. 术语约定

本文档中涉及多类“模板”，必须按对象边界精确命名：

| 中文术语 | 英文对象 | 含义 |
| --- | --- | --- |
| 智能体模板包 | Agent Template Package / NativeAgentTemplate | 智能体的可克隆配置快照，包含 persona、prompt seed、Skill Pack、Workflow Pack、Knowledge Space Profile、Metadata Template 和 Capability Grant Preset；产品上通过智能体管理承载，不单独管理 |
| 职位智能体模板包 | Role Agent Template Package | 面向 CEO、财务、营销、运营等职位职责的内置智能体来源快照 |
| 行业扩展模板包 | Industry Extension Template Package | 叠加到职位智能体来源快照上的行业语义扩展，不单独代表运行态智能体 |
| 元数据模板 | Metadata Template | 分类、标签、字典、资源类型等知识结构定义 |
| 流程模板 | Workflow Definition / Workflow Pack | 可发布的工作流定义或工作流组合 |
| 知识输出模板 | Knowledge Output Template | 知识库里的方案、brief、SOP、清单等内容格式模板 |

没有限定语的“模板”不作为正式产品术语。面向用户时优先说“克隆智能体”“内置智能体”“插件智能体”；只有在规格、审计、seed、导入导出和调试上下文中才使用“智能体模板包”。

## 6. 分类原则

采用“职位优先，行业扩展叠加”的原生智能体分类：

1. 职位优先：CEO、财务、营销、运营、销售、HR、产品、法务等企业通用岗位。
2. 行业扩展：金融、零售、制造、教育、医疗、专业服务等作为行业扩展叠加到职位智能体来源快照上。
3. 专家知识库智能体作为横向基础智能体，为任何岗位或行业提供知识库迭代能力。

这样可以避免“行业 x 职位”一开始就形成大量重复智能体，同时保留行业专业深度。

内置分类不是职位全集，也不要求覆盖每家公司真实组织结构。企业里的 CMO、营销总监、策划总监、区域负责人、业务合伙人等都可能职责重叠但知识资产不同。因此 PowerX 原生智能体必须支持：

1. 从内置智能体克隆为租户自有智能体。
2. 从零创建租户自定义智能体。
3. 通过插件市场安装垂类业务智能体。
4. 多个同类智能体并存，例如“上一任营销总监数字分身”和“新任营销总监数字分身”。
5. 每个智能体绑定独立或共享的 Knowledge Space、Skill、Workflow 和权限策略。

内置智能体只提供起点和最佳实践，不限制租户创建自己的岗位、专家、项目或插件垂类智能体。其背后的智能体模板包只是克隆和版本治理用的快照。

## 7. 目录结构

- `mechanisms/`：运行机制与对象关系。
  - `overview.md`：原生智能体总体运行机制。
  - `current-foundation-and-gap.md`：当前已实现基础、native-agent 复用方式和缺口。
  - `object-relationship.md`：Agent、Agent Team、Agent Template Package、Agent Instance、Skill、Workflow、Knowledge Space、Metadata、Capability 的关系。
  - `plugin-agent-integration.md`：插件市场垂类智能体如何进入 PowerX 原生智能体治理态。
- `rules/`：产品和治理规则。
  - `clone-and-source-rules.md`：克隆、自建、插件导入、数字分身和离职交接规则。
  - `knowledge-governance-rules.md`：知识库草稿、审核、发布、版本和归属规则。
  - `lifecycle-and-handover-rules.md`：智能体启用、变更、退役、交接和知识资产继承规则。
- `specs/`：规格和实现输入。
  - `object-specifications.md`：核心对象、字段、状态和审计。
  - `template-package-spec.md`：Agent Template Package 作为克隆快照的包结构、字段、版本和校验规则。
- `pages/`：页面设计。
  - `overview.md`：页面总体导航。
  - `native-agent-directory.md`：智能体管理中的模板/克隆视图。
  - `tenant-agent-instances.md`：智能体管理主页面。
  - `agent-create-wizard.md`：创建/克隆/导入向导。
  - `knowledge-curator-workbench.md`：专家知识库工作台。
- `knowledge/`：知识工程方法论。
  - `structure-methodology.md`：通用知识骨架与领域知识结构模板扩展。
- `catalog/`：内置智能体目录。
  - `builtin-agent-catalog.md`：PowerX 原生智能体目录。
- `examples/`：具象业务推演。
  - `marketing-agent/`：营销智能体、AgentInstance 拆分、知识结构和入库流程示例。

## 8. 外部趋势参考

当前企业 AI 知识工程方向正在从“上传文档做问答”转向“持续采集、结构化、图谱化、审核和迭代”。相关公开方案包括：

- LlamaIndex Ingestion Pipeline: https://developers.llamaindex.ai/python/framework/module_guides/loading/ingestion_pipeline/
- Microsoft GraphRAG: https://microsoft.github.io/graphrag/
- Databricks Human-in-the-loop: https://www.databricks.com/blog/human-in-the-loop
- Neo4j Agentic RAG: https://neo4j.com/blog/agentic-ai/what-is-agentic-rag/
