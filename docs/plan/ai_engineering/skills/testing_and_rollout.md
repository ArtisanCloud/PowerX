# Skills 测试与上线计划

本文定义 Skill 功能的测试矩阵、灰度发布与回滚策略。

## 1. 测试目标

1. 确保 `SKILL.md` 解析与校验稳定。
2. 确保双路径调用（Agent/Gateway）结果一致。
3. 确保授权、安全、租户隔离正确。

## 2. 测试矩阵

### 2.1 单元测试

1. Manifest 字段映射
2. 版本状态机迁移
3. 错误码与异常分类

### 2.2 集成测试

1. Admin 注册 -> 发布 -> 回滚全流程
2. Tenant 调用 `skills/invoke`
3. `tenant/invocations + preferred_protocol=skill`

### 2.3 契约测试

1. API 请求响应 JSON 结构
2. 错误码稳定性
3. 鉴权错误语义稳定性

### 2.4 回归测试

1. 不影响现有 `http/grpc/mcp/agent` 路由
2. 不破坏现有 capability selector 行为

### 2.5 候选分层与组合规划专项（T080/T081）

1. `T080`：验证 `system + agent` 同名候选去重优先级（agent 覆盖 system），并验证未授权候选不可见。  
   用例：`backend/tests/integration/skills/skill_agent_candidate_layering_test.go`
2. `T081`：验证组合规划 `workflow->skill/tooling` 可执行，且节点事件包含 `node_kind/node_ref/source_scope`。  
   用例：`backend/tests/integration/skills/skill_agent_composite_plan_test.go`
3. 建议回归命令：

```bash
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go test ./tests/integration/skills \
  -run 'TestSkillAgentCandidateLayering_SystemAgentDedupeAndAuthzFilter|TestSkillAgentCompositePlanExecuteWithEventSourceScope' \
  -count=1
```

## 3. 验收标准

1. 技能注册成功率达到既定目标（示例：99.9%）。
2. 调用链 trace 可完整回放。
3. 授权拒绝有明确错误码与审计记录。
4. 回滚可在不删历史的前提下完成。

## 4. 灰度策略

1. 灰度开关：按租户与环境控制。
2. 先灰度只读 Skill，再灰度有副作用 Skill。
3. 观察指标通过后全量开启。

## 5. 监控指标（建议）

- `skill_invocations_total`
- `skill_invocation_error_total`
- `skill_invocation_latency_ms`
- `skill_registry_publish_total`
- `skill_registry_rollback_total`

### 5.1 Agent Run Trace & Report 验收

Agent Runtime 调试报告作为独立验收项，必须覆盖：

1. 本地模式：一轮 Agent 消息执行后，`backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}/run.json`、`timeline.jsonl`、`nodes/*.json` 可生成。
2. 节点完整性：`receive_message/context_load/intent_recognition/planner/skill_invoke/final_response` 至少具备 start/end 或 error 事件。
3. Root 权限：非 root 用户访问 `/api/v1/admin/agent-traces/*` 必须返回 `AGENT_TRACE_ROOT_REQUIRED`。
4. 报告下载：root 用户可下载 Message 级 `report.md/report.json`，报告包含 Summary、User Message、Runtime Timeline、Skill/Tool Invocation、Final Response、Errors。
5. Loki 模式：生产配置启用 Loki Sink 后，可按 `tenant_uuid/session_id/message_id/run_id/node_kind/status` 查询到同一轮事件。
6. 脱敏策略：prompt、context、tool payload、executor result 的明细保存必须受 artifact policy 控制，不允许默认泄露完整敏感字段。

建议专项命令：

```bash
cd backend && go test ./internal/server/agent ./internal/service/agent_trace -run 'TestAgentRunTrace|TestAgentRunReport|TestAgentTraceRootOnly' -count=1
cd web-admin && npm run test:e2e -- tests/e2e/agent-run-trace-report.spec.ts
```

## 6. 回滚策略

1. 配置回滚：关闭 `protocol=skill` 选择权重。
2. 版本回滚：切换到上一个 `published` 版本。
3. 紧急回滚：将目标版本标记 `disabled`。

## 7. 里程碑

1. M1：文档与契约冻结
2. M2：注册与管理接口上线
3. M3：双路径运行时打通
4. M4：插件/第三方接入上线
5. M5：全量发布与运维交接
