# PowerXPlugin Agent / Skill Bridge 开发指南

本文是插件侧注册 Agent、Skill，并接入 PowerX Agent Runtime 的开发总入口。

如果你要实现类似 `template` 对象的 Agent 操作能力，例如“创建模板”“查询模板”“删除模板并二次确认”，优先阅读本文，再按链接进入底座规范或插件 SDK 细节。

## 1. 定位

PowerXPlugin Agent / Skill Bridge 用来把插件领域能力接入 PowerX 统一 Agent Runtime。

标准链路：

```text
PowerXPlugin 声明 Agent / Skill / Capability
        ↓
插件 Backend Proxy 同步到底座
        ↓
PowerX 生成治理态 Agent / Skill / Binding
        ↓
PowerX Agent Runtime 识别用户意图
        ↓
Skill action 映射到 capability_id
        ↓
PowerX Capability Invocation
        ↓
插件 Capability Handler 执行业务
```

不要把插件自有 Chat、插件业务 API、PowerX Agent Runtime 做成三套并行系统。会话、权限、租户上下文、Trace、缺参状态和最终回复由 PowerX 统一治理；插件只声明能力并实现业务 handler。

## 2. 权威文档

底座契约：

- [PowerX Skills 管理与治理 Spec](../../../specs/024-ai-engineering-skills/spec.md)
- [Agent Skill Bridge 机制设计](../../plan/ai_engineering/skills/agent_skill_bridge.md)
- [Skill 标准定义](../../plan/ai_engineering/skills/skill_standard_definition.md)
- [插件第三方集成](../../plan/ai_engineering/skills/plugin_third_party_integration.md)
- [Agent Run State Protocol](../../plan/ai_engineering/skills/agent_run_state_protocol.md)

能力调用：

- [Open Capability 开发指南](./open_capability/readme.md)
- [Plugin Auth Token Model](../auth/plugin_auth_token_model.md)
- [Metadata Governance 开发指南](./metadata_governance.md)

插件 SDK：

- [PowerX Plugin SDK Guide](../../standards/powerx/backend/integration/07_plugin_sdk/PowerX_Plugin_SDK_Guide.md)
- [Capability Contract Spec](../../standards/powerx/backend/integration/02_capability/Capability_Contract_Spec.md)

本文只做落地路径聚合。若本文和上述 spec 冲突，以 spec 和 contract 为准。

## 3. 插件侧必须声明什么

### 3.1 Agent Registry

插件可以维护开发态 Agent 记录，但运行态权威在 PowerX Core。

插件同步 Agent 到 PowerX 后，必须保存：

- `powerx_agent_uuid`
- 同步状态
- 同步错误
- 关联的插件 Agent ID
- 绑定的 Skill ID 列表

插件前端不得直接调用 PowerX Admin API。调用链必须是：

```text
PowerXPlugin Web -> Plugin Backend Proxy -> PowerX Admin/Agent API
```

### 3.2 Skill Manifest

Skill 是 Agent Runtime 识别和执行插件能力的语义包。

Skill 至少需要声明：

- `skill_id`
- 显示名称和描述 i18n
- 支持的 action
- action 入参 schema
- 缺参字段
- slot mapping
- executor
- `prepare_capability`
- `action_capabilities`

`prepare_capability` 用于判断是否已收齐参数、是否需要确认、是否可以执行真实业务 capability。

`action_capabilities` 用于把 action 映射成最终业务 capability。

### 3.3 Capability Catalog

插件每个可调用能力都必须进入 capability catalog。

每条 capability 必须包含：

- `capability_id`
- `permission_code`
- `agent_usable`
- `risk_level`
- `module`
- `display_name_i18n`
- `description_i18n`
- REST binding 或其他协议 binding
- `default_role_grants`，如果普通成员也要使用

缺少 `permission_code` 的 capability 不允许作为 Agent 可授权能力。

## 4. PowerX Core 负责什么

PowerX Core 负责：

- 治理态 Agent / Skill / Binding
- Skill 发布和可见性
- Agent 能力授权
- 用户权限与 Agent 权限交集计算
- Capability Registry
- Capability Invocation
- 多轮 pending state
- 二次确认状态
- Agent Run State
- Trace / Audit / Metrics

