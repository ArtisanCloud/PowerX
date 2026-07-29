# 插件智能体接入机制

## 1. 目标

插件市场可以提供垂类业务智能体，例如工艺流程专家、客户成功分析师、行业内容运营智能体。但插件智能体不能成为独立于 PowerX 的第二套 Agent 体系。

所有插件声明的 Agent 必须同步为 PowerX 治理态对象：

```text
Plugin Package
  -> plugin registry
  -> Agent source snapshot
  -> AgentInstance
  -> Skill / Workflow / Knowledge / Capability / Permission
```

## 2. 插件声明内容

插件可以声明：

- Agent source snapshot
- Skill
- Workflow Pack
- Knowledge Space Profile
- Metadata Template
- Capability
- Page/UX Preset

插件不能声明：

- 绕过 PowerX IAM 的私有用户体系作为默认运行权限。
- 绕过 Capability Registry 的私有执行通道。
- 使用数字 ID 作为跨域对象引用。
- 自动读取或写入租户知识库，除非安装时显式授权。

## 3. 同步路径

插件安装或启用时：

1. PowerX 读取插件 manifest。
2. 校验插件声明的 Agent、Skill、Workflow、Capability 和页面入口。
3. 把插件 Agent 转换为 `provider_type=plugin` 的智能体来源快照。
4. 由管理员选择是否创建 AgentInstance。
5. 创建 AgentInstance 时绑定租户内 Knowledge Space、Skill、Workflow 和 Capability Grants。
6. 写入审计事件。

插件升级时：

1. 新版本智能体来源快照生成新 `source_version`。
2. 已克隆的 AgentInstance 不被静默覆盖。
3. 管理员可查看差异并选择同步来源快照配置、保留本地漂移或重新克隆。

## 4. 运行路径

插件 Agent 的运行路径与内置 Agent 一致：

```text
User / Scheduled Job / Event
  -> AgentInstance
  -> WorkflowInstance
  -> Skill invoke
  -> Capability invoke
  -> PowerX Core or Plugin backend
  -> Trace / Audit / Event
```

规则：

1. 插件 Agent 不直接持有超级权限。
2. 插件后端调用 PowerX 底座能力必须走 STS、Capability Registry 和授权检查。
3. 用户态页面请求仍然使用用户 JWT、租户成员和 RBAC。
4. 服务态 STS 不能冒充用户访问后台全量管理 API。

## 5. 页面接入

插件可以提供业务页面，但页面入口必须挂入 PowerX 菜单治理：

- 菜单名称使用 i18n key。
- 页面权限绑定 role/user permission。
- 页面内对象显示业务名称，不把 UUID 作为主标签。
- 页面调用底座能力时遵守 provider mode：local 或 delegated。

插件页面可以成为 AgentInstance 的 Page/UX Preset，但不能替代 Agent 治理页。

## 6. 知识库接入

插件可以提供行业知识结构或导入流程，但不能默认污染租户正式知识库。

要求：

1. 插件导入的知识先进入 staging 或 draft。
2. 发布到 Knowledge Space 必须经过租户授权。
3. 插件提供的行业扩展只作为 Metadata Template 或 Knowledge Space Profile 的来源。
4. 插件私有业务数据与 PowerX Core public schema 必须隔离。

## 7. 失败处理

以下情况必须阻止插件 Agent 发布或启用：

1. capability 未登记。
2. permission code 未声明。
3. Skill 未发布。
4. WorkflowDefinition 缺失且智能体来源快照标记为必须依赖。
5. Knowledge Space owner scope 不合法。
6. 插件 manifest 使用 numeric id 作为外部引用。
7. 插件页面入口没有菜单权限声明。

失败必须返回明确错误，提示缺失对象和修复位置。
