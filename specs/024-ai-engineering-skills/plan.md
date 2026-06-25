# Implementation Plan: PowerX Skills 管理与治理

**Branch**: `024-ai-engineering-skills` | **Date**: 2026-03-09 | **Spec**: `/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/024-ai-engineering-skills/spec.md`
**Input**: Feature specification from `/specs/024-ai-engineering-skills/spec.md`

## Summary

交付 PowerX Skills 的平台级治理与 Agent 统一编排闭环：官方固有 Skills 目录管理、第三方 Bundle 受控导入、人工审批发布、版本回滚、Capability 绑定、Tenant/Agent 双路径调用一致性，以及“LLM 统一意图识别 + workflow/skill/tooling/llm 计划编排（串并行） + LLM Tool-Calling”统一执行。当前基线采用“上传 Bundle + 来源元数据登记、默认最新发布版本调用、checksum 强制校验、signature 策略可配置”，并通过多租户隔离与审计链路保障可追溯和可回放。
新增对齐目标：建立 Provider 无关的 Context 优化机制（分层上下文、预算裁剪、结构化摘要、Prompt/Context Cache 与 token 观测），在不改变 LLM 主路由原则下显著降低 token 成本与时延。
新增插件桥接目标：建立 PowerX Agent Skill Bridge，统一 PowerX Agent Runtime 与 PowerXPlugin Capability Handler 的边界；渠道和插件自有 Chat 必须进入 PowerX Agent Session，插件只声明源定义态 Skill 并提供 executor，PowerX 负责治理态 Skill、会话、权限、租户上下文、Planner 和审计。
新增插件 Plugin Registry 同步目标：PowerXPlugin 可在插件自有维护 Agent/Skill 开发态记录，但必须通过插件 backend proxy 同步到底座生成治理态 Skill、运行态 Agent 与 Agent-Skill Binding；PowerX 底座记录是 Agent Runtime 权威源，插件 Registry 只作为声明源、开发态配置和同步状态排障依据。
新增运行追踪目标：建立 PowerX Agent Run Trace & Report，按 Session/Message/Node 三层记录 Agent Runtime 结构化日志；本地开发写入 `backend/logs/agents`，生产写入 Loki；root 用户可在 Web Admin 查看节点链路并下载智能对话报告。
新增响应规划目标：建立 Agent ResponsePlanner / Context Builder / Final Response 分层机制；自然语言回答必须先生成 `response_plan`，按 `response_mode` 选择上下文，再由 final response 模型生成用户话术，并将 assistant message meta 落库用于去重、追问和 Trace 回放。
新增运行状态协议目标：建立 Agent Run State Protocol，统一 PowerX Agent Chat、Team Task、Agent Trace 与 PowerXPlugin 调试页对多任务、多 Agent、缺参等待、执行状态、结果链接和 trace 入口的展示语义。该协议以 `agent_run.*` 事件和 `AgentRunState` 历史快照为核心，不等同于 Google A2A，但 A2A handoff 必须映射到同一 task 状态模型。

## Technical Context

**Language/Version**: Go 1.24（backend services），Node 20 + Nuxt 4（web-admin）  
**Primary Dependencies**: Gin HTTP、google.golang.org/grpc（Buf）、GORM、Redis、PostgreSQL、OpenTelemetry、Nuxt UI、Pinia  
**Storage**: PostgreSQL（skills registry/execution trace/audit refs + plugin skill invocation trace + capability registry tooling catalog/trace）、Redis（selector/cache/policy snapshot）  
**Plugin Registry Storage**: PowerX 底座保存插件 Registry 来源映射与同步审计（`provider_plugin_id/plugin_agent_id/plugin_skill_id -> powerx_agent_uuid/powerx_skill_id`）；PowerXPlugin 插件侧保存开发态插件记录，二者通过同步 API 对齐。
**Agent Trace Storage**: Local File（`backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}`）+ Loki（生产日志源，可选）  
**Agent Context Storage**: Runtime Memory 仅保存本轮过程态；PostgreSQL 是 session/message/message meta/registry/binding/model policy/context_ref 权威源；Redis 只作为短 TTL planner/response_plan/candidate/recent-meta 缓存；Local File/Loki 保存 Trace artifact。  
**Agent Run State Storage**: SSE/WS 只负责实时 `agent_run.*` 事件；PostgreSQL 保存可恢复的 run state snapshot 与 message meta；Local File/Loki 保存完整 trace/report artifact；Redis 不作为历史权威。  
**Testing**: Go `go test`（unit/integration/contract）、OpenAPI/Proto 合约校验、web-admin 端 Vitest/Playwright 冒烟  
**Target Platform**: Linux server + modern browsers  
**Project Type**: CoreX backend module + web-admin management feature  
**Performance Goals**: 合法导入 95% 在 2 分钟内进入 draft；关键调用结果 99% 双路径语义一致  
**Constraints**: 多租户强隔离、发布前 checksum 强制、全量人工审批发布、首版禁止远程仓库在线拉取导入  
**Scale/Scope**: 100+ 租户，单租户 1k+ skill 版本记录，支持 10k+ 日调用审计检索

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

