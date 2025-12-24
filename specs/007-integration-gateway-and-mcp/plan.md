# Implementation Plan: Integration Gateway & MCP Server（Base Capability Exposure）

**Branch**: `007-integration-gateway-and-mcp` | **Date**: 2025-12-19 | **Spec**: `specs/007-integration-gateway-and-mcp/spec.md`
**Input**: Feature specification from `/specs/007-integration-gateway-and-mcp/spec.md`

## Summary

在现有多插件能力治理的基础上，本阶段要把 CoreX 媒资、事件、定时任务、知识库、Workflow 等底座能力纳入 Capability Registry + Integration Gateway 的统一接口层：
1. 为每个 CoreX 能力输出公开契约（OpenAPI + gRPC），由 Integration Gateway 汇总并生成 SDK；
2. Registry 增加 `source=corex|plugin` 字段，Platform 能力默认带 Tool Grant 并支持租户级限流；
3. `/tenant/capabilities`、`/tenant/invocations` 成为宿主/ Skeleton 插件统一的调用入口，Admin API 仅保留配置职责；
4. 观测、限流、审计沿用 FR-001~FR-016 的规范，确保 fallback、事件广播、Trace 一致，并在 Web Admin「设置 > 开放能力」页面向 `IsRoot` 管理员实时展示平台开放能力（按模块统计能力数量、协议种类与调试入口），方便宿主与 Skeleton 插件直接对照文档调用。

## Admin 开放能力页面设计（T057）

- **入口 & 权限**：Web Admin 侧边栏 “设置” 下新增 “开放能力” 菜单（建议路由 `/settings/open-capabilities`），仅当当前用户 `isRoot=true` 时渲染，同时接口层面也需校验。
- **数据源**：调用 Capability Registry 只读接口（复用 `/tenant/capabilities?source=corex` 或扩展 `/admin/capabilities/platform`）获取字段 `module`, `capability_id`, `title`, `protocols[*].channel`, `contracts`, `capabilities_hash`。
- **分组展示**：前端按 `module` 分组生成卡片，显示能力数量、最新版本、支持协议徽章；展开行列出单个能力，提供状态 Badge（active/disabled）、描述与 tags。
- **调试入口**：为每条能力提供 cURL/Insomnia snippet（自动填 `capability_id`, `preferred_protocol`, `X-PowerX-Tenant`），MCP Tool 名称、OpenAPI/gRPC 文档链接，Media 模块额外提供 `/media/assets` 调试跳转。
- **刷新策略**：进入页面自动加载，提供“立即同步”按钮重新请求；后续可监听 `capability.catalog.sync_*` 事件或轮询以保持实时。
- **安全 & 审计**：所有请求写入 Audit（记录 `admin_id`, `action=open_capability_view`），并遵守现有 RBAC/Feature Flag，确保非 Root 用户无法访问平台能力细节。

## Technical Context

**Language/Version**: Go 1.24（backend）、Node 20（脚本+CLI）  
**Primary Dependencies**: Gin、gRPC（Buf toolchain）、GORM、Redis、PostgreSQL、EventBus、OpenTelemetry  
**Storage**: PostgreSQL（CapabilityRecord/SyncJob/InvocationTrace）、Redis（SelectorPolicySnapshot、能力缓存）、MinIO/S3（媒资对象）  
**Testing**: `make contracts-test`、`make unit-test`、`go test ./backend/internal/...`；契约测试覆盖 HTTP+gRPC  
**Target Platform**: Linux container / Kubernetes，单体 CoreX 服务 + px-plugin CLI  
**Project Type**: Backend mono-repo（`backend` + `specs` + `powerx-plugin`）  
**Performance Goals**: Registry 更新 ≤3 分钟同步；Integration Gateway 读调用 p95 < 200ms，写调用全部 gRPC（幂等）  
**Constraints**: 多租户隔离、RBAC/Tool Grant 必须生效；所有协议携带 Trace/Audit；HTTP 与 gRPC 均需保持向后兼容  
**Scale/Scope**: 100+ 插件、每租户 1k+ 能力、每秒 200 次能力调用（读+写），Workflow/Agent 双栈共享治理

## Constitution Check

- **COREX_DECLARED / COREX_DOMAIN** ✅：Spec 已声明 `Domain Ownership: CoreX (corex.agent)`。  
- **HTTP_PRESENT** ✅：现有 Media/Integration Gateway HTTP 契约在 `specs/.../contracts/http-openapi.yaml`，计划中将继续扩充并在 plan/tasks 中跟踪。  
- **GRPC_PRESENT** ✅：`backend/api/grpc/contracts/powerx/integration_gateway/v1/*.proto` 为权威源，计划要求继续复用并扩展 CoreX 能力。  
- **PROTOBUF_DEFINED** ✅：`api/grpc/contracts/buf.yaml` 与 `buf.gen.yaml` 已存在，输出 `api/grpc/gen/go`。  
- **SERVER_DEFINED** ✅：gRPC 服务统一在 `backend/internal/server/grpc/server.go` 注册，Integration Gateway/CapabilityRegistry Handler 已挂载。  
- **MAKE_TARGETS** ✅：仓库已有 `make proto-gen|proto-lint|proto-clean`，计划将继续复用。  
- **COREX_MIGRATION_WIRING** ✅：所有 CoreX 模型在 `pkg/corex/db/database/migration.go` 注册，计划要求扩展 Media/Event 等能力时遵循相同挂载。

## Project Structure

### Documentation (this feature)

```
specs/007-integration-gateway-and-mcp/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
```

### Source Code（repository root）

```
backend/
├── internal/
│   ├── service/
│   │   ├── integration_gateway/
│   │   ├── media/
│   │   └── capability_registry/
│   ├── transport/
│   │   ├── http/admin/integration_gateway/
│   │   ├── http/admin/media/
│   │   ├── http/openapi/*
│   │   └── grpc/*
│   └── bootstrap/
├── api/grpc/
│   ├── contracts/powerx/*
│   └── gen/go/powerx/*
├── pkg/corex/db/persistence/{model,repository}/...
└── cmd/

specs/
├── 001-docs-media-storage/
├── 007-integration-gateway-and-mcp/
└── ...
```

**Structure Decision**: 继续沿用 CoreX 单体结构：合同放在 `specs/<feature>/contracts`（HTTP）与 `api/grpc/contracts/powerx/<domain>/v1`（gRPC），服务实现分别位于 `internal/transport/http/(admin|openapi)/<domain>` 与 `internal/transport/grpc/<domain>`，依赖注入统一通过 `internal/bootstrap/app.go` 与 `internal/server/grpc/server.go`。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _None_ | 当前计划完全遵守 CoreX Gates | 若尝试下放到插件会破坏单一事实来源与治理闭环 |
