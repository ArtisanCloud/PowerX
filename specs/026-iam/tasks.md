# Tasks: IAM 用户与角色 RBAC 统一能力

**Input**: Design documents from `/specs/026-iam/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: 本特性为高风险权限与身份上下文收敛，包含测试任务（后端 contract/integration + 前端 unit/e2e）。

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`
- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions
- Backend: `backend/internal/...`, `backend/tests/...`, `backend/api/grpc/contracts/...`
- Web Admin: `web-admin/app/...`, `web-admin/tests/...`
- Spec docs: `specs/026-iam/...`

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 建立本 feature 的开发与验证基线

- [x] T001 [P] [Shared] 同步 IAM 设计契约到实现参考文档：`specs/026-iam/contracts/http-openapi.yaml`、`specs/026-iam/contracts/iam-rbac-admin.proto`
- [x] T002 [P] [Shared] 建立回归测试目录骨架：`backend/tests/contract/iam/`、`backend/tests/integration/iam/`、`web-admin/tests/unit/settings-users/`、`web-admin/tests/e2e/iam/`
- [x] T003 [Shared] 在 `specs/026-iam/quickstart.md` 固化本地/部署环境验证命令与账号矩阵（root/admin/member）

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 任何用户故事开始前必须完成的基础能力

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T004 [Shared] 统一身份上下文 DTO 与返回语义，对齐 `backend/internal/transport/http/admin/user/auth/me_handler.go` 与 `backend/internal/service/auth/me_service.go`
- [x] T005 [P] [Shared] 收敛租户上下文提取/校验逻辑，统一 `backend/internal/transport/http/admin/iam/tenant_scope.go` 与 `backend/internal/transport/http/admin/system/tenant_context.go`
- [x] T006 [P] [Shared] 在 `web-admin/app/stores/user.ts` 增加上下文刷新与缓存失效策略（force refresh、跨标签页同步）
- [x] T007 [Shared] 增强统一错误映射（权限不足/上下文失效），涉及 `backend/internal/transport/http/admin/user/auth/me_handler.go`、`web-admin/app/components/settings/users/UsersShell.vue`
- [x] T008 [P] [Shared] 为 IAM gRPC 管理语义补齐合同占位与注释对齐：`backend/api/grpc/contracts/powerx/iam/v1/member.proto`、`backend/internal/transport/grpc/iam/`

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - 角色边界清晰可执行 (Priority: P1) 🎯 MVP

**Goal**: 明确并可验证 root / tenant admin / member 的权限边界

**Independent Test**: 三类角色账号登录后，用户管理能力范围与越权拦截符合预期

### Tests for User Story 1

- [x] T009 [P] [US1] 新增 HTTP 合同测试（角色边界矩阵）：`backend/tests/contract/iam/rbac_boundary_contract_test.go`
- [x] T010 [P] [US1] 新增集成测试（跨租户访问拒绝）：`backend/tests/integration/iam/tenant_admin_isolation_integration_test.go`

### Implementation for User Story 1

- [x] T011 [P] [US1] 收敛角色判定服务：`backend/internal/service/iam/rbac_service.go`
- [x] T012 [P] [US1] 收敛成员可见范围判定：`backend/internal/service/iam/member_service.go`
- [x] T013 [US1] 在成员查询/写入接口执行租户边界约束：`backend/internal/transport/http/admin/iam/member_handler.go`
- [x] T014 [US1] 在权限检查接口统一 root/admin/member 语义：`backend/internal/transport/http/admin/iam/rbac_handler.go`
- [x] T015 [US1] 更新角色边界文档与验收矩阵：`specs/026-iam/spec.md`、`specs/026-iam/quickstart.md`

**Checkpoint**: User Story 1 可独立演示与验证

---

## Phase 4: User Story 2 - 用户管理交互语义一致 (Priority: P1)

**Goal**: 用户管理页动作语义拆分，消除隐式跳转和隐式切租户

**Independent Test**: 在 `/settings/users` 中“查看详情/切换租户/跳转页面”三种动作行为互不串扰

### Tests for User Story 2

- [x] T016 [P] [US2] 新增前端单测（动作分离）：`web-admin/tests/unit/settings-users/users-actions-semantics.spec.ts`
- [x] T017 [P] [US2] 新增前端 E2E（点击行为回归）：`web-admin/tests/e2e/iam/users-actions-semantics.spec.ts`

