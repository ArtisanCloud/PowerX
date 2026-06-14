# PowerX Agent Skill Bridge 机制设计

本文定义 PowerX 底座 Agent Runtime 与 PowerXPlugin 插件业务能力之间的标准桥接机制。

## 1. 机制定位

PowerX Agent Skill Bridge 不是“渠道直连插件业务接口”，也不是“插件自行实现一套 Agent 对话系统”，而是底座统一会话/Agent Runtime 与插件 Skill Executor 之间的能力桥。

标准调用链：

```text
Telegram / Discord / 企业微信 / 微信 / SCRM / 移动端 / Web Chat
        ↓
PowerX Channel / Conversation / Agent Session
        ↓
PowerX Agent Runtime
        ↓
Intent Recognition / Planner / Skill Selection
        ↓
PowerX Agent Skill Bridge
        ↓
PowerXPlugin Skill Executor
        ↓
插件领域业务任务
```

禁止形成长期架构：

```text
用户消息 -> 渠道插件 -> 业务插件私有 API
```

渠道只负责消息进出；会话、身份、权限、租户、Agent 编排统一由 PowerX 管理；插件只声明 Skill 并执行领域业务。

## 2. 底座与插件职责边界

PowerX 底座负责：

1. Channel Inbound
2. Conversation / Agent Session
3. Agent Runtime
4. Intent Recognition
5. Planner / Skill Selection
6. Skill Registry 治理态维护
7. Tenant / User / Agent 权限校验
8. Skill Invocation Routing
9. Result Message Delivery
10. Trace / Audit / Metrics

插件负责：

1. Skill 源定义（`SKILL.md` 目录包）
2. Skill metadata / prompt / schema / executor 声明
3. Skill Executor 实现
4. 领域业务任务创建与状态查询
5. 结果回调或事件发布
6. 插件领域配置 UI

## 3. 双 Skill 模型

PowerX 与插件都维护 Skill，但语义不同：

```text
插件侧 Skill = 源定义态能力包
PowerX 侧 Skill = 治理态平台能力
```

插件侧 Skill 包含：

```text
skills/<skill_id>/SKILL.md
skills/<skill_id>/schema.input.json
skills/<skill_id>/schema.output.json
skills/<skill_id>/executor.yaml
skills/<skill_id>/scripts/
skills/<skill_id>/references/
skills/<skill_id>/assets/
```

`SKILL.md` 是唯一标准源入口；`executor.yaml/schema.*.json` 是可选补充文件，必须由 `SKILL.md` frontmatter 引用或被 package loader 纳入 checksum。

PowerX 侧 Skill Registry 包含：

```text
skill_id
version
source
status
provider_plugin_id
manifest_snapshot
bundle_uri
checksum
signature
tenant_visibility
agent_bindings
capability_binding
approval_state
trace/audit
```

插件安装、启用或同步时，PowerX 从插件源定义导入并校验，形成治理态 Skill。未经 PowerX 发布审批的插件 Skill 不得对租户和 Agent 可见。

### 3.1 插件自有 Local 与 PowerX 权威记录

PowerXPlugin 可以维护插件 Agent/Skill Local，用于插件开发态管理、同步状态展示和本地调试入口配置。但这些插件记录不是 Agent Runtime 的权威源。

```text
PowerXPlugin Plugin Skill
        ↓ sync
PowerX Skill RegistryRecord(source=plugin)
        ↓ publish / bind
PowerX Agent Runtime candidate pool
```

```text
PowerXPlugin Plugin Agent
        ↓ sync
PowerX Agent Record + AgentSkillBinding
        ↓ create session
PowerX Agent Session / Stream
```

职责边界：

1. 插件自有 Local 保存声明源、草稿配置、executor 设置、同步状态和 `sync_error`。
2. PowerX 底座记录保存治理态 Skill、运行态 Agent、绑定关系、权限、会话、Trace 与审计。
3. 插件调试页只能使用已同步成功的 `powerx_agent_uuid` 创建 PowerX Agent Session。
4. 当插件 Registry 与底座记录漂移时，运行态以 PowerX 底座记录为准，插件必须重新同步。
5. 插件前端不得直接调用 PowerX Admin API；必须走 `PowerXPlugin Web -> Plugin Backend Proxy -> PowerX API`。

## 4. PowerXPlugin Framework 分层

PowerXPlugin 需要提供两层封装。

### 4.1 Framework Runtime

用于插件向 PowerX 暴露能力：

1. `PluginSkillManifest`
2. `PluginSkillRegistry`
3. `PluginSkillExecutor`
4. `PluginSkillInvocation`
5. `PluginSkillInvocationContext`
6. `PluginSkillResult`
7. `PluginSkillError`
8. `PluginAuthMiddleware`

推荐插件统一暴露：

```text
GET  /api/v1/plugin/skills
GET  /api/v1/plugin/skills/:skill_id/schema
POST /api/v1/plugin/skills/invoke
GET  /api/v1/plugin/skills/invocations/:invocation_id
```

### 4.2 Framework Client

用于插件调用 PowerX 底座能力：

1. `STSClient`
2. `AgentSessionClient`
3. `AgentStreamClient`
4. `ConversationClient`
5. `CapabilityInvokeClient`
6. `EventBusClient`

