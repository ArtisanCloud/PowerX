# Tasks: Integration Gateway & MCP Server

**Input**: Design documents from `/specs/007-integration-gateway-and-mcp/`

## Phase 1: Setup（共享基础）

- [x] T001 在 `internal/service/integration_gateway/`、`internal/transport/http/{admin,openapi}/integration_gateway/`、`internal/transport/grpc/integration_gateway/`、`internal/server/mcp/tools/integration_gateway/` 建立包结构与占位 README，确保遵循 CoreX 模块约定（无空包）。
- [x] T002 更新 `go.mod` / `go.sum` 依赖，确认 `github.com/mark3labs/mcp-go`、buf 工具链、Gin 等版本满足新特性需求，并在 `Makefile` 中添加 `proto-gen` 目标的目录覆盖范围。
- [x] T003 [P] 在 `api/grpc/contracts/powerx/integration_gateway/v1/` 初始化 buf 配置与占位 proto（与 plan.md 一致），并在 `api/grpc/contract/buf.yaml`、`api/grpc/contract/buf.gen.yaml` 注册新包路径。

---

## Phase 2: Foundational（阻断性前置）

- [x] T004 [P][Foundation] 为 `IntegrationRoute` 实体创建 GORM 模型 `pkg/corex/db/persistence/model/integration_gateway/route.go`，含租户内唯一别名、速率策略 JSON 和审计字段。
- [x] T005 [P][Foundation] 为 `IntegrationRouteVersion` 创建模型 `pkg/corex/db/persistence/model/integration_gateway/route_version.go`，保存快照、版本、trace_id。
- [x] T006 [P][Foundation] 为 `IntegrationInvocationLog` 创建模型 `pkg/corex/db/persistence/model/integration_gateway/invocation_log.go`，记录路由、追踪、状态与响应摘要。
- [x] T007 [P][Foundation] 为 `EventPublication` 创建模型 `pkg/corex/db/persistence/model/integration_gateway/event_publication.go`，包含主题、状态、补偿信息。
- [x] T008 在 `pkg/corex/db/persistence/repository/integration_gateway/` 实现仓储（路由、版本、调用日志、事件发布）并嵌入 `BaseRepository`，提供查询与乐观锁写入。
- [x] T009 扩展 `cmd/database/migrate.go` 与 `pkg/corex/db/database/migration.go`，注册新的 AutoMigrate 钩子，确保迁移顺序与现有模块一致。
- [x] T010 更新 `config/defaults.go` 和 `etc/config.yaml`，新增 `integration_gateway` 节点（限流前缀、事件主题默认值），并在 `config/config.go` 加载校验。
- [x] T011 在 `internal/app/shared/deps.go` 中注入 `IntegrationGateway` Service 所需依赖（RouterSvc、CapabilityRegistrySvc、EventBus、RateLimiter），并注册 Redis 限流前缀 `integration_gateway:rl`。
- [x] T012 搭建 `internal/service/integration_gateway/instrumentation` 目录，封装指标注册、追踪 ID 透传及审计钩子，支持 HTTP/gRPC/MCP 统一使用。
- [x] T013 在 `internal/server/grpc/server.go`、`internal/http/router.go`、`internal/server/mcp/register/factory` 等处预留路由/服务注册入口，确保之后实现可被装配。

---

## Phase 3: 用户故事 1 - 管理端创建与治理统一入口 (Priority: P1)

**Goal**: 管理员可通过 Admin API/gRPC 创建、更新、暂停、退役集成入口，并发布事件。

**Independent Test**: 仅部署管理面 + EventBus，通过 API/GRPC 完成 CRUD、查看版本与事件，验证限流默认值写入。

### Tests（先于实现）

- [x] T014 [P][US1] 在 `tests/contract/integration_gateway/admin_routes_http_test.go` 编写 Admin HTTP 合同测试，覆盖创建、查询、更新、生命周期动作、冲突与校验错误（依据 `contracts/http-openapi.yaml`）。
- [x] T015 [P][US1] 在 `tests/contract/integration_gateway/admin_grpc_test.go` 编写 gRPC Admin Service 合同测试，使用 buf 生成桩调用 CreateRoute/ListRoutes/ChangeLifecycle。
- [x] T016 [US1] 在 `tests/integration/integration_gateway/admin_management_flow_test.go` 编写集成测试：模拟管理员创建 -> 更新 -> 暂停 -> 恢复 -> 退役，并断言事件发布、版本记录与审计日志写入。