### Implementation for User Story 2

- [x] T018 [P] [US2] 重构页面入口与 tab 行为：`web-admin/app/pages/settings/users/index.vue`
- [x] T019 [P] [US2] 重构用户壳组件动作分发：`web-admin/app/components/settings/users/UsersShell.vue`
- [x] T020 [P] [US2] root 视图点击行为拆分：`web-admin/app/components/settings/users/UsersRoot.vue`
- [x] T021 [P] [US2] tenant admin 视图点击行为拆分：`web-admin/app/components/settings/users/UsersTenantAdmin.vue`
- [x] T022 [US2] 调整 API 调用适配（查看详情 vs 切租户）：`web-admin/app/composables/api/services/tenantService.ts`、`web-admin/app/composables/api/services/userService.ts`
- [x] T023 [US2] 更新交互语义说明文档：`specs/026-iam/quickstart.md`

**Checkpoint**: User Stories 1 + 2 独立通过且互不回退

---

## Phase 5: User Story 3 - 上下文状态强一致 (Priority: P2)

**Goal**: 页面状态与服务端 me/context 强一致，修复缓存/多标签页漂移

**Independent Test**: 刷新、跨标签切换、会话更新后页面分流仍正确

### Tests for User Story 3

- [x] T024 [P] [US3] 新增后端合同测试（me/context 字段完整性）：`backend/tests/contract/iam/me_context_contract_test.go`
- [x] T025 [P] [US3] 新增前端单测（store 缓存失效策略）：`web-admin/tests/unit/settings-users/user-store-context.spec.ts`
- [x] T026 [P] [US3] 新增 E2E（跨标签页一致性）：`web-admin/tests/e2e/iam/context-consistency.spec.ts`

### Implementation for User Story 3

- [x] T027 [P] [US3] 强化 me/context 返回与签名透传：`backend/internal/transport/http/admin/user/auth/me_handler.go`
- [x] T028 [P] [US3] 强化上下文查询服务容错：`backend/internal/service/auth/me_service.go`
- [x] T029 [US3] 调整租户切换接口语义：`backend/internal/transport/http/admin/user/auth/me_extra_handler.go`
- [x] T030 [US3] 强化前端上下文 store 刷新逻辑：`web-admin/app/stores/user.ts`
- [x] T031 [US3] 调整鉴权初始化链路读取顺序：`web-admin/app/plugins/auth-init.client.ts`、`web-admin/app/middleware/01-auth.global.ts`
- [x] T032 [US3] 在用户管理壳组件强制上下文刷新并收敛错误提示：`web-admin/app/components/settings/users/UsersShell.vue`

**Checkpoint**: User Stories 1-3 在上下文一致性场景下可稳定通过

---

## Phase 6: User Story 4 - 新租户管理员路径可验证 (Priority: P3)

**Goal**: 新租户注册用户具备本租户 admin 初始能力，且不扩散跨租户权限

**Independent Test**: 新租户注册后可管理本租户成员，跨租户访问被拒绝

### Tests for User Story 4

- [x] T033 [P] [US4] 新增后端集成测试（注册后 admin 初始化）：`backend/tests/integration/iam/tenant_admin_bootstrap_integration_test.go`
- [x] T034 [P] [US4] 新增后端集成测试（跨租户拒绝）：`backend/tests/integration/iam/tenant_admin_isolation_integration_test.go`

### Implementation for User Story 4

- [x] T035 [P] [US4] 梳理注册后成员角色初始化：`backend/internal/service/auth/auth_service.go`
- [x] T036 [P] [US4] 梳理租户与成员绑定初始化：`backend/internal/service/tenant/tenant_service.go`、`backend/internal/service/iam/member_service.go`
- [x] T037 [US4] 校验用户管理页对新租户 admin 的视图路由：`web-admin/app/components/settings/users/UsersTenantAdmin.vue`、`web-admin/app/pages/settings/users/index.vue`
- [x] T038 [US4] 更新“租户管理员 vs root”语义文档：`specs/026-iam/spec.md`、`specs/026-iam/quickstart.md`

**Checkpoint**: 全部用户故事可独立验证

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 跨故事一致性、文档、回归与发布准备

