# Scheduler（统一调度器）

## 目标
- 给 PowerX 底座、插件、插件之间提供**统一的定时/计划机制**。
- 通过 `com.corex.scheduler.jobs` 暴露能力：创建/更新/暂停/删除/触发调度任务。
- 与 Event Bus 协同：**到点触发 → 发布事件 → 订阅者执行**。

## 统一能力模型（Capability）
- `com.corex.scheduler.jobs`  
  - intents: `workflow.scheduler.invoke`（可扩展）
  - tool scopes: `workflow.scheduler`

## 支持的计划类型
- `cron`（标准 5/6 段 cron）
- `interval`（如 `5m`、`2h`）
- `once`（指定时间点）

## 核心设计
- **Job Registry（注册表）**：统一保存调度任务（DB）。
- **Planner**：解析 cron/interval，计算 next_run_at。
- **Dispatcher**：到点触发 → 发布事件到 Event Bus。
- **Worker**：消费“调度触发事件”，调用插件或内部执行器。
- **状态机**：scheduled → running → succeeded/failed → rescheduled（含重试）。

## 数据模型（草案）
```
scheduler_jobs:
  job_id (uuid)
  tenant_uuid
  owner_type (core/plugin)
  owner_id (service_name/plugin_id)
  name
  schedule_type (cron/interval/once)
  schedule_expr
  payload (json)
  status (active/paused/deleted)
  next_run_at
  last_run_at
  misfire_policy (skip/run_catchup)
  retry_policy (max, backoff)
  created_at/updated_at
```

## API 草案（HTTP）
```
POST /api/v1/admin/scheduler/jobs
  body:
    tenant_uuid: string
    owner_type: core|plugin
    owner_id: string
    name: string
    schedule_type: cron|interval|once
    schedule_expr: string
    payload: object
    misfire_policy: skip|run_catchup
    retry_policy:
      max_attempts: number
      backoff_seconds: number

PATCH /api/v1/admin/scheduler/jobs/:jobId
  body: (same as create, partial)

POST /api/v1/admin/scheduler/jobs/:jobId/pause
POST /api/v1/admin/scheduler/jobs/:jobId/resume
POST /api/v1/admin/scheduler/jobs/:jobId/trigger
GET  /api/v1/admin/scheduler/jobs
GET  /api/v1/admin/scheduler/jobs/:jobId
```

## 查询与分页（建议）
- `GET /api/v1/admin/scheduler/jobs`
  - `page`（默认 1）
  - `page_size`（默认 20，最大 200）
  - `status`（active/paused）
  - `owner_id`（可选）
  - `schedule_type`（可选）

## OpenAPI Schema（草案）
```yaml
paths:
  /api/v1/admin/scheduler/jobs:
    post:
      summary: Create scheduler job
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                tenant_uuid: { type: string }
                owner_type: { type: string, enum: [core, plugin] }
                owner_id: { type: string }
                name: { type: string }
                schedule_type: { type: string, enum: [cron, interval, once] }
                schedule_expr: { type: string }
                payload: { type: object }
                misfire_policy: { type: string, enum: [skip, run_catchup] }
                retry_policy:
                  type: object
                  properties:
                    max_attempts: { type: integer }
                    backoff_seconds: { type: integer }
              required: [tenant_uuid, owner_type, owner_id, schedule_type, schedule_expr]
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/OkResponse"
        "401":
          description: unauthorized
        "403":
          description: forbidden
        "500":
          description: internal_error
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
  schemas:
    ErrorResponse:
      type: object
      properties:
        code: { type: string }
        message: { type: string }
        detail: { type: string }
      required: [code, message]
    OkResponse:
      type: object
      properties:
        ok: { type: boolean }
        data: { type: object }
      required: [ok]
```

