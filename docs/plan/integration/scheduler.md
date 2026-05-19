# Scheduler（统一调度器）

> 状态：规划契约，尚未完整落地为可供插件调用的生产服务。  
> 已落地的 `/admin/event-fabric/cron/jobs` 只支撑平台内置 Schedule 配置运维，不等同于本文定义的插件通用 SchedulerService。

## 1. 目标

1. 给 PowerX 底座、插件、插件之间提供统一的定时/计划机制。
2. 通过 `powerx.scheduler.v1.SchedulerService` 和 `com.corex.scheduler.jobs` 暴露创建、更新、暂停、恢复、触发、查询调度任务的能力。
3. 支持 `once`、`interval`、`cron` 三类调度。
4. Scheduler 只负责“何时触发”，业务执行统一通过 Event Bus / TaskBus / Framework handler 完成。
5. 插件业务代码只依赖 PowerXPlugin Framework scheduler facade，不直接调用底座 Admin Cron 接口，不自行维护本地内存 timer。

## 2. 当前状态

1. 已有 gRPC proto：`backend/api/grpc/contracts/powerx/scheduler/v1/scheduler.proto`。
2. 已有生成代码和 capability 记录：`com.corex.grpc.scheduler.schedulerservice.*`。
3. 当前仓库未发现真实 `SchedulerServiceServer` 注册与完整业务实现。
4. 已实现的 Event Fabric Cron 能力主要服务底座内部 worker，例如：
   - `event_fabric.retry_dispatch`
   - `event_fabric.authorization_challenge_timeout`
5. `/admin/event-fabric/cron/jobs` 只能作为底座内部 Cron 运维接口，不能作为插件通用 Scheduler API。

## 3. 职责边界

### 3.1 PowerX 底座

PowerX 底座负责生产环境可靠调度：

1. 持久化 job 与 run 记录。
2. 校验租户、owner、schedule 表达式和权限。
3. 计算 `next_run_at`。
4. 到点后发布标准调度触发事件。
5. 提供分布式锁或等效防重机制。
6. 提供至少一次触发语义。
7. 提供 pause、resume、trigger、get、list 等管理能力。
8. 写入审计、指标与结构化日志。

### 3.2 PowerXPlugin Framework

Framework 负责对插件屏蔽运行模式差异：

1. `host` 模式调用 PowerX 底座 `SchedulerService`。
2. `local` 模式使用 framework 本地 provider。
3. `dual` 模式仅用于迁移验证，不作为长期默认。
4. 到点事件由 framework 接收后分发到插件注册的 handler。
5. 业务插件只调用 `scheduler.CreateJob(...)` 和 `scheduler.RegisterHandler(...)` 等 framework 接口。

### 3.3 业务插件

业务插件负责声明计划任务和处理业务动作：

1. 创建 job 时设置 `owner_type=plugin`、`owner_id=<plugin_id>`。
2. 在 payload 中携带 `business_action` 和业务主键。
3. 注册 handler 处理 `business_action`。
4. handler 必须幂等处理，因为底座提供至少一次触发语义。
5. 不直接调用 `/admin/event-fabric/cron/jobs`。
6. 不在业务代码里写 host/local 分支。

## 4. 统一能力模型

Capability：`com.corex.scheduler.jobs`

建议 scope：

1. `scheduler.job.manage`：创建、更新、暂停、恢复、删除。
2. `scheduler.job.run`：手动触发。
3. `scheduler.job.read`：查询 job 和 run 状态。

现有 `workflow.scheduler` 只适合 workflow scheduler 场景，插件通用调度需要独立 scheduler scope，避免把 workflow 能力误复用成通用插件调度能力。

## 5. 数据模型

建议新增独立表，不直接复用平台内置 Schedule 运维接口作为插件公共契约。

