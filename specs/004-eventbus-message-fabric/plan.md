# Implementation Plan: EventBus & Message Fabric

**Branch**: `004-eventbus-message-fabric` | **Date**: 2025-10-17 | **Spec**: [`spec.md`](spec.md)
**Input**: Feature specification from `/specs/004-eventbus-message-fabric/spec.md`

## Summary

统一构建事件骨干，涵盖主题目录、ACL、发布/订阅合同、重试与死信处理、以及回放能力。方案基于现有 `pkg/event_bus` 能力扩展：新增主题与权限元数据持久层（Postgres）、路由/订阅控制服务、gRPC 流式投递与管理 REST API，并通过 Redis 支撑短期幂等窗口、延迟队列，实现默认至少一次投递、最多五次重试、死信落地 Postgres 的可靠语义。

## Technical Context

**Language/Version**: Go 1.26.7
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
- ✅ **Article III Multi-Tenant**：Topic/Acl/DLQ 全部带 `tenant_uuid`，ACL 校验与审计覆盖所有调用。  
- ✅ **Article V Observability**：计划纳入指标（投递成功率、重试次数、DLQ 积压）、trace_id/tenant_uuid 日志与 OTel。  
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

## Incremental Plan: Queue Driver Strategy (Phase 9)

### Goal

将任务消费主路径收敛到 Redis，避免数据库高频轮询；并预留 Kafka / RabbitMQ / NATS 驱动扩展能力，数据库仅作为 fallback。

### Scope & Order

1. **T065-T066（第一优先）**：抽象统一任务驱动接口并落地 Redis 阻塞消费默认路径。
2. **T069（第二优先）**：将数据库轮询下沉为 fallback，仅在主驱动不可用时启用并打降级告警。
3. **T067-T068（第三优先）**：接入 Kafka、RabbitMQ、NATS 驱动适配层。
4. **T070（收尾）**：补齐规范与联调文档，冻结切换/降级检查单。

### Design Constraints

- 默认 driver 必须是 Redis（与 `queue.driver=redis` 对齐）。
- 任务消费优先阻塞读取（如 BRPOP/XREADGROUP），避免短周期空转轮询。
- fallback 启用时必须输出统一告警字段（driver、tenant、reason、trace_id）。
- 不改变既有 ACL、审计、topic 契约，仅替换消费驱动路径。

### Validation Targets

- 空闲 1 分钟内数据库不出现高频重复轮询 SQL（仅保留低频健康查询）。
- Redis 主路径下任务投递/消费链路保持 ACK/NACK、重试、DLQ 语义一致。
- 驱动切换（redis -> fallback）时可在日志与指标中明确观测。


## Incremental Plan: Admin UI Operations Console（Phase 10）

### Goal

补齐 Event Fabric 在 Admin UI 的运维可视化与调试入口，覆盖统一任务队列主路径（含 authorization challenge timeout 任务）的可观测与手动处置。

### Scope & Order

1. **后端可观测扩展**：在 `/admin/event-fabric/overview` 增加任务队列维度（queue/deferred/processing/inflight）统计。
2. **前端设置页增强**：在 `web-admin/app/pages/settings/event-fabric.vue` 增加“任务队列”分区与筛选项（subscriber/topic/tenant）。
3. **运维动作闭环**：提供“重试/清理/刷新”动作入口，并与现有 DLQ Replay 联动。
4. **联调脚本与验收**：补齐 `docs/guides/event_fabric/operations.md` API 调试手册与 UI 验收清单。

### Design Constraints

- 不新增独立消费机制，所有展示与操作必须围绕统一 `TaskDriver`/Event Fabric 服务。
- UI 展示应与后端字段一一对应，避免二次计算导致口径漂移。
- 关键动作（重试/清理）必须保留 audit 字段：`operator_id`、`tenant_uuid`、`trace_id`。
- 页面权限沿用 Admin Root 可见策略，不扩大 tenant 侧权限面。

### Validation Targets

- 页面可在 5 秒内刷新并反映队列状态变化。
- Challenge 超时任务在 UI 可见，并可通过 API 与 Redis 侧数据交叉验证。
- 出现异常时可从 UI 跳转到 DLQ/Replay 处置，不需要手工 SQL 排查。


## Incremental Plan: Cron Scheduled Tasks（Phase 11）

### Goal

在统一 Event Fabric/TaskDriver 框架内提供通用 Cron 任务能力，满足智能体“定时触发”需求，同时保持单一投递与消费机制。

