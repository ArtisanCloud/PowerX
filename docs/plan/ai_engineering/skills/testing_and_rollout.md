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
3. Core-only A2A 发布准备 MVP：验证 seed / plan / trace / 部分失败 / 上下文隔离主链路。  
   用例：`backend/tests/integration/skills/skill_agent_a2a_release_readiness_mvp_test.go`
4. 建议回归命令：

```bash
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go test ./tests/integration/skills \
  -run 'TestSkillAgentCandidateLayering_SystemAgentDedupeAndAuthzFilter|TestSkillAgentCompositePlanExecuteWithEventSourceScope|TestSkillAgentA2AReleaseReadiness.*' \
  -count=1
```

### 2.6 Core-only A2A 多智能体专项

该专项只验证 PowerX Core 底座自有多智能体机制，不依赖 PowerXPlugin、MediaX、AI Craft 或任意插件 executor。

测试场景：发布准备多智能体作业。

1. Seed 初始化：
   - `release.coordinator`
   - `release.knowledge_analyst`
   - `release.workflow_planner`
   - `release.notification_scheduler`
   - `release.readiness.team`
   - `powerx.release.knowledge_analysis`
   - `powerx.release.workflow_planning`
   - `powerx.release.notification_schedule`
   - `powerx.release.report_synthesis`
2. MVP 执行：
   - 主 Agent 显式构造 3 个 `agent_handoff` 节点和 1 个汇总节点。
   - 三个子 Agent 分别返回风险分析、发布流程、通知计划。
   - 主 Agent 汇总发布准备报告。
3. 失败策略：
   - 任一子 Agent 失败且 `failure_policy=continue` 时，最终报告必须标注部分失败。
4. 上下文隔离：
   - 子 Agent 只接收主 Agent 下发的结构化上下文切片，不得默认继承完整 session。
5. Trace：
   - 可按 `team_id/team_name/handoff_task_id/child_agent_id` 定位节点。

建议测试文件：

```text
backend/tests/integration/skills/skill_agent_a2a_release_readiness_mvp_test.go
```

建议回归命令：

```bash
cd backend && go test ./tests/integration/skills \
  -run 'TestSkillAgentA2AReleaseReadinessSeed|TestSkillAgentA2AReleaseReadinessMVP|TestSkillAgentA2AReleaseReadinessPartialFailure' \
  -count=1
```

### 2.7 Agent Response Planning 专项

该专项验证 Agent 最终回复的分层链路，重点防止 LLM 把全局候选池误说成当前 Agent 能力。

测试范围：

1. ResponsePlanner：
   - 输出合法 `ResponsePlan`。
   - 支持 `capability_intro/capability_howto/skill_execution/clarify_params/normal_chat/error_explain`。
   - 非法 JSON 或非法 capability id 返回稳定错误。
2. Context Builder：
   - 按 `response_mode` 注入上下文。
   - 能力介绍只读取当前 Agent 绑定能力。
   - Redis/内存缓存不能改变 DB 权威候选边界。
3. Final Response：
   - 不输出机器 ID、schema 原文、executor path。
   - 缺参数时进入澄清，不直接失败。
   - 错误解释脱敏。
4. Message Meta：
   - assistant message 写入 `response_mode/capability_ids/response_plan_id/used_context_layers/final_response_model`。
   - 同一 session 重复询问能力时，基于 meta 去重，不靠文本匹配。
5. SSE / Trace：
   - Agent Stream 输出 `response_plan` event。
   - Agent Trace 中存在 `response_planner/context_builder/final_response/history_persist` 节点。

建议回归命令：

```bash
cd backend && go test ./internal/server/agent/... \
  -run 'Test.*ResponsePlan|Test.*ContextBuilder|Test.*FinalResponse|Test.*AssistantMessageMeta' \
  -count=1

cd backend && go test ./tests/integration/skills \
  -run 'TestSkillAgentResponsePlanning.*' \
  -count=1
```

验收失败时的第一定位点：

1. 回复出现未绑定能力：查 `response_plan.target_capability_ids` 与 `context_builder.used_capability_ids`。
2. 重复介绍能力：查 assistant message meta 是否落库。
3. 无法解释本轮回答：查 Agent Trace 是否缺少 `response_planner` 或 `context_builder` 节点。

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
