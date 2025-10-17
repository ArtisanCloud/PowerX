# Tasks: Capability Registry & Router

**Input**: Design documents from `/specs/003-capability-registry-router/`
**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 Initialize capability registry module scaffolding (`internal/service/capability_registry`, `internal/transport/http/admin/capability_registry`, `internal/transport/grpc/capability_registry`, `pkg/corex/db/persistence/repository/capability_registry`) per plan.md.
- [X] T002 Wire buf configs & make targets for new proto (`api/grpc/contracts/buf{.yaml,.gen.yaml}`) ensuring `capability_registry` package mappings.
- [X] T003 [P] Configure observability & tracing defaults for new services (reuse existing OpenTelemetry setup, ensure trace_id/tenant logging hooks available).

## Phase 2: Foundational (Blocking Prerequisites)

- [X] T004 Align IAM/Tool Grant scopes for capability registry APIs (ensure policy constants defined in `internal/service/capability_registry/registry` and registered with access control).
- [X] T005 Prepare database migration scaffolding for capability registry tables (create placeholder migration files under `pkg/corex/db/migration`, register in `cmd/database/migrate.go`).
- [X] T006 [P] Define shared domain errors/enums (status, strategy, fallback) in `internal/service/capability_registry/domain/constants.go` for reuse across stories.

---

## Phase 3: User Story 1 - 可观测且一致的能力注册源 (Priority: P1) 🎯

**Goal**: Registry 接收注册/更新请求、校验契约与 Tool Grant、生成版本化快照并发布事件。
**Independent Test**: 仅部署 Registry 服务，通过 API 注册能力、查询快照、校验事件推送。

### Tests (write first)

- [X] T007 [P][US1] REST 合同测试 `/admin/capabilities` & `/admin/capabilities/{capabilityId}/tenants/{tenantId}` (create failing tests in `specs/003-capability-registry-router/contracts/tests/registry_rest_test.md` + `internal/tests/http/admin/capability_registry/registry_contract_test.go`).
- [X] T008 [P][US1] gRPC 合同测试 `CapabilityRegistryService` RPCs in `internal/tests/grpc/capability_registry/registry_contract_test.go`.

### Implementation

- [X] T009 [US1] 数据模型实现：在 `pkg/corex/db/persistence/model/capability_registry/` 定义 `CapabilityRegistration`, `AdapterEndpoint`, `RoutingPolicy`, `FallbackPlan`, `HealthProbeResult`, `DiscoveryCacheEntry` 模型。
- [X] T010 [US1] ORM 仓库实现：在 `pkg/corex/db/persistence/repository/capability_registry/` 创建针对上述模型的 Repo + 接口，覆盖乐观锁、租户过滤、分页。
- [X] T011 [US1] Registry 服务层：在 `internal/service/capability_registry/registry/` 实现创建/更新/禁用逻辑，包含 Tool Grant & Contract 校验、审计、事件发布。
- [X] T012 [US1] 管理 REST Handler：在 `internal/transport/http/admin/capability_registry/registry_handler.go` 实现 POST/GET/PUT/DELETE，接入 Gin + 中间件。
- [X] T013 [US1] 管理 gRPC Handler：在 `internal/transport/grpc/capability_registry/registry_server.go` 实现与 REST 对应的 RPC。
- [X] T014 [US1] 事件推送集成：在 `internal/service/capability_registry/registry/events.go` 接入 EventBus，并更新 quickstart 中 `capability.registry.updated` 主题文档。

---

## Phase 4: User Story 2 - 基于健康与权重的实时路由 (Priority: P1)

**Goal**: Router 根据权重与健康状态选择适配器，提供降级 & 观测指标。
**Independent Test**: 启动 Router + 虚拟适配器，模拟健康波动验证权重分布与 fallback。

### Tests (write first)

- [X] T015 [P][US2] gRPC 合同测试 `CapabilityRouterService` (`Invoke`, `StreamInvoke`, `ReportHealth`) in `internal/tests/grpc/capability_registry/router_contract_test.go`.
- [X] T016 [P][US2] 路由策略集成测试：在 `internal/tests/service/capability_registry/router_strategy_test.go` 使用内存适配器模拟权重与健康变动，确保 500ms 内 fallback。
- [X] T017 [P][US2] Sandbox 接口合同测试：在 `internal/tests/grpc/capability_registry/router_sandbox_test.go` 和 `internal/tests/http/admin/capability_registry/router_sandbox_contract_test.go` 验证策略演练请求与响应。

### Implementation

