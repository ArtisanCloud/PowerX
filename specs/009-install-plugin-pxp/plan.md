# Implementation Plan: Plugin Release & Marketplace Publishing Foundation

**Branch**: `001-install-plugin-pxp` | **Date**: 2025-11-05 | **Spec**: `specs/001-install-plugin-pxp/spec.md`
**Input**: Feature specification from `/specs/001-install-plugin-pxp/spec.md`

## Summary

交付一个 CoreX `plugin_release` 模块（domain：`corex.plugin_release`，随 CoreX 一体交付），串起本地构建 (`px-plugin build/dev`)、测试租户流水线、审批计划、灰度/全量部署与 Marketplace 多渠道上架。技术方案：以 Postgres + GORM 建立 Release Candidate/Plan/OfflinePackage 模型，复用 Gin/gRPC 双入口承载审批与执行指令，CLI (`px publish/package/import`) 通过 gRPC 调度流水线，Prometheus + Grafana 观测栈输出 5 分钟内的异常告警，离线包落盘到现有对象存储并携带签名/指纹元数据。

补充：插件发布/安装包必须携带权限声明资产，供 `specs/007-integration-gateway-and-mcp` 的 Capability Sync Worker 登记 `menu/page/action/api` 细颗粒度权限。plugin_release 只负责打包、扫描和阻断不合法资产，不负责正式角色授权；授权入口归属 IAM 角色权限中心。

插件数据库角色隔离使用宿主实例级 `deployment.env`，不使用插件包或单次安装请求的环境标签。安装时沿用 `px_<plugin_slug>` Schema/Database，生成 `pxu_<env>_<plugin_slug>_<hash8>` Role/User，并把绑定写入 host-values、Registry 和审计。同一环境重新安装时，Core 数据库管理连接会把该 Schema/Database 内历史插件对象 owner 收敛到目标 Role 并撤销旧插件 Role 权限；缺少环境、环境不一致或管理账号无法转移 owner 时 fail-fast。实例环境变更仍仅能通过独立 dry-run repair/migration 工具处理。

## Technical Context

**Language/Version**: Go 1.26.7（backend），Node 20（Web Admin 热更新面板），Go 1.21（px-plugin CLI）
**Primary Dependencies**: Gin HTTP 栈、google.golang.org/grpc、Buf toolchain、GORM + PostgreSQL、Redis（队列与 Feature Flag）、MinIO/S3 SDK（离线包存储）、OpenTelemetry + Prometheus Exporter、PowerX CLI (`powerx`, `px-plugin`)  
**Storage**: Postgres（release candidates、计划、审批日志）、Redis（流水线状态/令牌）、对象存储 `media_storage` 集群（离线包体 + 校验文件）、Audit Trail（现有 pg schema）  
**Testing**: Go `testing` + `testify`（service/repository）、`httpexpect`（REST 合同）、`buf conn/grpce2e`（gRPC）、CLI smoke（`px-plugin` integration shell）、GitHub Actions make targets（proto/test/deps）  
**Target Platform**: Linux container（Kubernetes 部署的 CoreX）+ 企业租户 Web Admin 浏览器体验 + CLI（macOS/Linux/Win）  
**Project Type**: CoreX 后端模块 + CLI 扩展 + Marketplace Admin UI 动线  
**Performance Goals**: 测试租户流水线结果 ≤24h、灰度阶段异常→回滚 ≤5min、CLI 本地构建+安装循环 ≤15min、Marketplace 通知延迟 ≤5min、API p95 <200ms、插件发布吞吐 50 并发任 务  
**Constraints**: 多租户隔离 + RBAC、全链路审计 180 天留存、Prometheus/Grafana 唯一观测栈、对象存储复用、HTTP+gRPC 双入口强制、proto Buf 配置与 Make targets 复用、无额外服务器实例  
**Scale/Scope**: 100+ 插件团队、每月 500 次发布申请、Marketplace 3 个渠道（在线/离线/灰度）、离线包单体 ≤1GB、审计日志 180 天留存 ~5 亿行

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| COREX_DECLARED | ✅ `spec.md` 顶部声明 `CoreX (corex.plugin_release)` 归属 |
| HTTP_PRESENT | ✅ 交付 `specs/001-install-plugin-pxp/contracts/http-openapi.yaml`，并规划 `internal/transport/http/admin/plugin_release`（审批与计划）与 `internal/transport/http/openapi/plugin_release`（租户导入） Handler |
| GRPC_PRESENT | ✅ 交付 `specs/001-install-plugin-pxp/contracts/plugin_release.proto`，实现 `internal/transport/grpc/plugin_release` 服务供 CLI / Job 编排调用 |
| PROTOBUF_DEFINED | ✅ Proto 将落在 `api/grpc/contracts/powerx/plugin_release/v1/plugin_release.proto`，并更新全局 `buf.yaml`、`buf.gen.yaml` 及 `make proto-*`，当前设计文档已提供源稿 |
| SERVER_DEFINED | ✅ gRPC Service 由 `internal/server/grpc/server.go` 注册，复用现有拦截器链（auth/tenant/logging/recovery）并新增 release-specific interceptors（feature flag, audit） |
| MAKE_TARGETS | ✅ 继续依赖仓库既有 `make proto-gen proto-lint proto-clean`，无需额外 target；plan 已列出需要触发的 Make 命令 |

