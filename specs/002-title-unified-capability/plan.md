# Implementation Plan: Unified Capability Contracts & Transport Adapters

**Branch**: `002-title-unified-capability` | **Date**: 2025-10-13 | **Spec**: `specs/002-title-unified-capability/spec.md`
**Input**: Feature specification from `/specs/002-title-unified-capability/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

统一交付能力契约模型与传输适配接口：为所有能力定义单一的契约 schema、版本治理策略、错误分类，以及统一的 HTTP/gRPC/MCP 适配器抽象，确保一次声明即可被多协议调用并满足合规观测要求。

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: Go 1.24  
**Primary Dependencies**: `github.com/gin-gonic/gin`, `google.golang.org/grpc`, `github.com/bufbuild/buf` toolchain, `gorm.io/gorm`  
**Client SDK**: 复用现有 PowerX gRPC SDK（现成生成的客户端封装直接消费 CapabilityRegistryService）  
**Storage**: Postgres（CoreX 多租户实例 + `gorm.io/gorm`）  
**Testing**: Go `testing` 框架（`go test ./...`），需补充契约校验与适配器一致性测试  
**Target Platform**: Linux/Kubernetes 上的 PowerX CoreX 后端服务  
**Project Type**: CoreX 后端模块（双通道 HTTP/gRPC）  
**Performance Goals**: 契约发布 ≤10 分钟；协议切换保持契约一致性 ≥90%；API p95 <200ms（宪法基线）  
**Constraints**: 必须具备双传输通道、统一错误分类、审计与观测指标；跨租户隔离；需对接 IAM/Tool Grant 校验  
**Scale/Scope**: ≥100 能力 & 10+ 租户并发，单能力支持 2-3 版本共存，峰值查询 ~1k RPS

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- HTTP_PRESENT：PASS — `specs/002-title-unified-capability/contracts/capability-http-openapi.yaml` 提供 REST 合同，计划在 `internal/transport/http/admin/capability` 实现路由与 Handler  
- GRPC_PRESENT：PASS — `specs/002-title-unified-capability/contracts/capability-grpc.proto` 定义 gRPC 服务，遵循 `@dev-crud-grpc`  
- PROTOBUF_DEFINED：PASS — Proto 使用 `powerx/capability/v1` 命名，计划同步更新 `api/grpc/contracts/buf.yaml` 与 `buf.gen.yaml`  
- SERVER_DEFINED：PASS — gRPC Handler 将注册于 `internal/server/grpc/server.go`，沿用 auth/tenant/logging/recovery 拦截器链  
- MAKE_TARGETS：PASS — `make proto-gen`, `make proto-lint`, `make proto-clean` 将纳入新契约生成与清理流程

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
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->

```
specs/002-title-unified-capability/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    ├── capability-http-openapi.yaml
    ├── capability-grpc.proto
    └── transport-adapter-interface.md
```

### Source Code（按宪法约束）

```
api/grpc/contracts/powerx/capability/v1/
├── capability_contract.proto
├── capability_version_policy.proto
└── transport_adapter.proto

api/grpc/contracts/buf.yaml           # 更新 service 目录引用
api/grpc/contracts/buf.gen.yaml       # 确保 go_package_prefix、out、paths 一致

api/grpc/gen/go/powerx/capability/v1/ # 生成产物（buf）

internal/service/capability/
├── contract_service.go
├── version_policy_service.go
└── adapter_service.go

internal/transport/http/admin/capability/
├── api.go
├── contract_handler.go
└── router.go

internal/transport/grpc/capability/
├── contract_handler.go
└── adapter_handler.go

pkg/corex/db/persistence/model/capability/
├── capability_contract_gorm.go
├── capability_version_policy_gorm.go
└── transport_profile_gorm.go

internal/contract/capability/
└── validation.go                      # 契约校验逻辑（schema/error taxonomy）

internal/server/grpc/server.go         # 注册 Capability gRPC 服务

cmd/app/main.go                        # 挂载 HTTP 路由
```

**Structure Decision**: CoreX 后端模块，新增 `capability` 域覆盖契约、版本策略、传输适配；遵循现有 `internal/service/*` 与 `internal/transport/(http|grpc)` 分层，并在 `api/grpc/contracts/powerx` 下定义权威 Proto；契约查询客户端复用 PowerX 既有 gRPC SDK，仅在实现阶段验证生成的 stub 与新服务契合，无需新增 SDK 目录。

## Ruleset Alignment

- **@dev-crud-http**：OpenAPI 契约覆写管理端 REST 接口；按照规则集，Handler 将放置于 `internal/transport/http/admin/capability`，并通过 `internal/http/router.go` 注册；DTO 校验与响应结构复用现有 `pkg/dto` 抽象。  
- **@dev-crud-grpc**：`capability-grpc.proto` 位于 `api/grpc/contracts/powerx/capability/v1`，`buf` 生成到 `api/grpc/gen/go`；服务实现位于 `internal/transport/grpc/capability`，通过全局 gRPC server 注入依赖。  
- **Observability & Security**：Adapter 接口文档（`transport-adapter-interface.md`）描述 trace/metrics/audit 与 Scope/Tool Grant 校验流程，满足宪法 Observability 与零信任要求。

## Complexity Tracking

当前方案未引入超出宪法要求的额外复杂度，暂无需记录的豁免项。