1. **CoreX Boundary (PASS)**：该能力属于 CoreX Agent/Capability 生态治理，不走插件注册机制；实现位于 backend/web-admin 现有 CoreX 结构。  
2. **Dual Transport (PASS)**：规划同时产出 HTTP 合同与 gRPC 合同（specs contracts + backend 对应传输层实现计划），满足规则集 `@dev-crud-http`、`@dev-crud-grpc`。  
3. **Migration & Repository Wiring (PASS)**：数据实体进入 `pkg/corex/db/persistence/model` 与统一迁移入口 `pkg/corex/db/database/migration.go`。  
4. **Tenant Security & Audit (PASS)**：调用前租户与授权检查、调用后审计与 trace 强制记录。  
5. **No Gate Violation (PASS)**：无需要豁免条款。

## Ruleset Alignment (Constitution Gates)

- **HTTP_PRESENT**: 提供 `contracts/http-openapi.yaml`，覆盖 admin/tenant skills 生命周期与调用入口。**PASS**
- **GRPC_PRESENT**: 提供 `contracts/skills-service.proto`，规划 gRPC 服务合同与错误语义映射。**PASS**
- **PROTOBUF_DEFINED**: 计划将 proto 落到 `api/grpc/contracts/powerx/skills/v1` 并通过 buf 生成到 `api/grpc/gen/go`。**PASS**
- **SERVER_DEFINED**: 计划在 `internal/transport/grpc/skills` 实现服务并接入全局拦截器链。**PASS**
- **MAKE_TARGETS**: 复用 `proto-gen/proto-lint/proto-clean`。**PASS**

## Project Structure

### Documentation (this feature)

```
specs/024-ai-engineering-skills/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── skills-service.proto
└── tasks.md
```

### Source Code (repository root)

```
backend/
├── api/grpc/contracts/powerx/skills/v1/
├── api/grpc/gen/go/powerx/skills/v1/
├── internal/service/skills/
├── internal/service/agent_trace/
├── internal/transport/http/admin/skills/
├── internal/transport/http/admin/agenttrace/
├── internal/transport/http/openapi/skills/
├── internal/transport/grpc/skills/
├── pkg/corex/db/persistence/model/skills/
├── pkg/corex/db/persistence/repository/skills/
└── pkg/corex/db/database/migration.go

web-admin/
├── app/pages/settings/ai/skills.vue
├── app/pages/agent/traces/
├── app/components/settings/ai/skills/
├── app/components/agent/trace/
└── app/composables/api/services/skillsService.ts

powerx-plugin/
├── framework/
│   ├── skills/
│   └── client/
└── connectors/
```

**Structure Decision**: 采用 CoreX 单体后端 + Nuxt 管理端的既有结构；合同先行（HTTP + gRPC）驱动 service/transport 实现，并由 web-admin 消费 admin contracts。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | - | - |

## Phase 0 – Research Summary

Reference: [`research.md`](./research.md)

- 固化首版导入策略：仅上传 Bundle，来源信息只登记不拉取。
- 固化版本解析：未显式传 version 时默认最新 published。
- 固化发布门禁：全量人工审批，草稿不可直接面向租户。
- 固化完整性策略：checksum 强制，signature 按环境策略可升级为强制。
- 固化官方目录来源：后端内置官方 catalog，随平台版本发布。

## Phase 1 – Design & Contracts

### Data Model (`data-model.md`)

