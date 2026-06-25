# Skills 插件侧与第三方接入设计

本文描述插件开发者与第三方如何接入 Skill 能力。

插件与 PowerX Agent Runtime 的统一桥接机制见 [`agent_skill_bridge.md`](./agent_skill_bridge.md)。本文件重点描述插件侧和第三方接入流程。

## 1. 目标

1. 插件可发布 Skill 并绑定 capability。
2. 第三方可在受控流程中导入 Skill。
3. 既支持独立调用 Skill，也支持 Agent + Skill 组合调用。

## 2. 插件接入流程

1. 插件准备 Skill Bundle 与 `SKILL.md`。
2. 插件在安装/启用时暴露 `GET /api/v1/plugin/skills` 供 PowerX 发现。
3. PowerX 校验插件 Skill 源定义并导入为治理态 Skill。
4. 管理员审核发布后绑定 Agent、capability 与 tool grants。
5. 运行时由 PowerX Agent Runtime 选择 Skill，并通过 Agent Skill Bridge 调用插件统一 executor。

当插件需要提供插件 Agent/Skill 管理页面时，必须采用 Local + Sync 模式：

```text
PowerXPlugin 插件 Agent/Skill Local
        ↓
Plugin Backend Proxy
        ↓
PowerX Skill Registry / Agent Admin API
        ↓
PowerX 治理态 Skill + 运行态 Agent + Binding
```

插件自有 Local 保存开发态声明、草稿、同步状态和错误；PowerX 底座记录保存运行态权威数据。Agent Runtime、Session、权限、候选池和 Trace 一律读取 PowerX 底座记录。

推荐插件统一实现：

```text
GET  /api/v1/plugin/skills
GET  /api/v1/plugin/skills/:skill_id/schema
PowerX Capability Invocation
GET  /api/v1/plugin/skills/invocations/:invocation_id
```

插件领域接口（如 MediaX `/creation/video-automation/ingest`）只能作为 executor 内部实现，不作为渠道、移动端或 SCRM 的长期直接调用入口。

## 3. 第三方接入流程

1. 提交来源信息（仓库、版本、checksum、签名）。
2. 上传 Skill Bundle（平台不做远程仓库在线拉取）。
3. 平台校验资产与元数据。
4. 通过后进入 `draft` 状态。
5. 管理员审核发布到 `published`。

## 4. 开放模式

### 4.1 独立 Skill 模式

租户直接调用：

- `POST /api/v1/tenant/skills/invoke`

### 4.2 Agent + Skill 模式

Agent 在规划中引用 skill 节点，由 SkillRunner 执行。

### 4.3 统一 capability 模式

通过：

- `/api/v1/tenant/invocations`
- `preferred_protocol=skill`

### 4.4 Agent Skill Bridge 模式

Agent 主路径：

```text
用户消息 -> PowerX Agent Session -> Agent Runtime -> Skill Bridge -> Plugin Capability Handler
```

适用于 Telegram、Discord、企业微信、微信、SCRM、移动端、插件自有 Chat 等所有对话渠道。

插件自有 Chat 页面必须调用 PowerX 统一 Agent Session / Stream API，不得直接调用插件业务 executor 来模拟 Agent 行为。

### 4.5 Framework Client 模式

PowerXPlugin Framework Client 统一封装插件访问 PowerX 底座能力：

1. Agent Session HTTP
2. Agent SSE Stream
3. Agent WebSocket
4. STS Token Exchange
5. Capability Invocation
6. EventBus Publish/Subscribe

插件前端或后端只依赖 Framework Client，不直接维护 PowerX Agent SSE/WS 协议细节。

### 4.6 插件 Registry + PowerX Sync 模式

适用于 PowerXPlugin 这类需要在插件侧提供 Agent/Skill 管理体验的插件。

Skill 同步：

1. 插件自有创建 `PluginRegistrySkill`。
2. 插件 backend 保存 `skill_id/version/manifest/prompt/schema/executor/capability/checksum`。
3. 插件 backend 调 PowerX Skill Registry API，创建或更新 `source=plugin` 治理态 Skill。
4. PowerX 返回 `powerx_skill_id/sync_status`。
5. 插件 backend 回写本地 `sync_status/last_sync_at/sync_error`。

Agent 同步：

1. 插件自有创建 `PluginRegistryAgent`。
2. 插件选择已同步并已发布的 Skill。
3. 插件 backend 调 PowerX Agent Admin API 创建或更新 Agent，并传入 `skillIds`。
4. PowerX 写入 Agent 与 Agent-Skill Binding。
5. 插件 backend 回写 `powerx_agent_uuid/sync_status/last_sync_at/sync_error`。

调试约束：

1. 插件 Agent Chat 只能选择 `sync_status=synced` 且 PowerX 侧仍 active 的 Agent。
2. 会话创建和 SSE/WS 只能通过插件 backend proxy 调 PowerX Agent Session/Stream API。
3. 插件前端不得直接请求 PowerX `/api/v1/admin/*` 或 `/api/v1/agents/*`。
4. 插件 Agent/Skill Local 发生漂移时，必须提示重新同步，不能继续作为运行态配置使用。

## 5. 治理要求

1. 插件卸载前需处理 Skill 绑定关系。
2. 第三方 Skill 升级必须记录来源变更。
3. 不允许“无版本覆盖式更新”。
4. 插件 capability handler 调用必须携带 `tenant_uuid/user_uuid/agent_id/session_id/message_id/trace_id`。
5. 缺少关键上下文、插件未安装、Skill 未发布、capability 不匹配时必须 fail-fast。
6. 不允许为兼容旧渠道而保留绕过 PowerX Agent Runtime 的长期直连业务入口。
7. 插件 Agent/Skill Plugin Registry 同步动作必须写审计，至少包含 `provider_plugin_id/plugin_agent_id/plugin_skill_id/powerx_agent_uuid/powerx_skill_id/sync_status/trace_id`。
8. 插件同步 Agent 绑定未发布、未审批、不同租户或不同 provider 的 Skill 时必须被 PowerX 拒绝。

## 6. 最小对接清单

1. 提供 `SKILL.md`
2. 提供 bundle uri
3. 提供 checksum
4. 声明 entrypoints
5. 声明权限副作用
6. 暴露插件统一 Skill 发现与 invoke 接口
7. 接入 PowerXPlugin Framework Client（Agent HTTP/SSE/WS + STS）
8. 本地 Chat 走 PowerX Agent Session，不直连插件业务 API
9. 如提供插件侧 Agent/Skill 管理页，必须实现插件 Registry + PowerX Sync 状态回写
10. Agent Chat 下拉框只展示已同步成功的 PowerX Agent
