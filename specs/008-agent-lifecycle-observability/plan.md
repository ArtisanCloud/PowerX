# Implementation Plan: Agent Lifecycle & Observability

**Branch**: `008-agent-lifecycle-observability` | **Date**: 2025-10-22 | **Spec**: `specs/008-agent-lifecycle-observability/spec.md`
**Input**: Feature specification from `/specs/008-agent-lifecycle-observability/spec.md`

## Summary

交付统一的 Agent 生命周期治理：支持管理员注册与激活代理、运维按需启动/暂停/扩缩容、SRE 实时观测健康评分并接收告警，并提供可观测性订阅配置及退役后 13 个月的数据保留。新增覆盖 `UC-AGENT-REG-AUTO/TENANT/SHARE-001` 的插件自动注册、租户自助表单与跨租户共享流程，并把生命周期事件/告警桥接到 `SCN-AGENT-REACT-ORCH-001` 与 `SCN-AGENT-TASK-EXEC-001`。方案依托 CoreX Go 模块栈，复用 Postgres + GORM 存储代理档案与历史，使用 Redis 缓存实例容量，借助 EventBus 发布生命周期事件，统一通过 HTTP（Admin/OpenAPI）与 gRPC 控制面暴露操作，并串接 OpenTelemetry 指标模型、企业 IM 告警、沙箱验证及归档策略。

## Technical Context

**Language/Version**: Go 1.24  
**Primary Dependencies**: Gin HTTP 栈、grpc-go + buf 工具链、OpenTelemetry SDK、Redis（容量/健康缓存）、Postgres（CoreX GORM）、EventBus 抽象、企业 IM Webhook 适配、内部审计与限流组件  
**Storage**: Postgres（AgentProfile、LifecycleEvent、HealthSignalSnapshot）、Redis（实例容量与健康状态缓存）、Log/Trace 后端沿用现有基础设施  
**Testing**: Go `testing` + `testify`、HTTP Contract Tests（resty / httpexpect）、gRPC buf 生成桩、指标/告警模拟器（自研 fixture）、集成回归测试  
**Target Platform**: Linux 容器化（Kubernetes，与现有 CoreX 服务同部署域）  
**Project Type**: CoreX 后端服务模块  
**Performance Goals**: 生命周期指令 99% < 30s、健康异常 90% < 2 分钟告警、HTTP/gRPC 控制面 p95 < 200ms、告警信息 30s 内送达企业 IM 群组  
**Constraints**: 强制多租户隔离、RBAC + Tool Grant 校验、trace_id 全链路透传、容量阈值按实例数衡量、告警渠道为企业 IM、历史指标 + 审计保留 ≥ 13 个月  
**Scale/Scope**: 约 200 个代理、每代理 5~20 实例、日均生命周期指令 < 500、健康信号 1 分钟采集一次、保留 13 个月历史

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| HTTP_PRESENT | ✅ 将交付 `contracts/http-openapi.yaml` 并规划 `internal/transport/http/{admin,openapi}/agent` Handler |
| GRPC_PRESENT | ✅ 将交付 `contracts/agent_lifecycle.proto` 并实现于 `internal/transport/grpc/agentlifecycle` |
| PROTOBUF_DEFINED | ✅ Proto 落于 `api/grpc/contracts/powerx/agent/v1/agent_lifecycle.proto`，同步更新 buf 配置 |
| SERVER_DEFINED | ✅ 全局 gRPC Server (`internal/server/grpc/server.go`) 注册新 Service，复用现有拦截器链 |
| MAKE_TARGETS | ✅ 延续 `make proto-gen/proto-lint/proto-clean`，无需新增例外 |

## Project Structure

### Documentation (this feature)

```
specs/008-agent-lifecycle-observability/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── agent_lifecycle.proto
└── tasks.md        # /speckit.tasks 生成
```

### Source Code (repository root)

```
api/grpc/contracts/powerx/agent/v1/
└── agent_lifecycle.proto

internal/
├── service/agent_lifecycle/
│   ├── registry.go            # 注册/激活与依赖校验
│   ├── lifecycle.go           # start/stop/scale 状态机
│   ├── health.go              # 指标聚合与健康评分
│   ├── subscription.go        # 可观测性订阅配置与即时生效
│   ├── archive.go             # 退役数据保留与归档策略
│   └── instrumentation/       # 指标、追踪、审计封装
├── transport/
│   ├── http/admin/agent/      # 管理控制面 Handler + DTO（含订阅配置）
│   ├── http/openapi/agent/    # 运维/SRE 控制面 Handler
│   └── grpc/agentlifecycle/   # gRPC Service 实现（含订阅 RPC）
├── server/grpc/server.go      # 注册 AgentLifecycleService
└── notifications/im/          # 企业 IM 发送适配（复用或扩展现有模块）

pkg/corex/db/persistence/
├── model/agent/
│   ├── profile.go
│   ├── lifecycle_event.go
│   └── health_snapshot.go
└── repository/agent/
    ├── profile_repository.go
    ├── lifecycle_repository.go
    └── health_repository.go

tests/
├── contract/agent_lifecycle/      # HTTP/gRPC/订阅合同测试
├── integration/agent_lifecycle/
└── unit/agent_lifecycle/
```

**Structure Decision**: 采用 CoreX 模块分层（service + transport + repository），在同一服务目录聚合注册、生命周期、可观测逻辑；HTTP(Admin/OpenAPI) 与 gRPC Handler 共享 Service；通知适配单独目录方便复用，确保满足 Constitution 对多协议、buf 工具链与统一 Server 注册的要求。

## Complexity Tracking

（当前无额外复杂度豁免需求。）

## Use Case Addendum

为闭环先前未覆盖的 Use Case，本计划新增以下交付面：

- **插件自动注册（UC-AGENT-REG-AUTO-001）**：实现 manifest Webhook、签名/Schema 校验、IAM 策略绑定、沙箱执行与结果回写，并把成功/失败事件写入 Lifecycle/Audit。
- **租户自助 Agent（UC-AGENT-REG-TENANT-001）**：扩展 Tenant Agent Center API、策略冲突检测与审批编排，复用生命周期激活、审计与订阅能力，确保审批链路与运行态一致。
- **多租户共享（UC-AGENT-REG-SHARE-001）**：新增共享/撤销 API、Quota Provisioner、租户验证脚本与合规审计，输出共享事件供 StateBus 与 Compliance 消费。
- **ReAct & 任务执行桥接（SCN-AGENT-REACT-ORCH-001、SCN-AGENT-TASK-EXEC-001）**：统一生命周期事件 Schema、StateBus 推送与 Trace 查询接口，供 Thought/Action/Memory/Audit 以及 Plan/Coord/Recovery/Closure 阶段实时消费；同时暴露冻结/解冻、健康摘要与闭环报告 API。

对应的实现拆分已在 `tasks.md` 中新增 Phase 6~9 任务，确保测试、沙箱、审批与跨场景对齐均可追溯。