- 定义 `SkillRegistryRecord`、`SkillExecutionTrace`、`SkillCapabilityBinding`、`SkillLifecycleAudit`、`OfficialSkillCatalogEntry`。
- 约束多版本并存、单一最新发布、不可变发布版本、来源与完整性规则。
- 明确状态机和并发冲突策略（发布/回滚互斥）。

### API Contracts (`contracts/`)

- `http-openapi.yaml`：覆盖 admin 列表/导入/发布/回滚/绑定、tenant 直接调用与统一调用。
- `skills-service.proto`：定义 gRPC 管理与调用服务，保证双传输契约一致。

### Quickstart (`quickstart.md`)

- 提供本地启动、迁移、合同校验、导入/发布/调用/回滚的最小闭环步骤。
- 包含审计与指标检查点，确保上线前可复核。

### Agent Context Update

- 将执行 `.specify/scripts/bash/update-agent-context.sh codex`，同步本 feature 新增技术上下文到 Codex 记忆文件。

## Phase 2 – Runtime Baseline (Unified Planner + Tool-Calling)

1. **入口收敛**：以 `/api/v1/agents/invoke` 与 `/api/v1/agents/stream/sse` 为主入口承载完整闭环；tenant `/tenant/skills/invoke` 与 `/tenant/invocations` 保留为执行层接口。  
2. **统一候选池**：意图阶段输出 `workflow|skill|tooling` 多候选（top-k），并附 `score/reason/constraints`。  
3. **计划编排**：Planner 产出 DAG/阶段计划，节点包含 `workflow|skill|tooling|llm`，支持串行依赖、并行批次、失败策略。  
4. **LLM Tool-Calling**：在受控候选清单内进行 tool 选择，选择结果回落到 registry/binding 的硬过滤约束。  
5. **无命中回落**：当未命中可执行节点时，直接进入 `llm` 回复，不得强行匹配 Flow。  
6. **观测升级**：trace 从 `trace_id` 扩展到 `plan_id + node_id` 维度，审计可还原计划执行路径。
7. **执行器升级**：`node.kind=skill|tooling` 必须接真实服务调用链路，禁止占位返回；其中 tooling 以 capability registry 数据库为权威源。
8. **候选分层**：候选池按 `workflow|skill|tooling` 分区，并按 `system builtin + agent custom` 双层来源合并去重后再进入 LLM 决策。
9. **提示词分区**：LLM 决策输入采用结构化分区清单（workflow/skill/tooling + source 标记），禁止仅用未分区的长文本拼接。

## Phase 12 – Context Optimization Baseline

Reference: [`context-optimization.md`](./context-optimization.md)

1. **上下文分层**：统一实现 `L0-L5` 分层拼装链路（固定前缀 → 能力目录摘要 → 会话摘要 → 最近窗口 → 检索片段 → 当前输入），并确保稳定顺序。  
2. **预算管理**：引入请求前 token 预算器，超预算时执行“检索裁剪 → 最近窗口裁剪 → 摘要刷新”的降载策略。  
3. **结构化记忆**：将会话摘要升级为结构化 schema（facts/decisions/open_issues/constraints），替代纯文本占位摘要。  
4. **缓存策略**：引入 `cache_mode=auto|force_off|force_on`，按 provider/model 能力探测决定 Prompt/Context Cache 启用。  
5. **可观测性**：对每次调用补齐 `prompt_tokens/completion_tokens/cached_tokens/context_layers_size/trim_actions`，并落盘到 debug trace 与审计聚合字段。  
6. **灰度发布**：按“仅观测 → 软裁剪 → 默认启用”三阶段推进，保留租户级回滚开关。

## Phase 13 – Planner Latency Optimization

Reference: [`context-optimization.md`](./context-optimization.md)

1. **候选预筛**：在 Planner 前做 `workflow|skill|tooling` 分区召回与 Top-K 预筛，避免全量候选进入 LLM。  
2. **Prompt 瘦身**：压缩 Planner 候选描述与参数 schema，仅保留决策必要字段并保持 JSON-only 合同。  
3. **决策缓存**：引入短 TTL Planner 决策缓存（统一 Redis 缓存机制），以候选指纹保障安全复用。  
4. **观测增强**：增加 `planner_candidates_before/after`、`planner_cache_hit`、`planner_latency_ms`、`planner_parse_retry` 等可观测字段。  
5. **灰度上线**：按租户灰度启用并对比基线，满足性能门槛后全量开启。  

