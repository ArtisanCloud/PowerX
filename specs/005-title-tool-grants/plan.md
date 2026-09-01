# Implementation Plan: Tool Grants & Security Policy for Integration

**Branch**: `[005-title-tool-grants]` | **Date**: 2025-10-19 | **Spec**: `/specs/005-title-tool-grants/spec.md`  
**Input**: Feature specification from `/specs/005-title-tool-grants/spec.md`

**Note**: 本计划遵循 Spec-Kit `/speckit.plan` 工作流，并受 PowerX Constitution 约束。

## Summary

围绕事件骨干的工具授权域，交付能力/Grant 定义、请求评估、Challenge 审批、审计与告警的完整治理闭环。方案基于 Go CoreX 架构，提供 HTTP Admin API 与 gRPC 服务，结合 PostgreSQL + Redis 缓存及 OpenTelemetry 观测，确保多租户隔离、默认拒绝策略与 3 年审计留存。

## Technical Context

**Language/Version**: Go 1.26.7
**Primary Dependencies**: Gin、gRPC-Go、GORM、Redis（go-redis）、OpenTelemetry、buf CLI、stretchr/testify  
**Storage**: PostgreSQL（GORM 模型）、Redis（授权/审批缓存）  
**Testing**: Go `testing`、`stretchr/testify`、`httptest`、gRPC InProcess 测试、buf lint  
**Target Platform**: Linux/Kubernetes（CoreX 服务）  
**Project Type**: CoreX 后端单体中的事件骨干模块  
**Performance Goals**: 授权评估平均 ≤ 50ms、P99 ≤ 200ms；1,000 QPS 下无性能退化  
**Constraints**: 默认拒绝 + 最小权限；多租户隔离；Challenge 超时自动拒绝；审计 100% 写入；缓存 TTL 与 Grant 同步；KMS 配置加密  
**Scale/Scope**: 覆盖上千 Agent、月级 10^7 授权评估、审计留存 ≥ 3 年

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| HTTP_PRESENT | ✅ | 设计 `internal/transport/http/admin/event_fabric/authorization` + OpenAPI 合同 |
| GRPC_PRESENT | ✅ | 设计 `internal/transport/grpc/event_fabric/authorization` + gRPC 合同 |
| PROTOBUF_DEFINED | ✅ | 计划产出 `contracts/grpc-tool-grants.proto`，实施期落地到 `api/grpc/contracts/...` 并更新 buf 配置 |
| SERVER_DEFINED | ✅ | gRPC 服务通过 `internal/server/grpc/server.go` 统一注册，含认证/租户/日志拦截器 |
| MAKE_TARGETS | ✅ | Makefile 将扩展 `proto-gen/proto-lint/proto-clean` 以覆盖新 proto（列入 Phase 2 任务） |

## Project Structure

### Documentation (this feature)

```
specs/005-title-tool-grants/
├── plan.md          # 本文件
├── research.md      # Phase 0 研究成果
├── data-model.md    # Phase 1 数据建模
├── quickstart.md    # Phase 1 快速体验指南
├── contracts/       # Phase 1 HTTP & gRPC 合同
└── tasks.md         # /speckit.tasks 产出（本阶段不生成）
```

### Source Code (repository root)

```
internal/
├── service/event_fabric/
│   ├── authorization/          # 新增授权领域服务、SLA/审批逻辑
│   ├── metrics/                # 复用并扩展评估指标
│   └── security/               # 扩展中间件集成新策略
├── transport/http/admin/event_fabric/
│   └── authorization_handler.go 等
├── transport/grpc/event_fabric/
│   └── authorization_service.go 等
├── http/router.go              # 注册授权 Admin 路由
└── server/grpc/server.go       # 注册授权 gRPC 服务

pkg/corex/db/persistence/model/
└── event_fabric_authorization_models.go  # 能力/Grant/审批实体

pkg/corex/db/persistence/repository/
└── event_fabric_authorization_repository.go

api/grpc/contracts/powerx/event_fabric/v1/
└── authorization.proto         # 实施阶段目标

Makefile                        # 更新 proto 相关 target
```

**Structure Decision**: 沿用 CoreX 服务分层；授权领域新增在 `internal/service/event_fabric/authorization`，HTTP/GRPC Handler 分别落在现有 transport 子目录；数据模型放入统一 persistence 模块；buf/proto 目录遵循 PowerX 规范。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _None_ | - | 已按宪法要求设计，无新增复杂度需要豁免 |
