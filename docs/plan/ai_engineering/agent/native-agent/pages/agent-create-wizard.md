# 页面设计：创建智能体向导

## 1. 目标

统一支持三种创建路径：

- 克隆 PowerX 内置智能体或租户已有智能体。
- 从零创建。
- 从插件市场导入。

## 2. 第一步：选择来源

选项：

1. 克隆智能体
2. 从零创建
3. 从插件市场导入

每个选项必须显示影响：

- 是否带默认 persona。
- 是否带默认 Skill Pack。
- 是否带默认 Workflow Pack。
- 是否带默认 Knowledge Space Profile。
- 是否需要插件依赖。

## 3. 第二步：基础信息

字段：

- 智能体名称
- 职责描述
- owner scope
- owner 选择器
- 职位分类
- 行业扩展

规则：

- owner 选择器必须可搜索。
- 同 owner scope 内 agent_key 唯一。
- 名称可以重复，但必须能通过 owner 或描述区分。

## 4. 第三步：知识库策略

用户必须显式选择：

- 新建个人知识库
- 新建部门知识库
- 绑定已有知识库
- 从已有知识库 fork
- 暂不绑定知识库

数字分身默认推荐“新建个人知识库”。

## 5. 第四步：能力与流程

配置：

- Skill Pack
- Workflow Pack
- Capability Grant Preset
- Metadata Template

如果 Workflow runtime 未启用：

- 阻止启用依赖 Workflow 的智能体。
- 阻止发布知识库策展流程。
- 显示缺失的 WorkflowDefinition、节点类型和运行时依赖。

## 6. 第五步：权限与确认

配置：

- 可访问用户
- 可访问角色
- Agent capability grants
- 是否允许访问共享部门知识库

确认页必须展示：

- 来源类型
- 克隆来源
- 知识库策略
- Skill/Workflow 绑定摘要
- 权限摘要