（Phase 1 文档输出后复查以上 Gate，均保持通过状态，无需豁免。）

## Project Structure

### Documentation (this feature)

```
specs/001-install-plugin-pxp/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── plugin_release.proto
└── tasks.md                  # 由 /speckit.tasks 生成
```

### Source Code（repository root）

```
backend/
├── api/grpc/contracts/powerx/plugin_release/v1/
│   └── plugin_release.proto
├── internal/service/plugin_release/
│   ├── pipeline/                 # 构建/扫描/审批 orchestrator
│   ├── distribution/             # 离线包、Marketplace 渠道
│   ├── runtime/                  # 灰度执行与回滚控制
│   └── instrumentation/          # 指标、日志、审计封装
├── internal/transport/http/admin/plugin_release/
│   ├── handler.go                # 审批/计划 CRUD
│   └── routes.go
├── internal/transport/http/openapi/plugin_release/
│   └── handler.go                # 租户导入、自助灰度触发
├── internal/transport/grpc/plugin_release/
│   ├── server.go                 # gRPC Service 实现
│   └── mapper.go                 # DTO ↔ domain
├── pkg/corex/db/persistence/model/plugin_release/
│   ├── release_candidate.go
│   ├── release_plan.go
│   ├── offline_package.go
│   └── canary_record.go
├── pkg/corex/db/persistence/repository/plugin_release/
│   └── *.go                      # 继承 BaseRepository
├── cmd/px/commands/publish/      # `px publish/package/import` 子命令
├── cmd/database/migrate.go       # 注册 AutoMigrate 钩子
├── config/schema/plugin_release.yaml  # Feature gate & 默认阈值
├── tests/contract/plugin_release/http|grpc/
└── tests/integration/plugin_release/
```

**Structure Decision**: 采用 CoreX 模块分层（model → repository → service → transport），以 `shared.Deps` 注入数据库/缓存/对象存储；HTTP Admin + OpenAPI 负责人机交互，gRPC 供 CLI / 自动化调用，确保 Constitution 要求的双传输、统一 Buf 配置与 Make 流程。上述目录严格遵循 Constitution 0.2 表格中的 `pkg/corex` + `internal/{service,transport}` 约束，所有模型与仓储均落在 CoreX 规定的路径下。

### Plugin Database Isolation Alignment

- 配置真源：`Config.Deployment.Env`，允许值 `dev/test/staging/prod`。
- 安装边界：在解包落盘、DDL 和 migration 前完成配置与旧绑定校验。
- 命名边界：环境段和稳定 `hash8` 不可被长度裁剪移除。
- 权限边界：专用账号仅访问本环境、本插件对象。
- 生命周期边界：install/replace/restore/migration/purge 使用 Registry 中记录的实际对象名并校验环境。
- metadata 边界：发布渠道只使用 `metadata.release_channel`；旧 `metadata.environment` 返回字段弃用错误，不作为配置别名且不保留隐式翻译。
- 迁移边界：启动不做旧名称兼容；同环境安装可执行受控的对象 owner 收敛，实例环境变更 repair 工具默认 dry-run 并要求显式批准。

详细部署与迁移设计见 `docs/plan/deploy/plugin-database-isolation.md`。

### Web Admin 扩展（Node 20）

```
web-admin/
└── app/pages/plugin-release/
    ├── OfflinePackages.vue        # 表单 + 列表，调用 /offline-packages
    ├── MarketplaceListings.vue    # 审核列表
    └── ReviewDetail.vue           # 审核详情与操作
```

