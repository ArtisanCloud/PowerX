# Quickstart — PowerX Skills 管理与治理

## 1. 前置条件

- Go 1.24、Node 20、Nuxt 4 开发环境可用
- PostgreSQL、Redis 已启动并可连接
- 已切换到分支 `024-ai-engineering-skills`

## 2. 合同准备

```bash
# 确认本 feature 的合同文件
ls specs/024-ai-engineering-skills/contracts/

# 后续实现阶段同步到 backend proto 合同时执行
make proto-lint
make proto-gen
```

## 3. 数据迁移准备（实现阶段）

```bash
# 迁移脚本接入后执行
make db-migrate
```

预期：Skills registry 与 trace/audit 相关表创建成功，重复执行幂等。

补充校验（tooling 落库权威）：

```sql
-- capability registry（tooling catalog）应存在数据
select count(*) from capability_records;
-- skills registry（skill catalog）应存在数据
select count(*) from skills_registry_records;
```

## 4. 管理侧最小闭环验收

1. 访问 Web Admin `设置 -> AI -> Skills`。
2. 查看“官方固有 Skills 目录”（来源为后端内置 catalog）。
3. 上传一个第三方 skill bundle，并填写 `source_url/source_ref` 元数据。
4. 记录应进入 `draft`。
5. 管理员执行人工审批发布。
6. 绑定 capability。

预期：发布前若 checksum 校验失败必须阻断。

## 5. 调用侧最小闭环验收

### 5.0 Agent 主入口（统一编排，推荐）

```bash
curl -N -G "$POWERX_BASE_URL/api/v1/agents/stream/sse" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  --data-urlencode "agent_id=1001" \
  --data-urlencode "q=请先调用合适技能汇总告警，再给我修复建议"
```

预期：

- 请求只传自然语言与 `agent_id/session_id`，不传 `flow_id`。
- SSE 事件顺序包含：`intent -> plan -> node_start/node_end(可选) -> final -> end`。
- 计划节点可为 `workflow|skill|tooling|llm`，不再限定 flow。
- `node.kind=skill|tooling` 为真实执行调用（非占位结果）。
- 若无可执行意图，直接输出普通上下文回答（`final`），不中断会话。

### 5.1 直接调用

```bash
curl -X POST "$POWERX_BASE_URL/api/v1/tenant/skills/invoke" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "skill_id": "incident-triage",
    "payload": {"incident_id": "INC-1001"}
  }'
```

### 5.2 统一入口调用

```bash
curl -X POST "$POWERX_BASE_URL/api/v1/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.powerx.skill.incident-triage.invoke",
    "preferred_protocol": "skill",
    "payload": {"skill_id": "incident-triage", "incident_id": "INC-1001"}
  }'
```

预期：tenant 两条路径返回一致的 `status` 语义，且带 `trace_id`。

## 6. 治理与审计检查

- 检查审计日志存在：`import/publish/rollback/bind_capability`。
- 检查调用 trace 包含：`tenant_uuid/skill_id/version/entrypoint/status`。
- 验证跨租户访问 trace 被拒绝。
- 验证未传 `version` 时路由到最新 published。

## 7. 回滚演练

1. 发布版本 `1.1.0`。
2. 触发异常后执行回滚到 `1.0.0`。
3. 再次调用验证默认版本已切回 `1.0.0`。

预期：历史版本保留，latest published 指针正确切换。

## 8. Skill Source Policy（动态下发）

统一入口 `preferred_protocol=skill` 的匹配流程支持 source allowlist 动态策略，优先级如下：

1. 请求 `context.skill_source_allowlist`（或 `context.skills_source_allowlist`）
2. Agent 级策略：`agent_settings.quota_policy.skill_source_allowlist`
3. 租户级策略：`cfg_tenant_settings.key = ai.skills.source_allowlist`
4. 默认值：`["builtin","plugin","third_party"]`

示例（统一入口，按请求上下文收敛来源）：

```bash
curl -X POST "$POWERX_BASE_URL/api/v1/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.powerx.skill.incident-triage.invoke",
    "preferred_protocol": "skill",
    "context": {
      "agent_id": 1001,
      "skill_source_allowlist": ["builtin","plugin"]
    },
    "tool_grant_ids": ["ops.read"],
    "payload": {"query": "incident triage"}
  }'
```

