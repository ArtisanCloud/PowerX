# Tasks: Integration Gateway & MCP Server（多插件能力对齐）

**Input**: Design documents from `/specs/007-integration-gateway-and-mcp/`
**Prerequisites**: spec.md, plan.md, research.md, data-model.md, contracts/, quickstart.md

## Phase 1: Setup（共享基础）

- [x] **T001** Configure capability registry defaults in `backend/config/config.yaml`, `.env.example`, and `backend/internal/config/app_config.go`（新增 `capability_registry` redis prefix、event topics、默认限流参数；同步 `.env` 可覆盖变量，补齐 `AppConfig` 结构体与加载逻辑）。
- [x] **T002 [P]** Extend build tooling：更新 `backend/Makefile` 与 `backend/make_files/proto.mk`，确保 `proto-gen`, `proto-lint`, `proto-clean` 处理 `api/grpc/contracts/powerx/integration_gateway/v1`，并在 `buf.yaml/buf.gen.yaml` 注册新包（含 go_package_prefix/managed 模块配置）。
- [x] **T003 [P]** 创建代码骨架目录：`backend/internal/service/capability_registry/`, `backend/internal/transport/http/{admin,openapi}/capability_registry/`, `backend/internal/transport/grpc/capability_registry/`, `backend/pkg/corex/db/persistence/{model,repository}/capability_registry/`, `backend/tests/{contract,integration}/capability_registry/`（每个目录提供最小 `doc.go`/README，避免空包）。

## Phase 2: Foundational（阻塞任务）

- [x] **T004 [P]** 数据模型：在 `backend/pkg/corex/db/persistence/model/capability_registry/capability_record.go` 定义 `CapabilityRecord` 与嵌入 `ProtocolBinding`，包含 JSONB 字段、状态、索引与 GORM tag。
- [x] **T005 [P]** 数据模型：在 `backend/pkg/corex/db/persistence/model/capability_registry/workflow_template_ref.go` 定义 `WorkflowTemplateRef`（含 `requires_manual_upgrade`、hash 快照、steps JSON）。
- [x] **T006 [P]** 数据模型：在 `backend/pkg/corex/db/persistence/model/capability_registry/capability_sync_job.go` 定义 `CapabilitySyncJob`（状态、hash_before/after、error_summary）。
- [x] **T007 [P]** 数据模型：实现 `SelectorPolicySnapshot` Redis DTO 与序列化助手，放在 `backend/internal/agent/toolstore/policy_snapshot.go`。
- [x] **T008 [P]** 数据模型：实现 `InvocationTrace`（及可复用的 `EventPublication`）于 `backend/pkg/corex/db/persistence/model/capability_registry/invocation_trace.go`，方便审计与追踪。
- [x] **T009** 迁移与仓储：在 `backend/pkg/corex/db/database/migration.go` 注册上述模型，并在 `pkg/corex/db/persistence/repository/capability_registry/` 创建对应仓储（含 Redis/DB 访问、BaseRepository 嵌入）。
- [x] **T010** 事件 & 观测：在 `backend/internal/eventbus/topics.go`、`backend/internal/observability/metrics/capability_registry.go` 定义 `integration.gateway.*` 事件与 `powerx_capability_invoke_*` 指标，确保 Trace tag 包含 `capability_id/plugin_id/protocol`。
- [x] **T011** CLI 与 cron：在 `backend/cmd/capability_sync/main.go`（或现有 cmd）新增 worker 入口与 `Makefile` target `capability-sync`，并接入日志/配置。

*Checkpoint：模型、仓储、事件、工具齐备，可进入用户故事。*

## Phase 3: User Story 1 – 3 分钟能力目录同步与治理 (Priority: P1)
**目标**：插件提交后 3 分钟内完成 Capability Sync，Admin/Tenant API、MCP `/tools/list` 均可读取统一 schema。
**独立测试**：运行 quickstart 步骤 1-2，验证 Admin/Tenant API 列表与 Redis 缓存一致。