Agent 通讯必须由 Framework Client 封装 HTTP/SSE/WS 细节，插件不得长期维护私有 Agent 通讯实现。

示例抽象：

```go
client.Agent().CreateSession(ctx, req)
client.Agent().Invoke(ctx, req)
client.Agent().StreamSSE(ctx, req)
client.Agent().ConnectWS(ctx, req)
```

## 5. 统一调用契约

PowerX 调插件 Skill Executor 的请求必须包含完整上下文：

```json
{
  "skill_id": "mediax.video_rebuilder.cn",
  "version": "1.0.0",
  "input": {
    "urls": ["https://example.com/video.mp4"],
    "template_hint": "篮球模板"
  },
  "context": {
    "tenant_uuid": "tenant_xxx",
    "user_uuid": "user_xxx",
    "agent_id": "agent_xxx",
    "session_id": "session_xxx",
    "message_id": "message_xxx",
    "channel": "telegram",
    "locale": "zh-CN",
    "trace_id": "trace_xxx"
  }
}
```

插件返回：

```json
{
  "success": true,
  "skill_id": "mediax.video_rebuilder.cn",
  "status": "queued",
  "message": "已创建视频重构任务",
  "task_id": "video-automation-task-xxx",
  "data": {
    "task_url": "/creation/video-automation/tasks/xxx"
  },
  "trace_id": "trace_xxx"
}
```

缺少 `tenant_uuid`、`user_uuid`、`agent_id`、`session_id`、`trace_id`、`skill_id` 等关键上下文时，插件必须 fail-fast，禁止降级为匿名调用。

## 6. 插件自有 Agent Chat

PowerXPlugin 可以提供插件 Agent Chat 页面，用于插件开发调试。但该页面必须调用 PowerX 统一 Agent Session 接口：

```text
Plugin Debug Chat UI
        ↓
PowerXPlugin Framework Client
        ↓
PowerX Agent Session / Stream API
        ↓
PowerX Agent Runtime
        ↓
Agent Skill Bridge
        ↓
当前插件 Skill Executor
```

禁止本地 Chat 长期直接调用插件业务接口绕过 PowerX Agent Runtime。

插件 Agent Chat 的 Agent 来源必须是插件 backend proxy 返回的已同步 Agent 列表：

```text
PowerXPlugin Web
        ↓
Plugin Backend Proxy
        ↓
PowerX Agent List / Session / Stream API
```

未同步、同步失败、绑定未发布 Skill、PowerX 侧已禁用的 Agent 只能在插件管理页展示为草稿或异常状态，不得出现在可运行 Agent 下拉框中。

## 6.1 插件 Agent/Skill 同步流程

插件侧创建模板对象 CRUD Skill 的推荐流程：

```text
创建 Local Skill Local
        ↓
Plugin Backend 保存 manifest/prompt/schema/executor/capability
        ↓
Plugin Backend 调 PowerX Skill Sync API
        ↓
PowerX 创建 source=plugin 的 Skill RegistryRecord
        ↓
PowerX 返回 powerx_skill_id/sync_status
        ↓
Plugin Backend 回写插件 Registry
```

插件侧创建 Agent 的推荐流程：

```text
创建 Local Agent Local
        ↓
选择已同步并已发布的 Plugin Skill
        ↓
Plugin Backend 调 PowerX Agent Admin API
        ↓
PowerX 创建 Agent + AgentSkillBinding
        ↓
PowerX 返回 powerx_agent_uuid/sync_status
        ↓
Plugin Backend 回写插件 Registry
```

同步请求必须携带 `provider_plugin_id/tenant_uuid/operator/trace_id`。同步失败时，插件自有记录必须保留 `sync_error` 和 `last_sync_at`，用于页面排障。

## 7. 与其他 feature 的关系

1. `024-ai-engineering-skills`：Agent Skill Bridge 的主归属，负责 Skill 治理、Agent 编排、Planner、Trace/Audit。
2. `007-integration-gateway-and-mcp`：提供 Gateway、STS、Capability Registry、底座能力对外暴露与 delegated 调用契约。
3. `009-install-plugin-pxp`：负责插件安装包、启用、卸载和插件生命周期钩子。
4. `006-workflow-and-agent`：负责 Workflow 被 Agent/Skill 计划节点引用时的编排语义。

## 8. MediaX 示例

MediaX 插件应声明：

```json
{
  "skill_id": "mediax.video_rebuilder.cn",
  "provider": "com.powerx.plugin.mediax-studio",
  "title": "视频智能重构",
  "intent_examples": [
    "帮我重构这个 shorts",
    "用篮球模板处理这个视频"
  ],
  "input_schema": {
    "type": "object",
    "required": ["urls"],
    "properties": {
      "urls": {"type": "array", "items": {"type": "string"}},
      "template_hint": {"type": "string"}
    }
  },
  "executor": {
    "type": "plugin_http",
    "method": "POST",
    "path": "/api/v1/plugin/skills/invoke",
    "capability": "creation.video_automation.ingest"
  }
}
```

MediaX 内部可以继续将该调用映射到领域接口，例如：

```text
POST /api/v1/creation/video-automation/ingest
```

但 PowerX 只应调用统一 Skill Executor 入口。