### Implementation

- [x] T017 [US1] 在 `internal/service/integration_gateway/manager/service.go` 实现路由管理 Service：创建/更新入口、维护版本快照、发布 `integration.gateway.route.*` 事件，并结合仓储层乐观锁写入，同时记录配置变更审计日志（成功与失败场景）。
- [x] T018 [US1] 在 `internal/service/integration_gateway/manager/validator.go` 实现参数校验（Tool Grant 校验、rate_limit 默认兜底、生命周期状态机约束）。
- [x] T019 [US1] 在 `internal/transport/http/admin/integration_gateway/handlers.go` 实现 Admin HTTP Handler（Gin DTO、鉴权、响应包装、ETag 处理、事件 trace）。
- [x] T020 [US1] 在 `internal/transport/grpc/integration_gateway/admin_server.go` 实现 gRPC Admin Service，映射 proto 请求到 manager service。
- [x] T021 [US1] 更新 `internal/transport/http/admin/routes.go` & `api/docs` 相关聚合，注册新的 `/admin/integration/routes` 路由与 Swagger 组件。
- [x] T022 [US1] 在 `internal/service/integration_gateway/manager/events.go` 编写事件发布与失败补偿逻辑，确保写入 `EventPublication` 并触发补偿队列。

**Checkpoint**: 管理员端 API/gRPC 可独立部署、通过测试，并记录事件。

---

## Phase 4: 用户故事 2 - 租户通过统一 API 触发能力 (Priority: P1)

**Goal**: 租户可调用统一 API 触发已授权能力，得到标准响应、追踪 ID 与限流治理。

**Independent Test**: 租户调用 API 时通过 Router 获取能力，验证限流、异常与事件告警；可独立运行。

### Tests

- [x] T023 [P][US2] 在 `tests/contract/integration_gateway/tenant_routes_http_test.go` 编写租户 HTTP 合同测试，覆盖列表、查询、invoke 与限流超限响应。
- [x] T024 [P][US2] 在 `tests/contract/integration_gateway/tenant_grpc_test.go` 编写 gRPC Tenant Service 合同测试，验证 ListRoutes/GetRoute/InvokeRoute。
- [x] T025 [US2] 在 `tests/integration/integration_gateway/tenant_invocation_flow_test.go` 编写集成测试：租户调用 -> Router 调度 -> 成功事件与失败事件发布 -> 限流路径，并验证成功/失败调用均生成对应审计记录。

### Implementation

- [x] T026 [US2] 在 `internal/service/integration_gateway/tenant/service.go` 实现租户调用 Service：校验 Tool Grant、拉取路由快照、调用 Router、记录 `IntegrationInvocationLog`、发布成功/失败事件，并在限流、权限或执行失败时写入审计日志。
- [x] T027 [US2] 在 `internal/service/integration_gateway/tenant/ratelimit.go` 集成 Redis 令牌桶，支持 per_route 与 per_route_per_tenant，返回剩余额度与 retry 提示。
- [x] T028 [US2] 在 `internal/transport/http/openapi/integration_gateway/handlers.go` 实现租户 HTTP Handler：身份解析、请求标准化、统一响应结构。
- [x] T029 [US2] 在 `internal/transport/grpc/integration_gateway/tenant_server.go` 实现 gRPC Tenant Service，对接 Service。
- [x] T030 [US2] 在 `internal/service/integration_gateway/tenant/telemetry.go` 记录指标（invocations_total、rate_limit_hits_total）、trace span，并针对成功与失败调用统一封装审计写入辅助。
- [x] T031 [US2] 更新 `internal/app/shared/deps.go`，注入租户 Service 所需依赖（RouterSvc、EventBus、RateLimiter、Instrumentation），并在 HTTP/Gin 中间件链路注入 `trace_id`。