### Tests
- [x] **T012 [P][US1]** HTTP 合同测试：使用 Dredd/Prism 在 `backend/tests/contract/integration_gateway/http_contract_test.go` 校验 `contracts/http-openapi.yaml` 中 `/admin/capabilities*`、`/tenant/capabilities`、`/tenant/invocations*`。
- [x] **T013 [P][US1]** gRPC 合同测试：使用 Buf breaking + gRPC 客户端在 `backend/tests/contract/integration_gateway/grpc_contract_test.go` 覆盖 `integration-gateway.proto` 所有 RPC。
- [x] **T014 [US1]** 集成测试：在 `backend/tests/integration/capability_registry/sync_flow_test.go` 模拟 `.pxp` 提交 → Worker → Admin/Tenant API 查询的完整链路。
- [x] **T043 [P][US1]** Worker 失败场景测试：在 `backend/tests/contract/integration_gateway/worker_asset_validation_test.go` 模拟缺失 `contracts/exposure/*` 或 schema 无法解析的 `.pxp`，断言触发 `capability.catalog.sync_failed`、通知记录与能力下架。
- [x] **T044 [P][US1]** 统一错误合同测试：在 `backend/tests/contract/integration_gateway/error_contract_test.go` 针对 Admin/Tenant HTTP 与 gRPC Invoke，验证错误响应与手动升级提示均遵循 `pkg/dto` 统一结构。

### Implementation
- [x] **T015 [US1]** Capability Sync Worker：在 `backend/internal/service/capability_registry/sync_worker.go` 实现 `.pxp` 解析、lint、hash 计算、DB+Redis 写入与 `capability.catalog.sync_*` 事件。
- [x] **T016 [US1]** Registry Service：在 `backend/internal/service/capability_registry/registry_service.go` 暴露 `ListCapabilities`, `GetCapability`, `ListJobs`，支持租户/协议过滤与缓存回源。
- [x] **T017 [US1]** Admin HTTP API：实现 `GET/POST/PATCH /admin/capabilities*` + `/admin/capability-sync/jobs` Handler、DTO、绑定逻辑，文件位于 `backend/internal/transport/http/admin/capability_registry/` 并在 `internal/http/router.go` 装配。
- [x] **T018 [US1]** Tenant HTTP API：实现 `GET /tenant/capabilities`、`GET/POST /tenant/invocations*` 读取授权、封装 `CapabilityInvokeRequest`，位于 `backend/internal/transport/http/openapi/capability_registry/`。
- [x] **T019 [US1]** gRPC 服务：在 `backend/internal/transport/grpc/capability_registry/server.go` 实现 `IntegrationGatewayService` RPC，并添加至 `internal/server/grpc/server.go`。
- [x] **T020 [US1]** MCP Tool Registry：在 `backend/internal/agent/toolstore/mcp_registry.go` 生成 `integration.route.list`/`integration.route.invoke` 工具，直接消费 Registry 缓存。
- [x] **T021 [US1]** 缓存刷新 & Redis Key 设计：实现 `capability_registry:cache:{capability_id}`、`toolstore:policy:{hash}` TTL 策略及广播通道，位置 `backend/internal/service/capability_registry/cache.go`。
- [x] **T022 [US1]** 事件/日志：在 `backend/internal/service/capability_registry/audit.go` 写入 `CapabilitySyncJob`、`EventPublication`、Trace 传播逻辑，确保 quickstart 指标可用。
- [x] **T045 [US1]** 缺失资产告警与阻断：扩展 `backend/internal/service/capability_registry/sync_worker.go` 与新增 `alerting.go`，当 `contracts/exposure` 缺失或 schema 解析失败时，向插件开发者/运营者发出通知、落库告警记录，并阻止能力写入 Registry。
- [x] **T046 [US1]** 统一错误 DTO：在 `backend/internal/dto/capability_registry/error.go` 定义统一错误响应，更新 Admin/Tenant HTTP 与 gRPC Handler 共用 `pkg/dto` 回包及手动升级提示。

*Checkpoint：Admin/Tenant API + gRPC + MCP 均可从 Registry 获取能力并可追踪。*

