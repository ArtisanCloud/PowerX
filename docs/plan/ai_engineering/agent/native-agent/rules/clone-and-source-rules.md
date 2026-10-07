# 智能体来源、克隆与数字分身规则

## 1. 来源规则

PowerX 支持五类智能体来源：

| 来源 | 说明 |
| --- | --- |
| `builtin_template` | PowerX 原生智能体来源快照，只作为创建起点 |
| `tenant_clone` | 租户从内置智能体、插件智能体或已有实例克隆 |
| `tenant_custom` | 租户从零创建 |
| `plugin_agent` | 插件通过 registry 声明并同步 |
| `imported_agent` | 外部智能体包导入 |

所有来源最终都必须形成治理态 `AgentInstance`。运行时只认 AgentInstance，不直接运行智能体模板包或插件原始声明。用户界面展示“智能体/智能团”，模板包只作为来源快照和审计信息。

## 2. 克隆规则

克隆复制：

- persona snapshot
- prompt seed
- Skill Pack 绑定
- Workflow Pack 绑定
- Metadata Template 绑定
- Knowledge Space Profile
- Capability Grant Preset

克隆不复制：

- 私有知识内容
- 用户私有权限
- 运行历史
- 审核记录
- token/secret

如果用户要复制知识内容，必须执行显式 Knowledge Space fork/import，并记录来源审计。

## 3. 自建规则

从零创建智能体时，必须显式配置：

- 名称
- owner scope
- persona
- 至少一个可执行 Skill 或 Workflow
- 权限策略

知识库可暂不绑定，但如果智能体声明为知识库策展类，则必须绑定 Knowledge Space 或 Knowledge Space Profile。

## 4. 插件智能体规则

插件市场智能体必须：

1. 在插件声明中提供 Agent、Skill、Capability 的源定义。
2. 通过 PowerX Plugin Registry 同步。
3. 转换为 PowerX 治理态 AgentInstance。
4. 使用 PowerX 统一 Agent 权限、Skill 绑定、Capability Grant 和审计模型。

插件不得绕过 PowerX 创建独立运行时智能体，也不得在运行时动态注入未治理 Skill。

## 5. 数字分身规则

数字分身是特殊的 owner-scoped AgentInstance。

默认规则：

- `owner_scope=user`
- 绑定个人 Knowledge Space
- 个人知识默认不共享给其他同岗位用户
- 离职后进入 `retired` 或 `read_only`

岗位交接：

- 新任负责人应创建新的 AgentInstance。
- 新 Agent 可以从同一岗位内置智能体或来源快照克隆。
- 部门公共知识库可以经审批绑定给新 Agent。
- 旧个人知识库不能因岗位名称相同自动合并到新 Agent。

## 6. 命名与重复

同一租户允许多个同类智能体并存。

示例：

- 营销总监智能体 - 张三
- 营销总监智能体 - 李四
- CMO 智能体
- 策划总监智能体

UI 主标签显示业务名称；UUID 只在调试或审计区显示。
