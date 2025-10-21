# Implementation Plan: Integration Gateway & MCP Server

**Branch**: `007-integration-gateway-and-mcp` | **Date**: 2025-10-21 | **Spec**: `specs/007-integration-gateway-and-mcp/spec.md`
**Input**: Feature specification from `/specs/007-integration-gateway-and-mcp/spec.md`

## Summary

交付一个面向多租户的集成网关：管理端 API 负责创建/更新入口、配置速率限制与事件策略；租户 API 统一发起能力调用并输出规范化响应与追踪；MCP Server 暴露同一能力集以服务智能体。技术上复用 CoreX GORM 模型记录入口与版本、Redis 令牌桶限流器控制租户与入口配额、Capability Registry/Router 解析能力、EventBus 发布 `integration.gateway.*` 事件，并通过 HTTP/Gin、gRPC 与 MCP tool registry 构建三种访问面，保证链路观测与审计一致。

## Technical Context

**Language/Version**: Go 1.24  
**Primary Dependencies**: Gin HTTP 栈、gRPC + buf toolchain、mark3labs/mcp-go、Redis（限流）、Postgres（CoreX GORM）、EventBus 抽象、OpenTelemetry/日志设施  
**Storage**: Postgres（入口配置、版本、审计挂钩）、Redis（限流计数与入口缓存）、EventBus（事件传递）  
**Testing**: Go `testing` + `testify`、HTTP Contract Tests（resty/httpexpect）、gRPC buf 测试桩、MCP 工具集成回归  
**Target Platform**: Linux 容器（Kubernetes 部署，与现有 CoreX 服务同布）  
**Project Type**: CoreX 后端服务模块  
**Performance Goals**: 租户调用首次成功率 ≥99%；事件发布 95% ≤60s；HTTP/gRPC p95 ≤200ms；MCP 调用平均 ≤1s  
**Constraints**: 多租户隔离、RBAC + Tool Grant 校验、统一 trace_id/tenant 日志、默认 1 分钟窗口令牌桶+2 倍突发、事件发布失败需补偿、禁止偏离 CoreX 模块目录约束  
**Scale/Scope**: 预估百级租户、每租户 50~100 条入口、峰值 QPS 500（HTTP+MCP 合并），事件订阅方数十

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| HTTP_PRESENT | ✅ 规划输出 `contracts/http-openapi.yaml`，并在 `internal/transport/http/admin|openapi/integration_gateway` 实现路由 |
| GRPC_PRESENT | ✅ 规划输出 `contracts/integration-gateway.proto`，并在 `internal/transport/grpc/integration_gateway` 注册服务 |
| PROTOBUF_DEFINED | ✅ proto 将放置于 `api/grpc/contracts/powerx/integration_gateway/v1` 并维护 buf 配置 |
| SERVER_DEFINED | ✅ 新增 gRPC handler 纳入 `internal/server/grpc/server.go`，复用既有拦截器链 |
| MAKE_TARGETS | ✅ 继续复用全局 `make proto-gen/proto-lint/proto-clean`，无额外例外 |

> Phase 1 文档与合同均已生成，复核后上述 Gate 依旧满足，无需额外豁免。

## Project Structure

### Documentation (this feature)

```
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)
```
specs/007-integration-gateway-and-mcp/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── integration-gateway.proto
└── tasks.md               # 由 /speckit.tasks 生成

api/
└── grpc/
    └── contracts/powerx/integration_gateway/v1/   # 新增 proto + buf 配置

internal/
├── app/shared/                                   # 扩展 Deps、配置加载、wiring
├── service/integration_gateway/
│   ├── manager/                                   # 管理端 CRUD、版本管理、事件发布
│   ├── tenant/                                    # 租户调用编排（路由、限流、追踪）
│   ├── mcp/                                       # MCP 适配层（工具处理 & schema）
│   └── instrumentation/                           # 统一指标、追踪、审计封装
├── transport/
│   ├── http/admin/integration_gateway/            # 管理端 Gin Handler + DTO
│   ├── http/openapi/integration_gateway/          # 租户侧 HTTP Handler + DTO
│   └── grpc/integration_gateway/                  # gRPC Service 实现
├── server/mcp/tools/integration_gateway/          # MCP 工具定义与注册
└── transport/http/admin/routes.go                 # 注入新增路由（同时更新 openapi 汇总）

pkg/
└── corex/db/persistence/
    ├── model/integration_gateway/                 # GORM 模型
    └── repository/integration_gateway/            # 仓储封装

cmd/database/migrate.go                            # 注册 AutoMigrate 钩子
config/                                            # 默认值与 YAML Schema 增补
tests/contract/integration_gateway/                # HTTP/gRPC/MCP 合同测试
tests/integration/integration_gateway/             # 端到端调用链测试
```

**Structure Decision**: 采用 CoreX 模块标准分层（service + transport + repository），以共享 `shared.Deps` 注入依赖；HTTP、gRPC、MCP 三入口复用同一 Service 层，实现 Constitution 对多传输协议的一致性要求。

## Complexity Tracking

（当前无额外复杂度豁免需求。）
