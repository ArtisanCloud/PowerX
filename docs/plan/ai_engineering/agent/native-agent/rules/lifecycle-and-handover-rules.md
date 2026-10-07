# 智能体生命周期与交接规则

## 1. 生命周期状态

`AgentInstance` 建议状态：

| 状态 | 含义 |
| --- | --- |
| `draft` | 已创建但未完成配置 |
| `ready` | 依赖校验通过，等待启用 |
| `active` | 已启用，可被用户或流程调用 |
| `suspended` | 临时停用，不处理新任务 |
| `read_only` | 只读保留，可查询历史知识和审计 |
| `retired` | 退役，不再参与业务运行 |
| `archived` | 归档，仅保留审计和历史引用 |

状态变化必须写审计。

## 2. 启用规则

AgentInstance 从 `draft` 进入 `ready` 前必须完成预检：

1. 名称、owner scope、owner uuid 合法。
2. Skill Pack 中的 Skill 已发布。
3. Workflow 依赖存在；如果 runtime 未启用，必须阻止启用需要 Workflow 的 Agent。
4. Knowledge Space 绑定明确。
5. Capability Grants 已授权。
6. 用户或角色访问策略已配置。
7. 插件来源 Agent 的 provider plugin 处于 enabled 状态。

预检失败不得自动跳过依赖。

## 3. 克隆规则

从内置智能体、来源快照或已有实例克隆时：

1. 必须生成新的 `agent_uuid`。
2. 默认复制配置快照，不复制私有知识内容。
3. 复制 Skill/Workflow/Metadata/Capability 绑定时必须重新校验租户权限。
4. 若选择复制知识内容，必须使用 Knowledge Space fork/import，并记录来源。
5. 克隆后的 AgentInstance 与来源解耦，后续修改不回写来源对象。

## 4. 人员数字分身

个人数字分身是 owner-scoped Agent。

规则：

1. 默认 `owner_scope=user`。
2. 个人知识库默认只属于该用户或被明确授权的继承人。
3. 同一岗位可以存在多个个人分身。
4. 不能因为岗位名称相同自动合并知识库。
5. 离职后默认进入 `read_only` 或 `retired`，由管理员决定是否开放历史查询。

示例：

```text
旧营销总监数字分身
  -> read_only
  -> 部门知识库中经审批的内容继续可用
  -> 个人未审批内容不自动给新任

新营销总监数字分身
  -> 从营销内置智能体重新克隆
  -> 绑定新个人知识库
  -> 可申请导入部门知识库或旧分身已发布知识
```

## 5. 岗位交接

岗位交接不是 Agent 自动替换，而是知识资产和权限的受控转移。

交接流程：

1. 选择旧 AgentInstance。
2. 生成可交接资产清单：Knowledge Space、Workflow、Skill、审计、待处理草稿。
3. 区分个人知识、部门知识、客户知识和插件业务数据。
4. 管理员选择转移、共享、fork 或保留只读。
5. 新 AgentInstance 绑定被批准的 Knowledge Space 或 fork。
6. 写入交接审计。

禁止行为：

1. 自动把旧用户个人知识并入新用户。
2. 自动继承旧用户私有权限。
3. 自动把旧 Agent 的 pending draft 发布到新 Agent。
4. 使用相同岗位名称覆盖旧 Agent。

## 6. 插件 Agent 生命周期

插件 AgentInstance 还受插件生命周期影响：

| 插件状态 | Agent 行为 |
| --- | --- |
| `installed` | 可查看插件提供的智能体来源，不自动启用 |
| `enabled` | 可创建或运行 AgentInstance |
| `disabled` | AgentInstance 应进入 `suspended` 或阻止新运行 |
| `upgrading` | 新任务可暂停，已有任务按 Workflow 策略处理 |
| `uninstalled` | AgentInstance 进入 `retired` 或保留只读审计 |

插件卸载不得删除租户已发布知识，除非管理员显式选择删除并通过二次确认。

## 7. 智能体来源快照升级

智能体来源快照升级不能静默改写 AgentInstance。

升级方式：

1. 查看智能体来源版本差异。
2. 选择同步 persona/prompt。
3. 选择同步 Skill Pack。
4. 选择同步 Workflow Pack。
5. 选择同步 Metadata Template。
6. 选择是否新增 Capability Grant 申请。

已经本地修改过的字段必须显示漂移状态，由管理员决定是否覆盖。

## 8. 审计要求

以下事件必须记录：

- 创建 AgentInstance。
- 克隆智能体。
- 从插件导入。
- 启用、停用、退役、归档。
- 修改 Skill/Workflow/Knowledge/Capability 绑定。
- 知识库 fork/import/share。
- 人员交接。
- 智能体来源快照升级同步。

审计记录必须包含：

- `tenant_uuid`
- `agent_uuid`
- `source_type`
- `source_uuid`
- `operator_user_uuid`
- `change_summary`
- `trace_id`
- `created_at`
