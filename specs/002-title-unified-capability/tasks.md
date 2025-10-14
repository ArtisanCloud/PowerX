# Tasks: Unified Capability Contracts & Transport Adapters

**Input**: `specs/002-title-unified-capability/{spec.md,plan.md,research.md,data-model.md,contracts/}`  
**Prerequisites**: 已确认 `plan.md`、`spec.md`、`research.md`、`data-model.md`、`contracts/` 就绪，并具备 `buf`、`protoc`、Postgres 与基础 Go 构建环境。  
**Tests**: 按需为核心服务/Handler 编写单元测试或契约测试；若无额外要求，至少确保 `make unit-test`、`make proto-lint` 通过。  
**组织方式**: 任务按用户故事切分，便于并行实现与独立验收。

## Format: `[ID] [P?] [Story] 描述`
- **[P]**：可并行执行（不同文件且无依赖关系）。  
- **[Story]**：任务归属的用户故事（如 `US1`、`US2`、`US3`，或全局 `Core`）。  
- 描述中需写明受影响的具体文件/目录，利于分工协作。

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 落地协议契约骨架，确保生成工具链可用。

- [x] T001 [P] [Core] 在 `api/grpc/contracts/powerx/capability/v1/` 新建 `capability_contract.proto`、`capability_version_policy.proto`、`transport_adapter.proto`，从 `specs/002-title-unified-capability/contracts/capability-grpc.proto` 拆分字段并补充 `package`/`go_package` 选项。
- [x] T002 [Core] 更新 `api/grpc/contracts/buf.yaml` 与 `api/grpc/contracts/buf.gen.yaml`，注册 `powerx/capability/v1` 模块并将生成产物指向 `api/grpc/gen/go/powerx/capability/v1`。
- [x] T003 [Core] 执行 `make proto-lint proto-gen`，确认新生成的 `api/grpc/gen/go/powerx/capability/v1` 代码编译无误并纳入版本控制。

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 建立统一能力域的持久层与启动钩子，所有故事开始前必须完成。

- [ ] T010 [Core] 在 `pkg/corex/db/persistence/model/capability/capability_contract_gorm.go` 定义契约实体、IO Schema、错误分类结构及 GORM 关联，并更新 `pkg/corex/db/persistence/model/tables.go` 增加相关表常量。
- [ ] T011 [Core] 新建 `pkg/corex/db/persistence/model/capability/transport_profile_gorm.go`，建模传输配置（模式/超时/重试/QoS/健康状态）并建立外键关系。
- [ ] T012 [Core] 创建 `pkg/corex/db/persistence/repository/capability/` 仓储（如 `contract_repository.go`、`transport_repository.go`），封装多租户过滤、分页与事务写入基元。
- [ ] T013 [Core] 在 `pkg/corex/db/database/migration.go` 的 `MigrateCoreModels` 中注册 capability 相关 `AutoMigrate` 调用（必要时提取 `MigrateCapabilityModels` 并由入口调用），确保核心模型随平台启动迁移。

---

## Phase 3: User Story 1 - 一次声明全渠道复用的能力契约 (Priority: P1) 🎯

**Goal**: 提供统一契约草稿、发布与查询能力，让一次定义即可被 HTTP/gRPC/MCP 调用。  
**Independent Test**: 仅部署契约模型与校验服务，通过创建契约草稿、发布并在多协议通道读取验证。

- [ ] T101 [P] [US1] 在 `internal/contract/capability/validation.go` 实现契约校验（Schema 完整性、Scope/ToolGrant 存在性、传输偏好互斥、Error Taxonomy 映射），返回详尽的 `ValidationIssue`。
- [ ] T102 [US1] 编写 `internal/service/capability/contract_service.go`，整合仓储、校验、审计与 EventBus，支持草稿写入、发布、查询、列表等操作并保证多租户隔离。
- [ ] T103 [US1] 在 `internal/transport/http/admin/capability/{api.go,contract_handler.go,router.go}` 构建 REST Handler 与路由，挂载 `/admin/capabilities` 系列接口，并更新 `internal/transport/http/admin/routes.go` 注册模块。
- [ ] T104 [US1] 在 `internal/transport/grpc/capability/contract_handler.go` 实现 `CapabilityRegistryService` 契约 RPC，处理 pb↔domain 映射，并更新 `internal/server/grpc/server.go` 注册服务与健康检查。
- [ ] T105 [US1] 在契约发布/废弃流程中写入审计日志与事件（`integration.capability.*`），包含版本、替代关系、租户信息以及调用备注。
- [ ] T106 [P] [US1] 为校验器或契约服务编写单元测试（如 `internal/contract/capability/validation_test.go`），覆盖合法、缺失 Scope、传输偏好冲突等场景。
- [ ] T107 [US1] 复用并验证现有 PowerX gRPC SDK，可直接消费 `CapabilityRegistryService` 新增 RPC；如需补充生成或配置，更新相应文档而非创建新客户端目录。