## Phase 14 – A2A 多 Agent 协作基线

1. **团队模型**：新增主 Agent 与子 Agent 的 Team 配置模型，支持角色定义（planner/retriever/executor/reviewer）与启停控制。  
2. **任务分发**：在 Planner 中支持 `node.kind=agent_handoff`，由主 Agent 按依赖把子任务分发到指定子 Agent。  
3. **上下文隔离**：handoff 仅透传结构化输入与上下文引用（`context_ref`），禁止默认传递完整历史会话。  
4. **失败策略**：统一支持 `fail-fast|continue|retry-once`，并在最终汇总中返回子任务级状态。  
5. **审计与观测**：新增 `team_id/task_id/parent_agent_id/child_agent_id/handoff_trace_id` 字段，接入审计与 trace。  
6. **最小用例验证**：以“发布准备多智能体作业（1 主 3 子 + 1 汇总）”作为 PowerX Core-only 验收基线，验证分发、回收、部分失败与越权阻断。  
7. **Seed 初始化**：通过 PowerX Core seed upsert 初始化 `release.coordinator`、`release.knowledge_analyst`、`release.workflow_planner`、`release.notification_scheduler`、`release.readiness.team` 与 `powerx.release.*` 内置 demo Skills；这些记录是底座运行态数据，不依赖 PowerXPlugin 或插件同步。
8. **MVP 执行方式**：首版测试可显式构造 ExecutionPlan 并注入 deterministic handoff invoker，用于验证运行时语义；Team-aware Planner 自然语言自动拆分作为后续产品化任务。
9. **设计文档**：详细业务故事、seed 对象、计划结构、trace 字段和测试矩阵见 `docs/plan/ai_engineering/skills/multi_agent_a2a.md`。

## Phase 16 – PowerX Agent Skill Bridge 与插件 Framework 对齐

Reference: [`docs/plan/ai_engineering/skills/agent_skill_bridge.md`](../../docs/plan/ai_engineering/skills/agent_skill_bridge.md)

1. **Skill Package 源格式**：PowerX 与插件统一采用 `SKILL.md` 目录包作为 Skill 源格式；manifest/DTO/DB 仅作为解析后对象与治理态索引。
2. **桥接契约**：定义 `PluginSkillPackage/PluginSkillManifest/Invocation/Context/Result/Error`，明确插件源定义态 Skill 与 PowerX 治理态 Skill 的转换关系。
3. **Framework Runtime**：在 `powerx-plugin/framework/skills` 提供 Skill Package loader、validator、注册、schema 暴露、capability 映射和上下文校验封装。
4. **Framework Client**：在 `powerx-plugin/framework/client` 封装 STS、Agent Session HTTP、Agent SSE Stream、Agent WebSocket、Capability Invoke。
5. **插件发现导入**：PowerX 插件安装/启用时调用 `GET /api/v1/plugin/skills`，校验后导入为 `source=plugin/source_format=skill_package` 草稿 Skill，审批发布后进入 Agent 候选池。
6. **Registry 入库字段**：Skill Registry 保存 `raw_markdown/frontmatter_json/body_markdown/package_uri/package_checksum`，用于审计、导出和漂移检测。
7. **Executor 调用链路**：Agent `node.kind=skill` 命中插件来源 Skill 时，通过 Agent Skill Bridge 调用 PowerX Capability Invocation，并注入 `tenant_uuid/user_uuid/agent_id/session_id/message_id/trace_id`。
8. **本地 Chat 约束**：插件自有 Chat 页面只调用 PowerX Agent Session/Stream API，不直连插件领域业务接口；SSE/WS 事件由 Framework Client 解码。
9. **审计与阻断**：缺少关键上下文、插件未安装、executor capability 不匹配、Skill 未发布或未绑定 Agent 时必须 fail-fast 并记录审计。
10. **MediaX 验证样例**：以 `mediax.video_rebuilder.cn` 的 `SKILL.md` 包作为插件 Skill 样例，验证渠道消息、Web Chat 和插件自有 Chat 走同一 Agent Runtime 链路。

## Phase 16A – 插件 Agent/Skill Plugin Registry 同步