## API 草案（gRPC）
```
service SchedulerService {
  rpc CreateJob(SchedulerJobCreateRequest) returns (SchedulerJobReply);
  rpc UpdateJob(SchedulerJobUpdateRequest) returns (SchedulerJobReply);
  rpc PauseJob(SchedulerJobControlRequest) returns (SchedulerJobReply);
  rpc ResumeJob(SchedulerJobControlRequest) returns (SchedulerJobReply);
  rpc TriggerJob(SchedulerJobControlRequest) returns (SchedulerJobReply);
  rpc GetJob(SchedulerJobGetRequest) returns (SchedulerJobReply);
  rpc ListJobs(SchedulerJobListRequest) returns (SchedulerJobListReply);
}
```

## Proto Message（草案）
```
message SchedulerJob {
  string job_id = 1;
  string tenant_uuid = 2;
  string owner_type = 3;
  string owner_id = 4;
  string name = 5;
  string schedule_type = 6;
  string schedule_expr = 7;
  bytes payload_json = 8;
  string status = 9;
  string next_run_at = 10;
  string last_run_at = 11;
}
message SchedulerJobCreateRequest { SchedulerJob job = 1; }
message SchedulerJobUpdateRequest { SchedulerJob job = 1; }
message SchedulerJobControlRequest { string job_id = 1; string tenant_uuid = 2; }
message SchedulerJobGetRequest { string job_id = 1; string tenant_uuid = 2; }
message SchedulerJobListRequest { string tenant_uuid = 1; int32 limit = 2; }
message SchedulerJobReply { SchedulerJob job = 1; }
message SchedulerJobListReply { repeated SchedulerJob jobs = 1; }
```

## SDK 示例（伪代码）
```
sch := scheduler.NewClient(baseURL, token)
sch.CreateJob(ctx, SchedulerJob{
  TenantUUID: "...",
  OwnerType: "plugin",
  OwnerID: "com.powerx.helloworld",
  ScheduleType: "cron",
  ScheduleExpr: "0 * * * *",
  Payload: map[string]any{
    "plugin_action": "knowledge.sync",
    "params": map[string]any{"space_id": "..."},
  },
})
```

## 字段校验与时区
- `schedule_type=cron`：支持 5/6 段 Cron，默认使用 `UTC`。
- `schedule_type=interval`：支持 `Ns/Nm/Nh/Nd` 格式（如 `5m`、`2h`）。
- `schedule_type=once`：`schedule_expr` 为 RFC3339 时间。
- `timezone`：默认 `UTC`，可扩展为 `Asia/Shanghai` 等。

## 幂等与重复创建
- 建议支持 `Idempotency-Key` 头。
- 同一 `owner_id + name` 默认视为唯一（冲突返回 `scheduler.conflict`）。

## 并发与重叠策略
- `overlap_policy`（可选）：`skip` / `queue` / `parallel`。
- 默认 `skip`：上次未完成时跳过本次。

## 限流与负载
- 默认限流：按租户 + owner_id 维度限制创建频率。
- 超限错误：`scheduler.rate_limited`（HTTP 429）。

## 示例（HTTP）
```
POST /api/v1/admin/scheduler/jobs
Authorization: Bearer <TOKEN>
x-tenant-uuid: <TENANT_UUID>
Idempotency-Key: 7c4d...

{
  "tenant_uuid": "...",
  "owner_type": "plugin",
  "owner_id": "com.powerx.helloworld",
  "name": "sync-knowledge",
  "schedule_type": "cron",
  "schedule_expr": "0 * * * *",
  "payload": { "plugin_action": "knowledge.sync", "params": { "space_id": "..." } },
  "misfire_policy": "skip",
  "retry_policy": { "max_attempts": 3, "backoff_seconds": 30 }
}
```

## 认证与租户头部
- `Authorization: Bearer <TOKEN>`
- `x-tenant-uuid: <TENANT_UUID>`（可选，优先于 token 中租户）

## 错误码（建议）
- `scheduler.invalid_schedule`
- `scheduler.job_not_found`
- `scheduler.permission_denied`
- `scheduler.conflict`
 - `scheduler.rate_limited`