## 9. 本地验证记录（2026-03-19）

本轮按 `T046-T049` 执行的本地验证命令与结果如下：

```bash
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go test ./internal/service/skills \
  -run 'TestLifecycleService_PublishRollbackStateMachine|TestIntegrityPolicy_ValidateImportAndPublish|TestInvokeService_ResolveDefaultVersion'
# 结果：ok   github.com/ArtisanCloud/PowerX/internal/service/skills

cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go test ./tests/integration/skills -run TestSkillNonFuncBaseline_ImportInvokeAudit
# 结果：ok   github.com/ArtisanCloud/PowerX/tests/integration/skills
```

说明：

- 当前验证覆盖了状态机、完整性策略、默认版本解析，以及非功能基线（导入耗时、调用一致性、审计写入）。
- 涉及 HTTP/SSE 的 quickstart 实机链路仍依赖 `POWERX_BASE_URL/ADMIN_TOKEN/TENANT_TOKEN`，未在本地离线测试中执行。

## 10. 候选分层与组合规划回归（T080/T081）

```bash
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go test ./tests/integration/skills \
  -run 'TestSkillAgentCandidateLayering_SystemAgentDedupeAndAuthzFilter|TestSkillAgentCompositePlanExecuteWithEventSourceScope' \
  -count=1
```

预期：

- `T080`：同名候选保留 `agent custom`，`system builtin` 被覆盖；未授权候选不会进入候选池。
- `T081`：`workflow->skill/tooling` 组合计划可执行，`node_start/node_end` 事件均带 `node_kind/node_ref/source_scope`。

## 11. Context 优化回归（Phase 12）

目标：验证 Context 分层、预算裁剪、结构化摘要、缓存策略和 token 观测字段是否对齐。

### 11.1 启用配置（示例）

```yaml
agent:
  context_optimizer:
    enabled: true
    max_prompt_tokens: 12000
    reserved_completion_tokens: 1200
    recent_messages: 8
    retrieval_top_k: 6
    cache_mode: auto
    summary_refresh_interval_sec: 900
```

### 11.2 多轮会话压测（建议 30+ 轮）

1. 在 Agent 聊天页连续发送 30 轮以上混合请求（短问答 + 检索型问答 + skills 编排型问答）。
2. 观察是否出现上下文超窗错误。
3. 删除会话后重建并重复，确认行为稳定。

### 11.3 日志与观测检查

- 检查 `backend/logs/agent_debug/<YYYYMMDD>/trace-*_stream_*.json`：
  - `prompt_tokens`
  - `completion_tokens`
  - `cached_tokens`（provider 支持时）
  - `context_layers_size`
  - `trim_actions/trim_reason`
- 检查 Planner 调试日志 `trace-*_planner_*.json` 的 prompt 前缀顺序是否稳定（L0-L2 固定）。

### 11.4 验收门槛（与 spec Success Criteria 对齐）

- `prompt_tokens` P50 相比基线下降 >= 30%。
- 缓存能力模型前缀命中率 >= 60%。
- 30+ 轮会话中超窗失败率 <= 1%，并可通过 `trim_actions` 回放定位。

## 12. Planner 提速回归（Phase 13）

目标：验证候选预筛（Top-K）、Prompt 瘦身与决策缓存是否生效，并确认输出仍为真实 skill 结果透传。

### 12.1 启用配置（示例）

```yaml
agent:
  planner_optimizer:
    enabled: true
    candidate_top_k: 32
    prompt_slim_mode: compact
    decision_cache_enabled: true
    decision_cache_ttl_sec: 60
    per_kind_quota:
      workflow: 8
      skill: 16
      tooling: 16
      llm: 4
```

### 12.2 UI 复测建议（hello-echo / prompt-template）