```text
scheduler_jobs:
  job_id uuid primary key
  tenant_uuid uuid not null
  owner_type text not null          # core | plugin
  owner_id text not null            # service_name | plugin_id
  name text not null
  schedule_type text not null       # once | interval | cron
  schedule_expr text not null       # RFC3339 | duration | cron expr
  timezone text not null default 'UTC'
  topic text not null               # powerx.runtime.scheduler.triggered.v1
  payload_json jsonb not null
  status text not null              # active | paused | deleted
  next_run_at timestamptz
  last_run_at timestamptz
  misfire_policy text not null      # skip | run_catchup
  overlap_policy text not null      # skip | queue | parallel
  retry_policy_json jsonb
  idempotency_key text
  created_by text
  created_at timestamptz not null
  updated_at timestamptz not null

scheduler_job_runs:
  run_id uuid primary key
  job_id uuid not null
  tenant_uuid uuid not null
  owner_type text not null
  owner_id text not null
  trigger_source text not null      # once | interval | cron | manual | retry
  scheduled_at timestamptz
  fired_at timestamptz
  status text not null              # triggered | skipped | failed
  event_id text
  trace_id text
  error_code text
  error_message text
  created_at timestamptz not null
```

约束：

1. `tenant_uuid + owner_type + owner_id + name` 必须唯一。
2. `owner_type=plugin` 时 `owner_id` 必须是插件 ID。
3. `topic` 首期固定为 `powerx.runtime.scheduler.triggered.v1`，不允许业务插件自定义任意 topic。
4. `tenant_uuid` 只能来自 token claims 或受信任宿主上下文，不接受未授权覆盖。

## 6. 调度类型

1. `once`：`schedule_expr` 必须是 RFC3339 时间。
2. `interval`：`schedule_expr` 必须是 Go duration 风格，例如 `5m`、`2h`。
3. `cron`：`schedule_expr` 支持标准 5/6 段 cron。
4. `timezone` 默认 `UTC`，允许显式设置 `Asia/Shanghai` 等 IANA timezone。

非法表达式必须 fail-fast 返回 `scheduler.invalid_schedule`，不得静默修正。

## 7. gRPC 契约

目标服务：`powerx.scheduler.v1.SchedulerService`

当前 proto 路径：`backend/api/grpc/contracts/powerx/scheduler/v1/scheduler.proto`

方法：

```text
CreateJob
UpdateJob
PauseJob
ResumeJob
TriggerJob
GetJob
ListJobs
```

实现要求：

1. 服务必须在 gRPC server 注册，不能只存在 generated 代码。
2. 所有方法必须经过统一 authn/authz interceptor。
3. `tenant_uuid` 与 token claims 不一致时必须拒绝。
4. `owner_type=plugin` 时必须校验调用方插件身份与 `owner_id` 一致，或具备显式管理授权。

## 8. HTTP 管理 API

HTTP API 只作为管理端与调试入口，不是 framework host provider 的首选调用方式。

```text
POST   /api/v1/admin/scheduler/jobs
PATCH  /api/v1/admin/scheduler/jobs/:job_id
POST   /api/v1/admin/scheduler/jobs/:job_id/pause
POST   /api/v1/admin/scheduler/jobs/:job_id/resume
POST   /api/v1/admin/scheduler/jobs/:job_id/trigger
GET    /api/v1/admin/scheduler/jobs
GET    /api/v1/admin/scheduler/jobs/:job_id
GET    /api/v1/admin/scheduler/jobs/:job_id/runs
```

要求：

1. Admin API 必须写审计。
2. Admin API 不替代插件 framework API。
3. `/admin/event-fabric/cron/jobs` 继续只用于 Event Fabric 内置 worker 运维。

## 9. 标准触发事件

统一 topic：

```text
powerx.runtime.scheduler.triggered.v1
```

旧草案名称 `scheduler.job.triggered` 不再作为新实现推荐 topic。

Payload 最小结构：

```json
{
  "job_id": "uuid",
  "job_name": "sample_progress_50",
  "owner_type": "plugin",
  "owner_id": "com.powerx.plugins.ai-craft",
  "tenant_uuid": "uuid",
  "trigger_source": "once",
  "scheduled_at": "2026-05-16T10:00:00+08:00",
  "fired_at": "2026-05-16T10:00:00+08:00",
  "trace_id": "trace-id",
  "idempotency_key": "tenant:plugin:job:business-key",
  "business_action": "sample_progress_50",
  "payload": {}
}
```