- 页面依赖现有 Admin API（Bearer Token）进行调用，沿用 RBAC/审计。
- 所有 API 交互均通过 `@/services/pluginRelease` 包裹，便于单元测试与错误提示。

### Frontend Architecture

- **Framework**: Nuxt 4 + Vue 3 + TypeScript（与当前 web-admin 项目一致）
- **UI Library**: Nuxt UI +自定义组件（结合 Tailwind/Valibot 验证），可按需封装表单、表格、Skeleton、Modal
- **State/数据流**: Pinia（`@pinia/nuxt`）集中管理插件发布页面的过滤条件与列表缓存
- **API Client**: `$fetch` + 全局拦截器，统一注入 Authorization header、处理错误 toast
- **Routing**: `/admin/plugin-release/offline-packages`、`/admin/plugin-release/marketplace`、`/admin/plugin-release/marketplace/:id`
- **Error Handling**: 全局 error handler + toast，展示 audit reference；页面提供 loading skeleton 与局部 Spin

## Additional Workstreams from New Usecases

### Workstream A – Developer Bootstrap & Third-party Import (SCN-DEV-PLUGIN-INIT-001)

1. **Template Registry & CLI Integration**
   - Extend `powerx-plugin` template index + lockfiles (`powerx-plugin/templates/*`, mirrored到 `config/plugins/templates/index.yaml`)，暴露 `px plugin init --template <id>` 选择器。
   - Backend 提供 `POST /internal/plugins/bootstrap/validate`（新建 `backend/internal/service/plugin_bootstrap`），校验 manifest、权限模板、CLI 版本与租户配额。
   - CLI 侧（`px-plugin/cmd/init`）串联模板拉取、依赖安装、Git 注册（调用 `POST /internal/git/register`），落地 FR-014。
2. **Team Clone & Environment Doctor**
   - 新增 `plugin doctor` 子命令（`px-plugin/cmd/doctor`）+ `backend/internal/service/plugin_bootstrap/doctor.go`，读取 `.powerxci/onboarding.yaml` 校验多语言运行时、env 模板、pre-commit。
   - 将校验结果写入 `audit.plugin.bootstrap` 表，失败时返回 machine-readable JSON 供 CLI 展示 & VS Code 扩展消费，覆盖 FR-015。
3. **Third-party Import & Compliance Guardrail**
   - 在 `backend/internal/service/plugin_import` 建立上传、解包、扫描流程；依赖现有合规微服务或通过 `internal/service/security` 包装 `licensescan`、`vulnscan` gRPC。
   - 生成 `ImportRiskReport`（存于 `pkg/corex/db/persistence/model/plugin_import`），并在审批通过后调用 `POST /internal/git/register` + 模板适配脚本（复用 `powerx-plugin/scaffold`），满足 FR-016。

### Workstream B – Rapid Debug, Diagnostics & Sandbox Validation (SCN-DEV-PLUGIN-DEBUG-001)

1. **Host Simulator & Hot Reload**
   - 引入 `backend/internal/service/plugin_debug/host`（守护宿主模拟器生命周期）与 CLI `px host start --mock`，通过 gRPC `plugin_debug.HostService` 与本地 watcher 通信，确保热更新 <2s（FR-017），同时在 OTel 中记录 `debug.hot_reload.*`/`debug.host.version_mismatch_total`。
   - 维护 `config/plugins/debug/host_simulator.yaml` + `PX_PLUGIN_HOST_SIMULATOR` flag，在 Mac/Win/Linux 预编译模拟器镜像。
2. **Diagnostics & Error Reporting**
   - 构建 `backend/internal/service/plugin_debug/diagnostics`，暴露 `POST /internal/debug/report`、`POST /internal/debug/logs/export`；整合日志、Tracing、metrics 并脱敏（FR-018），固化 `config/plugins/debug/report_template.yaml` + `config/security/data_masking_rules.yaml`。
   - 接入 ticket bridge（`backend/internal/service/integration/ticket_bridge`）与通知模块，确保 60s 内生成报告 + 触发工单。
3. **Sandbox Validation Orchestrator**
   - 新建 `backend/internal/service/plugin_sandbox` + Job runner（基于 `workflow` queue）来驱动 `POST /internal/sandbox/deploy|dataset/load|test/run`（FR-019），依赖 `config/plugins/debug/data_suite.yaml` 维护数据集映射。
   - 对接数据脱敏服务、Feature Flag `plugin-sandbox-suite`，生成 `sandbox_validation_runs` 表和报告上传逻辑，输出给 QA Portal。