- [x] T039 [P] [Polish] 补齐部署排障文档中 IAM 日志查看章节：`docs/guides/deploy/025-powerx-docker-systemd/systemd/01-deploy-config-start.md`
- [x] T040 [P] [Polish] 对齐 Nginx API 前缀与 setup 访问说明：`docs/guides/deploy/025-powerx-docker-systemd/systemd/00-nginx-install-config.md`
- [x] T041 [Polish] 同步 feature 说明给开发文档索引（specs 目录导航）：`specs/README.md`（若不存在则新建）
- [x] T042 [Polish] 执行后端测试回归并记录结果：`go test ./backend/tests/contract/iam ./backend/tests/integration/iam`
- [x] T043 [Polish] 执行前端单测与 E2E 回归并记录结果：`cd web-admin && npm run test -- tests/unit/settings-users && npm run test:e2e -- tests/e2e/iam`
- [x] T044 [Polish] 最终更新 quickstart 验收记录：`specs/026-iam/quickstart.md`

---

## Phase 8: User Story 5 - SaaS 自助开通租户 (Priority: P1)

**Goal**: 提供公开 SaaS signup，事务化创建 tenant、owner user/member、默认角色和基础租户配置。

**Independent Test**: 新邮箱创建新租户成功；已有邮箱正确密码创建第二租户成功；错误密码/重复 key/初始化失败不残留半成品数据。

### Tests for User Story 5

- [ ] T045 [P] [US5] 新增 SaaS signup HTTP 合同测试：`backend/tests/contract/iam/saas_signup_contract_test.go`
- [ ] T046 [P] [US5] 新增 SaaS signup 集成测试：`backend/tests/integration/iam/saas_signup_bootstrap_integration_test.go`
- [ ] T047 [P] [US5] 新增事务回滚集成测试：`backend/tests/integration/iam/saas_signup_rollback_integration_test.go`

### Implementation for User Story 5

- [ ] T048 [US5] 新增公开 SaaS signup handler：`backend/internal/transport/http/public/saas/signup_handler.go`
- [ ] T049 [US5] 新增 SaaS signup service，封装 tenant/user/member/role/default settings 事务：`backend/internal/service/auth/saas_signup_service.go`
- [ ] T050 [US5] 扩展租户 bootstrap 初始化 owner/admin/user 角色绑定：`backend/internal/service/tenant/tenant_service.go`
- [ ] T051 [US5] 注册公开路由并保持 `/api/v1/public/saas/signup` 不依赖已有租户上下文：`backend/internal/http/router.go`
- [ ] T052 [US5] 前端新增或改造 SaaS 注册页：`web-admin/app/pages/auth/register.vue`
- [ ] T053 [US5] 更新 quickstart 的 SaaS signup 验收步骤：`specs/026-iam/quickstart.md`

---

## Phase 9: User Story 6 - Root 平台身份与租户身份隔离 (Priority: P1)

**Goal**: root 默认进入 Platform Console，不自动成为业务租户 admin；进入业务租户必须通过 Support Session。

**Independent Test**: root 登录后看不到租户 AI Settings 和租户插件业务入口；Support Session 创建后可按只读/写入模式审计访问。

### Tests for User Story 6

- [ ] T054 [P] [US6] 新增 root 默认菜单合同测试：`backend/tests/contract/iam/root_platform_boundary_contract_test.go`
- [ ] T055 [P] [US6] 新增 root support session 集成测试：`backend/tests/integration/iam/root_support_session_integration_test.go`
- [ ] T056 [P] [US6] 新增前端 root 菜单分流单测：`web-admin/tests/unit/iam/root-platform-boundary.spec.ts`

### Implementation for User Story 6

- [ ] T057 [US6] 收紧 `isCurrentTenantAdmin` 语义，禁止由 root 推导租户 admin：`web-admin/app/stores/user.ts`
- [ ] T058 [US6] 调整 root 默认菜单和 Platform Console 入口：`web-admin/app/middleware/01-auth.global.ts`、`web-admin/app/layouts/default.vue`
- [ ] T059 [US6] 新增 root support session 模型与迁移：`backend/pkg/corex/db/persistence/model/iam/root_support_session_gorm.go`、`backend/pkg/corex/db/database/migration.go`
- [ ] T060 [US6] 新增 root support session service/handler：`backend/internal/service/iam/root_support_session_service.go`、`backend/internal/transport/http/admin/root/support_session_handler.go`
- [ ] T061 [US6] AI Settings 与租户业务菜单按 root/support/tenant admin 语义分流：`web-admin/app/pages/settings/ai/index.vue`、`web-admin/app/composables/menu/*`
- [ ] T062 [US6] 写操作审计记录 support session id：`backend/internal/service/audit/*`

