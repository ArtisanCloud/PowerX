# Implementation Plan: Plugin Release & Marketplace Publishing Foundation

**Branch**: `001-install-plugin-pxp` | **Date**: 2025-11-05 | **Spec**: `specs/001-install-plugin-pxp/spec.md`
**Input**: Feature specification from `/specs/001-install-plugin-pxp/spec.md`

## Summary

交付一个 CoreX `plugin_release` 模块（domain：`corex.plugin_release`，随 CoreX 一体交付），串起本地构建 (`px-plugin build/dev`)、测试租户流水线、审批计划、灰度/全量部署与 Marketplace 多渠道上架。技术方案：以 Postgres + GORM 建立 Release Candidate/Plan/OfflinePackage 模型，复用 Gin/gRPC 双入口承载审批与执行指令，CLI (`powerx publish/package/import`) 通过 gRPC 调度流水线，Prometheus + Grafana 观测栈输出 5 分钟内的异常告警，离线包落盘到现有对象存储并携带签名/指纹元数据。

## Technical Context

**Language/Version**: Go 1.24（backend），Node 20（Web Admin 热更新面板），Go 1.21（px-plugin CLI）  
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
├── cmd/powerx/commands/publish/  # `powerx publish/package/import` 子命令
├── cmd/database/migrate.go       # 注册 AutoMigrate 钩子
├── config/schema/plugin_release.yaml  # Feature gate & 默认阈值
├── tests/contract/plugin_release/http|grpc/
└── tests/integration/plugin_release/
```

**Structure Decision**: 采用 CoreX 模块分层（model → repository → service → transport），以 `shared.Deps` 注入数据库/缓存/对象存储；HTTP Admin + OpenAPI 负责人机交互，gRPC 供 CLI / 自动化调用，确保 Constitution 要求的双传输、统一 Buf 配置与 Make 流程。上述目录严格遵循 Constitution 0.2 表格中的 `pkg/corex` + `internal/{service,transport}` 约束，所有模型与仓储均落在 CoreX 规定的路径下。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _None_ | - | - |
