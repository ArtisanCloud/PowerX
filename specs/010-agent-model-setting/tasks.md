# 任务清单：Agent Model Hub Connectivity & Governance

**输入**：`specs/010-agent-model-setting/` 下的 plan.md、spec.md、research.md、data-model.md、contracts/、quickstart.md  
**目的**：按阶段完成多模态 Provider 接入、路由治理、成本配额守护、外部平台连接器以及观测指标，满足规格与宪章要求。

## 执行节奏

```
1. 契约/生成工具 → 先让测试失败（TDD）
2. 数据模型 & 仓储 → 领域服务 → 传输层
3. 每个 User Story 独立可交付；完成后再进入下一个
4. 观测与成功指标验证收尾
```

## 阶段任务

> 说明：勾选框 `[ ]` 代表尚未完成，`[X]` 代表已完成。请依次执行，只有同一阶段中标记 `[P]` 的任务在依赖满足后才可并行。

### Phase 0 – 既有实现修复（阻塞所有阶段）

- [X] **T000a** 补齐后端 `buildEntitiesFromPayload`（`backend/internal/transport/http/admin/agent/setting_handler.go`）对 AudioTTS/AudioASR/Rerank 模态的实体生成，确保可落库。
- [X] **T000b** 将 `saveSettings` 中的租户获取改为 `reqctx.RequireTenantIDFromGin`，无租户信息直接返回 4xx，防止写入租户 0。
- [X] **T000c** 扩展 `AgentSettingService.TestConnection/QuickCall` 与 HTTP handler，让所有模态可测试（移除 “非 LLM 返回 501” 的分支）。
- [X] **T000d** 前端 `saveSettings()` 根据模态序列化特定参数（例如 image.size、tts.voice、rerank.topK），并保证 payload 与后端结构一致。
- [X] **T000e** 前端 `testConnection/testQuickCall` 使用 `env.value`，移除硬编码 `"default"`；同时在 store 层为 API 请求传入 env。
- [X] **T000f** `useAISettingsStore` 增加 `initialize()` 调用与环境切换监听（env 改变时重新加载 active profile / models / credentials）。
- [X] **T000g** 为现有 `/admin/agents/settings` 系列接口编写回归用例，记录当前功能缺口，作为 Phase3 任务的参考基线。
- [X] **T000h** 新增契约测试，确保所有 HTTP 响应不会返回 `api_key` 等敏感字段（`tests/contract/http/admin/agent/credentials_contract_test.go`）。
- [X] **T000i** 针对 Secret Rotation 编写自动化测试（`tests/integration/agent_model_hub/secret_rotation_test.go`），验证 Vault sealed 值与 rotation workflow 正常。

> ❗ **Phase 0 未完成前禁止进入 Phase 1**，否则会与现有实现冲突。

### Phase 1 – 工具与契约初始化

- [X] **T001** 更新 `api/grpc/contracts/powerx/agent_model_hub/v1/`、`buf.yaml`、`buf.gen.yaml`，纳入 `contracts/grpc-agent-model-hub.proto` 并确保 `make proto-lint && make proto-gen` 通过。
- [X] **T002 [P]** 将 `contracts/http-openapi.yaml` 接入 `backend/internal/openapi` 生成 `/openapi.min.json`，供 HTTP 契约测试使用。
- [X] **T003 [P]** 扩展 CI 与新增 `scripts/ci/agent_model_hub_tests.sh`，让新的契约/集成测试自动运行，并在 `quickstart.md` 写明命令。

### Phase 2 – 基础数据与治理（阻塞所有 User Story）

- [X] **T004** 建立 ProviderProfile 模型 `pkg/corex/db/persistence/model/agent_model_hub/provider_profile.go` 及 AutoMigrate。
- [X] **T005 [P]** 建立 RoutingPolicy 模型/仓储 `.../routing_policy.go` 与 `repository/routing_policy_repository.go`。
- [X] **T006 [P]** 建立 ConnectorInstance 模型与仓储骨架。
- [X] **T007 [P]** 建立 CostQuotaLedger 模型与仓储骨架。
- [X] **T008** 在 `backend/internal/service/{provider_registry,model_routing,connector_guard,cost_quota}` 建立共享服务基础（Vault 加密、Redis 缓存、Telemetry 输出）。
- [X] **T008a** 在 `provider_registry` 服务中实现 Vault sealed secret 存储（零明文落库），并封装通用结构（FR-002 核心）。
- [X] **T008b** 建立 Secret Rotation 调度器（结合 `scripts/ops/provider-release.mjs` 和定时任务），确保持久化密钥自动轮换。
- [X] **T008c** 审计所有密钥操作：在 `provider_registry`、`cost_quota` 等服务中记录审计事件并与 `pkg/audit` 集成。
- [X] **T009** 在 `backend/config/agents/{providers.d,routing.d,feature_flags}` 预置多环境开关与默认配置。