---

## Phase 10: User Story 7 - 租户插件实例隔离 (Priority: P2)

**Goal**: 插件物理包全局安装，租户实例独立启用/停用，菜单和代理入口强制校验当前租户实例。

**Independent Test**: 租户 A 启用插件、租户 B 未启用；B 看不到菜单，直接访问插件 admin/api 被拒绝。

### Tests for User Story 7

- [ ] T063 [P] [US7] 新增租户插件菜单过滤合同测试：`backend/tests/contract/plugin/tenant_plugin_menu_contract_test.go`
- [ ] T064 [P] [US7] 新增插件代理租户 enabled guard 集成测试：`backend/tests/integration/plugin/tenant_plugin_proxy_guard_integration_test.go`
- [ ] T065 [P] [US7] 新增前端插件市场租户实例单测：`web-admin/tests/unit/plugins/tenant-plugin-instance.spec.ts`

### Implementation for User Story 7

- [ ] T066 [US7] 明确 TenantPluginInstance service 读写接口：`backend/internal/service/plugin/tenant_instance_service.go`
- [ ] T067 [US7] 调整租户插件启用/停用 handler，禁止删除全局物理包：`backend/internal/transport/http/admin/plugin/tenant_handler.go`
- [ ] T068 [US7] 菜单聚合按当前租户 enabled 实例过滤：`backend/internal/transport/http/admin/plugin/menus_agg.go`
- [ ] T069 [US7] 插件 admin/api 代理入口增加租户 enabled guard：`backend/internal/infra/plugin/manager/router/router.go`
- [ ] T070 [US7] 前端插件市场区分“全局插件包”和“本租户启用状态”：`web-admin/app/pages/plugins/index.vue`
- [ ] T071 [US7] 更新插件隔离验收说明：`specs/026-iam/quickstart.md`
- [ ] T085 [US7] 收敛插件全局运行时启动 env，移除对单一租户 STS/Gateway 凭证的进程级绑定：`backend/internal/infra/plugin/manager/lifecycle.go`
- [ ] T086 [US7] 明确插件运行时状态接口展示全局进程与租户实例的关系：`backend/internal/transport/http/admin/plugin/status_handler.go`
- [ ] T087 [P] [US7] 新增多租户共享插件进程回归测试：`backend/tests/integration/plugin/tenant_plugin_shared_runtime_integration_test.go`
- [ ] T088 [US7] 实现租户暂停/归档时的插件实例业务入口和后台任务暂停策略：`backend/internal/service/tenant/tenant_service.go`、`backend/internal/service/plugin/tenant_instance_service.go`
- [ ] T089 [US7] 实现插件包全局停用/卸载前的租户实例影响检查：`backend/internal/infra/plugin/manager/uninstall.go`、`backend/internal/transport/http/admin/plugin/uninstall_handler.go`
- [ ] T090 [P] [US7] 新增插件生命周期权限矩阵测试：`backend/tests/contract/plugin/plugin_lifecycle_boundary_contract_test.go`

---

## Phase 11: User Story 8 - 历史数据语义迁移可控 (Priority: P2)

**Goal**: 保留现有 root/setup/组织架构数据，通过只读巡检和可审计补齐迁移完成 SaaS IAM 语义切换。

**Independent Test**: 巡检能发现 root/system tenant/owner/admin 缺失；自动补齐只处理缺 owner 且有 admin 的租户；缺 admin 只报告。

### Tests for User Story 8

- [ ] T072 [P] [US8] 新增 IAM migration report service 单测：`backend/tests/service/iam/iam_migration_report_service_test.go`
- [ ] T073 [P] [US8] 新增 IAM migration report HTTP 合同测试：`backend/tests/contract/iam/iam_migration_report_contract_test.go`
- [ ] T074 [P] [US8] 新增 owner 自动补齐集成测试：`backend/tests/integration/iam/iam_owner_autofix_integration_test.go`