- [X] T018 [US2] Router 领域实现：在 `internal/service/capability_registry/router/` 编写策略选择、限流、sticky、观测输出逻辑，集成 Redis 权重缓存。
- [X] T019 [US2] 健康探测 orchestrator：在 `internal/service/capability_registry/health/` 实现主动探测、被动熔断、冷却时间控制，更新 `HealthProbeResult`。
- [X] T020 [US2] Router gRPC Handler：在 `internal/transport/grpc/capability_registry/router_server.go` 实现 `Invoke`、`StreamInvoke`、`ReportHealth`，接入拦截器链。
- [X] T021 [US2] Router REST 入口：在 `internal/transport/http/admin/capability_registry/router_handler.go` 实现 `/router/invoke` 与健康上报路由，保证与 gRPC 行为一致。
- [X] T022 [US2] Fallback 状态同步：在 `internal/service/capability_registry/router/fallback_sync.go` 实现 Router fallback 结果回写 Registry/HealthProbeResult。
- [X] T023 [US2] Sandbox 服务逻辑：在 `internal/service/capability_registry/sandbox/` 复用 Router 策略生成模拟结果并隔离权限。
- [X] T024 [US2] Sandbox Handler：在 `internal/transport/grpc/capability_registry/sandbox_server.go` 与 `internal/transport/http/admin/capability_registry/sandbox_handler.go` 提供 sandbox API。
- [X] T025 [US2] 观测与指标：在 `internal/service/capability_registry/router/metrics.go` 记录选路决策、降级次数、错误码、sandbox 调用指标，并打通 OpenTelemetry。

---

## Phase 5: User Story 3 - 缓存与失败切换体验 (Priority: P2)

**Goal**: 客户端持久化快照、TTL 管理、registry 不可用时 fallback。
**Independent Test**: 构建客户端模拟，验证 TTL、强制刷新以及 fallback 能力响应。

### Tests (write first)

- [ ] T026 [P][US3] Discovery REST 合同测试 `/discovery/{tenantId}/{capabilityId}` 与 `/discovery/sync` in `internal/tests/http/admin/capability_registry/discovery_contract_test.go`.
- [ ] T027 [P][US3] 客户端缓存集成测试：在 `internal/tests/service/capability_registry/discovery_cache_test.go` 模拟缓存过期、Registry 离线、fallback 返回。
- [ ] T028 [P][US3] 跨区域同步测试：在 `internal/tests/service/capability_registry/cross_region_sync_test.go` 验证多集群快照一致性与故障切换。

### Implementation

- [ ] T029 [US3] Redis 缓存封装：在 `internal/infra/cache/discovery/` 实现快照读写、TTL、强制刷新逻辑，并输出命中率指标。
- [ ] T030 [US3] Discovery 服务层：在 `internal/service/capability_registry/discovery/` 实现缓存命中、回源、fallback 能力选择与 TTL 管控。
- [ ] T031 [US3] Discovery HTTP/gRPC Handler：在 `internal/transport/http/admin/capability_registry/discovery_handler.go` 与 `internal/transport/grpc/capability_registry/discovery_server.go` 提供查询接口。
- [ ] T032 [US3] FallbackPlan 支撑：完善 `FallbackPlan` 模型与服务逻辑，确保 fallback 能力调用或静态响应生效。
- [ ] T033 [US3] 跨区域复制组件：在 `internal/infra/replication/capability_registry/` 构建快照复制与冲突解决模块。
- [ ] T034 [US3] 跨区域同步编排：在 `internal/service/capability_registry/discovery/cross_region.go` 调度 replication、远端 Router 同步与降级策略。

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T035 [P] 写 Quickstart 验证脚本：根据 `quickstart.md` 编写 E2E 脚本（`scripts/demo/capability_registry_route.sh`）验证注册、选路、缓存流程。
- [ ] T036 安全加固与审计：在 `internal/service/capability_registry/registry/audit.go` 增强审计日志，确保工具权限检查覆盖所有操作。
- [ ] T037 [P] 性能与延迟验证：编写基准测试/压测脚本评估 Registry 查询与 Router 调用（99% ≤150ms、fallback ≤500ms），记录结果。
- [ ] T038 [P] 缓存命中率监控：在 Observability 中新增命中率指标、告警与测试脚本，验证命中率 ≥80%。
- [ ] T039 文档与部署说明：更新 `docs/` 或 `specs/003-capability-registry-router/quickstart.md` 的部署参数、事件主题、sandbox 使用方式与跨区域拓扑。

---

## Parallel Execution Guidance

- `[P]` 标记任务可并行执行（不同文件/模块）。例如：
  - T007 与 T008 均为合同测试，可同时进行。
  - T015 与 T016 互不冲突，可并行验证路由协议与策略。
  - T024 与 T025 涉及不同层级（infra vs service），可在完成模型依赖后并行。
- 任务依赖关系：Setup → Foundational → 各故事（按优先级 P1 → P2）→ Polish。确保前置测试任务在实现前完成并验证失败。

--- 
