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

## 13. A2A 最小可理解用例（1 主 2 子）

目标：验证主 Agent 能把一次复杂请求分发给两个子 Agent 并汇总返回，且保留子任务状态。

### 13.1 准备团队（动态团队，不写死 ID）

先在团队管理页创建一个可用团队：

- 路径：`/settings/ai/agent-teams`
- 约束：
  - TL 唯一 `planner`
  - 子 Agent 仅 `retriever/executor/reviewer`
  - 子 Agent 必须是同租户、active、非 system、非内置

创建后记录：

- `TEAM_ID`（本次验收团队）
- `PARENT_AGENT_ID`（TL 对应 agent）
- `TEAM_NAME`

通过团队任务页进入：

- `/agent/team-tasks?team_id=<TEAM_ID>`

### 13.2 发起测试请求（最小并行协作）

```bash
curl -N -G "$POWERX_BASE_URL/api/v1/agents/stream/sse" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  --data-urlencode "agent_id=<PARENT_AGENT_ID>" \
  --data-urlencode "q=请先检索 INC-1001 的最近变更，再给出修复建议并做风险复核"
```

页面预期（必须看到）：

1. 助手消息中出现“执行过程”卡片。
2. 卡片内出现 `Intent：候选 N 个` 与 `Plan：节点 M 个`。
3. 节点列表出现至少 2 条记录，且状态从 `running` 进入 `completed/failed`。
4. 最终正文包含“检索结果 + 建议 + 汇总结论”。

### 13.3 失败策略验证（continue）

做法：临时让子 Agent B 的一个依赖 capability 不可用，再重复 13.2 请求。

页面预期：

1. 主流程不整体失败。
2. 最终返回“部分成功”，并给出失败步骤说明。

审计预期（页面不可见字段）：

1. `GET /admin/skills/traces?team_id=<TEAM_ID>&limit=50`
2. 可定位 `handoff_task_id`、`team_id`、`node_status`、`error_summary`。

### 13.4 关键日志核对

检查：

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

### 13.5 验收记录（每次执行必须留档）

1. 执行时间
2. 团队名称 / `TEAM_ID`
3. TL 名称 / `PARENT_AGENT_ID`
4. 输入指令
5. 页面可见节点数
6. 最终状态：成功 / 部分成功 / 失败
7. `trace_id`
8. 失败节点与原因（如有）