1. 新建会话发送：`把 INC-1001 原样返回给我。`
2. 发送：`请使用 prompt-template 输出：事故 影响 ，修复建议 。其中 id=INC-1001，scope=华东支付，action=先回滚 v2.3.7。`
3. 预期：
   - `intent` 与 `plan` 都能显示候选与最终节点；
   - 最终回答是 skill 实际内容（不是“任务已执行完成”占位语）；
   - 第二次发送同类请求，planner latency 明显缩短（命中短 TTL 决策缓存时更明显）。

### 12.3 定向集成测试

```bash
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go test ./tests/integration/skills \
  -run TestSkillAgentPlannerLatencyAndTokenRegression \
  -count=1
```

### 12.4 日志检查

- `backend/logs/agent_debug/<YYYYMMDD>/trace-*_planner_*.json` 关注：
  - `candidates_before / candidates_after`
  - `prompt_slim_mode`
  - `planner_cache_hit`
  - `parse_retry_count`
- `backend/logs/agent_debug/<YYYYMMDD>/trace-*_stream_*.json` 关注：
  - `usage.total_prompt_tokens / total_completion_tokens / total_tokens`
  - `usage.hops[phase=planner].latency_ms`
  - `usage.planner_candidates_before / usage.planner_candidates_after`

## 13. A2A 页面验收（搭团队 + 三个场景）

目标：只通过页面操作验证“1 个 TL（planner）调度多个子智能体协作执行”已跑通。

### 13.1 页面搭建团队（先做）

1. 打开：`/settings/ai/agent-teams`，点击 `创建团队`。
2. 选择 2~3 个可用智能体（至少 `1 TL + 1 子智能体`）。
3. 设定 TL：在成员卡片点击 `设为 TL`。
4. 角色设置：
   - TL 固定为 `planner`（不可改）
   - 子智能体可选：`retriever / executor / reviewer`
5. 团队名建议：`a2a-minimal-demo`，创建后确认团队状态为 `active`。
6. 在团队列表点击该团队的 `进入任务`（或手动打开 `/agent/team-tasks` 后在顶部选择器选团队）。

推荐角色组合：

1. TL：planner
2. 子智能体 A：retriever
3. 子智能体 B：executor（可选）
4. 子智能体 C：reviewer（可选）

### 13.2 场景 A：最小并行协作（先跑）

在 `/agent/team-tasks` 会话输入：

`请并行完成两件事：1）检索 INC-支付网关-延迟飙高 最近24小时变更；2）给出三条可执行修复建议。最后汇总成一个结论。`

页面通过标准：

1. 助手消息出现“执行过程”卡片。
2. 出现 `Intent：候选 N 个`，`N >= 2`。
3. 出现 `Plan：节点 M 个`，`M >= 2`。
4. 节点状态最终进入 `completed` 或 `failed`（不能长期停留在 `running`）。
5. 最终正文包含：变更摘要 + 3 条建议 + 汇总结论。
6. 若有子步骤失败，正文必须明确失败步骤（不能假装全成功）。

### 13.3 场景 B：上下文串联协作（先查再判）

推荐使用：`retriever + reviewer` 子智能体组合。

会话输入：

`先查询 INC-订单服务-连接池耗尽 的变更记录，再根据查询结果输出风险复核结论（高/中/低）和依据。`

页面通过标准：

1. 执行过程先出现检索类节点，再出现复核类节点。
2. 节点数至少 2 条，且体现串联关系。
3. 最终正文包含“风险等级 + 依据”。
4. 依据与前序检索结果一致，若证据不足必须明确说明。

### 13.4 场景 C：插件组合协作（业务闭环）

前置：告警/工单/通知等插件能力已接入，否则该场景不计通过。

会话输入：

`拉取当前 P1 告警，自动创建工单，并发送值班通知。最后返回告警数、工单号、通知状态。`

页面通过标准：

1. 执行过程节点数 >= 3。
2. 节点状态完整展示执行生命周期。
3. 最终正文包含：`告警数`、`工单号`、`通知状态` 三项回执。
4. 某步失败时，返回“部分成功 + 失败步骤说明”。

### 13.5 页面审计核验（推荐）

1. 打开：`/settings/ai/skills`。
2. 点击：`按 Team 查看 A2A 审计`。
3. 输入 `team_id`（来自团队管理列表），可选 `handoff_task_id` / `handoff_trace_id` 过滤。
4. 预期可见字段：`team / task / trace / node / protocol / status`。
5. 失败场景应可对上 `error_summary` 与失败节点。

