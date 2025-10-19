# Tasks: Tool Grants & Security Policy for Integration

**Input**: Design documents from `/specs/005-title-tool-grants/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: 本任务列表按需安排测试（未要求 TDD，全量测试为可选项）。

**Organization**: 按用户故事拆分，确保每个故事可独立开发与验证。

## Format: `[ID] [P?] [Story] Description`
- **[P]**: 可并行执行（不同文件、无直接依赖）
- **[Story]**: 所属用户故事或阶段（如 `Setup`, `Found`, `US1` 等）
- 每个任务需引用具体文件路径

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 为授权域准备配置与工具链基础

- [X] T001 [Setup] 更新 `config/config.go` 与 `config/defaults.go`，新增事件骨干授权配置（缓存 TTL、Challenge SLA、Kafka 主题、审计留存等字段）。
- [X] T002 [Setup] 扩展 `internal/app/shared/options.go` 与相关 Option 绑定，暴露新的授权配置到 `EventFabricOptions`。
- [X] T003 [Setup] 调整 `pkg/make_files/proto.mk` 与根 `Makefile` 的 `proto-gen/proto-lint/proto-clean` 目标，纳入授权 proto 路径。

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 建立所有用户故事共同依赖的核心能力  
**⚠️ CRITICAL**: 完成前禁止进入各用户故事开发

- [ ] T004 [Found] 新增授权相关 GORM 模型于 `pkg/corex/db/persistence/model/event_fabric/authorization_models.go`，并创建迁移文件及更新 `pkg/corex/db/database/migration.go`。
- [ ] T005 [Found] 实现仓储层 `pkg/corex/db/persistence/repository/event_fabric/authorization_repository.go`，提供 Capability/Grant/Condition/ApprovalTicket 查询与写入。
- [ ] T006 [Found] 创建领域服务骨架 `internal/service/event_fabric/authorization/service.go`，声明依赖、接口方法与错误定义。
- [ ] T007 [Found] 实现缓存组件 `internal/service/event_fabric/authorization/cache.go`，封装 Redis + 本地 LRU 访问与失效逻辑。
- [ ] T008 [Found] 搭建 Challenge 派发与 SLA 定时器骨架 `internal/service/event_fabric/authorization/challenge_dispatcher.go` 及 `internal/app/shared/workers/event_fabric_authorization_timeout.go`。
- [ ] T009 [Found] 在 `internal/app/shared/deps.go`、`internal/app/shared/options.go` 等处装配授权服务到 `shared.Deps.EventFabric.Authorization`，并更新 `cmd/app/main.go` 依赖注入。
- [ ] T035 [Found] 新增密钥管理封装 `internal/service/event_fabric/authorization/secrets.go`，对接 KMS 客户端并提供配置加解密/轮换辅助。
- [ ] T036 [Found] 更新 `internal/app/shared/deps.go` 与 `internal/service/event_fabric/authorization/service.go` 构造函数，注入 KMS 提供的密钥材料并实现轮换策略。
- [ ] T037 [Found] 扩展 `internal/service/event_fabric/security/middleware.go` 与相关沙箱配置，限制 Agent/插件出网与数据访问，记录违规告警事件。

**Checkpoint**: 基础设施就绪，可启动用户故事开发

---

## Phase 3: User Story 1 - 安全架构师配置授权模型 (Priority: P1) 🎯 MVP

**Goal**: 使管理员可管理能力、Grant、Challenge 审批，并生成审计记录  
**Independent Test**: 通过 Admin API 创建/更新/撤销 Grant 并检查审计事件与 Challenge 处理

### Tests for User Story 1 (Optional)

- [ ] T017 [US1] 编写授权生命周期单元测试 `internal/service/event_fabric/authorization/service_test.go`，覆盖创建/更新/撤销及审计事件生成。

### Implementation for User Story 1

- [ ] T010 [US1] 在 `internal/service/event_fabric/authorization/service.go` 实现 Capability CRUD 逻辑与冲突校验。
- [ ] T011 [US1] 在同文件实现 Grant 创建/更新/撤销流程，校验 Scope/Condition、维护版本号并刷新缓存。
- [ ] T012 [US1] 实现 Challenge 手工审批流程与票据状态机 `internal/service/event_fabric/authorization/challenge_service.go`，触发审计与缓存失效。
- [ ] T013 [P] [US1] 定义 Admin API DTO/验证器 `internal/transport/http/admin/event_fabric/authorization_dto.go`。
- [ ] T014 [US1] 实现 Admin HTTP Handler `internal/transport/http/admin/event_fabric/authorization_handler.go`，调用服务层并返回统一响应。
- [ ] T015 [US1] 更新 `internal/transport/http/admin/event_fabric/routes.go` 注册 `/capabilities`、`/grants`、`/challenges` 等路由。
- [ ] T016 [US1] 集成审计：在服务层调用 `deps.EventFabric.Audit` 记录 Grant 生命周期事件与审批决策。
- [ ] T040 [US1] 实现授权模板服务 `internal/service/event_fabric/authorization/template_service.go` 与仓储方法，支持模板创建、查询、继承参数。
- [ ] T041 [P] [US1] 扩展 Admin API 路由（如 `/grant-templates`）与 Handler，实现模板 CRUD 与应用接口。

**Checkpoint**: Admin API 可独立运作，完成 Grant 管理闭环

---

## Phase 4: User Story 2 - 编排网关评估请求 (Priority: P2)

**Goal**: 网关可通过 gRPC/HTTP 接口执行授权评估、缓存控制与 Challenge 流程  
**Independent Test**: 模拟 Allow/Block/Challenge 请求，验证决策、缓存命中与 SLA 超时拒绝

### Tests for User Story 2 (Optional)

- [ ] T026 [US2] 编写评估与缓存集成测试 `internal/service/event_fabric/authorization/evaluator_test.go`，覆盖 Allow/Block/Challenge 场景及失效流程。

### Implementation for User Story 2

- [ ] T018 [US2] 新增 proto `api/grpc/contracts/powerx/event_fabric/v1/authorization.proto` 并更新 `buf.yaml`、`buf.gen.yaml`、`buf.work.yaml`（如有）。
- [ ] T019 [US2] 运行 `make proto-gen`，提交 `api/grpc/gen/go/powerx/event_fabric/v1/authorization.pb.go` 等生成文件并整理 `go.mod/go.sum`。
- [ ] T020 [US2] 实现评估核心逻辑 `internal/service/event_fabric/authorization/evaluator.go`，包含缓存命中、条件检查、速率限制与审计写入。
- [ ] T021 [US2] 完成 Challenge 派发与 SLA 自动拒绝逻辑，填充 `challenge_dispatcher.go` 与 `event_fabric_authorization_timeout.go`。
- [ ] T022 [P] [US2] 编写 gRPC Handler `internal/transport/grpc/event_fabric/authorization_service.go`，映射 Evaluate/Invalidate/GetSnapshot。
- [ ] T023 [US2] 扩展 HTTP 与 gRPC 缓存失效接口，实现 `POST /grants/cache:invalidate` 及对应服务方法。
- [ ] T024 [US2] 更新 `internal/server/grpc/server.go` 和 `internal/http/router.go` 注册授权服务，确保中间件链完整。
- [ ] T025 [US2] 在 `internal/service/event_fabric/metrics/metrics.go` 增加授权评估指标（延迟、缓存命中率、Challenge 计数），并于服务调用处打点。
- [ ] T038 [US2] 集成安全告警：在 `internal/service/event_fabric/authorization/evaluator.go` 与相关模块触发策略缺失、越权与评估失败告警，并对接告警平台。
- [ ] T039 [P] [US2] 编写安全告警测试 `internal/service/event_fabric/authorization/evaluator_alerts_test.go`，覆盖告警触发与抑制逻辑。

**Checkpoint**: gRPC/HTTP 评估链路上线，可供网关独立集成

---

## Phase 5: User Story 3 - 审计与合规团队追踪访问 (Priority: P3)

**Goal**: 提供审计查询接口，支持多租户过滤与导出  
**Independent Test**: 通过 Admin API 检索指定租户在时间范围内的授权/评估事件，验证过滤与导出

### Tests for User Story 3 (Optional)

- [ ] T030 [US3] 为审计查询编写单元/集成测试 `internal/service/event_fabric/authorization/reporting_test.go`，覆盖租户/主体/时间过滤。

### Implementation for User Story 3

- [ ] T027 [US3] 实现审计查询逻辑 `internal/service/event_fabric/authorization/reporting.go`，聚合 ClickHouse/对象存储数据并支持分页。
- [ ] T028 [P] [US3] 在 `internal/transport/http/admin/event_fabric/authorization_handler.go` 或独立文件中添加审计查询端点（如 `GET /audit/authorization`）。
- [ ] T029 [US3] 增加导出与权限控制，支持 CSV/JSON 响应并校验租户隔离。

**Checkpoint**: 合规团队可独立查询与导出授权审计数据

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 全局优化、文档与运行手册

- [ ] T031 [Polish] 更新 `scripts/demo/event_fabric_quickstart.sh` 与 `specs/005-title-tool-grants/quickstart.md`，验证全链路体验步骤。
- [ ] T032 [P] [Polish] 补充文档（如 `docs/runbooks/event_fabric_authorization.md`），描述告警、指标与应急流程。
- [ ] T033 [Polish] 更新 `docs/api/` 目录或 Swagger 生成流程，发布新的 OpenAPI 与 gRPC 合同引用。
- [ ] T034 [P] [Polish] 执行 `make unit-test`、`make proto-lint`、`make test-all` 并记录结果，确保发布前质量门槛。
- [ ] T042 [Polish] 验证审计事件 3 年留存与冷存储归档流程，编写核对脚本并记录验证报告。

---

## Dependencies & Execution Order

- **Phase 1 → Phase 2** → 所有用户故事 → Phase 6。  
- 用户故事间按优先级：US1 完成后可独立交付 MVP；US2、US3 可在基础完工后并行推进，但各自不依赖彼此。  
- 每个故事内部遵循：模型/仓储 → 服务 → 传输层 → 审计/指标 → 测试。

### Story Completion Graph

`Setup → Foundational → US1 → (US2 ∥ US3) → Polish`

---

## Parallel Execution Examples

- **US1**：T013 DTO 与 T014 Handler 可与 T012 按顺序/并行协同，但需在 T010/T011 后绑定。  
- **US2**：在完成 T018-T021 后，可并行推进 T022（gRPC Handler）与 T023（缓存失效接口）。  
- **US3**：T028 审计端点实现可与 T029 导出逻辑并行（确保共享服务接口已在 T027 完成）。  
- **Polish**：T032 文档与 T034 验证脚本互不阻塞，可并行。

---

## Implementation Strategy

1. **MVP**：完成 US1，交付可配置的能力/Grant 与 Challenge 审批流。  
2. **扩展**：在稳定管理面后上线 US2 gRPC 评估能力，满足网关集成与性能目标。  
3. **合规**：最终交付 US3 审计查询与导出，闭环审计与合规需求。  
4. **收尾**：执行 Polish 任务，更新文档、脚本并验证发布质量。
