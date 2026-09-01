# Implementation Plan: Workflow & Agent Orchestration

**Branch**: `006-title-workflow-agent` | **Date**: 2025-10-20 | **Spec**: `specs/006-workflow-and-agent/spec.md`
**Input**: Feature specification + research/data-model deliverables under `specs/006-workflow-and-agent/`

## Summary

设计目标是为 CoreX 提供工作流与多智能体编排能力：支持工作流定义的版本化发布、实例运行时的重试与补偿、Agent 工具授权校验，以及审计导出与观测指标。技术路线遵循研究结论——以 PostgreSQL/GORM 持久化工作流状态，Redis 实现调度与延迟队列，gRPC/HTTP 双协议暴露接口并复用现有 Tool Grant 安全栈；安全层面将通过 HTTP 与 gRPC 双协议的 JWT/JWKS + RBAC 契约测试与拦截器复用进行验证。

2026-07 Runtime Completion 更新：现有实现已覆盖定义、发布、实例、基础 step record、控制和导出骨架，但还未满足 native-agent 知识库增量迭代所需的完整 Workflow Runtime。补齐范围以 `docs/plan/ai_engineering/workflow/README.md` 为准，必须新增 WorkflowRunner、NodeAdapterRegistry、Node Catalog API、Human Review、Workflow Pack seed、Skill/Capability/Knowledge/Metadata adapters，并移除 Web Admin workflow mock 数据。

## Technical Context

**Language/Version**: Go 1.26.7（遵循仓库 go.mod）
**Primary Dependencies**: GORM、Redis（通过 go-redis 客户端）、OTEL SDK、现有 EventBus、Proto 生成工具链（buf + protoc）  
**Storage**: PostgreSQL（工作流定义/实例/步骤表），Redis Sorted Set/Stream（调度延迟队列），ClickHouse（审计投影）  
**Testing**: Go `testing` + testify（契约/集成/单元测试），`make unit-test`、`make test-all`  
**Target Platform**: Linux 服务（CoreX 后端集群），通过 CLI/CI 管线部署  
**Project Type**: 单体后端服务，扩展 CoreX 内核模块  
**Performance Goals**: Workflow API p95 < 200ms；调度器在 SLA 前触发重试；Agent 失败恢复 < 5 分钟（对齐 SC-002/SC-003）  
**Constraints**: 必须遵循 JWT/JWKS 认证、统一 RBAC、租户隔离；禁止创建插件路径（CoreX 模式）；所有新目录需含有效实现  
**Scale/Scope**: 初始目标 100 并发实例/租户、每实例 20 步骤，支持 90 天审计数据导出

## Constitution Check

| Gate | Status | 说明 |
|------|--------|------|
| HTTP_PRESENT | ✅ `specs/006-workflow-and-agent/contracts/http-openapi.yaml` 将定义 admin API；tasks Phase 1(T003) + Phase 3(T026,T036,T042) 实施 HTTP Handler，并在 plan 中明确路由位于 `internal/transport/http/admin/workflow/` |
| GRPC_PRESENT | ✅ gRPC 合同放置于 `api/grpc/contracts/powerx/workflow/v1/`，Task T002/T025/T035/T043 负责接口实现，服务注册在 `internal/server/grpc/server.go` |
| PROTOBUF_DEFINED | ✅ 需更新 `api/grpc/contracts/buf.yaml`、`buf.gen.yaml`，Task T001/T004 保证生成流程；plan 强制维护 `make proto-*` 目标 |
| SERVER_DEFINED | ✅ Workflow gRPC Handler 通过 `internal/server/grpc/server.go` 注册，复用统一拦截器；调度器由 `internal/service/workflow/scheduler.go` 管理，并在 `internal/app/shared/deps.go` 装配 |
| MAKE_TARGETS | ✅ Makefile 应补充 `proto-gen`, `proto-lint`, `proto-clean` 依赖 Workflow 合同；Task T001 指定更新 |

## Project Structure

### Documentation

```
specs/006-workflow-and-agent/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── http-openapi.yaml
└── tasks.md
```

### Source Code & Assets

```
api/grpc/contracts/powerx/workflow/v1/workflow.proto   # gRPC 服务合同
api/grpc/gen/go/powerx/workflow/v1/                    # 生成代码（T004）
internal/service/workflow/                             # 服务层：definition.go, instance.go, control.go, scheduler.go, compensation.go, reporting.go, validator.go, event_emitter.go
internal/service/workflow/runner.go                    # WorkflowRunner：租约、推进、状态收敛
internal/service/workflow/node_adapter.go              # NodeAdapterRegistry 和 adapter 接口
internal/service/workflow/node_catalog.go              # Builder/Runtime 共享节点目录
internal/service/workflow/human_review.go              # Human Review first-class task
internal/service/workflow/workflow_pack_seed.go        # Workflow Pack seed
internal/transport/grpc/workflow/                      # gRPC Handlers（definition/control/reporting）
internal/transport/http/admin/workflow/                # HTTP Handlers（definitions/instances/export/node-catalog/review/packs）
pkg/corex/db/persistence/model/workflow/               # 数据模型（definition/instance/step/compensation/assignment/event）
pkg/corex/db/persistence/repository/workflow/          # 仓储封装
backend/config/workflow_packs/                         # 内置 Workflow Pack seed
web-admin/app/pages/workflow/                          # Workflow Builder 和 Instance Monitor，必须接真实 Admin API
internal/app/shared/{deps.go,options.go}               # 依赖注入
internal/server/grpc/server.go                         # gRPC 注册入口
deploy/observability/workflow_dashboard.json           # 观测与告警模板
docs/runbooks/*.md                                     # 运维文档/发布说明
tests/contract/workflow/                               # gRPC + HTTP 契约测试
tests/integration/workflow/                            # 端到端场景验证
tests/unit/                                            # 核心工具与策略单测
```

**Structure Decision**: 采用 CoreX Module 目录规范，所有实现位于 `internal/service`, `internal/transport`, `pkg/corex/db/persistence` 等既有层级；禁止进入 `plugins/`。

## Complexity Tracking

无宪法例外需求，遵循标准 CRUD/service/ruleset。