**Checkpoint**: 租户接口可独立运行，具备限流、事件、日志、追踪能力。

---

## Phase 5: 用户故事 3 - MCP Server 暴露智能体能力 (Priority: P2)

**Goal**: MCP 客户端可列举已授权能力并以统一 schema 调用，沿用事件与追踪。

**Independent Test**: 仅启用 MCP Server，与 register 工厂集成，完成 handshake、list、invoke，并触发事件与限流。

### Tests

- [ ] T032 [P][US3] 在 `tests/contract/integration_gateway/mcp_tools_test.go` 编写 MCP 工具测试，使用示例客户端调用 `integration.route.list` 与 `integration.route.invoke`，验证 schema、授权过滤、错误返回。
- [ ] T033 [US3] 在 `tests/integration/integration_gateway/mcp_agent_flow_test.go` 编写端到端测试：模拟代理列举能力、执行调用、记录追踪与事件。

### Implementation

- [ ] T034 [US3] 在 `internal/server/mcp/tools/integration_gateway/list_tool.go` 注册 `integration.route.list` 工具：过滤租户权限、输出 schema、缓存策略。
- [ ] T035 [US3] 在 `internal/server/mcp/tools/integration_gateway/invoke_tool.go` 注册 `integration.route.invoke` 工具：调用租户 Service，映射错误码与 trace。
- [ ] T036 [US3] 更新 `internal/server/mcp/register/registry.go`，将新工具挂载到注册表并加入监控指标。
- [ ] T037 [US3] 在 `internal/service/integration_gateway/mcp/context_adapter.go` 封装 MCP 上下文与租户 Service 对接逻辑，确保 trace 统一。

**Checkpoint**: MCP 工具可与 HTTP/gRPC 共用逻辑，满足事件与追踪要求。

---

## Phase 6: Polish & Cross-Cutting

- [ ] T038 [P] 在 `docs/runbooks/` 补充运行手册：新增 `integration_gateway.md`，说明限流策略、事件主题、MCP 使用示例。
- [ ] T039 在 `deploy/observability/workflow_dashboard.json` 或新增仪表盘中加入 `integration_gateway_*` 指标、EventBus 告警。
- [ ] T040 [P] 根据 `quickstart.md` 编写脚本或文档验证流程（创建 -> 调用 -> MCP），确保 README/Quickstart 同步。
- [ ] T041 在 `tests/unit/` 补充关键单元测试（validator、rate limiter 适配、事件补偿），提升覆盖率。
- [ ] T042 运行 `make format vet unit-test proto-gen` 并整理提交说明，确保所有 gate 通过。

---

## Parallel Execution 示例

```bash
# 并行运行管理端合同测试
Task "T014 [P][US1] Admin HTTP 合同测试"
Task "T015 [P][US1] Admin gRPC 合同测试"

# 并行编写核心实体模型
Task "T004 [P] IntegrationRoute 模型"
Task "T005 [P] IntegrationRouteVersion 模型"
Task "T006 [P] IntegrationInvocationLog 模型"
Task "T007 [P] EventPublication 模型"

# 并行实施 MCP 工具
Task "T034 [P][US3] 注册 list 工具"
Task "T035 [P][US3] 注册 invoke 工具"
```

---

## Phase 依赖关系

- **Phase 1 → Phase 2**：结构与依赖准备完成后才能添加模型与配置。
- **Phase 2 完成后** 才能启动各用户故事开发；此阶段为所有故事的阻断前置。
- **Phase 3/4/5** 可在 Phase 2 完成后按优先级或团队能力并行推进，但各故事内需遵循“测试先行 → 服务 → 接口”的顺序。
- **Phase 6** 在核心故事完成后执行，用于完善文档、指标与质量收尾。

---

## MVP 策略

1. 完成 Phase 1 与 Phase 2，确保数据库、配置、依赖齐备。
2. 实现 Phase 3（管理端故事），可作为最小可演示版本：管理员创建路由 + 事件发布。
3. 按优先级扩展至租户调用（Phase 4）与 MCP 能力（Phase 5），每个故事均可独立测试与交付。