### 13.6 API/日志核验（可选兜底）

API：

- `GET /admin/skills/traces?team_id=<TEAM_ID>&limit=50`

日志：

- `backend/logs/agent_debug/<YYYYMMDD>/trace-*_stream_*.json`
- `backend/logs/agent_debug/<YYYYMMDD>/trace-*_planner_*.json`

重点字段：

- `plan_id`
- `team_id`
- `task_id`
- `parent_agent_id`
- `child_agent_id`
- `failure_policy`
- `node_status`

### 13.7 验收记录（每次执行必须留档）

1. 执行时间
2. 团队名称 / `TEAM_ID`
3. TL 名称 / `PARENT_AGENT_ID`
4. 场景编号（A/B/C）
5. 输入指令
6. 页面可见节点数
7. 最终状态：成功 / 部分成功 / 失败
8. 回执摘要
9. `trace_id`
10. 失败节点与原因（如有）

## 14. Agent Skill Bridge 插件验收（Phase 16）

目标：验证插件 Skill 源定义、PowerX 治理态导入、Agent Runtime 调用插件 executor、插件调试 Chat 统一走 PowerX Agent Session。

### 14.1 插件 Skill 发现

示例插件暴露：

```bash
curl -X GET "$PLUGIN_BASE_URL/api/v1/plugin/skills" \
  -H "Authorization: Bearer $PLUGIN_RUNTIME_TOKEN"
```

预期返回包含：

```json
{
  "skill_id": "mediax.video_rebuilder.cn",
  "provider": "com.powerx.plugin.mediax-studio",
  "version": "1.0.0",
  "executor": {
    "type": "plugin_http",
    "path": "/api/v1/plugin/skills/invoke",
    "capability": "creation.video_automation.ingest"
  }
}
```

强校验：

1. 缺少 `skill_id/version/description/input_schema/executor` 时，PowerX 导入必须拒绝。
2. 插件源定义只进入 `draft`，不得直接对租户或 Agent 可见。
3. 发布后 `source=plugin`，并能在 Agent Skill 候选池看到。

### 14.2 Agent 调用插件 Skill

通过 PowerX Agent Stream 发起自然语言请求：

```bash
curl -N -G "$POWERX_BASE_URL/api/v1/agents/stream/sse" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  --data-urlencode "agent_id=1001" \
  --data-urlencode "q=帮我用篮球模板重构这个视频：https://example.com/video.mp4"
```

预期：

1. SSE 出现 `intent -> plan -> node_start -> node_end -> final -> end`。
2. `plan` 中存在 `node.kind=skill`，`skill_id=mediax.video_rebuilder.cn`。
3. PowerX 调用插件：

```text
POST /api/v1/plugin/skills/invoke
```

4. 插件收到完整 context：

```json
{
  "tenant_uuid": "tenant_xxx",
  "user_uuid": "user_xxx",
  "agent_id": "agent_xxx",
  "session_id": "session_xxx",
  "message_id": "message_xxx",
  "trace_id": "trace_xxx"
}
```

5. 最终回复来自插件 `PluginSkillResult` 归一化结果，例如任务号、状态、摘要。

### 14.3 插件调试 Chat 验收

打开插件调试 Chat 页面，发送同样请求。

预期：

1. 页面通过 PowerXPlugin Framework Client 调用 PowerX Agent Session/Stream。
2. 网络请求目标是 PowerX：

```text
POST /api/v1/agents/invoke
GET  /api/v1/agents/stream/sse
WS   /api/v1/agents/stream/ws
```

3. 页面不得直接调用：

```text
POST /api/v1/creation/video-automation/ingest
```

4. 本地 Chat 与 Web Agent Chat 的 trace 均能关联到 `session_id/skill_id/plugin_id`。

### 14.4 Fail-fast 验收

构造缺失上下文请求：