插件不得要求 Core 为某个业务对象硬编码字段。例如 `template_uuid`、`template_ref`、删除确认文案、详情链接都应该来自 Skill manifest、prepare result 或 capability result，而不是 Core 写死。

## 5. Metadata Governance Client

插件如果需要读取底座统一字典、分类、标签或替换标签绑定，必须接入 Metadata Governance 合同。

delegated 模式：

- 通过 `/api/v1/tenant/invocations` 调用底座 capability。
- 读取资源类型：`com.corex.metadata.resource_type.read`
- 读取字典项：`com.corex.metadata.dictionary.read`
- 读取分类节点：`com.corex.metadata.taxonomy.read`
- 读取标签：`com.corex.metadata.tag.read`
- 替换标签绑定：`com.corex.metadata.tag.manage`
- payload 使用 `{ method, endpoint, query, body }` 的 REST selector 结构。

local 模式：

- 只能使用底座 canonical seed 同源文件。
- seed 缺失、schema 错误或没有 canonical definitions 时，初始化必须失败。
- 不允许插件维护另一套私有默认字典、分类或标签作为 fallback。

详细规则见 [Metadata Governance 开发指南](./metadata_governance.md)。

## 6. Template 对象示例

以 `powerxplugin.template.basic.local` 为例，典型 action：

- `create`
- `list`
- `get`
- `update`
- `delete`

建议 capability 拆分：

- `com.powerx.plugins.base.local.template.prepare`
- `com.powerx.plugins.base.local.template.create`
- `com.powerx.plugins.base.local.template.list`
- `com.powerx.plugins.base.local.template.get`
- `com.powerx.plugins.base.local.template.update`
- `com.powerx.plugins.base.local.template.delete`

### 6.1 删除流程

用户输入：

```text
请删除掉测试模板2
```

Agent Runtime 命中 Skill：

```json
{
  "action": "delete",
  "template_ref": "测试模板2"
}
```

插件 `prepare_capability` 负责解析 `template_ref`：

- 如果没有找到模板，返回明确错误。
- 如果找到多个同名模板，返回候选列表和详情链接，让用户选择。
- 如果只找到一个模板，返回二次确认消息，并写入 `state_patch`。

推荐 `state_patch`：

```json
{
  "action": "delete",
  "template_uuid": "018f6d8a-9c32-7a61-bf10-5c0b6f61a101",
  "template_ref": "测试模板2",
  "template_name": "测试模板2"
}
```

用户回复：

```text
确认
```

PowerX Core 会读取 pending state，并在缺少 `confirmation` 或 `confirmed` 时补齐：

```json
{
  "confirmation": "确认",
  "confirmed": true
}
```

然后再次调用 `prepare_capability`。插件确认 `confirmed=true` 后返回真实执行请求：

```json
{
  "ready_to_execute": true,
  "capability_request": {
    "capability_id": "com.powerx.plugins.base.local.template.delete",
    "payload": {
      "action": "delete",
      "template_uuid": "018f6d8a-9c32-7a61-bf10-5c0b6f61a101"
    }
  }
}
```

### 6.2 重名处理

如果 `template_ref` 命中多个对象，不要要求用户手动猜内部 ID。

推荐返回：

- 模板名称
- 状态
- 更新时间
- 详情链接
- 短 ID 或业务编号作为二级信息

用户选择后，再进入二次确认。

### 5.3 详情链接

插件返回给用户的详情链接应是业务页面 URL，例如：

```text
/templates/crud?template_uuid=018f6d8a-9c32-7a61-bf10-5c0b6f61a101
```

Core 只负责透传和展示，不解析插件私有业务对象。

## 6. 初始化和同步顺序

本地开发推荐顺序：

1. 启动 PowerX Core。
2. 启动插件后端。
3. 插件注册 debug-host。
4. 插件同步 capability catalog。
5. 插件同步 Skill Registry。
6. 插件同步 Agent Registry。
7. 在 PowerX Web Admin 检查 Agent、Skill、Capability 和权限。
8. 在插件调试页创建 PowerX Agent Session 并测试。

Core 重启后，当前 `.local` 插件的 `/_p/{plugin_id}/api` 代理注册会丢失，因为 debug-host 挂载是进程内状态。继续调试前必须让插件重新注册 debug-host，通常就是重启插件。