---

## Phase 4: User Story 2 - 版本演进与兼容策略治理 (Priority: P1)

**Goal**: 管理版本策略、兼容矩阵与废弃提醒，确保升级不破坏现有调用。  
**Independent Test**: 独立部署版本管理模块，通过创建 v1/v2 契约、设置兼容标记并验证调用中的版本选择逻辑。

- [ ] T201 [P] [US2] 新建 `pkg/corex/db/persistence/model/capability/capability_version_policy_gorm.go`，描述默认策略、兼容矩阵、废弃配置，并纳入自动迁移。
- [ ] T202 [US2] 在 `pkg/corex/db/persistence/repository/capability/version_policy_repository.go` 实现策略的查询、乐观锁更新与兼容矩阵持久化。
- [ ] T203 [US2] 完成 `internal/service/capability/version_policy_service.go`，执行兼容性评估、默认策略决策与废弃通知逻辑，并与契约发布流程协同。
- [ ] T204 [P] [US2] 新建 `internal/transport/http/admin/capability/version_policy_handler.go`，提供获取/更新版本策略与废弃提醒的 REST 接口，并在路由中挂载。
- [ ] T205 [US2] 实现 gRPC 层的 `GetVersionPolicy`/`UpsertVersionPolicy`，必要时拆分至 `internal/transport/grpc/capability/version_policy_handler.go` 复用服务逻辑。
- [ ] T206 [US2] 版本策略变更时触发事件 `integration.capability.version_policy.updated` 与审计记录，覆盖兼容失败与替代建议场景。

---

## Phase 5: User Story 3 - 统一 Transport Adapter 接口 (Priority: P2)

**Goal**: 提供统一适配器接口与传输配置，保证多协议一致的上下文与错误模型。  
**Independent Test**: 构建 Adapter 规范，通过模拟 HTTP/gRPC/MCP 调用验证统一上下文、度量与错误映射。

- [ ] T301 [P] [US3] 在 `internal/service/capability/adapter_service.go` 定义 `TransportAdapter` 接口及默认实现，封装 Invoke/Stream/HealthCheck/Close 与超时重试策略。
- [ ] T302 [US3] 扩展 `pkg/corex/db/persistence/repository/capability/transport_repository.go`，提供协议偏好与健康状态的持久化操作。
- [ ] T303 [P] [US3] 新建 `internal/transport/http/admin/capability/adapter_handler.go`，实现传输配置管理与健康检查 REST 接口，输出统一错误模型。
- [ ] T304 [US3] 在 `internal/transport/grpc/capability/adapter_handler.go` 落地 `ListTransportProfiles` 等 Adapter 控制 RPC，处理结构体转换与错误码映射。
- [ ] T305 [US3] 在适配器服务中集成指标与 Tracing（如 Prometheus 指标、OpenTelemetry Span），并将超时/重试/下游故障映射到 `ErrorTaxonomy`。
- [ ] T306 [US3] 实现协议偏好降级逻辑（prefer→fallback），缺失传输通道时返回临时不可用错误并写入审计/事件。

---

## Phase N: Polish & Cross-Cutting Concerns

**Purpose**: 文档、测试与交付前验证。

- [ ] T901 [Core] 更新 `docs/integration`、`docs/knowledge_base` 中相关章节，描述契约治理、传输配置与观测指标接入流程。
- [ ] T902 [P] [Core] 为新增 HTTP/gRPC Handler 或服务编写集成/回归测试，验证主要 Happy Path 与关键错误分支。
- [ ] T903 [Core] 复核 `specs/002-title-unified-capability/quickstart.md` 示例命令与响应，确保与实现保持一致。
- [ ] T904 [Core] 运行 `make proto-lint unit-test` 并整理构建日志，确认所有新包通过编译与 Lint。