发布要求：

1. 事件必须带 `tenant_uuid`、`owner_id`、`job_id`、`trace_id`。
2. 事件发布失败必须记录 run 失败，并按 retry policy 处理。
3. 消费方 ack/nack 归属 Event Bus / TaskBus，不由 Scheduler 直接调用业务代码。

## 10. Framework 对接方式

业务插件侧目标代码：

```go
scheduler.CreateJob(ctx, scheduler.JobSpec{
    TenantUUID:   tenantUUID,
    OwnerType:    "plugin",
    OwnerID:      "com.powerx.plugins.ai-craft",
    Name:         "sample_progress_50",
    ScheduleType: "once",
    ScheduleExpr: eta50.Format(time.RFC3339),
    Topic:        "powerx.runtime.scheduler.triggered.v1",
    Payload: map[string]any{
        "business_action": "sample_progress_50",
        "design_task_id":  designTaskID,
        "order_id":        orderID,
    },
})
```

Handler 注册：

```go
scheduler.RegisterHandler("sample_progress_50", func(ctx context.Context, job scheduler.TriggeredJob) error {
    return sampleService.HandleProgress50(ctx, job.Payload)
})
```

要求：

1. `host` provider 调底座 SchedulerService。
2. `local` provider 在 framework 本地实现相同语义。
3. host provider 失败时必须返回明确错误，不得自动降级到 local provider。生产环境静默降级会造成重复触发、漏触发和审计断链。
4. `dual` provider 只能用于迁移验证，必须显式开启。

## 11. 可靠性与一致性

1. Scheduler 提供至少一次触发。
2. Consumer 必须基于 `idempotency_key` 或业务主键幂等。
3. `overlap_policy=skip` 时，上次未完成不得并发触发同 job。
4. `misfire_policy=skip` 时，错过窗口直接记录 skipped run。
5. `misfire_policy=run_catchup` 时，只补跑一次，不追赶多次历史窗口。
6. 分布式部署必须使用 Redis/DB 锁或同等机制防重复触发。

## 12. 观测与审计

日志字段必须包含：

1. `job_id`
2. `job_name`
3. `tenant_uuid`
4. `owner_type`
5. `owner_id`
6. `schedule_type`
7. `schedule_expr`
8. `trigger_source`
9. `trace_id`
10. `event_id`

指标建议：

1. `scheduler_trigger_total`
2. `scheduler_trigger_failed_total`
3. `scheduler_misfire_total`
4. `scheduler_latency_ms`
5. `scheduler_active_jobs`

审计动作：

1. create
2. update
3. pause
4. resume
5. trigger
6. delete

## 13. 与现有 Event Fabric Cron 的关系

1. Event Fabric Cron 是当前已实现的底座内部调度能力。
2. 插件通用 SchedulerService 是面向插件和核心模块的公共调度契约。
3. 两者可以复用底层 Task/EventBus/Retry/DLQ 能力。
4. 两者不共享 Admin API 语义。
5. 不能把 `/admin/event-fabric/cron/jobs` 暴露为插件通用 scheduler。

## 14. 最小落地顺序

1. 补 `SchedulerService` server 实现与注册。
2. 补 `scheduler_jobs`、`scheduler_job_runs` 模型、迁移、repository。
3. 实现 Create/Update/Pause/Resume/Trigger/Get/List service。
4. 实现 due scanner/dispatcher，发布 `powerx.runtime.scheduler.triggered.v1`。
5. 接入 capability registry 与权限 scope。
6. 接入审计、指标、结构化日志。
7. 提供 framework host provider 联调用例。
8. 再让 AI Craft 等插件接入业务 job。

## 15. 非目标

1. 不扩展 `/admin/event-fabric/cron/jobs` 作为插件通用 Scheduler API。
2. 不让 Scheduler 直接调用插件业务函数。
3. 不允许 host provider 失败后静默切换 local provider。
4. 不在插件业务代码中维护本地内存 timer。
5. 不把 workflow scheduler scope 直接当作插件通用 scheduler scope。