### Workstream C – Version Governance & Compatibility Guard (SCN-DEV-PLUGIN-VERSION-COMPAT-001)

1. **Version Scan Scheduler & Notification**
   - 新模块 `backend/internal/service/plugin_governance` 定时扫描 `plugin_release_candidates` + 租户 manifest，计算升级建议（FR-020）。
   - CLI `px version scan` 与 Admin 面板共用 `GET /api/admin/plugin-release/governance/reports`，推送升级卡片、风险等级和决策 API。
2. **Compatibility Engine & Exception Workflow**
   - 新建 `backend/internal/service/plugin_compat`，提供 `POST /internal/version/compat/check`、`exception`、`approve`，内存缓存 `config/version/compat_matrix.yaml`（FR-021）。
   - 审批链复用现有 `approval` service，输出 `compat_exceptions` 表 + 审计事件，默认阻断矩阵缺失的操作。
3. **Multi-tenant Version Board & Automation**
   - 构建 `backend/internal/service/plugin_governance/multitenant.go`，根据版本漂移生成批量灰度/对齐计划，产出报告存储至 `governance_reports`（FR-022）。
   - Web Admin 新增 `/admin/plugin-release/governance` 页面或 CLI `px version board`，支持筛选租户、生成策略与回滚预案。

### Cross-cutting Considerations

- **Telemetry**: 扩展 `backend/internal/service/plugin_release/instrumentation`，新增 `debug.hot_reload.*`、`debug.host.version_mismatch_total`、`debug.report.generate_ms`、`sandbox.*`、`version.scan.*` 指标与 trace。
- **Storage**: 需要新的表/视图：`plugin_scaffold_templates`（元数据）、`plugin_import_runs`、`debug_sessions`、`sandbox_validation_runs`、`version_governance_reports`、`compat_exceptions`。
- **Docs & Tooling**: Update `specs/009-install-plugin-pxp/quickstart.md` + README，新增 CLI 手册章节；在 `docs/use_cases/_from_hub/...` 反向链接新的实现路径。
- **Permission Manifest Gate**: 发布与安装校验必须检查插件包内权限声明资产是否存在、schema 是否合法、i18n key 是否完整、REST method/path 与 actor/resource scope 是否明确；失败时阻断发布或安装。

## Registry Publish API（Phase 4~6）落地补充

- 对齐 `docs/guides/develop/dev_hotload.md` 中对 “publish” 步骤的描述，Phase 4~6 需要专门交付 `/internal/plugins/releases` REST 网关：CLI `px-plugin publish`、Web Admin 发布入口与未来离线上传都用该路由，并由 pipeline service 驱动 release candidate 生命周期。
- 具体交付项：
  1. **HTTP Handler**：在 `backend/internal/transport/http/admin/plugin_release/` 新增 `publish_handler.go` 与 routing，覆盖 `POST /internal/plugins/releases`（接收 artifact、manifest、meta 并创建 release candidate）、`GET /internal/plugins/releases/:id`、`PATCH /internal/plugins/releases/:id`（补件、推进审批）、`POST /internal/plugins/releases/:id/artifacts`（追加离线包或扫描报告），复用 AdminOnly/Tenant 校验与审计。
  2. **Service 集成**：为 pipeline service 补充 `SubmitRelease`/`UpdateRelease` 方法，将 HTTP 入参映射到 `ReleaseCandidateRepository`、对象存储（MinIO/S3）与通知通道；失败场景需返回机器可读错误码供 CLI 展示。
  3. **合约测试**：新增 `backend/tests/contract/plugin_release/http_admin_publish_api_test.go`、`backend/tests/integration/plugin_release/test_registry_publish_flow.go` 验证 `package -> publish -> plan` 流程，以及 CLI smoke（`scripts/ci/smoke_px_plugin_publish.sh`）确保 `px-plugin publish --registry http://localhost:8077/api/v1` 能 200。
  4. **文档/CLI 对齐**：更新 `specs/009-install-plugin-pxp/quickstart.md`、`docs/guides/develop/dev_hotload.md`、`powerx-plugin` README 描述 publish 流程与错误提示；CLI `px-plugin publish` 默认为 `/internal/plugins/releases`，并在配置里暴露 `publishApi.baseUrl`。
- 上述交付与 Phase 5（灰度 execution）/Phase 6（Marketplace 分发）共享数据模型，因此需要在实现时同步补充 ReleaseCandidate 状态机、审批字段以及审计指纹，避免后续迭代重复施工。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _None_ | - | - |
