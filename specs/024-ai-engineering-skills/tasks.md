# Tasks: PowerX Skills 管理与治理

**Input**: Design documents from `/specs/024-ai-engineering-skills/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: 本特性要求双路径一致性、审计追溯和多租户隔离，需包含契约测试与集成测试。  
**Organization**: 任务按用户故事分组，保证每个故事可独立实现、独立验证与独立演示。

## Format: `[ID] [P?] [Story] Description`
- **[P]**: 可并行（不同文件且无直接依赖）
- **[Story]**: 对应用户故事（US1/US2/US3/US4）
- 每个任务均包含明确文件路径

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 [P] [Shared] 建立 Skills 模块目录骨架与占位 README：`backend/internal/service/skills/`, `backend/internal/transport/http/admin/skills/`, `backend/internal/transport/http/openapi/skills/`, `backend/internal/transport/grpc/skills/`, `backend/pkg/corex/db/persistence/model/skills/`, `backend/pkg/corex/db/persistence/repository/skills/`
- [X] T002 [P] [Shared] 将合同文件同步到后端权威位置并对齐命名：`backend/api/grpc/contracts/powerx/skills/v1/skills.proto`（基于 `specs/024-ai-engineering-skills/contracts/skills-service.proto`）
- [X] T003 [Shared] 生成并校验 proto 产物：`backend/api/grpc/gen/go/powerx/skills/v1/`
- [X] T004 [P] [Shared] 新建前端服务入口与页面壳：`web-admin/app/composables/api/services/skillsService.ts`, `web-admin/app/pages/settings/ai/skills.vue`

---

## Phase 2: Foundational (Blocking Prerequisites)

**⚠️ CRITICAL**: 本阶段完成前，不进入任何用户故事实现。

- [X] T005 [Shared] 定义核心实体模型 `SkillRegistryRecord` 与 `OfficialSkillCatalogEntry`：`backend/pkg/corex/db/persistence/model/skills/skill_registry_record.go`, `backend/pkg/corex/db/persistence/model/skills/official_skill_catalog_entry.go`
- [X] T006 [P] [Shared] 定义核心实体模型 `SkillCapabilityBinding` 与 `SkillLifecycleAudit`：`backend/pkg/corex/db/persistence/model/skills/skill_capability_binding.go`, `backend/pkg/corex/db/persistence/model/skills/skill_lifecycle_audit.go`
- [X] T007 [P] [Shared] 定义核心实体模型 `SkillExecutionTrace`：`backend/pkg/corex/db/persistence/model/skills/skill_execution_trace.go`
- [X] T008 [Shared] 挂载统一迁移入口：`backend/pkg/corex/db/database/migration.go`
- [X] T009 [P] [Shared] 实现 repository：`backend/pkg/corex/db/persistence/repository/skills/skill_registry_repository.go`
- [X] T010 [P] [Shared] 实现 repository：`backend/pkg/corex/db/persistence/repository/skills/skill_trace_repository.go`
- [X] T011 [Shared] 实现基础 service（状态机、latest published 指针、互斥发布/回滚）：`backend/internal/service/skills/lifecycle_service.go`
- [X] T012 [P] [Shared] 实现基础 service（导入校验、checksum/signature 策略、来源元数据登记）：`backend/internal/service/skills/import_service.go`
- [X] T013 [P] [Shared] 实现基础 service（调用路由、默认版本解析、权限前置检查）：`backend/internal/service/skills/invoke_service.go`
- [X] T014 [Shared] 接入审计与 trace 写入中间能力：`backend/internal/service/skills/audit_trace_service.go`

**Checkpoint**: 迁移、实体、仓储、核心服务可用；可进入用户故事开发。

---

## Phase 3: User Story 1 - 管理员统一管理 Skills 生命周期 (Priority: P1) 🎯 MVP

**Goal**: 管理员可完成登记、发布、回滚、停用与官方目录查看。  
**Independent Test**: 单个 Skill 可从 draft 发布、再回滚，且审计记录完整。

### Tests for User Story 1

- [X] T015 [P] [US1] HTTP 合同契约测试（admin 路由全集）：`backend/tests/contract/skills/http_admin_skills_contract_test.go`（对应 `specs/024-ai-engineering-skills/contracts/http-openapi.yaml`）
- [X] T016 [P] [US1] gRPC 合同契约测试（SkillAdminService）：`backend/tests/contract/skills/grpc_skill_admin_contract_test.go`（对应 `backend/api/grpc/contracts/powerx/skills/v1/skills.proto`）
- [X] T017 [P] [US1] 集成测试：发布/回滚状态机与 latest 指针正确：`backend/tests/integration/skills/skill_lifecycle_integration_test.go`

### Implementation for User Story 1

- [X] T018 [US1] 实现官方目录查询接口 `GET /admin/skills/catalog`：`backend/internal/transport/http/admin/skills/catalog_handler.go`
- [X] T019 [US1] 实现注册与列表接口 `POST/GET /admin/skills`：`backend/internal/transport/http/admin/skills/registry_handler.go`
- [X] T020 [US1] 实现发布接口 `POST /admin/skills/{skillId}/publish`（人工审批门禁）：`backend/internal/transport/http/admin/skills/publish_handler.go`
- [X] T021 [US1] 实现回滚接口 `POST /admin/skills/{skillId}/rollback`：`backend/internal/transport/http/admin/skills/rollback_handler.go`
- [X] T022 [US1] 实现管理路由装配与权限约束：admin root only：`backend/internal/transport/http/admin/skills/routes.go`
- [X] T023 [US1] 实现 gRPC SkillAdminService（List/Import/Publish/Rollback/Bind）：`backend/internal/transport/grpc/skills/admin_service.go`
- [X] T024 [US1] Web Admin 页面首版（目录 + registry 列表 + 发布/回滚按钮）：`web-admin/app/pages/settings/ai/skills.vue`

**Checkpoint**: US1 独立可演示（管理闭环完成）。

---

## Phase 4: User Story 2 - 开发者与第三方可受控导入 Skills (Priority: P1)

**Goal**: 支持上传 Bundle 导入、来源元数据登记、导入失败可解释。  
**Independent Test**: 完整导入成功进入 draft，缺失校验信息时被拒绝。

### Tests for User Story 2

- [X] T025 [P] [US2] 集成测试：上传导入 + metadata 追溯 + draft 状态：`backend/tests/integration/skills/skill_import_integration_test.go`
- [X] T026 [P] [US2] 集成测试：checksum 缺失/不匹配拒绝发布：`backend/tests/integration/skills/skill_integrity_integration_test.go`

### Implementation for User Story 2

- [X] T027 [US2] 实现导入接口 `POST /admin/skills/import`（仅 upload 模式）：`backend/internal/transport/http/admin/skills/import_handler.go`
- [X] T028 [US2] 实现导入业务规则（禁用远程仓库在线拉取）：`backend/internal/service/skills/import_service.go`
- [X] T029 [US2] 实现完整性策略开关（checksum 强制、signature 可配置强制）：`backend/internal/service/skills/integrity_policy.go`
- [X] T030 [US2] 实现 Web Admin 导入表单（bundle + source_url/source_ref）：`web-admin/app/components/settings/ai/skills/ImportForm.vue`
- [X] T031 [US2] 实现前端 API 调用与错误提示映射：`web-admin/app/composables/api/services/skillsService.ts`

**Checkpoint**: US2 独立可演示（受控导入闭环完成）。

---

## Phase 5: User Story 3 - 租户与 Agent 双路径调用一致 (Priority: P2)

**Goal**: `tenant/skills/invoke` 与 `tenant/invocations` 返回一致语义，默认版本解析稳定。  
**Independent Test**: 同一 skill 两条路径调用结果一致，未传 version 自动命中 latest published。

### Tests for User Story 3

- [X] T032 [P] [US3] 集成测试：直接调用路径（tenant/skills/invoke）：`backend/tests/integration/skills/skill_invoke_direct_integration_test.go`
- [X] T033 [P] [US3] 集成测试：统一入口路径（tenant/invocations, preferred_protocol=skill）：`backend/tests/integration/skills/skill_invoke_unified_integration_test.go`
- [X] T034 [P] [US3] 集成测试：未传 version 默认 latest published：`backend/tests/integration/skills/skill_default_version_integration_test.go`

### Implementation for User Story 3

- [X] T035 [US3] 实现 `POST /tenant/skills/invoke`：`backend/internal/transport/http/openapi/skills/invoke_handler.go`
- [X] T036 [US3] 实现统一入口 skill 适配（preferred_protocol=skill）：`backend/internal/service/skills/adapter_service.go`
- [X] T037 [US3] 实现 gRPC SkillInvokeService：`backend/internal/transport/grpc/skills/invoke_service.go`
- [X] T038 [US3] 实现 capability 绑定接口 `POST /admin/skills/{skillId}/bind-capability`：`backend/internal/transport/http/admin/skills/binding_handler.go`
- [X] T039 [US3] 对齐错误码与统一响应 envelope：`backend/internal/service/skills/response_mapper.go`

**Checkpoint**: US3 独立可演示（双路径一致性达成）。

---

## Phase 6: User Story 4 - 审计与隔离满足治理要求 (Priority: P2)

**Goal**: 全链路审计可追溯、跨租户访问严格阻断。  
**Independent Test**: 调用与管理动作均可按 trace_id 检索，跨租户查询被拒绝。

### Tests for User Story 4

- [X] T040 [P] [US4] 集成测试：关键管理动作审计记录完整：`backend/tests/integration/skills/skill_audit_integration_test.go`
- [X] T041 [P] [US4] 集成测试：跨租户 trace 查询阻断：`backend/tests/integration/skills/skill_tenant_isolation_integration_test.go`

### Implementation for User Story 4

- [X] T042 [US4] 实现审计查询接口与筛选：`backend/internal/transport/http/admin/skills/audit_handler.go`
- [X] T043 [US4] 补全审计字段（import/publish/rollback/bind/invoke）：`backend/internal/service/skills/audit_trace_service.go`
- [X] T044 [US4] 补全 trace 指标与标签（skill_id/version/tenant_uuid）：`backend/internal/service/skills/metrics.go`
- [X] T045 [US4] Web Admin 增加审计与版本历史抽屉：`web-admin/app/components/settings/ai/skills/AuditDrawer.vue`

**Checkpoint**: US4 独立可演示（治理与隔离闭环达成）。

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T046 [P] [Shared] 补充单元测试（状态机、完整性策略、默认版本解析）：`backend/internal/service/skills/*_test.go`
- [X] T047 [P] [Shared] 运行 quickstart 全链路并记录结果：`specs/024-ai-engineering-skills/quickstart.md`
- [X] T048 [P] [Shared] 文档回写（实现偏差、接口样例、运维注意事项）：`specs/024-ai-engineering-skills/*.md`
- [X] T049 [Shared] 性能与可靠性基线验证（导入耗时、调用一致性、审计写入成功率）：`backend/tests/integration/skills/skill_nonfunc_integration_test.go`
- [X] T050 [Shared] 设计并落地 Skill 匹配硬过滤层（tenant/scope/status/tool_grants/source）：`backend/internal/server/agent/*`, `backend/internal/service/skills/*`
- [X] T051 [P] [Shared] 实现候选召回与重排流程编排（硬过滤后 top-k 输出）：`backend/internal/server/agent/intent/*`, `backend/internal/server/agent/manager_intent.go`
- [X] T052 [P] [Shared] 增加高基数场景回归测试（单 Agent 10k Skill 不走全量扫描主路径）：`backend/tests/integration/skills/skill_matching_scale_integration_test.go`
- [X] T053 [Shared] 增加租户来源策略配置接口（GET/PUT source allowlist）并接入租户设置存储：`backend/internal/transport/http/admin/agent/*`, `backend/internal/service/skills/*`, `backend/pkg/corex/db/persistence/repository/setting/*`
- [X] T054 [P] [Shared] 在 AI 设置页增加 Skills 来源策略可视化配置卡片（含校验与保存提示）：`web-admin/app/pages/settings/ai/*`, `web-admin/app/composables/api/services/*`

---

## Phase 8: Agent 统一编排基线（LLM Intent + Planner + Tool-Calling）

- [X] T055 [Shared] 在 Spec/Contract 层补充 Agent 主入口闭环契约（invoke/stream 的 plan 结构、node 事件语义）：`specs/024-ai-engineering-skills/contracts/http-openapi.yaml`, `docs/plan/ai_engineering/skills/api_contracts.md`
- [X] T056 [P] [Shared] 扩展技能候选模型（skills match candidate 增加 intent/tags/semantic metadata）：`backend/pkg/corex/db/persistence/model/skills/*`, `backend/pkg/corex/db/persistence/repository/skills/*`
- [X] T057 [Shared] 实现多技能召回与重排输出（top-k + reason），并接入 Agent 意图阶段：`backend/internal/server/agent/intent/*`, `backend/internal/service/skills/adapter_service.go`
- [X] T058 [Shared] 实现 Planner DAG（串并行分组、依赖、失败策略）并接入 Agent invoke 主链路：`backend/internal/transport/http/admin/agent/chat_handler.go`, `backend/internal/server/agent/runtime/*`
- [X] T059 [Shared] 在 Planner 决策层接入 LLM Tool-Calling（受控工具清单 + 参数校验 + fallback）：`backend/internal/server/agent/*`, `backend/internal/service/skills/*`
- [X] T060 [P] [US3] 新增集成测试：多技能并行计划执行一致性：`backend/tests/integration/skills/skill_multi_plan_integration_test.go`
- [X] T061 [P] [US3] 新增集成测试：Tool-Calling 命中未授权技能应被硬过滤拒绝：`backend/tests/integration/skills/skill_tool_call_authz_integration_test.go`
- [X] T062 [US4] 扩展审计/追踪模型（plan_id/node_id/node_status/retry_trace）并提供查询：`backend/internal/service/skills/audit_trace_service.go`, `backend/internal/transport/http/admin/skills/audit_handler.go`
- [X] T063 [P] [US1] Web Admin 增加 Planner/执行图可视化（按 plan_id 查看节点轨迹）：`web-admin/app/pages/settings/ai/skills.vue`, `web-admin/app/components/settings/ai/skills/*`

---

## Phase 9: 去除旧策略（移除 Flow-only 规划路径）

- [X] T064 [Shared] 文档对齐：移除“Flow-only/按 flow 路由候选”叙述，统一为 LLM 意图识别 + `workflow|skill|tooling|llm` 计划编排：`specs/024-ai-engineering-skills/*.md`, `docs/plan/ai_engineering/skills/*.md`
- [X] T065 [Shared] 重构 Agent 候选装载层：以统一候选池替换 `routesByFlow` 单一来源，并按 agent 绑定/租户策略过滤：`backend/internal/server/agent/manager_tool_calling.go`, `backend/internal/server/agent/manager_intent.go`, `backend/internal/server/agent/bootstrap/*`
- [X] T066 [P] [US3] 重构 Planner 节点模型：显式 `node.kind=workflow|skill|tooling|llm`，禁止以 flow 作为计划唯一节点类型：`backend/internal/server/agent/runtime/*`, `backend/pkg/dto/stream_events.go`
- [X] T067 [P] [US3] 重构执行分发器：按 `node.kind` 路由到 workflow/skill/tooling/llm 执行器，未命中时回落普通对话：`backend/internal/server/agent/runtime/engine.go`, `backend/internal/server/agent/manager_execute.go`
- [X] T068 [P] [US3] 集成测试：验证“仅 seed third-party skill、无 flow 路由”也能被 Agent 识别与执行：`backend/tests/integration/skills/skill_agent_unified_orchestration_test.go`
- [X] T069 [P] [US3] 集成测试：验证无意图命中时返回普通 LLM 回复，且 SSE 事件包含 `intent` + `final` 无执行节点：`backend/tests/integration/skills/skill_agent_no_intent_fallback_test.go`
- [X] T070 [US4] 观测增强：统一输出 `intent/plan/node_start/node_end/final` 事件结构，并标注 `planner_mode=unified`：`backend/internal/transport/http/admin/agent/chat_handler.go`, `backend/pkg/dto/stream_events.go`

---

## Phase 10: 真实执行链路升级（Skill/Tooling 落库权威）

- [X] T071 [Shared] 将 Agent `node.kind=skill` 执行器接入真实 Skill 服务链（direct invoke + unified adapter），移除占位返回：`backend/internal/server/agent/manager.go`, `backend/internal/server/agent/manager_execute.go`, `backend/internal/server/agent/bootstrap/init.go`
- [X] T072 [Shared] 将 Agent `node.kind=tooling` 执行器接入 Capability InvocationService（capability registry 为 tooling 落库权威）：`backend/internal/server/agent/manager.go`, `backend/internal/server/agent/manager_execute.go`, `backend/internal/server/agent/bootstrap/init.go`
- [X] T073 [P] [US3] 执行上下文补全：透传 `tenant_uuid/user_id/env/trace_id` 到统一编排执行元数据，保证 skill/tooling 调用具备租户语义：`backend/internal/server/agent/runtime/engine.go`
- [X] T074 [P] [US3] 单元测试：校验 `skill/tooling` 节点调用器已接线并返回统一结果元数据：`backend/internal/server/agent/manager_execute_unified_invoker_test.go`
- [X] T075 [Shared] 文档回写：明确 tooling 的数据库权威源为 capability registry（非内存-only），并同步开发计划：`specs/024-ai-engineering-skills/*.md`, `docs/plan/ai_engineering/skills/*.md`

---

## Phase 11: 候选分层与组合规划（System + Agent）

- [X] T076 [Shared] 设计并实现候选聚合器：按 `workflow|skill|tooling` 分区，合并 `system builtin + agent custom` 两层来源并去重：`backend/internal/server/agent/*`, `backend/internal/service/skills/*`, `backend/internal/service/capability_registry/*`
- [X] T077 [Shared] 统一硬过滤前置到候选构建阶段（tenant/visibility/status/source/tool_grants/agent binding），禁止未授权候选进入 LLM：`backend/internal/server/agent/manager_intent.go`, `backend/internal/server/agent/manager_tool_calling.go`
- [X] T078 [P] [US3] 重构 LLM 决策输入：从“单段工具文本”升级为结构化分区清单（workflow/skill/tooling + source 标记 + 参数 schema）：`backend/internal/server/agent/manager_tool_calling.go`
- [X] T079 [P] [US3] 扩展 Planner 节点元信息：新增 `source_scope=system|agent` 及组合依赖标注（workflow->skill/tooling）：`backend/pkg/corex/flow/schemas/plan.go`, `backend/internal/server/agent/manager_plan.go`, `backend/internal/server/agent/runtime/*`
- [X] T080 [P] [US3] 集成测试：验证“system + agent 同名候选去重优先级”和“未授权候选不可见”：`backend/tests/integration/skills/skill_agent_candidate_layering_test.go`
- [X] T081 [P] [US3] 集成测试：验证组合规划（workflow->skill/tooling）可执行且事件中包含 `node_kind/node_ref/source_scope`：`backend/tests/integration/skills/skill_agent_composite_plan_test.go`
- [X] T082 [Shared] 文档回写：补齐 system/agent 双层候选策略、组合规划与测试矩阵：`specs/024-ai-engineering-skills/*.md`, `docs/plan/ai_engineering/skills/*.md`

---

## Phase 12: Context 优化机制（Token 成本与延迟治理）

- [X] T083 [Shared] 设计并实现 Context 分层拼装器（L0-L5：固定前缀/能力目录摘要/结构化摘要/最近窗口/检索片段/当前输入）：`backend/internal/server/agent/runtime/*`, `backend/internal/service/agent/*`
- [X] T084 [Shared] 实现请求前 token 预算器与裁剪策略（retrieval trim -> recent window trim -> summary refresh）：`backend/internal/server/agent/runtime/*`, `backend/internal/service/agent/*`
- [X] T085 [P] [Shared] 将会话摘要升级为结构化 schema（facts/decisions/open_issues/constraints）并兼容旧文本摘要：`backend/internal/service/agent/chat_history_service.go`, `backend/internal/server/agent/persistence/model/session_gorm.go`
- [X] T086 [P] [Shared] 新增 context optimizer 配置（enabled/max_prompt_tokens/reserved_completion_tokens/recent_messages/retrieval_top_k/cache_mode/summary_refresh_interval_sec）：`etc/config.yaml`, `backend/internal/server/agent/config/*`, `backend/internal/server/agent/bootstrap/*`
- [X] T087 [Shared] 实现 Provider 无关缓存策略探测与透传（OpenAI/Anthropic/Gemini/自托管能力探测）：`backend/internal/service/ai/*`, `backend/internal/server/agent/runtime/*`
- [X] T088 [P] [US4] 观测增强：记录并落盘 `prompt_tokens/completion_tokens/cached_tokens/context_layers_size/trim_actions`：`backend/internal/transport/http/admin/agent/chat_handler.go`, `backend/internal/service/skills/audit_trace_service.go`, `backend/logs/agent_debug/*`
- [X] T089 [P] [US3] 集成测试：30+轮会话下预算裁剪可用且无上下文爆窗：`backend/tests/integration/skills/skill_agent_context_budget_integration_test.go`
- [X] T090 [P] [US3] 集成测试：缓存支持模型的前缀命中率与响应时延回归：`backend/tests/integration/skills/skill_agent_prompt_cache_integration_test.go`
- [X] T091 [Shared] 文档与操作手册回写：新增 Context 优化章节与验证步骤：`specs/024-ai-engineering-skills/context-optimization.md`, `specs/024-ai-engineering-skills/quickstart.md`, `docs/guides/agent/skills/*`

---

## Phase 13: Planner 提速专项（候选压缩 + Prompt 瘦身 + 决策缓存）

- [X] T092 [Shared] 实现 Planner 候选预筛（Top-K + 分区配额），禁止全量候选直接拼接进入 Planner：`backend/internal/server/agent/manager_tool_calling.go`, `backend/internal/server/agent/manager_intent.go`
- [X] T093 [P] [Shared] 实现 Planner Prompt 瘦身（compact schema + 去冗余说明），并保持 JSON-only 决策契约：`backend/internal/server/agent/manager_tool_calling.go`
- [X] T094 [P] [US3] 实现 Planner 决策缓存（短 TTL + 候选指纹），并接入统一缓存机制（Redis driver）：`backend/internal/server/agent/*`, `backend/internal/service/agent/*`
- [X] T095 [P] [US4] 增强观测字段（before/after 候选数、planner cache hit、planner parse retry），落盘到 `agent_debug` 与审计：`backend/internal/transport/http/admin/agent/chat_handler.go`, `backend/internal/service/skills/audit_trace_service.go`
- [X] T096 [P] [US3] 集成测试：hello-echo/prompt-template 场景下 planner 延迟与 token 回归（对比基线）：`backend/tests/integration/skills/skill_agent_planner_latency_integration_test.go`
- [X] T097 [Shared] Web Admin 配置扩展：Context Optimizer 页面增加 Planner Optimizer 配置项与发布流程：`web-admin/app/pages/settings/ai/context-optimizer.vue`, `web-admin/app/composables/api/services/aiSettingService.ts`, `web-admin/i18n/locales/zh.json`, `web-admin/i18n/locales/en.json`
- [X] T098 [Shared] 文档回写：补充 Planner 提速操作手册、灰度方案、回滚策略：`specs/024-ai-engineering-skills/context-optimization.md`, `specs/024-ai-engineering-skills/quickstart.md`, `docs/guides/agent/skills/test_use_cases/04_intent_to_planner_decision.md`

---

## Phase 14: A2A 多 Agent 协作（主 Agent 分发与子 Agent 回收）

- [X] T099 [Shared] 扩展 Agent 计划模型：新增 `node.kind=agent_handoff` 与 handoff metadata（team_id/task_id/failure_policy）：`backend/pkg/corex/flow/schemas/plan.go`, `backend/internal/server/agent/manager_plan.go`
- [X] T100 [P] [Shared] 新增 A2A 数据模型与迁移：`AgentTeam/AgentTeamMember/AgentHandoffTask/AgentSharedContextRef`：`backend/pkg/corex/db/persistence/model/agent/*`, `backend/pkg/corex/db/database/migration.go`
- [X] T101 [Shared] 实现 Team 管理服务（创建团队、成员绑定、启停、策略更新）：`backend/internal/service/agent/team_service.go`, `backend/pkg/corex/db/persistence/repository/agent/*`
- [X] T102 [P] [US3] 实现 A2A 分发执行器（主 Agent -> 子 Agent 调用、结果回传、超时控制）：`backend/internal/server/agent/runtime/engine.go`, `backend/internal/server/agent/manager_execute.go`
- [X] T103 [P] [US4] 实现上下文引用隔离与授权校验（context_ref allowlist）：`backend/internal/service/agent/context_ref_service.go`, `backend/internal/server/agent/runtime/*`
- [X] T104 [US4] 扩展审计与追踪字段（team/task/handoff 维度）：`backend/internal/service/skills/audit_trace_service.go`, `backend/internal/transport/http/admin/skills/audit_handler.go`
- [X] T105 [P] [US3] 集成测试：最小 A2A 并行用例（1 主 2 子）端到端通过：`backend/tests/integration/skills/skill_agent_a2a_basic_integration_test.go`
- [X] T106 [P] [US3] 集成测试：子 Agent 失败策略 `continue` 返回部分成功：`backend/tests/integration/skills/skill_agent_a2a_partial_failure_integration_test.go`
- [X] T107 [P] [US4] 集成测试：子 Agent 越权访问上下文被阻断并记录审计：`backend/tests/integration/skills/skill_agent_a2a_context_authz_integration_test.go`
- [X] T108 [US1] Web Admin 增加 A2A 团队配置页（主 Agent、子 Agent、失败策略）：`web-admin/app/pages/settings/ai/agent-teams.vue`, `web-admin/app/composables/api/services/agentTeamService.ts`
- [X] T109 [Shared] 文档回写与 runbook：A2A 调试步骤、回滚策略、指标口径：`specs/024-ai-engineering-skills/*.md`, `docs/guides/agent/skills/*`

---

## Phase 15: A2A 团队体验收敛（可见性、可操作性、验收一致性）

- [X] T110 [US1] 团队管理页信息层级重构：默认展示 Agent 名称/Key，ID 下沉为次级信息，修正成员表格与弹窗排版：`web-admin/app/pages/settings/ai/agent-teams.vue`
- [X] T111 [US1] 团队任务入口去硬编码：统一从路由参数与团队选择器驱动 `team_id`，并在无有效 team 时给出可操作提示：`web-admin/app/components/agent/AgentWorkspace.vue`, `web-admin/app/pages/agent/team-tasks.vue`
- [X] T112 [US1] 角色配置体验增强：团队页面提供 TL 选择与固定子 Agent Role 选择，子 Agent role 为 `retriever/executor/reviewer`：`web-admin/app/pages/settings/ai/agent-teams.vue`, `web-admin/i18n/locales/zh.json`, `web-admin/i18n/locales/en.json`
- [X] T113 [US3] 协作过程展示强化：执行过程卡片补充节点分组与状态可读性，减少“只看到状态看不懂语义”的情况：`web-admin/app/components/agent/MessageItem.vue`
- [X] T114 [US4] 增加 A2A 审计查询前端能力（按 team_id/handoff_task_id 过滤）并与会话页面建立跳转：`web-admin/app/composables/api/services/skillsService.ts`, `web-admin/app/pages/settings/ai/skills.vue`, `web-admin/app/components/settings/ai/skills/*`
- [X] T115 [US4] 后端补充 A2A trace 查询契约回归测试（team/handoff 维度筛选与租户隔离）：`backend/tests/integration/skills/skill_agent_a2a_trace_filter_integration_test.go`
- [X] T116 [US3] 前端 E2E：覆盖“1 主 2 子并行 + 部分失败 continue”场景，断言页面出现 Intent/Plan/Node 与最终汇总：`web-admin/tests/e2e/agent-team-collab.spec.ts`
- [X] T117 [US1] 文档对齐：将团队协作验收剧本接入 024 快速验收主线，补齐“页面可见 vs 审计可查”步骤：`specs/024-ai-engineering-skills/quickstart.md`, `docs/guides/agent/multi_agent/09_a2a_team_collab_progressive.md`
- [X] T118 [Shared] 回归与发布门禁：执行 `go test ./internal/transport/http/admin/agent ./internal/service/agent` 与前端 lint/E2E 基线，记录证据：`specs/024-ai-engineering-skills/quickstart.md`

---

## Phase 16: PowerX Agent Skill Bridge 与插件 Framework 对齐

- [X] T119 [Shared] 文档对齐：新增 Agent Skill Bridge 机制总说明并挂接 runtime/plugin/API/spec：`docs/plan/ai_engineering/skills/agent_skill_bridge.md`, `docs/plan/ai_engineering/skills/*.md`, `specs/024-ai-engineering-skills/*.md`
- [X] T120 [P] [US6] 定义插件 Skill Runtime 标准类型：`powerx-plugin/framework/skills`（`PluginSkillManifest/PluginSkillInvocation/PluginSkillInvocationContext/PluginSkillResult/PluginSkillError`）
- [X] T121 [P] [US6] 定义插件 Framework Client 标准接口：`powerx-plugin/framework/client`（STS、Agent Invoke、Agent SSE、Agent WS、Capability Invoke）
- [X] T122 [US6] 实现插件 Skill 发现路由封装：`GET /api/v1/plugin/skills`, `GET /api/v1/plugin/skills/:skill_id/schema`，并提供 manifest 校验器：`powerx-plugin/framework/skills`
- [X] T123 [US6] 实现插件 Capability Handler 路由封装：PowerX Capability Invocation，按 `skill_id` 分发到插件 capability handler：`powerx-plugin/framework/skills`
- [X] T124 [US6] 实现插件 capability handler 上下文强校验：缺少 `tenant_uuid/user_uuid/agent_id/session_id/skill_id/trace_id` 必须 fail-fast：`powerx-plugin/framework/skills`
- [X] T125 [US6] 在 PowerX 插件安装/启用流程接入 Skill 发现导入：调用插件 `GET /api/v1/plugin/skills`，导入为 `source=plugin` 草稿 Skill：`backend/internal/infra/plugin/manager/*`, `backend/internal/service/skills/import_service.go`
- [X] T126 [US6] 在 Agent Skill 执行链路接入插件 capability handler 调用：`node.kind=skill` 且 `source=plugin` 时调用 PowerX Capability Invocation：`backend/internal/server/agent/manager_execute.go`, `backend/internal/service/skills/adapter_service.go`
- [X] T127 [US6] 补齐插件调用 STS/delegated context 注入：禁止静态旧 token，按 `007-integration-gateway-and-mcp` delegated contract 获取 bearer：`backend/internal/infra/plugin/manager/*`, `powerx-plugin/framework/client`
- [X] T128 [P] [US6] 新增插件 Skill Invocation Trace 模型与审计字段：`backend/pkg/corex/db/persistence/model/skills/*`, `backend/pkg/corex/db/persistence/repository/skills/*`, `backend/internal/service/skills/audit_trace_service.go`
- [X] T129 [P] [US6] 扩展错误码映射：`skill.plugin_not_installed`, `skill.plugin_executor_unavailable`, `skill.plugin_context_missing`, `skill.plugin_capability_mismatch`：`backend/internal/service/skills/response_mapper.go`
- [X] T130 [US6] 插件调试 Chat 示例接入 Framework Client：通过 PowerX Agent Session/Stream API 调试插件 Skill，不直连插件业务 API：`powerx-plugin/connectors/*`, `web-admin/app/pages/plugins/*`
- [X] T131 [P] [US6] 集成测试：插件 Skill 发现导入为草稿，非法 manifest 被拒绝：`backend/tests/integration/skills/skill_plugin_discovery_import_integration_test.go`
- [X] T132 [P] [US6] 集成测试：Agent 命中插件 Skill 后调用插件 capability handler，并校验上下文完整：`backend/tests/integration/skills/skill_plugin_bridge_invoke_integration_test.go`
- [X] T133 [P] [US6] 集成测试：缺少上下文、capability 不匹配、插件未安装时 fail-fast 并写审计：`backend/tests/integration/skills/skill_plugin_bridge_failfast_integration_test.go`
- [X] T134 [P] [US6] E2E 验证：插件调试 Chat 与 Web Agent Chat 走相同 Agent Runtime 事件链路：`web-admin/tests/e2e/plugin-agent-skill-bridge.spec.ts`
- [X] T135 [Shared] Quickstart 回写：补充 MediaX `mediax.video_rebuilder.cn` 样例、插件调试 Chat、SSE/WS 验收和审计查询步骤：`specs/024-ai-engineering-skills/quickstart.md`

---

## Phase 17: Agent Run Trace & Report（Root 调试与智能对话报告）

- [X] T136 [Shared] 文档对齐：新增 Agent Run Trace & Report 机制说明，并挂接 runtime/spec/contracts/tasks：`docs/plan/ai_engineering/skills/agent_run_trace_report.md`, `docs/plan/ai_engineering/skills/runtime_architecture.md`, `specs/024-ai-engineering-skills/*.md`
- [X] T137 [P] [US7] 定义 Agent Trace DTO 与 Logger 接口：`backend/internal/service/agent_trace/types.go`, `backend/internal/service/agent_trace/logger.go`
- [X] T138 [P] [US7] 实现 Local Agent Trace Sink：按 `backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}` 写入 `run.json/timeline.jsonl/nodes/*.json`：`backend/internal/service/agent_trace/local_sink.go`
- [X] T139 [P] [US7] 实现 Loki Agent Trace Sink 与 label 规范：`backend/internal/service/agent_trace/loki_sink.go`, `backend/internal/service/agent_trace/config.go`
- [X] T140 [US7] 实现 Composite Sink 与 fail-fast context 校验：缺少 `tenant_uuid/session_id/message_id/run_id/trace_id` 必须返回稳定错误：`backend/internal/service/agent_trace/logger.go`
- [X] T141 [US7] Agent Runtime 接入 StartRun/CompleteRun：`backend/internal/server/agent/manager.go`, `backend/internal/server/agent/runtime/engine.go`
- [X] T142 [US7] Agent Runtime 关键节点接入 StartNode/EndNode/FailNode：覆盖 `receive_message/session_restore/permission_check/context_load/intent_recognition/planner/skill_invoke/tool_invoke/llm_call/final_response/history_persist`：`backend/internal/server/agent/*`
- [X] T143 [US7] Agent Skill Bridge 与 A2A 节点补齐 trace 关联字段：`trace_id/run_id/plan_id/node_id/skill_id/plugin_id/capability_id/team_id/handoff_task_id`：`backend/internal/server/agent/manager_execute.go`, `backend/internal/service/skills/*`
- [X] T144 [P] [US7] 实现 Agent Run Report Builder（`report.md/report.json`）：`backend/internal/service/agent_trace/report_builder.go`
- [X] T145 [US7] 实现 root-only Agent Trace HTTP API：`GET /api/v1/admin/agent-traces/messages/:message_id`, `/timeline`, `/report`, `/sessions/:session_id/report`：`backend/internal/transport/http/admin/agenttrace/*`
- [X] T146 [US7] Web Admin 新增 root-only Agent Trace 页面：指标卡片、Message 列表、节点链路、节点详情、下载按钮：`web-admin/app/pages/agent/traces/`, `web-admin/app/components/agent/trace/*`
- [X] T147 [P] [US7] 前端 API service 与类型定义：`web-admin/app/composables/api/services/agentTraceService.ts`, `web-admin/app/composables/api/types/agentTrace.ts`
- [X] T148 [P] [US7] 后端单元测试：Local Sink 目录与文件格式、Logger fail-fast、Report Builder 输出：`backend/internal/service/agent_trace/*_test.go`
- [X] T149 [P] [US7] 后端集成测试：触发 Agent Stream 后可按 `message_id` 查询 timeline 与下载报告：`backend/tests/integration/skills/agent_run_trace_report_integration_test.go`
- [X] T150 [P] [US7] 权限测试：非 root 查询/下载 Agent Trace 返回 `AGENT_TRACE_ROOT_REQUIRED`：`backend/tests/contract/http/agent_trace/root_only_contract_test.go`
- [X] T151 [P] [US7] E2E：root 页面查看 Message Trace、节点详情并下载报告：`web-admin/tests/e2e/agent-run-trace-report.spec.ts`
- [X] T152 [Shared] Quickstart 回写：记录本地 `backend/logs/agents` 验收、Loki 查询样例、报告下载样例与回滚策略：`specs/024-ai-engineering-skills/quickstart.md`

---

## Phase 18: Runtime Intent 与节点级模型策略

- [X] T153 [Shared] 文档对齐：补齐 Runtime Intent / Control Command、自然语言意图识别边界、节点级模型选择策略：`docs/plan/ai_engineering/skills/runtime_architecture.md`, `docs/plan/ai_engineering/skills/api_contracts.md`, `specs/024-ai-engineering-skills/spec.md`
- [X] T154 [Shared] OpenAPI 对齐：Agent invoke/stream 增加结构化 `intent` 与 `model_policy` 契约，明确 `agent.bound_capabilities` 可绕过 LLM/Planner：`specs/024-ai-engineering-skills/contracts/http-openapi.yaml`
- [X] T155 [Shared] 实现 Runtime Intent Router：仅接受结构化 intent，禁止自然语言关键词穷举触发控制面查询：`backend/internal/server/agent/runtime/intent_router.go`, `backend/internal/transport/http/admin/agent/chat_handler.go`
- [X] T156 [Shared] 实现节点级模型策略骨架：默认继承 Agent 默认模型，预留 `runtime_intent/intent_classifier/planner/skill_param_extractor/final_response/reviewer` 节点选择结果：`backend/internal/server/agent/runtime/model_policy.go`
- [X] T157 [Shared] Planner 接入节点模型选择预留点：Planner LLM 调用读取 `planner` 节点 provider/model，默认不改变现有模型行为：`backend/internal/server/agent/manager_tool_calling.go`
- [X] T158 [P] [US4] 观测对齐：SSE meta/final metadata 输出 `runtime_intent`、`model_policy`、`model_selection`、`llm_bypassed`、`planner_bypassed`：`backend/internal/transport/http/admin/agent/chat_handler.go`

---

## Phase 19: 声明式 A2A 团队示例

- [X] T159 [Shared] 文档对齐：定义由 Team 配置、已发布 Skill Revision 和通用 Runtime 组成的 A2A 团队边界，业务场景不进入 Core 路由：`docs/guides/agent/runtime/declarative-skill-runtime.md`, `specs/024-ai-engineering-skills/*.md`
- [X] T160 [Shared] 实现声明式 A2A demo seed：创建营销活动复盘团队、成员绑定、对象存储包与营销 Skill Revision；不以业务 key 选择 Core executor：`backend/cmd/database/seed/seed_native_marketing_agents.go`, `backend/cmd/database/seed/seed.go`
- [X] T161 [P] [US5] Seed 回归测试：重复 seed 三次后 Agent、Skill、Binding、Team、TeamMember 数量稳定，Skill 均为 latest published：`backend/tests/integration/skills/skill_agent_a2a_release_readiness_mvp_test.go`
- [X] T162 [US5] Core-only A2A MVP 执行测试：读取 seed 数据，显式构造 3 个 `agent_handoff` 节点和 1 个汇总节点，注入 deterministic handoff invoker，验证最终报告包含风险摘要、发布流程、回滚步骤、通知计划：`backend/tests/integration/skills/skill_agent_a2a_release_readiness_mvp_test.go`
- [X] T163 [P] [US5] A2A 部分失败测试：`failure_policy=continue` 时单个子 Agent 失败，主 Agent 返回部分成功并在 trace/report 中标注失败子任务：`backend/tests/integration/skills/skill_agent_a2a_release_readiness_mvp_test.go`
- [X] T164 [P] [US5] A2A 上下文隔离测试：子 Agent 只收到主 Agent 显式下发的 release metadata/context_refs/上游摘要，不能默认继承完整 session 或未授权候选：`backend/tests/integration/skills/skill_agent_a2a_release_readiness_mvp_test.go`
- [X] T165 [US7] Agent Trace 对齐：`agent_handoff` 节点写入 `team_uuid/team_key/parent_agent_uuid/child_agent_uuid/handoff_task_id/failure_policy/child_run_id`，Message 报告展示三个子 Agent 输入输出摘要：`backend/internal/server/agent/manager_execute.go`
- [X] T166 [Shared] Quickstart 回写：记录 seed 命令、SQL 校验、Core-only 测试命令和 Agent Trace 验收证据：`specs/024-ai-engineering-skills/quickstart.md`

---

## Phase 20: A2A Fixed Team Role Enum

- [X] T167 [Shared] 明确 Team Role 为平台固定协作枚举，集中维护 `planner/retriever/executor/reviewer`，不建立数据库字典表：`backend/pkg/corex/db/persistence/model/agent/a2a_gorm.go`
- [X] T168 [US5] Team Service 写入成员时基于集中枚举校验子 Agent role，拒绝 `planner` 或未知 role：`backend/internal/service/agent/team_service.go`
- [X] T169 [US5] Team 管理页面使用固定子 Agent role 选项 `retriever/executor/reviewer`，不调用角色目录接口：`web-admin/app/composables/api/services/agentTeamService.ts`, `web-admin/app/pages/settings/ai/agent-teams.vue`
- [X] T170 [P] [US5] A2A release readiness 文档和测试移除 role catalog 依赖，保留 TeamMember 固定 role 验收：`backend/tests/integration/skills/skill_agent_a2a_release_readiness_mvp_test.go`, `docs/plan/ai_engineering/skills/multi_agent_a2a.md`, `specs/024-ai-engineering-skills/quickstart.md`

---

## Phase 21: Agent Response Planning

- [X] T171 [Shared] 文档对齐：补齐 Agent ResponsePlanner / Context Builder / Final Response 分层设计、上下文存储驱动、SSE event、message meta 和回归步骤：`docs/plan/ai_engineering/skills/agent_response_planning.md`, `docs/plan/ai_engineering/skills/runtime_architecture.md`, `docs/plan/ai_engineering/skills/api_contracts.md`, `specs/024-ai-engineering-skills/*.md`
- [X] T172 [Shared] 定义 ResponseMode / ResponsePlan / AssistantMessageMeta 类型与 schema 校验：`backend/internal/server/agent/runtime/response_plan.go`
- [X] T173 [Shared] 实现 ResponsePlanner 骨架：支持结构化 JSON 输出、schema 校验、非法输出稳定错误 `agent.response_plan_invalid`，首版模型选择继承 Agent 默认模型：`backend/internal/server/agent/runtime/response_planner.go`
- [X] T174 [Shared] 将节点级模型策略扩展到 `response_planner/context_builder/error_explain`，Trace/SSE metadata 输出节点模型选择结果：`backend/internal/server/agent/runtime/model_policy.go`
- [X] T175 [US3] 改造 Context Builder 按 `response_mode` 注入上下文，`capability_intro/capability_howto` 只能读取当前 Agent 绑定能力，禁止全局候选池进入用户可见上下文：`backend/internal/server/agent/runtime/context_builder.go`
- [X] T176 [US3] 实现 Final Response mode-specific prompt 模板：覆盖 `capability_intro/capability_howto/skill_execution/clarify_params/normal_chat/error_explain`：`backend/internal/server/agent/runtime/final_response_prompt.go`
- [X] T177 [US3] 持久化 assistant message meta：写入 `response_mode/capability_ids/response_plan_id/used_context_layers/tool_calls/final_response_model/model_selection`，并提供最近 capability intro 查询：`backend/internal/server/agent/runtime/sink_history.go`, `backend/internal/service/agent/chat_history_service.go`
- [X] T178 [US3] Agent Stream 输出 `response_plan` debug event，并确保插件 Chat/Web Chat/SSE 调试面板可接收同一事件语义：`backend/internal/transport/http/admin/agent/chat_handler.go`
- [X] T179 [US7] Agent Trace 增加 `response_planner/context_builder/final_response/history_persist` 节点快照字段：response mode、context layers、target capability ids、model selection、error summary：`backend/internal/service/agent_trace/*`, `backend/internal/server/agent/*`
- [X] T180 [P] [US3] 单元测试：ResponsePlan schema、mode 选择、非法 JSON、missing fields、repeat intro meta 去重：`backend/internal/server/agent/runtime/*_test.go`
- [X] T181 [P] [US3] 集成测试：只绑定单个 Skill 的 Agent 询问“你能做什么”时，最终回答与 context package 只包含当前绑定能力，不包含全局 system/public 候选：`backend/tests/integration/skills/skill_agent_response_planning_test.go`
- [X] T182 [P] [US3] 集成测试：`capability_howto/clarify_params/skill_execution/error_explain` 四种模式可通过 Trace 和 message meta 验证：`backend/tests/integration/skills/skill_agent_response_planning_test.go`
- [X] T183 [US7] Web Admin Agent Trace 页面展示 response plan、context layers、message meta 摘要，并支持从 Chat 消息跳转到对应 Message Trace：`web-admin/app/pages/agent/traces/`, `web-admin/app/components/agent/trace/*`

---

## Phase 22: Agent Run State Protocol（多任务/多智能体执行状态 UI 协议）

- [X] T184 [Shared] 文档对齐：新增 Agent Run State Protocol 设计并挂接 Runtime、Response Planning、Skill 标准、024 spec/plan/tasks：`docs/plan/ai_engineering/skills/agent_run_state_protocol.md`, `docs/plan/ai_engineering/skills/*.md`, `specs/024-ai-engineering-skills/*.md`
- [X] T185 [Shared] 定义 `AgentRunState/AgentTaskState/AgentRunEvent` DTO 与 schema 校验，覆盖 `agent_run.*` 事件和 `pending|awaiting_params|running|completed|failed|skipped` 状态：`backend/internal/server/agent/runtime/*`, `backend/pkg/dto/*`
- [X] T186 [Shared] Runtime SSE/WS 输出 `agent_run.started/response_plan/intent_detected/plan_created/task_status/task_started/awaiting_params/task_completed/task_failed/final/ended` 标准事件：`backend/internal/server/agent/runtime/engine.go`, `backend/internal/transport/http/admin/agent/chat_handler.go`
- [X] T187 [US3] 实现 Pending Task 状态存储与跨轮 slot merge，按 Skill manifest `action_required_args/slot_mapping/pending_task_policy` 校验缺参：`backend/internal/server/agent/runtime/*`, `backend/internal/service/agent/*`
- [X] T188 [US7] Agent Trace/Report 保存 run state snapshot，并支持按 `session_id/message_id/task_id/node_id` 精确定位：`backend/internal/service/agent_trace/*`, `backend/internal/transport/http/admin/agenttrace/*`
- [X] T189 [US3] Web Admin Agent Chat、Team Task 与 Trace 页面渲染统一 Run State 面板：`web-admin/app/components/agent/*`, `web-admin/app/pages/agent/traces/`, `web-admin/app/pages/agent/team-tasks*`
- [X] T190 [US6] 与 PowerXPlugin Framework 对齐 AgentRunState typed events/reducer，确保插件调试 Chat 与 PowerX Web Chat 消费同一状态协议：`PowerXPlugin/framework/backend/go/runtime/powerx/agent/*`, `PowerXPlugin/skeleton/web-admin/*`
- [X] T191 [P] [US3] 单元测试：缺参任务进入 `awaiting_params`、补参后进入 `running/completed`、无真实 result 时禁止 success 文案：`backend/internal/server/agent/runtime/*_test.go`
- [X] T192 [P] [US5] 集成测试：Core-only A2A 多 Agent 任务映射为 run state task 列表，失败子任务可跳转 trace：`backend/tests/integration/skills/*agent_run_state*_test.go`
- [X] T193 [Shared] 将 Agent Run State Protocol 收敛为唯一对外运行状态合同，禁止 UI 消费旧 `intent/plan/node_start/node_end/final/end` 作为状态来源：`docs/plan/ai_engineering/skills/agent_run_state_protocol.md`, `backend/internal/server/agent/runtime/run_state_events.go`
- [X] T194 [Shared] 扩展 Run Summary / Task Graph 字段：`total_tasks/current_stage/depends_on/stage/parallel_group/parent_task_id/failure_policy`，覆盖串行、并行、多 Agent handoff：`backend/pkg/dto/stream_events.go`, `backend/internal/service/agent_trace/types.go`
- [X] T195 [US7] Trace snapshot 保存 `summary` 与计划阶段任务，页面刷新后仍能恢复总任务状态和任务拓扑：`backend/internal/service/agent_trace/local_sink.go`
- [X] T196 [US6] PowerXPlugin 调试页展示 Run 总览、阶段分组、并行/依赖信息，使用 `agent_run.plan_created` 预置 pending 任务并由后续 task 事件更新：`PowerXPlugin/skeleton/web-admin/nuxt/app/pages/_p/com.powerx.plugins.base/admin/agent-skill-bridge/index.vue`

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 无依赖，可立即开始。
- Phase 2 依赖 Phase 1，且阻塞全部用户故事。
- Phase 3-6 均依赖 Phase 2 完成。
- Phase 7 依赖至少一个用户故事完成，最终收敛前需全部故事完成。
- Phase 15 依赖 Phase 14，作为 A2A 体验与验收收口阶段，发布前必须完成。
- Phase 16 依赖 Phase 8-11 的 Agent Skill 执行链路与 Phase 5 的 Skill invoke 基线；同时依赖 `007-integration-gateway-and-mcp` 的 STS/delegated gateway 契约和 `009-install-plugin-pxp` 的插件生命周期钩子。
- Phase 17 依赖 Phase 8 的 Agent Runtime 闭环、Phase 12 的 Context 观测字段、Phase 14 的 A2A trace 字段与 Phase 16 的插件 Skill Bridge trace 字段；首版可先交付 Local Sink + Root Report，Loki Sink 可作为生产增强。
- Phase 18 依赖 Phase 8 的 Agent Stream 主入口、Phase 13 的 Planner 优化上下文与 Phase 17 的 trace metadata；首版模型策略只提供默认继承与节点级预留点。
- Phase 19 依赖 Phase 14 的 A2A 数据模型与执行器、Phase 17 的 Agent Trace 报告、Phase 18 的模型策略 metadata；Phase 19 是 PowerX Core-only 验收线，不依赖 Phase 16 的插件桥接。
- Phase 22 依赖 Phase 17 的 Agent Trace、Phase 19 的 A2A task 字段与 Phase 21 的 Response Planning；它是 UI/Runtime 共享状态协议，不改变插件业务事实源边界。

### User Story Dependencies

- US1 是 MVP，建议优先完成并先验收。
- US2 依赖 US1 的 registry 与审批能力。
- US3 依赖 US1 的发布/绑定结果与 US2 的导入资产。
- US4 贯穿全局，但可在 US1 完成后并行推进。
- US6 依赖 US1/US2 的 Skill Registry 与导入治理能力，依赖 US3 的 Agent/Skill 执行链路。
- US7 依赖 Agent 主入口与 Runtime 节点存在稳定 `session_id/message_id/trace_id`，并与 Skill/Tooling/A2A/Plugin Bridge 事件字段对齐。

### Within Each Story

- 先写并运行失败测试（契约/集成），再实现功能。
- 模型/仓储在前，服务在中，传输层与前端在后。
- 同一文件任务顺序执行，不标记 `[P]`。

---

## Parallel Execution Examples

### Phase 2 可并行

```bash
Task: "定义 SkillCapabilityBinding 与 SkillLifecycleAudit 模型（T006）"
Task: "定义 SkillExecutionTrace 模型（T007）"
Task: "实现 skill_registry_repository（T009）"
Task: "实现 skill_trace_repository（T010）"
```

### US1 测试可并行

```bash
Task: "HTTP 合同契约测试 admin 路由全集（T015）"
Task: "gRPC 合同契约测试 SkillAdminService（T016）"
Task: "生命周期与 latest 指针集成测试（T017）"
```

### US3 双路径测试可并行

```bash
Task: "直接调用路径集成测试（T032）"
Task: "统一入口路径集成测试（T033）"
Task: "默认版本解析集成测试（T034）"
```

---

## Implementation Strategy

### MVP First

1. 完成 Phase 1-2。
2. 完成 US1（Phase 3）并独立验收。
3. 若需快速交付，先发布 US1 管理闭环。

### Incremental Delivery

1. US1（管理闭环）  
2. US2（受控导入）  
3. US3（双路径调用一致）  
4. US4（审计与隔离）  
5. Phase 7 收敛

### Notes

- 每完成一个 Phase 或一个 US，建议单独提交并记录验证证据。
- 若合同变更，优先更新 `contracts/` 并同步契约测试再改实现。