### Implementation for User Story 8

- [ ] T075 [US8] 新增 IAM migration report service：`backend/internal/service/iam/iam_migration_report_service.go`
- [ ] T076 [US8] 新增 IAM migration report/fix-owner handler：`backend/internal/transport/http/admin/iam/migration_handler.go`
- [ ] T077 [US8] 巡检保留 root user、system tenant member、setup 完成记录：`backend/cmd/database/seed/seed_admin.go`、`backend/internal/transport/http/admin/system/setup_handler.go`
- [ ] T078 [US8] owner 自动补齐写审计，缺 admin 只报告：`backend/internal/service/iam/iam_migration_report_service.go`
- [ ] T079 [US8] 补充发布前巡检命令或 Make 任务：`Makefile`、`backend/cmd/*`
- [ ] T080 [US8] 更新生产数据迁移说明：`docs/plan/iam/saas-account-and-plugin-isolation.md`、`specs/026-iam/quickstart.md`

---

## Phase 12: Final SaaS IAM Regression

**Purpose**: 对 SaaS IAM、root、插件隔离、历史迁移执行全链路回归。

- [ ] T081 [P] [Polish] 更新 OpenAPI 与 gRPC 合同索引：`specs/026-iam/contracts/http-openapi.yaml`、`specs/026-iam/contracts/iam-rbac-admin.proto`
- [ ] T082 [P] [Polish] 补齐前端截图/操作验收说明：`specs/026-iam/quickstart.md`
- [ ] T083 [Polish] 执行后端 IAM/Plugin 回归并记录结果：`go test ./backend/tests/contract/iam ./backend/tests/integration/iam ./backend/tests/contract/plugin ./backend/tests/integration/plugin`
- [ ] T084 [Polish] 执行前端 IAM/Plugin 回归并记录结果：`cd web-admin && npm run test -- tests/unit/iam tests/unit/plugins`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3+)**: All depend on Foundational phase completion
- **Polish (Phase 7)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational - 无需依赖其他故事
- **User Story 2 (P1)**: Can start after Foundational - 与 US1 并行，但需复用 US1 边界语义
- **User Story 3 (P2)**: Depends on US1 + US2 的上下文与页面语义收敛结果
- **User Story 4 (P3)**: Depends on US1 角色边界稳定后再扩展注册路径

### Within Each User Story

- 测试任务先于实现任务
- 后端判定逻辑先于前端消费
- Store/上下文链路先于页面最终分流
- 每个故事完成后都要执行独立回归

### Parallel Opportunities

- Setup 与 Foundational 中标记 [P] 的任务可并行
- US1 的 service 与 transport 测试/实现可并行（不同文件）
- US2 的 root/admin 视图组件可并行
- US3 的后端 me/context 与前端 store 优化可并行
- US4 的注册链路与前端视图校验可并行

---

## Parallel Example: User Story 2

```bash
# 并行推进用户管理页动作语义拆分
Task: "重构页面入口与 tab 行为 in web-admin/app/pages/settings/users/index.vue"
Task: "重构用户壳组件动作分发 in web-admin/app/components/settings/users/UsersShell.vue"
Task: "root 视图点击行为拆分 in web-admin/app/components/settings/users/UsersRoot.vue"
Task: "tenant admin 视图点击行为拆分 in web-admin/app/components/settings/users/UsersTenantAdmin.vue"
```

---

## Implementation Strategy

### MVP First (US1 + US2)

1. 完成 Phase 1-2
2. 优先完成 US1（角色边界）
3. 完成 US2（交互语义）
4. **STOP and VALIDATE**：执行角色矩阵 + 页面行为回归

### Incremental Delivery

1. Setup + Foundational
2. 交付 US1（权限边界可验收）
3. 交付 US2（用户管理交互稳定）
4. 交付 US3（上下文强一致）
5. 交付 US4（新租户管理员路径）

### Parallel Team Strategy

1. 开发 A：US1 后端角色边界
2. 开发 B：US2 前端交互语义
3. 开发 C：US3 上下文一致性与测试
4. 收敛后由同一人完成 US4 与最终文档

---

## Notes

- [P] tasks = different files, no dependencies
- 每个故事都绑定可独立验证的回归标准
- 本特性为权限与上下文敏感域，禁止跳过测试直接上线
