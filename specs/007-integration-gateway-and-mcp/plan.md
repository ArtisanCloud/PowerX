# Implementation Plan: Integration Gateway & MCP Server（多插件能力对齐）

**Branch**: `007-integration-gateway-and-mcp` | **Date**: 2025-12-15 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/007-integration-gateway-and-mcp/spec.md`

## Summary

- 交付 Capability Sync Worker + Registry 扩展，让 `.pxp` 包在 3 分钟内写入 Postgres/Redis，并向 Integration Gateway、Agent Hub、Workflow Builder 广播 `capability.catalog.sync_*`。
- 构建统一的 CapabilityRegistryService（REST + gRPC + MCP）暴露 `capability_id/plugin_id/protocols/tool_scope/workflow_template_ref`，Selector 以 `CapabilityInvokeRequest` 优先 MCP，失败自动切换 gRPC/Workflow。
- 加固可观测与升级治理：所有入口复用 W3C Trace，事件主题 `integration.gateway.*`，Workflow/Agent 默认为旧模板，需管理员显式升级，4 种协议均共享限流、审计与幂等治理。

## Technical Context

**Language/Version**: Go 1.24（backend 单体，Buf toolchain）  
**Primary Dependencies**: Gin HTTP 栈、google.golang.org/grpc、Buf、GORM、Redis、PostgreSQL、EventBus、OpenTelemetry、px-plugin CLI  
**Storage**: PostgreSQL（CapabilityRecord, CapabilitySyncJob, InvocationTrace）、Redis（Capability cache、ToolStore、RateLimit、SelectorPolicySnapshot）、MinIO/S3（插件 workflow/composite 资产引用，仅存 URI）  
**Testing**: `go test`（service/adapter 单测）、Buf breaking tests、OpenAPI contract tests（Dredd/Prism）、MCP 工具集成测试（px-plugin fixtures）  
**Target Platform**: Linux/Kubernetes 宿主（PowerX CoreX 单体 + Agent Hub + Workflow Engine）  
**Project Type**: CoreX Backend Module（backend/）  
**Performance Goals**: 能力同步 ≤3 分钟；读调用 90% MCP or REST 并发成功；写调用 100% gRPC；协议 fallback 成功率 ≥98%；事件/追踪在 1 分钟内可查询  
**Constraints**: 多租户隔离、W3C Trace、统一观测指标、Registry 为单一事实来源、Workflow/Agent 手动模板升级、不可引用插件目录  
**Scale/Scope**: 50+ 插件（每插件 10~30 能力）、每日 ≥5k 调用、Agent Hub MCP session 50+、Workflow 模板 >200 节点

## Constitution Check

| Gate | Status | 说明 |
|------|--------|------|
| COREX_DECLARED | ✅ | 集成网关+Capability Registry 归属 CoreX `corex.agent` 子域，非插件路径 |
| NO_PLUGIN_REGISTRY | ✅ | 所有实现位于 `backend/internal/...`，不触及 `plugins/registry.json` |
| COREX_LAYOUT_MATCH | ✅ | 将在 `internal/service/capability_registry`、`internal/transport/http/(admin|openapi)/capability_registry`、`internal/transport/grpc/capability_registry` 建立目录，遵循 Article 0.2 |
| COREX_DUAL_TRANSPORT | ✅ | 同步维护 `contracts/http-openapi.yaml` 与 `api/grpc/contracts/powerx/integration_gateway/v1/*.proto`，REST + gRPC 均实现 |
| COREX_BUF_CONFIG | ✅ | Buf 配置沿用 `backend/api/grpc/contracts/{buf.yaml,buf.gen.yaml}` 并输出至 `api/grpc/gen/go/powerx/integration_gateway/v1` |
| COREX_SERVER_WIRING | ✅ | HTTP 通过 `backend/internal/http/router.go`，gRPC 由 `backend/internal/server/grpc/server.go` 统一装配 + 拦截器链 |
| COREX_MIGRATION_WIRING | ✅ | 新模型落在 `backend/pkg/corex/db/persistence/model/capability_registry` 并在 `pkg/corex/db/database/migration.go` → `MigrateCoreModels` 注册 |

*Phase 1 设计产物已完成复核，上述 gate 均保持通过。*

## Project Structure

### Documentation (this feature)

```
specs/007-integration-gateway-and-mcp/
├── plan.md              # 本文件
├── research.md          # Phase 0 研究
├── data-model.md        # Phase 1 数据模型
├── quickstart.md        # Phase 1 快速入门
├── contracts/           # Phase 1 合同 (OpenAPI + Proto)
└── tasks.md             # Phase 2 输出
```

### Source Code (repository root)

```
backend/
├── api/
│   └── grpc/
│       ├── contracts/powerx/integration_gateway/v1/ (新 proto + buf)
│       └── gen/go/powerx/integration_gateway/v1/ (buf 生成)
├── internal/
│   ├── dto/capability_registry/
│   ├── service/capability_registry/        (sync worker、registry service、selector policy)
│   ├── transport/
│   │   ├── http/admin/capability_registry/ (Admin API)
│   │   ├── http/openapi/capability_registry/ (Tenant API)
│   │   └── grpc/capability_registry/       (gRPC 服务)
│   ├── agent/toolstore/                    (缓存刷新 & Selector)
│   └── bootstrap/ + server/grpc/           (装配)
├── pkg/corex/db/persistence/
│   ├── model/capability_registry/
│   └── repository/capability_registry/
├── cmd/database/migrate.go                 (迁移入口)
└── tests/
    ├── contract/integration_gateway/      (REST/gRPC contract 测试)
    └── integration/capability_registry/
```

**Structure Decision**: 该特性扩展 CoreX Agent/Workflow 基座，不创建新项目；HTTP/gRPC Handler 与 Service/Repository 均置于 `backend/internal` & `pkg/corex/db`，复用现有单体装配、EventBus、Redis 与 Buf Toolchain。

## Complexity Tracking

无额外复杂度豁免需求。
