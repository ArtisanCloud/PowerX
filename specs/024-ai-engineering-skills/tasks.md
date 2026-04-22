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

- [ ] T110 [US1] 团队管理页信息层级重构：默认展示 Agent 名称/Key，ID 下沉为次级信息，修正成员表格与弹窗排版：`web-admin/app/pages/settings/ai/agent-teams.vue`
- [ ] T111 [US1] 团队任务入口去硬编码：统一从路由参数与团队选择器驱动 `team_id`，并在无有效 team 时给出可操作提示：`web-admin/app/components/agent/AgentWorkspace.vue`, `web-admin/app/pages/agent/team-tasks.vue`
- [ ] T112 [US1] 角色配置体验增强：明确“TL 唯一 planner，子 Agent 仅 retriever/executor/reviewer”的前端交互与文案：`web-admin/app/pages/settings/ai/agent-teams.vue`, `web-admin/i18n/locales/zh.json`, `web-admin/i18n/locales/en.json`
- [ ] T113 [US3] 协作过程展示强化：执行过程卡片补充节点分组与状态可读性，减少“只看到状态看不懂语义”的情况：`web-admin/app/components/agent/MessageItem.vue`
- [ ] T114 [US4] 增加 A2A 审计查询前端能力（按 team_id/handoff_task_id 过滤）并与会话页面建立跳转：`web-admin/app/composables/api/services/skillsService.ts`, `web-admin/app/pages/settings/ai/skills.vue`, `web-admin/app/components/settings/ai/skills/*`
- [ ] T115 [US4] 后端补充 A2A trace 查询契约回归测试（team/handoff 维度筛选与租户隔离）：`backend/tests/integration/skills/skill_agent_a2a_trace_filter_integration_test.go`
- [ ] T116 [US3] 前端 E2E：覆盖“1 主 2 子并行 + 部分失败 continue”场景，断言页面出现 Intent/Plan/Node 与最终汇总：`web-admin/tests/e2e/agent-team-collab.spec.ts`
- [ ] T117 [US1] 文档对齐：将团队协作验收剧本接入 024 快速验收主线，补齐“页面可见 vs 审计可查”步骤：`specs/024-ai-engineering-skills/quickstart.md`, `docs/guides/agent/multi_agent/09_a2a_team_collab_progressive.md`
- [ ] T118 [Shared] 回归与发布门禁：执行 `go test ./internal/transport/http/admin/agent ./internal/service/agent` 与前端 lint/E2E 基线，记录证据：`specs/024-ai-engineering-skills/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 无依赖，可立即开始。
- Phase 2 依赖 Phase 1，且阻塞全部用户故事。
- Phase 3-6 均依赖 Phase 2 完成。
- Phase 7 依赖至少一个用户故事完成，最终收敛前需全部故事完成。
- Phase 15 依赖 Phase 14，作为 A2A 体验与验收收口阶段，发布前必须完成。

### User Story Dependencies

- US1 是 MVP，建议优先完成并先验收。
- US2 依赖 US1 的 registry 与审批能力。
- US3 依赖 US1 的发布/绑定结果与 US2 的导入资产。
- US4 贯穿全局，但可在 US1 完成后并行推进。

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