Reference: [`docs/plan/ai_engineering/skills/plugin_third_party_integration.md`](../../docs/plan/ai_engineering/skills/plugin_third_party_integration.md)

1. **同步边界**：PowerXPlugin 插件 Agent/Skill Local 是开发态声明源；PowerX 底座 `SkillRegistryRecord/Agent/AgentSkillBinding` 是运行态权威源。
2. **Skill 同步**：插件 backend proxy 提交插件 Skill manifest、prompt/schema、executor、capability、checksum；PowerX 校验后创建或更新 `source=plugin` 治理态 Skill，并返回 `powerx_skill_id/sync_status`。
3. **Agent 同步**：插件 backend proxy 提交插件 Agent 配置、模型配置引用、system prompt、已同步 Skill 列表；PowerX 创建或更新 Agent，并写入 Agent-Skill Binding。
4. **绑定校验**：Agent 同步请求绑定的 Skill 必须已发布、已审批、租户可见且来源插件匹配；不满足条件时 fail-fast，不创建半成品 Agent。
5. **状态刷新**：插件可通过后端 proxy 查询 PowerX 侧 Agent/Skill 状态，回写插件 Registry 的 `sync_status/sync_error/last_sync_at`。
6. **调试约束**：插件 Agent Chat 只能选择已同步成功的 `powerx_agent_uuid` 创建 PowerX Agent Session；未同步、漂移或失败记录只能展示为草稿/异常。
7. **漂移检测**：PowerX 保存 `manifest_snapshot/checksum/prompt_snapshot/bound_skill_ids`，用于识别插件自有 Local 与底座运行态记录差异。
8. **审计**：所有同步动作写入 `PluginRegistrySyncAudit`，字段至少包含 `provider_plugin_id/plugin_agent_id/plugin_skill_id/powerx_agent_uuid/powerx_skill_id/sync_action/sync_status/trace_id`。
9. **页面对齐**：PowerX 底座 Agent/Skill 管理页继续管理运行态记录；PowerXPlugin 可做对称页面，但其所有底座操作必须经插件 backend proxy。

## Phase 17 – Agent Run Trace & Report

Reference: [`docs/plan/ai_engineering/skills/agent_run_trace_report.md`](../../docs/plan/ai_engineering/skills/agent_run_trace_report.md)

1. **Trace DTO 与 Logger**：新增 `AgentRunMeta/AgentTraceEvent/AgentTraceNode/AgentRunReport` 与 `AgentTraceLogger`，作为 Agent Runtime 唯一结构化追踪入口。
2. **Local Sink**：实现 `PluginAgentTraceSink`，按 `backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}` 写入 `run.json/timeline.jsonl/nodes/*.json/artifacts/*`。
3. **Loki Sink**：实现 `LokiAgentTraceSink`，使用低基数 label 写入生产日志源；不将完整 prompt/payload 作为 label。
4. **Runtime 接入**：在 Agent Session/Runtime/Planner/Executor 关键节点接入 `StartNode/EndNode/FailNode`，覆盖 receive、context、intent、planner、skill/tool/llm、final、history。
5. **Root-only API**：新增 `/api/v1/admin/agent-traces/*` 查询与下载接口；后端强制 root 权限，非 root 返回 `AGENT_TRACE_ROOT_REQUIRED`。
6. **报告生成**：支持 Message 级 `report.md/report.json`，预留 Session 级与 zip 下载；报告必须包含 Summary、User Message、Runtime Timeline、Skill/Tool Invocation、Final Response、Errors。
7. **Web Admin 页面**：新增 root-only Agent Trace 页面，参考 AI Craft 报告布局：指标卡片、Message 列表、节点链路、Slot/Context 快照、下载按钮。
8. **脱敏与大小限制**：artifact 默认 redacted，支持 max bytes 限制；敏感字段必须通过策略进入报告。
9. **关联审计**：Agent Trace 必须与 `SkillExecutionTrace/InvocationTrace/A2A Handoff Trace` 通过 `trace_id/run_id/plan_id/node_id` 可互跳。

## Phase 21 – Agent Response Planning

Reference: [`docs/plan/ai_engineering/skills/agent_response_planning.md`](../../docs/plan/ai_engineering/skills/agent_response_planning.md)