### Scope & Order

1. **任务模型与存储**：新增 `scheduled_tasks` 与执行记录模型（`next_run_at`,`last_run_at`,`misfire_policy`）。
2. **Cron 计算与投递**：调度器仅负责计算下次触发并投递标准 Task Event。
3. **运维接口与页面联动**：提供任务 CRUD/启停/手动触发/执行记录查询，并接入 Admin UI。
4. **可靠性闭环**：Cron 触发后的执行统一走 Retry/DLQ/Replay，不新增私有补偿器。

### Design Constraints

- 不引入新的消费主路径；不得回退为数据库高频轮询。
- Cron 触发事件必须带标准 trace 与 tenant 字段，租户仅从 JWT claims 解析。
- `run-now`、`enable/disable`、`reschedule` 均需审计落库。
- Misfire 策略至少支持：`skip`、`fire_now`、`catch_up`。

### Validation Targets

- 可创建 Cron 任务并正确计算 `next_run_at`。
- 到点后事件入队并可在 Task Queue 与日志中观察。
- 失败执行进入统一重试/DLQ，运维台可重放与追踪。
- 与现有 Event Fabric ACL/审计/指标口径保持一致。

## Phase 12: Topic Governance Unified Refactor（新增）

**Purpose**: 彻底收敛 Topic 机制，统一到“`event_topics` 注册治理 + JWT-only 租户 + ACL + 缓存”，移除 WS 内存动态注册与租户实例化 Topic 双轨。

### Stage A（先稳定）

- 统一 topic 入参语义：`namespace.name`，不再要求 tenant 前缀。
- 统一错误口径：`topic not found` 为 4xx 业务错误，不返回 500。
- 运行时解析链路固定：`tenant -> global -> system`。

### Stage B（注册治理 + 缓存）

- 收敛 `event_topics` 作为 Topic 唯一事实源（DB）。
- 引入 Redis cache：
  - `event:topic:resolve:{topic_key}`
  - `event:acl:{scope}:{topic_id}:{principal}:{action}`
- 采用 cache-aside（写 DB 后删缓存），TTL 60~300s。

### Stage C（WS/Task 统一）

- WS publish/subscribe 改为查统一 `event_topics` + ACL。
- 下线 WS 内存动态注册（`publishDynamicTopics`）。
- `/internal/ws-bus/grant` 改为注册中心入口（或仅做权限绑定动作）。

### Stage D（删除旧链路）

- 下线 WS 内存动态注册与旧兜底逻辑。
- 清理目录写接口中的“租户复制 Topic 实例”行为。
- 保留 `event_topics`，不再执行删表路线。

### Validation Gate

1. API/WS/Task topic 语义一致（均不带 tenant 前缀）。
2. tenant 仅来自 JWT。
3. 运行时无高频 `event_topics` 兜底查询噪音。
4. 关键接口（publish/replay/ws subscribe）在同一套 ACL 口径下通过。

## Incremental Plan: Event ACL Governance UI（Phase 13）

### Goal

在“系统设置”中提供 Topic-Role ACL 治理页，统一权限配置入口，并与监控中心形成“配置态/运行态”分层。

### Scope & Order

1. **信息架构**：在 `系统设置` 下新增 `事件权限（Event ACL）` 导航与页面骨架。
2. **ACL 矩阵视图**：按 Topic 展示角色授权状态，支持批量授予/撤销 `publish/subscribe/replay`。
3. **共享 Topic 支持**：治理页可直接配置 `global/system` Topic 权限，避免租户逐个建 Topic。
4. **监控联动跳转**：`/settings/monitor?tab=event-fabric` 增加“管理权限”跳转至 ACL 页。
5. **审计与验收**：授权变更审计落库，补齐 API/UI 验证脚本。

### Design Constraints

- 不在监控中心直接承载高风险 ACL 编辑动作。
- UI 交互与后端契约保持 1:1，不在前端二次拼接 tenant/topic 真相。
- ACL 写接口需兼容共享 Topic（`tenant -> global -> system` 解析链路）。
- 所有变更动作必须包含 `operator_id`、`tenant_uuid`、`trace_id`。

### Validation Targets

- 新租户首次进入无需建 Topic 即可完成角色授权。
- ACL 改动后可立即影响 WS publish/subscribe 与 replay 行为。
- 监控中心与系统设置页面的 Topic 可见性与授权结果一致。