> ✅ 阶段完成标志：所有 GORM 模型与仓储可 AutoMigrate，服务基础可被依赖。

### Phase 3 – User Story 1：Provider Onboarding（P1）

- [ ] **T010 [P]** HTTP 契约测试 `tests/contract/http/admin/agent_model_hub_provider_test.go` 覆盖 `/providers /models /settings/*`.
- [ ] **T011 [P]** gRPC 契约测试 `tests/contract/grpc/agent_model_hub_provider_test.go` 覆盖 Register/Validate/Publish。
- [ ] **T012a** 扩展 `internal/service/agent/agent_setting_service.go`，让 `SaveCredentialAndProfile` + 仓储同时支持 7 种模态（含默认参数、Tags、能力标签）。
- [ ] **T012b** 在 `agent_setting_service.go` 中实现多模态健康检查逻辑（PingLLM/PingImage/...），确保 `TestConnection/QuickCall` 可按照模态走差异化验证。
- [ ] **T012c** 更新 `backend/internal/transport/http/admin/agent/setting_handler.go` 的 `buildEntitiesFromPayload`、`saveSettings` 及请求校验，与服务层保持一致（此任务依赖 T012a/T012b）。
- [ ] **T012d** 扩展 `scripts/ops/provider-validator.mjs`，使其可针对不同模态调用真实接口并产出健康报告，为 T015 的工件管道提供输入。
- [ ] **T013** （保留）实现模态化 Ping/Test 时的 HTTP 入口逻辑，并覆盖日志/审计。
- [ ] **T014** 新增 gRPC handler `internal/transport/grpc/agent_model_hub/provider_handler.go`。
- [ ] **T015** 在 `backend/internal/service/provider_registry/validation_artifacts.go` 构建验证工件管道：写入 `minio://agent/providers/<provider>/<timestamp>.json`，Vault 存储 sealed audit 引用，未通过禁止发布。
- [ ] **T016** Web UI (`web-admin/app/pages/settings/ai/index.vue` + store/service)：初始化 store、监听环境变化、将 env 透传至 API、序列化模态参数并展示后端错误。
- [ ] **T017** 集成测试 `tests/integration/agent_model_hub/provider_onboarding_test.go` 覆盖成功/失败路径。
- [ ] **T039** 扩展 `provider_registry/rollout_service.go` 与 `scripts/ops/provider-release.mjs`，实现租户灰度、百分比发布与 5 分钟内回滚（FR-004）。

> ✅ 阶段完成标志：Provider 可多模态接入、验证、灰度上线，前后端功能可独立演示。

### Phase 4 – User Story 2：Routing Policies & Safe-Mode（P2）

- [ ] **T018 [P]** HTTP 契约测试 `tests/contract/http/admin/agent_model_hub_routing_test.go`。
- [ ] **T019 [P]** gRPC 契约测试 `tests/contract/grpc/agent_model_hub_routing_test.go`。
- [ ] **T020** RoutingPolicy 服务 `backend/internal/service/model_routing/routing_policy_service.go`（审批、版本、Telemetry）。
- [ ] **T021** HTTP/gRPC handler (`backend/internal/transport/http/admin/agent/routing_handler.go` + gRPC) 支持 BU 自定义审批与 safe-mode 开关。
- [ ] **T022** 优化 Go 决策引擎 `backend/internal/service/model_routing/decision_engine.go`，接入健康/成本信号，保证 ≤200ms。
- [ ] **T023** 更新 `scripts/ops/routing-simulator.mjs` 以回放新策略。
- [ ] **T024** 集成测试 `tests/integration/agent_model_hub/routing_policy_test.go`。
- [ ] **T041** 在 `backend/internal/service/model_routing/safe_mode_monitor.go` 建立命中率/回退率阈值 → Redis safe-mode 自动触发，并配置告警（满足 FR-007）。

### Phase 5 – User Story 3：Cost & Quota Guard（P3）