```bash
curl -X POST "$PLUGIN_BASE_URL/api/v1/plugin/skills/invoke" \
  -H "Authorization: Bearer $PLUGIN_RUNTIME_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "skill_id": "mediax.video_rebuilder.cn",
    "input": {"urls": ["https://example.com/video.mp4"]},
    "context": {"trace_id": "trace_missing_tenant"}
  }'
```

预期：

```json
{
  "success": false,
  "error": {
    "code": "skill.plugin_context_missing",
    "message": "tenant_uuid is required for plugin skill invocation"
  }
}
```

审计检查：

```bash
curl -X GET "$POWERX_BASE_URL/api/v1/admin/skills/traces?skill_id=mediax.video_rebuilder.cn&limit=20" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

重点字段：

1. `trace_id`
2. `tenant_uuid`
3. `provider_plugin_id`
4. `skill_id`
5. `agent_id`
6. `session_id`
7. `executor_path`
8. `status`
9. `error_code`

### 14.5 本地验证记录（2026-06-08）

```bash
cd backend && go test ./internal/service/skills ./internal/server/agent -count=1
# 结果：ok

cd backend && go test ./tests/integration/skills -count=1
# 结果：ok

cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/Plugins/PowerXPlugin/framework/backend/go && \
  go test ./runtime/skills ./runtime/powerx/agent ./runtime/powerx/sts -count=1
