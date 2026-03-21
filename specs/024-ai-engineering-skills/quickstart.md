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