## 触发链路
1) Job 到点 → Scheduler 生成触发事件 `scheduler.job.triggered`
2) Event Bus 投递给订阅者（插件/核心模块）
3) Subscriber 执行任务 → ack / nack
4) 失败按 retry_policy 重试（延迟队列）

## 插件注册与消费
- **注册**：插件通过 Manifest 或 API 注册 cron/interval/once。
  - `owner_type=plugin`，`owner_id=plugin_id`
  - payload 内包含 `plugin_action`、`params`
- **消费**：插件声明订阅 `scheduler.job.triggered`，由插件 handler 执行。

## Plugin Manifest 示例
```
scheduler:
  jobs:
    - name: "sync-knowledge"
      schedule_type: "cron"
      schedule_expr: "0 * * * *"
      payload:
        plugin_action: "knowledge.sync"
        params:
          space_id: "..."
```

## PowerXPlugin Scheduler Bridge
- 插件通过统一 Scheduler Bridge 调用 PowerX Scheduler。
- 模式建议：
  - `local`：仅本地调度（不依赖底座）
  - `corex`：调用 PowerX Scheduler（HTTP/gRPC/SDK）
  - `dual`：双写/双读，便于灰度与回滚
- 兜底策略：底座不可用时自动降级到本地调度。
- 触发事件：`scheduler.job.triggered`，payload 包含：
  - `job_id`、`tenant_uuid`、`owner_id`、`scheduled_at`、`payload`

## `scheduler.job.triggered` Payload Schema（草案）
```
{
  "job_id": "uuid",
  "tenant_uuid": "uuid",
  "owner_type": "core|plugin",
  "owner_id": "service_name|plugin_id",
  "scheduled_at": "RFC3339",
  "payload": {
    "plugin_action": "string",
    "params": {}
  },
  "attempt": 1
}
```

## 最小落地实现建议
- 存储：`scheduler_jobs` + `scheduler_job_runs`（runs 记录执行结果）。
- 触发器：独立 worker 每 N 秒扫描 `next_run_at`（DB + Redis 锁）。
- 锁：`scheduler:lock:{job_id}`，TTL 覆盖执行窗口。
- 误触发策略：
  - `skip`：错过就跳过
  - `run_catchup`：补跑一次（不多次追赶）
- 对接 Event Bus：统一发布 `scheduler.job.triggered` 事件。

## 接口与队列策略
- 接口优先级：HTTP 为第一优先；gRPC 与 SDK 作为后续扩展。
- 队列默认：Redis（延迟/重试、租户隔离、轻量落地）。
- 可替换：通过 Provider 接口切换 Kafka/NATS/SQS 等。

## 管理 API（草案）
```
POST   /api/v1/admin/scheduler/jobs
PATCH  /api/v1/admin/scheduler/jobs/:jobId
POST   /api/v1/admin/scheduler/jobs/:jobId/pause
POST   /api/v1/admin/scheduler/jobs/:jobId/resume
POST   /api/v1/admin/scheduler/jobs/:jobId/trigger
GET    /api/v1/admin/scheduler/jobs
GET    /api/v1/admin/scheduler/jobs/:jobId
```

## 可靠性与一致性
- 至少一次触发，消费方需幂等。
- 使用 Redis/DB 锁避免重复触发。
- 支持“错过执行”策略：跳过或补跑。

## 与现有模块的关系
- Event Bus 已有延迟重试能力（可复用）。
- workflow scheduler / knowledge source sync / plugin upgrade 等逐步迁移到统一 Scheduler。

## 观测与审计
- 指标：scheduler.trigger_total、scheduler.missed_total、scheduler.latency_p95、scheduler.retry_total。
- 日志：job_id、tenant_uuid、owner_id、schedule_expr、trace_id。
- 审计：创建/修改/暂停/恢复记录入 audit trail。

## 迁移建议
1) 先接入 core 模块（knowledge source sync、corpus check）。
2) 再开放给插件注册。
3) 逐步替换现有分散 cron/内部定时器。