- [ ] **T025 [P]** HTTP 契约测试 `tests/contract/http/admin/agent_model_hub_cost_test.go`。
- [ ] **T026** 成本/配额服务 `backend/internal/service/cost_quota/cost_quota_service.go`（写入 ledger、发布 `agent.provider.cost.anomaly`、人工确认）。
- [ ] **T027** HTTP/gRPC cost handler。
- [ ] **T028** 在 `web-admin/app/pages/settings/ai/index.vue` 与新建 `web-admin/app/pages/dashboards/tenants/[tenantId].vue` 展示租户路由命中率、连接器状态、成本配额与限流标记（含指标解释与告警链接）。
- [ ] **T029** Grafana/PagerDuty 及 `scripts/qa/provider-drill.mjs` 校验告警与确认流。
- [ ] **T030** 集成测试 `tests/integration/agent_model_hub/cost_guard_test.go`（异常 → 人工确认 → 回滚）。

### Phase 6 – User Story 4：Connector Platform（P4）

- [ ] **T031 [P]** HTTP/gRPC 契约测试覆盖 `/connector-platforms/*` 与 Upsert/Pause RPC。
- [ ] **T032** Connector 服务 `backend/internal/service/connector_guard/connector_service.go`：OAuth、映射、签名校验、实例级降级。
- [ ] **T033** HTTP/gRPC handler (`backend/internal/transport/http/admin/agent/connector_handler.go` 等)。
- [ ] **T034** 更新 `powerx-plugin/connectors/{coze,n8n}` 适配新契约与 Trace ID。
- [ ] **T035** 集成测试 `tests/integration/agent_model_hub/connector_flow_test.go`（注册→调用→实例暂停）。

### Phase 7 – 观测、文档与成功指标验证

- [ ] **T036** 在 `backend/internal/service/{provider_registry,model_routing,connector_guard,cost_quota}` 增加 OTel 指标/日志，实现 spec 中的 Telemetry 列表。
- [ ] **T037 [P]** 更新 `docs/use_cases/_from_hub/SCN-AGENT-MODEL-HUB-001/*`、`quickstart.md`、web-admin README。
- [ ] **T038 [P]** 按 quickstart 全链路回归，收集日志并附加至 checklist。
- [ ] **T042 [P]** `scripts/qa/provider-onboard-benchmark.mjs` 验证 SC-001：模拟批量上线，统计 95% 以内上线时间，并结合静态扫描 + HTTP 契约测试确认“零明文密钥”。
- [ ] **T042a [P]** 在 `tests/contract/http/admin/agent_model_hub_provider_test.go` 增加断言：响应中禁止出现 `api_key`/`secret` 字段，配合 Vault 审计日志验证。
- [ ] **T042b [P]** 基准脚本输出 JSON 报告，若任何 Provider 超过 24h 或秘钥泄露即失败。
- [ ] **T043 [P]** 扩展 `scripts/ops/routing-simulator.mjs` + Grafana，验证 SC-002：命中率 ≥90%、回退成功率 ≥95%、safe-mode 自动触发并在 5 分钟内回滚。
- [ ] **T043a [P]** Chaos 测试：人为注入主模型故障，确认 safe-mode 指标/告警与 Redis 切换生效。
- [ ] **T044 [P]** `tests/integration/agent_model_hub/connector_reliability_test.go` 验证 SC-003：模拟 100 次调用，统计成功率/签名失败率，并将结果写入 Grafana 面板。
- [ ] **T045 [P]** 在 `scripts/qa/provider-drill.mjs` 增加成本金丝雀，记录异常触发到告警送达/推荐执行耗时，需 <5 分钟（SC-004）。
- [ ] **T046 [P]** `tests/integration/agent_model_hub/audit_latency_test.go` 验证 SC-005：测量 rollout/enforcement 审计写入 + 查询耗时，并生成 Trace 供稽核。


## 依赖关系与并行指导

1. Phase 0 → Phase 1 → Phase 2 → User Stories → Phase 7，严格顺序执行。  
2. 标记 `[P]` 的任务只在依赖完成后并行，例如 T010/T011、T018/T019。  
3. 各 User Story 完成（契约 → 服务 → 传输 → 集成测试）后即可独立演示。  
4. Phase 7 任务依赖所有 User Story 完成，尤其是 T036 之后再执行成功指标验证（T042–T046）。

## 进度查看方式

- 每个任务前的 `[ ] / [X]` 即状态；完成后将 `[ ]` 改为 `[X]`。  
- 可配合 Git 提交信息或 issue 状态追踪，保证与任务编号一致。  
- 若需更细粒度，可在同一行结尾补充 `（完成人/日期）` 备注。
