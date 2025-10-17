# Implementation Plan: Capability Registry & Router

**Branch**: `003-capability-registry-router` | **Date**: 2025-10-15 | **Spec**: `specs/003-capability-registry-router/spec.md`
**Input**: Feature specification from `/specs/003-capability-registry-router/spec.md`

## Summary

实现一套多租户的能力注册中心与路由服务：Registry 提供能力快照、健康策略与事件推送，Router 根据权重、健康度和租户策略在 HTTP/gRPC 适配器之间选路，并在 500ms 内完成降级。方案依托 Postgres 记录版本化快照、Redis 缓存热快照与权重、EventBus 分发增量事件，并通过乐观并发控制与 2 分钟客户端缓存 TTL 确保一致性；同时提供 sandbox 模拟接口与跨区域同步机制，支持策略演练与多集群容灾。

## Technical Context

**Language/Version**: Go 1.24  
**Primary Dependencies**: gRPC + buf、Gin HTTP 栈、OpenTelemetry、Kafka/NATS EventBus、Redis、Postgres  
**Storage**: Postgres (持久化注册 & 审计)、Redis (Discovery 缓存)、Kafka/NATS (事件广播)  
**Testing**: Go `testing` + testify、gRPC 交互用 buf generated stubs、HTTP contract tests via resty  
**Target Platform**: Linux 容器 (Kubernetes)  
**Project Type**: CoreX 后端服务模块  
**Performance Goals**: Registry 99% 请求 ≤150ms；Router 降级耗时 ≤500ms；事件推送 95% ≤30s；客户端缓存命中 ≥80%  
**Constraints**: 多租户隔离、审计留痕、默认冷却 60s、缓存 TTL 120s、使用统一拦截器链 (auth/tenant/logging/recovery)、sandbox 接口隔离权限、跨区域一致性  
**Scale/Scope**: 数百租户、每租户上百能力、每能力最多 10 个适配器，Router QPS 1000+

## Constitution Check

| Gate | Status | Notes |
|------|--------|-------|
| HTTP_PRESENT | ✅ | 提供 `contracts/http-openapi.yaml`，规划 `internal/transport/http/admin/capability_registry` |
| GRPC_PRESENT | ✅ | 提供 `contracts/capability-registry.proto`，规划 `internal/transport/grpc/capability_registry` |
| PROTOBUF_DEFINED | ✅ | 设计 proto 包含 go_package，待后续同步至 `api/grpc/contracts` |
| SERVER_DEFINED | ✅ | 计划在全局 gRPC server 中注册并复用拦截器链 |
| MAKE_TARGETS | ✅ | 将复用既有 `proto-gen/proto-lint/proto-clean` 目标，不新增例外 |

（Phase 1 设计完成后复核，若实现阶段有变动需重新评估。）

## Project Structure

### Documentation (this feature)

```
specs/003-capability-registry-router/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── capability-registry.proto
└── tasks.md          # 由 /speckit.tasks 生成
```

### Source Code (repository root)

```text
api/
└── grpc/
    └── contracts/powerx/capability/registry/v1/   # proto 落地位置

internal/
├── service/capability_registry/                   # 领域服务（含注册、路由、sandbox、健康、缓存逻辑）
│   ├── domain/                                    # 常量、枚举、策略定义
│   ├── registry/                                  # Registry 读写、审计与事件发布
│   ├── router/                                    # 路由策略执行与指标输出
│   ├── sandbox/                                   # 策略演练接口与模拟调度
│   ├── health/                                    # 主动/被动健康检查 orchestrator
│   └── discovery/                                 # 客户端快照缓存、跨区域同步协调
├── transport/
│   ├── http/admin/capability_registry/            # REST Handler (Gin，包括 sandbox 路径)
│   └── grpc/capability_registry/                  # gRPC Service 实现（含 sandbox RPC）
└── infra/
    ├── cache/discovery/                           # Redis 存取封装（可复用的缓存适配层）
    └── replication/capability_registry/           # 跨区域/多集群同步辅助组件

pkg/corex/db/persistence/
├── model/                                         # 新增模型定义
└── repository/capability_registry/                # Postgres 访问层

cmd/agent/                                         # Router/Registry 启动 wiring (延用现有入口)
```

**Structure Decision**: 采用 CoreX 模块风格，在 `internal` 分层内分别落地 service/repository/transport，保持 HTTP + gRPC 双通道，实现与 Constitution 指南一致。

## Complexity Tracking

（当前无需额外复杂度豁免。）