## Phase 4: User Story 2 – Agent Hub 多协议 Selector 自动路由 (Priority: P1)
**目标**：Agent Hub 根据 `policy.prefer` 自动选择 MCP/gRPC/Workflow，并在协议失败时 fallback 与打点。
**独立测试**：运行 quickstart 步骤 3，模拟 MCP 断链并观察 fallback。

### Tests
- [x] **T023 [P][US2]** Integration Test：`backend/tests/integration/capability_registry/selector_fallback_test.go` 模拟 MCP 失败 → gRPC fallback，验证事件 `integration.gateway.invocation.fallback` 与 `InvocationTrace` 记录。
- [x] **T024 [P][US2]** Load/RateLimit Test：在 `backend/tests/integration/capability_registry/rate_limit_test.go` 验证租户/入口限流策略（令牌桶默认 + 自定义）。
- [x] **T041 [P][US2]** Trace Completeness Test：在 `backend/tests/integration/capability_registry/trace_completeness_test.go` 构建批量调用，验证 95% 请求在 1 分钟内写入 Trace/Audit（SC-003）。
- [x] **T042 [P][US2]** Event Latency Probe：实现 `backend/tests/integration/capability_registry/event_latency_test.go` 或脚本，测量 `integration.gateway.invocation.*` 事件 95% 在 60s 内送达（SC-002/SC-003 支撑）。
- [x] **T047 [P][US2]** 版本锁定集成测试：`backend/tests/integration/capability_registry/version_lock_test.go` 模拟插件发布新 `capabilities_hash`，验证 Agent Hub/Selector 在管理员确认前继续使用旧版本并拒绝新 hash。

### Implementation
- [x] **T025 [US2]** SelectorPolicySnapshot 生成器：在 `backend/internal/service/capability_registry/policy_generator.go` 根据 Registry 数据写入 Redis 并附带 `capabilities_hash`。
- [x] **T026 [US2]** Agent ToolStore 刷新：扩展 `backend/internal/agent/toolstore/store.go` 监听 `capability.catalog.sync_*` 事件并刷新内存缓存。
- [x] **T027 [US2]** Selector Adapter：在 `backend/internal/service/capability_registry/selector.go` 实现 `CapabilityInvokeRequest` -> MCP/REST 并发 + gRPC fallback + Workflow 调度，注入幂等键与可观测性。
- [x] **T028 [US2]** Tenant Invocation Handler：在 `backend/internal/transport/http/openapi/capability_registry/invoke_handler.go` & gRPC Invoke RPC 调用 Selector，统一错误结构/Trace。
- [x] **T029 [US2]** EventBus/Metric Hook：在 Selector 中发出 `integration.gateway.invocation.*` 事件、更新 Prometheus 指标，文件 `backend/internal/service/capability_registry/metrics.go`。
- [x] **T030 [US2]** Safe-mode/Tool Grant Enforcement：在 Selector/Tenant Handler 中校验租户 Feature Flag、Tool Grant、Safe Mode，扩展 `backend/internal/service/capability_registry/authz.go`。
- [x] **T048 [US2]** Agent 版本锁执行器：在 `backend/internal/agent/toolstore/version_lock.go`（新增）及 `selector.go` 中持久化租户绑定的 `capabilities_hash`，收到新 hash 时返回“需升级”错误并记录 `capability.policy.degraded` 事件，直至管理员确认。

*Checkpoint：意图路由具备协议优先级、fallback、限流治理，可独立运行。*

## Phase 5: User Story 3 – Workflow Builder 引入插件模板并手动升级 (Priority: P2)
**目标**：Workflow Builder Catalog 导入插件模板，执行时遵循 Selector 策略，模板升级需管理员显式确认。
**独立测试**：运行 quickstart 步骤 4，验证模板导入、执行与手动升级提示。