验证命令：

```bash
curl -sS http://127.0.0.1:8077/__debug/plugins \
  | jq '.apis["com.powerx.plugins.base.local"]'
```

返回不为 `null` 后，插件能力代理才可用。

## 7. 调用模式

### delegated 模式

插件通过 PowerX Gateway 调用底座能力。

优先使用：

```text
POST /api/v1/tenant/invocations
```

payload 使用规范 REST 调用结构：

```json
{
  "capability_id": "com.corex.example.manage",
  "preferred_protocol": "rest",
  "payload": {
    "method": "GET",
    "endpoint": "/api/v1/admin/example",
    "query": {},
    "body": {}
  }
}
```

插件不应该自己拼 STS direct 后台路径绕过 capability registry。

### local 模式

local 模式只用于插件独立开发。

要求：

- seed 必须和底座 canonical seed 同源。
- 缺少 seed 或 capability descriptor 必须启动或初始化失败。
- 不允许用空列表、默认假数据或隐式 fallback 掩盖配置错误。

## 8. 权限模型

最终执行权限是交集：

```text
用户 IAM 权限 ∩ Agent capability grants ∩ Capability registry 可授权范围
```

插件必须确保：

- capability 有 `permission_code`
- 普通成员要使用时声明 `default_role_grants: [role_user]`
- Agent 所需的插件自身能力默认进入 baseline grants
- 其他插件能力由管理员在 PowerX 中选择授权

如果报错：

```text
agent.capability_denied: reason=user_permission_missing
```

优先检查用户角色是否拥有 capability 的 `permission_code`。

如果报错：

```text
capability not granted for tenant
```

优先检查 capability 是否已 seed/sync 到租户能力目录，并且 API key 或租户授权是否覆盖该 capability。

## 9. 常见错误

### plugin api upstream unavailable

原因：

- Core 重启后 `.local` debug-host 没有重新注册。

处理：

- 重启插件或重新调用 debug-host 注册。
- 用 `__debug/plugins` 确认 `apis[plugin_id]` 存在。

### prepare capability missing

原因：

- Skill manifest 没有声明 `executor.prepare_capability`。
- Capability catalog 没有同步对应 capability。

处理：

- 补齐 Skill manifest。
- 补齐 capability descriptor。
- 重新同步 capability catalog 和 Skill Registry。

### missing grantable capability permission codes

原因：

- capability 缺少 `permission_code`。
- permission_code 格式不符合 `module.resource:action`。

处理：

- 在 capability descriptor 中补齐 `security.permission_code`。
- 重新同步 capability catalog。

### repeated confirmation

原因：

- pending state 没有恢复。
- `state_patch` 缺少业务对象标识。
- 用户确认没有转成 `confirmed=true`。

处理：

- 检查 `agent_session_skill_states` 是否存在 `awaiting_params` 状态。
- 检查 `state_patch` 是否包含 action 和业务对象引用。
- 检查 Core 是否已包含 `awaiting_params` pending 读取修复。

### 用户被要求输入内部 ID

原因：

- 插件没有把自然语言引用解析成候选对象。
- 重名场景没有返回候选列表。

处理：

- 插件 `prepare_capability` 必须先按名称、别名或业务字段查询对象。
- 重名时返回候选和详情链接。
- 单一命中时只做二次确认，不要求用户输入内部 ID。

## 10. 最小验收清单

插件侧：

- Agent Registry 能同步到底座并回写 `powerx_agent_uuid`。
- Skill Registry 能同步到底座并进入 Agent 绑定。
- Capability Catalog 能同步到底座。
- 每个 Agent action 都能映射到 prepare 或 action capability。
- 删除类高风险 action 有二次确认。

PowerX 侧：

- Agent 管理页能看到插件来源 Agent。
- Agent 权限配置页能看到插件能力。
- 用户和 Agent 权限交集计算正确。
- Agent Run State 显示缺参、待确认、执行中、完成、失败。
- Trace 能定位 planner、prepare、capability invocation。

联调：

- `create/list/get/update/delete` 至少各跑通一次。
- 重名删除返回候选，不直接执行。
- 单一命中删除只确认一次。
- Core 重启后插件重新注册 debug-host 后可恢复调用。
