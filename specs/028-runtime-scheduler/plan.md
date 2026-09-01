# Runtime Scheduler Implementation Plan

## 目标

将已有 `powerx.scheduler.v1.SchedulerService` 契约落地为 PowerX Core 生产服务，并让插件通过 PowerXPlugin Framework scheduler facade 使用宿主调度能力。

## 技术栈

- Go 1.26.7
- Gin HTTP
- gRPC / Buf
- GORM
- PostgreSQL
- Redis 或 DB lock
- EventBus / TaskBus
- OpenTelemetry / Metrics / Audit

## 最小落地顺序

1. 实现 `SchedulerService` server 并注册到 gRPC server。
2. 新增 `scheduler_jobs`、`scheduler_job_runs` GORM 模型、迁移、repository。
3. 实现 service 层：Create、Update、Pause、Resume、Trigger、Get、List、ListRuns。
4. 实现 schedule expression 校验和 `next_run_at` 计算。
5. 实现 due scanner/dispatcher，发布 `powerx.runtime.scheduler.triggered.v1`。
6. 接入 Admin HTTP API：`/api/v1/admin/scheduler/jobs`。
7. 接入 Capability Registry：`com.corex.scheduler.jobs`。
8. 接入 authz：tenant 校验、plugin owner 校验、scope 校验。
9. 接入审计、指标、结构化日志。
10. 提供 Framework host provider 联调用例。
11. 再让 AI Craft 等业务插件迁移到 Framework scheduler facade。

## 路径决策

1. 管理端 HTTP 路径使用 `/api/v1/admin/scheduler/jobs`。
2. gRPC 权威服务使用 `powerx.scheduler.v1.SchedulerService`。
3. 插件业务不直接调用 `/admin/event-fabric/cron/jobs`。
4. Event Fabric Cron 继续只用于底座内部 worker 运维。

## 鉴权决策

1. Admin HTTP 使用管理端 JWT 与 RBAC。
2. Framework host provider 使用 STS/delegated bearer。
3. `owner_type=plugin` 时必须校验当前插件身份与 `owner_id` 一致。
4. 租户只能来自 token claims 或受信宿主上下文。

## 可靠性决策

1. Scheduler 提供至少一次触发。
2. 插件 handler 必须幂等。
3. 分布式 scanner 必须使用 Redis/DB lock 或状态 CAS。
4. misfire 和 overlap 必须记录 run，不得静默跳过。

## 风险

1. 与 Event Fabric Cron 概念混淆，导致插件误用内部运维接口。
2. host provider 静默降级 local 导致重复触发或审计断链。
3. scanner 多实例重复触发。
4. 插件自定义 topic 带来事件权限扩散。

## 回滚

1. 关闭 due scanner。
2. 保留 job/run 表用于审计。
3. Framework host provider 返回明确错误，插件不得自动本地补偿。
4. 不影响 Event Fabric Cron 既有内部 worker。
