# 当前基础与缺口

## 1. 结论

PowerX 已经具备 Agent、Skill、Knowledge、Capability、Metadata 和 Plugin Bridge 的基础能力。native-agent feature 的目标不是重建这些基础设施，而是把它们组织成：

```text
智能体/智能团
  -> 来源快照
  -> 克隆/自建/插件导入
  -> AgentInstance
  -> Workflow runtime
  -> Skill / Knowledge Space / Capability / Metadata / Permission
```

当前最大缺口是 Workflow runtime，以及围绕 AgentInstance 来源、克隆快照、发布快照和生命周期的产品化治理模型。知识库增量迭代必须以 Workflow runtime 为主干，不设计 Skill 编排替代路径。

## 2. 已实现基础

| 能力域 | 已有基础 | 对 native-agent 的价值 |
| --- | --- | --- |
| Agent | 智能体管理、Agent 绑定、运行态调用、权限配置、effective permissions、Agent trace | 可以承载运行态智能体和权限审计 |
| Skill | Skill Registry、Skill 管理、插件 Skill 同步、Agent 绑定 Skill、Skill invoke、pending/确认链路 | 可以承载知识整理、内容抽取、业务动作等单步能力 |
| Knowledge | Knowledge Space、文件上传、ingestion、pgvector、知识库管理基础 | 可以承载个人知识库、部门方法论库、行业知识库 |
| Capability | platform_capabilities、Capability Registry、Tenant Invocation、STS、capability-gen/audit/check | 可以把底座和插件能力变成统一授权单元 |
| Metadata | 分类体系、标签、数据字典、资源类型、seed、页面/API 基础 | 可以统一知识结构、业务标签和资源引用 |
| Plugin Bridge | local/delegated provider mode、插件能力、插件页面、gateway 调用规范 | 可以让插件垂类智能体进入同一套治理态 |

## 3. 未完成或需要重构的部分

### 3.1 Workflow runtime

已有 WorkflowDefinition、WorkflowInstance、StepRecord、基础发布和实例启动能力，但还不是 native-agent 所需的完整 Runtime。还需要补齐：

- 自动推进 DAG 的 WorkflowRunner。
- Node Adapter Registry。
- `skill.invoke`、`capability.invoke`、`metadata.classify`、`knowledge.stage`、`knowledge.publish` 等语义节点。
- Human Review 任务模型和审核页面。
- 条件分支。
- 重试和补偿。
- 发布、版本、回滚。
- 与 Skill、Capability、Knowledge Space 的节点级绑定。

知识库策展依赖 Workflow runtime。Workflow runtime 不具备时，不开放完整知识库迭代能力；相关 Agent 启用、页面发布和自动入库必须预检失败。

Workflow Runtime 的完整开发计划见 `../workflow/README.md`。

### 3.2 智能体来源快照

还需要实现正式对象或包规格：

- Agent Template Package / NativeAgentTemplate。
- 职位智能体来源快照。
- 行业扩展来源快照。
- 插件智能体来源快照。
- 模板包版本、发布、弃用、升级差异。
- 模板包 seed。

该对象不需要独立用户管理页。它应嵌入现有智能体管理：用于内置智能体展示、克隆预览、发布版本、插件导入、来源追溯和审计。

### 3.3 AgentInstance 治理模型

现有 Agent 能支撑运行，但 native-agent 需要补齐治理语义：

- 来源：builtin_template、tenant_clone、tenant_custom、plugin_agent、imported_agent。
- 克隆快照：persona、prompt、Skill、Knowledge、Capability、Metadata。
- owner scope：tenant、department、user、project、customer。
- 生命周期：draft、ready、active、suspended、read_only、retired、archived。
- 人员交接：旧 Agent read_only/retired，新 Agent 重新克隆或 fork 知识库。

### 3.4 知识库策展业务闭环

已有 Knowledge 与 Skill 基础，但还需要 native-agent 层把它组织成业务闭环：

```text
Source
  -> parse
  -> extract knowledge objects
  -> classify/tag
  -> detect conflict
  -> human review
  -> publish to Knowledge Space
  -> version and audit
```

## 4. 开发主线

1. 实现 Workflow runtime 最小完整闭环：WorkflowDefinition、WorkflowInstance、节点状态、`skill.invoke`、`human.review`、`knowledge.stage`、`knowledge.publish`、失败重试、补偿和审计。
2. 在现有智能体管理中实现 AgentInstance 来源/克隆模型和来源快照。
3. 实现营销、财务、运营等内置智能体 seed，其底层可以由智能体模板包快照承载。
4. 将专家知识库策展完整建模为 Workflow Pack，不提供 Skill 编排替代路径。
5. 插件智能体通过 Plugin Registry 接入同一套来源快照、Workflow Pack 和 AgentInstance 机制。

## 5. 非目标

1. 不把每个岗位和行业组合都硬编码成内置智能体。
2. 不让插件绕过 PowerX Agent、Skill、Capability、Knowledge 和 Permission 治理。
3. 不把 Workflow 缺口伪装成已经完整实现，也不提供第二条执行链路。
4. 不把知识库只做成普通文档问答，必须支持结构化、审核、版本和归属。