### Tests
- [ ] **T031 [P][US3]** Workflow Catalog Integration Test：`backend/tests/integration/capability_registry/workflow_catalog_test.go` 验证模板导入、执行节点协议、Selector 调用一致。
- [ ] **T032 [P][US3]** Template Upgrade Test：`backend/tests/integration/capability_registry/template_upgrade_test.go` 模拟插件 hash 变更，确认未升级前拒绝调用，执行 `/admin/workflow-templates/{id}/upgrade` 后恢复。
- [ ] **T049 [P][US3]** Workflow 采纳度遥测测试：在 `backend/tests/integration/capability_registry/workflow_telemetry_test.go` 验证模板导入/执行会写入 adoption metrics、成功率与手动升级提示。

### Implementation
- [ ] **T033 [US3]** Workflow Catalog Sync：在 `backend/internal/service/capability_registry/workflow_catalog.go` 从 Registry 推送模板至 Workflow Builder Catalog（含 hash 校验、requires_manual_upgrade）。
- [ ] **T034 [US3]** Admin Upgrade Endpoint：实现 `POST /admin/workflow-templates/{templateId}/upgrade` Handler，调用新服务以更新绑定模板 hash/版本。
- [ ] **T035 [US3]** Workflow Engine Adapter：扩展 `backend/internal/workflow/engine/plugins.go`（或相关）以在执行节点时注入 `CapabilityInvokeRequest` 并复用 Selector。
- [ ] **T036 [US3]** Builder UI 数据管道：更新 `backend/internal/transport/http/admin/capability_registry/workflow_handler.go`（新增）提供模板列表、升级状态给 Web Admin。
- [ ] **T050 [US3]** Workflow 遥测管线：在 `backend/internal/workflow/engine/telemetry.go`（新增）与 `workflow_catalog.go` 中注入 adoption metrics/Trace 记录，并将数据暴露给 Prometheus + quickstart。

*Checkpoint：Workflow Builder 能导入/运行插件模板并受控升级。*

## Phase 6: Polish & Cross-Cutting

- [ ] **T037 [P]** 文档：更新 `docs/plan/AI_engineering/multi_plugin_capability_guide.md` 与 `specs/007.../quickstart.md` 的 CLI 例子，新增“缓存刷新”“模板升级”章节。
- [ ] **T038 [P]** CI & Scripts：实现 `scripts/capability_registry/verify.sh`，串联 quickstart 步骤 1-4 并在 CI 执行。
- [ ] **T039** 性能与容错：编写负载测试脚本（`tests/integration/capability_registry/load/`) 验证 5k+ 调用、Redis 缓存击穿保护，并对 Selector fallback 做 chaos 测试。
- [ ] **T040** 最终 QA：运行 prometheus/otel 验证、检查事件补偿逻辑、审阅日志格式，更新 `AGENTS.md` 及 README 片段。

## Dependencies & Parallel Execution

1. **Phase 1 → Phase 2**：完成配置与目录后方可定义模型与仓储。
2. **Phase 2 → User Stories**：模型、仓储、事件、CLI 均为三大用户故事共享依赖，必须先完成。
3. **User Stories**：US1、US2（同为 P1）可在 Phase 2 完成后并行，US3（P2）建议等待 US1 稳定的 Registry 接口。
4. **Tests vs Implementation**：每个用户故事的测试任务（T012/T013/T014、T023/T024、T031/T032）需先于同故事实现启动，以保持 TDD。
5. **Polish**：所有核心功能完成后再执行。

### 平行执行示例
```bash
# 并行启动两项合同测试生成
/specs/007-integration-gateway-and-mcp/tasks run "T012"  # HTTP contract test scaffolding
/specs/007-integration-gateway-and-mcp/tasks run "T013"  # gRPC contract test scaffolding

# Phase 2 模型任务可同时执行
/specs/007-integration-gateway-and-mcp/tasks run "T004"
/specs/007-integration-gateway-and-mcp/tasks run "T005"
/specs/007-integration-gateway-and-mcp/tasks run "T006"
```

> 按上述依赖执行，可确保多插件能力目录在 3 分钟内同步、Selector 自动路由、Workflow 模板受控升级，满足 spec 中的成功标准。