# 结果：ok
```

当前实现边界：

1. 已完成插件 Framework runtime/client、插件 skill discovery service、plugin HTTP executor、context fail-fast、trace 字段与错误码映射。
2. `T125` 的服务能力已具备，但尚未挂入 `backend/internal/infra/plugin/manager/*` 的安装/启用生命周期。
3. `T130/T134` 已补验收规格，真实插件调试 Chat 页面仍需在插件 connector 或 Web Admin 插件页实现后再完成勾选。
4. `web-admin/tests/e2e/plugin-agent-skill-bridge.spec.ts` 当前为 `describe.skip`，原因是本阶段尚未交付插件调试 Chat 页面与可用连接前置；启用前需先完成 `T130`。

## 15. Agent Run Trace & Report 验收

目标：验证一轮 Agent 消息可以生成结构化运行时日志，root 用户可查看节点链路并下载智能对话报告。

### 15.1 本地配置

建议本地开发默认启用 Local Sink：

```bash
export AGENT_TRACE_ENABLED=true
export AGENT_TRACE_LOCAL_ENABLED=true
export AGENT_TRACE_LOCAL_DIR=backend/logs/agents
export AGENT_TRACE_ARTIFACT_POLICY=redacted
export AGENT_TRACE_MAX_ARTIFACT_BYTES=1048576
```

预期写入目录：

```text
backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}/
  run.json
  timeline.jsonl
  nodes/*.json
  artifacts/*
  report.md
  report.json
```

### 15.2 触发一轮 Agent Run

```bash
curl -N -G "$POWERX_BASE_URL/api/v1/agents/stream/sse" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  --data-urlencode "agent_id=$AGENT_ID" \
  --data-urlencode "session_id=$SESSION_ID" \
  --data-urlencode "q=帮我分析这个视频并给出重构建议"
```

记录返回或服务端生成的：

```text
tenant_uuid
session_id
message_id
trace_id
run_id
```

本地文件检查：

```bash
find backend/logs/agents -path "*/$SESSION_ID/*" -maxdepth 5 -type f | sort
```

必须至少存在：

1. `run.json`
2. `timeline.jsonl`
3. `nodes/*receive_message*.json`
4. `nodes/*intent_recognition*.json`
5. `nodes/*planner*.json`
6. `nodes/*final_response*.json`

### 15.3 Root 查询 Message Trace

```bash
curl -G "$POWERX_BASE_URL/api/v1/admin/agent-traces/messages/$MESSAGE_ID" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  --data-urlencode "tenant_uuid=$TENANT_UUID" \
  --data-urlencode "source=local"
```

预期响应包含：

```json
{
  "tenant_uuid": "tenant_xxx",
  "session_id": "session_xxx",
  "message_id": "message_xxx",
  "run_id": "run_xxx",
  "trace_id": "trace_xxx",
  "summary": {
    "status": "completed",
    "node_count": 6,
    "event_count": 12,
    "error_count": 0
  }
}
```

### 15.4 下载 Message 报告

Markdown：

```bash
curl -L "$POWERX_BASE_URL/api/v1/admin/agent-traces/messages/$MESSAGE_ID/report?format=markdown&source=local" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -o agent-message-report.md
```

JSON：

```bash
curl -L "$POWERX_BASE_URL/api/v1/admin/agent-traces/messages/$MESSAGE_ID/report?format=json&source=local" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -o agent-message-report.json
```

报告必须包含：

1. Summary
2. User Message
3. Runtime Timeline
4. Intent Recognition
5. Planner
6. Skill / Tool Invocation
7. Final Response
8. Errors / Warnings

### 15.5 Root-only 权限验收

非 root token：

```bash
curl -i "$POWERX_BASE_URL/api/v1/admin/agent-traces/messages/$MESSAGE_ID" \
  -H "Authorization: Bearer $ADMIN_OR_TENANT_TOKEN"
```

预期：

```json
{
  "code": 403,
  "message": "AGENT_TRACE_ROOT_REQUIRED"
}
```

### 15.6 Loki 查询样例

生产环境启用：

```bash
export AGENT_TRACE_LOKI_ENABLED=true
export AGENT_TRACE_LOKI_ENDPOINT=http://loki:3100
```

LogQL 示例：

```logql
{service="powerx-agent", component="agent-runtime", tenant_uuid="$TENANT_UUID", session_id="$SESSION_ID", message_id="$MESSAGE_ID"}
```

节点筛选：

```logql
{service="powerx-agent", component="agent-runtime", run_id="$RUN_ID", node_kind="skill_invoke", status="success"}
```

### 15.7 页面验收

Root 用户打开：

```text
/agent/traces
```

验收点：

1. 可按 `tenant_uuid/session_id/message_id/run_id/trace_id` 搜索。
2. 顶部指标卡显示状态、节点、事件、错误。
3. 中间展示节点链路 timeline。
4. 右侧展示节点快照、Skill/Tool 输入输出、错误详情。
5. 可点击下载 Message JSON 与 Markdown 报告。

### 15.8 回归命令

```bash
cd backend && go test ./internal/service/agent_trace ./internal/server/agent ./internal/server/agent/runtime ./internal/transport/http/admin/agenttrace -count=1
cd backend && go test ./tests/integration/skills -run TestAgentRunTraceReportQueryByMessageID -count=1
cd backend && go test ./tests/contract/http/agent_trace -run TestAgentTraceRootOnlyContract -count=1
cd web-admin && npm run test:e2e -- tests/e2e/agent-run-trace-report.spec.ts
```

### 15.9 本地验证记录（2026-06-08）

本轮已执行：

```bash
cd backend && go test ./internal/bootstrap ./internal/infra/plugin/manager ./internal/transport/http/admin/agenttrace ./tests/contract/http/agent_trace ./tests/integration/skills -run 'TestAgentRunTraceReportQueryByMessageID|TestSkillAgentCompositePlanExecuteWithEventSourceScope|TestSkillAgentNoIntentFallbackToNormalReply'
cd backend && go test ./tests/contract/http/agent_trace -run TestAgentTraceRootOnlyContract
cd web-admin && npm run build
```

结果：

1. Agent Trace API、root-only contract、目标 integration 均通过。
2. Nuxt build 通过；存在项目既有 Browserslist、dynamic import、chunk size warning。
3. `go test ./tests/integration/skills` 全量当前仍受既有 SQLite `agent_settings` 表缺失测试影响，目标 Agent Trace/Skill Bridge 用例已定向通过。

### 15.10 回滚策略

1. 设置 `AGENT_TRACE_ENABLED=false` 可关闭 Agent Trace Logger。
2. 设置 `AGENT_TRACE_LOKI_ENABLED=false` 可仅保留本地文件 sink。
3. 删除或隐藏 `/agent/traces` 菜单入口不影响后端 API；后端 root-only 仍保留权限边界。
4. 插件 Skill 发现接入在插件 enable 阶段执行；如插件暂未实现 `GET /api/v1/plugin/skills`，启用会 fail-fast，应先补插件 Framework Skill 路由再启用。
