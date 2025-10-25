# Implementation Plan: EventBus & Message Fabric

**Branch**: `004-eventbus-message-fabric` | **Date**: 2025-10-17 | **Spec**: [`spec.md`](spec.md)
**Input**: Feature specification from `/specs/004-eventbus-message-fabric/spec.md`

## Summary

统一构建事件骨干，涵盖主题目录、ACL、发布/订阅合同、重试与死信处理、以及回放能力。方案基于现有 `pkg/event_bus` 能力扩展：新增主题与权限元数据持久层（Postgres）、路由/订阅控制服务、gRPC 流式投递与管理 REST API，并通过 Redis 支撑短期幂等窗口、延迟队列，实现默认至少一次投递、最多五次重试、死信落地 Postgres 的可靠语义。

## Technical Context

**Language/Version**: Go 1.24  
**Primary Dependencies**: `google.golang.org/grpc`, Gin HTTP（既有 admin API）、`pkg/event_bus`（现有实现）、Redis 客户端 `github.com/redis/go-redis/v9`, GORM + PostgreSQL, OpenTelemetry SDK  
**Storage**: PostgreSQL（主题目录、ACL、DLQ、回放任务）、Redis（重试/幂等窗口、订阅游标缓存）  
**Testing**: `go test`（单元/集成）、`internal/tests/...` 合同测试、基准测试覆盖重试/投递性能  
**Target Platform**: Linux/Kubernetes 后端服务（CoreX 运行时）  
**Project Type**: 多模块后端（CoreX 服务 + Admin API + gRPC 服务）  
**Performance Goals**: 发布路径 P95 ≤100ms、重试附加延迟均值 ≤50ms / P99 ≤200ms、投递成功率 ≥99.9%  
**Constraints**: 多租户隔离、默认 At-Least-Once、gRPC 长连接推送、死信持久化 Postgres、事件载荷默认 JSON（Topic 可覆盖）  
**Scale/Scope**: 5000 msg/s 峰值发布、全球租户、回放窗口 24h、每租户主题上限 200

## Constitution Check

- ✅ **Article 0 CoreX Module**：事件骨干属于 CoreX，落地在 `internal/service/...` 与 `internal/transport/...`，不使用插件装载。  
- ✅ **Article II Spec-Driven**：已完成 `spec.md` + Clarifications，本计划严格来源规格。  
- ✅ **Article III Multi-Tenant**：Topic/Acl/DLQ 全部带 `tenant_id`，ACL 校验与审计覆盖所有调用。  
- ✅ **Article V Observability**：计划纳入指标（投递成功率、重试次数、DLQ 积压）、trace_id/tenant_id 日志与 OTel。  
全部 GATE 条件满足，可进入 Phase 0/Phase 1。

## Project Structure

### Documentation (this feature)

```
specs/004-eventbus-message-fabric/
├── plan.md          # 当前实现计划
├── research.md      # Phase 0 调研结论
├── data-model.md    # Phase 1 领域/数据模型
├── quickstart.md    # Phase 1 快速体验脚本与步骤
├── contracts/       # Phase 1 API/消息合同（REST + gRPC + Schema）
└── tasks.md         # 后续 /speckit.tasks 生成
```

### Source Code（按域分层）

```
pkg/
└── event_bus/                      # 扩展总线接口、重试/订阅抽象

pkg/corex/db/persistence/
├── model/event_fabric/             # TopicDefinition、AclBinding、DlqMessage 等
└── repository/event_fabric/        # 元数据与查询仓储

internal/service/event_fabric/
├── directory/                      # 主题生命周期 & 查询
├── acl/                            # 授权/审计校验
├── delivery/                       # 发布/订阅、重试、回放 orchestrator
├── dlq/                            # 死信处理与补偿
└── audit/                          # 事件与安全审计扩展

internal/transport/http/admin/event_fabric/
├── directory_handler.go            # 主题管理 REST
├── acl_handler.go                  # ACL 管理 REST
├── dlq_handler.go                  # 死信查询/补偿 REST
└── routes.go                       # Admin 路由注册

internal/transport/grpc/event_fabric/
├── publisher_server.go             # Publish / Ack RPC
└── subscriber_server.go            # gRPC 流式订阅

internal/app/shared/deps.go         # 依赖注入与默认总线实例装配

internal/tests/event_fabric/
├── contract/                       # HTTP/gRPC 合同测试
├── integration/                    # 重试 + DLQ 集成测试
└── perf/                           # 投递/回放基准
```

**Structure Decision**: 采用 CoreX 既定分层（model → repository → service → transport）。新增 `event_fabric` 域，复用现有 `pkg/event_bus` 并扩展接口；所有接口在 Admin HTTP 与 gRPC 服务下注册；测试沿用 `internal/tests/<domain>/...` 结构。

## Complexity Tracking

无额外超出宪章要求的复杂度增加，暂无需记录。