1. **分层链路**：Agent 主入口 final 阶段拆为 `ResponsePlanner -> Context Builder -> Final Response LLM -> Persist Message Meta`，禁止把全局候选池直接塞进通用 prompt。
2. **ResponseMode**：定义 `capability_intro/capability_howto/skill_execution/clarify_params/normal_chat/error_explain`，由结构化 `ResponsePlan` 决定本轮回答模式。
3. **上下文按需注入**：Context Builder 按 `response_mode` 选择上下文；能力介绍只能读取当前 Agent 已绑定、已发布、租户可见且权限通过的能力。
4. **Message Meta**：assistant message 必须保存 `response_mode/capability_ids/response_plan_id/used_context_layers/tool_calls/final_response_model/model_selection`，用于同 session 去重和追问。
5. **模型策略**：新增 `response_planner` 节点模型选择预留点，首版可继承 Agent 默认模型；`context_builder` 为 deterministic，不使用模型做授权判断。
6. **SSE 与 Trace**：Agent Stream 输出 `response_plan` debug event；Agent Trace 记录 `response_planner/context_builder/final_response/history_persist` 节点。
7. **上下文驱动**：Runtime Memory 只保存过程态；DB 是业务上下文权威源；Redis 只做短 TTL 缓存；Local File/Loki 保存调试追踪。

## Phase 22 – Agent Run State Protocol

Reference: [`docs/plan/ai_engineering/skills/agent_run_state_protocol.md`](../../docs/plan/ai_engineering/skills/agent_run_state_protocol.md)

1. **标准事件**：定义 `agent_run.started/response_plan/intent_detected/plan_created/task_status/task_started/awaiting_params/task_completed/task_failed/final/ended`，作为 UI 首选事件语义。
2. **任务状态模型**：统一 `pending|awaiting_params|running|completed|failed|skipped` 状态，并要求 task payload 携带 run/session/message/trace/task/agent/skill/capability/action/result/error。
3. **缺参闭环**：Skill manifest 的 `action_required_args/slot_mapping/pending_task_policy` 是缺参判断和跨轮补参合并的业务事实源；Core 只执行通用校验和状态流转。
4. **结果展示**：Skill manifest 的 `result_presentation` 决定业务对象摘要与跳转链接；Final Response 没有真实 task result 时禁止输出成功性结论。
5. **历史恢复**：建立 `AgentRunState` 快照，页面刷新或从 Trace 页面进入时可恢复 session/message/task 状态。
6. **UI 对齐**：PowerX Agent Chat、Team Task、Agent Trace 与 PowerXPlugin Agent Chat 调试页必须消费同一 reducer 语义和组件模型。
7. **A2A 映射**：A2A `agent_handoff` 仍是多智能体调度能力，但必须映射为 `agent_run.task_status` 让用户看见子 Agent 节点状态。

## Implementation Backwrite (2026-03-19)

- 已完成 `T046`：新增 `backend/internal/service/skills/lifecycle_integrity_invoke_test.go`，覆盖状态机、完整性策略、默认版本解析单元测试。
- 已完成 `T049`：新增 `backend/tests/integration/skills/skill_nonfunc_integration_test.go`，覆盖导入耗时、调用一致性、审计写入基线。
- 运维注意事项：当前 `SkillExecutionTrace` 以 `trace_id` 唯一键存储，单次调用“resolved + completed”双写会发生后写冲突。现阶段基线统计以 `resolved` 事件为准，后续建议将 trace 写入改为 Upsert 或拆分事件模型。
- 已完成 `T071-T075`：Agent `skill/tooling` 节点已从占位执行升级为真实调用（skill invoke/adaptor + capability invocation），并补齐 `tenant_uuid/user_id/env/trace_id` 透传；文档已同步 tooling 落库权威说明。
- 已完成 `T076-T081`：候选池支持 `system builtin + agent custom` 双层聚合与同名去重（agent 优先），统一硬过滤在候选构建阶段前置执行；组合规划可串联 `workflow->skill/tooling`，并在 `plan/tasks` 与 SSE 节点事件中透传 `node_kind/node_ref/source_scope`。
- 测试矩阵已补齐：新增 `skill_agent_candidate_layering_test.go`（双层候选去重 + 未授权不可见）与 `skill_agent_composite_plan_test.go`（组合规划执行 + 事件字段校验），并纳入运行时回归清单。
