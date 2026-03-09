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

- [ ] T015 [P] [US1] HTTP 合同契约测试（admin 路由全集）：`backend/tests/contract/skills/http_admin_skills_contract_test.go`（对应 `specs/024-ai-engineering-skills/contracts/http-openapi.yaml`）
- [ ] T016 [P] [US1] gRPC 合同契约测试（SkillAdminService）：`backend/tests/contract/skills/grpc_skill_admin_contract_test.go`（对应 `backend/api/grpc/contracts/powerx/skills/v1/skills.proto`）
- [ ] T017 [P] [US1] 集成测试：发布/回滚状态机与 latest 指针正确：`backend/tests/integration/skills/skill_lifecycle_integration_test.go`

### Implementation for User Story 1

- [ ] T018 [US1] 实现官方目录查询接口 `GET /admin/skills/catalog`：`backend/internal/transport/http/admin/skills/catalog_handler.go`
- [ ] T019 [US1] 实现注册与列表接口 `POST/GET /admin/skills`：`backend/internal/transport/http/admin/skills/registry_handler.go`
- [ ] T020 [US1] 实现发布接口 `POST /admin/skills/{skillId}/publish`（人工审批门禁）：`backend/internal/transport/http/admin/skills/publish_handler.go`
- [ ] T021 [US1] 实现回滚接口 `POST /admin/skills/{skillId}/rollback`：`backend/internal/transport/http/admin/skills/rollback_handler.go`
- [ ] T022 [US1] 实现管理路由装配与权限约束：admin root only：`backend/internal/transport/http/admin/skills/routes.go`
- [ ] T023 [US1] 实现 gRPC SkillAdminService（List/Import/Publish/Rollback/Bind）：`backend/internal/transport/grpc/skills/admin_service.go`
- [ ] T024 [US1] Web Admin 页面首版（目录 + registry 列表 + 发布/回滚按钮）：`web-admin/app/pages/settings/ai/skills.vue`

**Checkpoint**: US1 独立可演示（管理闭环完成）。

---

## Phase 4: User Story 2 - 开发者与第三方可受控导入 Skills (Priority: P1)

**Goal**: 支持上传 Bundle 导入、来源元数据登记、导入失败可解释。  
**Independent Test**: 完整导入成功进入 draft，缺失校验信息时被拒绝。

### Tests for User Story 2

- [ ] T025 [P] [US2] 集成测试：上传导入 + metadata 追溯 + draft 状态：`backend/tests/integration/skills/skill_import_integration_test.go`
- [ ] T026 [P] [US2] 集成测试：checksum 缺失/不匹配拒绝发布：`backend/tests/integration/skills/skill_integrity_integration_test.go`

### Implementation for User Story 2

- [ ] T027 [US2] 实现导入接口 `POST /admin/skills/import`（仅 upload 模式）：`backend/internal/transport/http/admin/skills/import_handler.go`
- [ ] T028 [US2] 实现导入业务规则（禁用远程仓库在线拉取）：`backend/internal/service/skills/import_service.go`
- [ ] T029 [US2] 实现完整性策略开关（checksum 强制、signature 可配置强制）：`backend/internal/service/skills/integrity_policy.go`
- [ ] T030 [US2] 实现 Web Admin 导入表单（bundle + source_url/source_ref）：`web-admin/app/components/settings/ai/skills/ImportForm.vue`
- [ ] T031 [US2] 实现前端 API 调用与错误提示映射：`web-admin/app/composables/api/services/skillsService.ts`

**Checkpoint**: US2 独立可演示（受控导入闭环完成）。

---

## Phase 5: User Story 3 - 租户与 Agent 双路径调用一致 (Priority: P2)

**Goal**: `tenant/skills/invoke` 与 `tenant/invocations` 返回一致语义，默认版本解析稳定。  
**Independent Test**: 同一 skill 两条路径调用结果一致，未传 version 自动命中 latest published。

### Tests for User Story 3

- [ ] T032 [P] [US3] 集成测试：直接调用路径（tenant/skills/invoke）：`backend/tests/integration/skills/skill_invoke_direct_integration_test.go`
- [ ] T033 [P] [US3] 集成测试：统一入口路径（tenant/invocations, preferred_protocol=skill）：`backend/tests/integration/skills/skill_invoke_unified_integration_test.go`
- [ ] T034 [P] [US3] 集成测试：未传 version 默认 latest published：`backend/tests/integration/skills/skill_default_version_integration_test.go`

### Implementation for User Story 3

- [ ] T035 [US3] 实现 `POST /tenant/skills/invoke`：`backend/internal/transport/http/openapi/skills/invoke_handler.go`
- [ ] T036 [US3] 实现统一入口 skill 适配（preferred_protocol=skill）：`backend/internal/service/skills/adapter_service.go`
- [ ] T037 [US3] 实现 gRPC SkillInvokeService：`backend/internal/transport/grpc/skills/invoke_service.go`
- [ ] T038 [US3] 实现 capability 绑定接口 `POST /admin/skills/{skillId}/bind-capability`：`backend/internal/transport/http/admin/skills/binding_handler.go`
- [ ] T039 [US3] 对齐错误码与统一响应 envelope：`backend/internal/service/skills/response_mapper.go`

**Checkpoint**: US3 独立可演示（双路径一致性达成）。

---

## Phase 6: User Story 4 - 审计与隔离满足治理要求 (Priority: P2)

**Goal**: 全链路审计可追溯、跨租户访问严格阻断。  
**Independent Test**: 调用与管理动作均可按 trace_id 检索，跨租户查询被拒绝。

### Tests for User Story 4

- [ ] T040 [P] [US4] 集成测试：关键管理动作审计记录完整：`backend/tests/integration/skills/skill_audit_integration_test.go`
- [ ] T041 [P] [US4] 集成测试：跨租户 trace 查询阻断：`backend/tests/integration/skills/skill_tenant_isolation_integration_test.go`

### Implementation for User Story 4

- [ ] T042 [US4] 实现审计查询接口与筛选：`backend/internal/transport/http/admin/skills/audit_handler.go`
- [ ] T043 [US4] 补全审计字段（import/publish/rollback/bind/invoke）：`backend/internal/service/skills/audit_trace_service.go`
- [ ] T044 [US4] 补全 trace 指标与标签（skill_id/version/tenant_uuid）：`backend/internal/service/skills/metrics.go`
- [ ] T045 [US4] Web Admin 增加审计与版本历史抽屉：`web-admin/app/components/settings/ai/skills/AuditDrawer.vue`

**Checkpoint**: US4 独立可演示（治理与隔离闭环达成）。

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T046 [P] [Shared] 补充单元测试（状态机、完整性策略、默认版本解析）：`backend/internal/service/skills/*_test.go`
- [ ] T047 [P] [Shared] 运行 quickstart 全链路并记录结果：`specs/024-ai-engineering-skills/quickstart.md`
- [ ] T048 [P] [Shared] 文档回写（实现偏差、接口样例、运维注意事项）：`specs/024-ai-engineering-skills/*.md`
- [ ] T049 [Shared] 性能与可靠性基线验证（导入耗时、调用一致性、审计写入成功率）：`backend/tests/integration/skills/skill_nonfunc_integration_test.go`

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 无依赖，可立即开始。
- Phase 2 依赖 Phase 1，且阻塞全部用户故事。
- Phase 3-6 均依赖 Phase 2 完成。
- Phase 7 依赖至少一个用户故事完成，最终收敛前需全部故事完成。

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
