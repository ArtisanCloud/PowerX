# Runtime Scheduler Data Model

## scheduler_jobs

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| job_id | uuid | yes | 主键 |
| tenant_uuid | uuid | yes | 来自 token claims 或受信宿主上下文 |
| owner_type | text | yes | `core` 或 `plugin` |
| owner_id | text | yes | core service name 或 plugin_id |
| name | text | yes | owner 范围内唯一任务名 |
| schedule_type | text | yes | `once`、`interval`、`cron` |
| schedule_expr | text | yes | RFC3339、Go duration 或 cron expr |
| timezone | text | yes | IANA timezone，默认 `UTC` |
| topic | text | yes | 首期固定 `powerx.runtime.scheduler.triggered.v1` |
| payload_json | jsonb | yes | 业务 payload，必须包含或可解析 `business_action` |
| status | text | yes | `active`、`paused`、`deleted` |
| next_run_at | timestamptz | no | 下一次计划触发时间 |
| last_run_at | timestamptz | no | 上一次触发时间 |
| misfire_policy | text | yes | `skip`、`run_catchup` |
| overlap_policy | text | yes | `skip`、`queue`、`parallel` |
| retry_policy_json | jsonb | no | 事件发布失败后的补偿策略 |
| idempotency_key | text | no | 幂等键 |
| created_by | text | no | 创建主体 |
| created_at | timestamptz | yes | 创建时间 |
| updated_at | timestamptz | yes | 更新时间 |

## scheduler_job_runs

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| run_id | uuid | yes | 主键 |
| job_id | uuid | yes | 关联 `scheduler_jobs.job_id` |
| tenant_uuid | uuid | yes | 租户 |
| owner_type | text | yes | 冗余归属类型，便于查询 |
| owner_id | text | yes | 冗余归属 ID，便于查询 |
| trigger_source | text | yes | `once`、`interval`、`cron`、`manual`、`retry` |
| scheduled_at | timestamptz | no | 计划触发时间 |
| fired_at | timestamptz | no | 实际触发时间 |
| status | text | yes | `triggered`、`skipped`、`failed` |
| event_id | text | no | 发布到 EventBus 后的事件 ID |
| trace_id | text | yes | 链路追踪 ID |
| error_code | text | no | 失败码 |
| error_message | text | no | 失败说明 |
| created_at | timestamptz | yes | 创建时间 |

## Indexes And Constraints

1. `scheduler_jobs.job_id` primary key.
2. `scheduler_job_runs.run_id` primary key.
3. `scheduler_jobs(tenant_uuid, owner_type, owner_id, name)` unique.
4. `scheduler_jobs(tenant_uuid, owner_type, owner_id, status, next_run_at)` for scanner and list.
5. `scheduler_job_runs(job_id, created_at desc)` for run history.
6. `scheduler_job_runs(tenant_uuid, owner_type, owner_id, created_at desc)` for owner history.

## State Rules

1. `active -> paused -> active` 允许。
2. `active|paused -> deleted` 允许。
3. `deleted` 为终态，不允许恢复。
4. `trigger` 对 `paused/deleted` job 必须拒绝，除非后续规格显式定义 override。
5. `once` job 成功触发后不得再次自动触发；手动 trigger 必须记录 `trigger_source=manual`。
