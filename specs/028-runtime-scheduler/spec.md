# Feature Specification: Runtime Scheduler

**Domain Ownership**: CoreX (`corex.runtime.scheduler`)

**Feature Branch**: `028-runtime-scheduler`  
**Created**: 2026-05-20  
**Status**: Draft  
**Input**: 插件业务需要将计划任务远程注册到 PowerX 宿主，由宿主持久化、触发并通过标准事件交付给插件 handler。

## 背景与动机

PowerX 当前已经有 Event Fabric Cron 运维接口，但该接口只服务底座内部 worker，例如重试投递扫描和授权超时处理。AI Craft 等业务插件需要注册“付款后提醒、交付前提醒、延期检查”等业务 schedule，这类任务必须由宿主统一持久化、审计、触发和观测，不能复用内部 Cron 运维接口，也不能让插件在 host 模式下维护本地内存 timer。

本特性将 `powerx.scheduler.v1.SchedulerService` 提升为正式运行时能力，并通过 PowerXPlugin Framework scheduler facade 屏蔽 host/local 模式差异。

## 范围声明

### In Scope

- 插件和核心模块通用调度任务模型：`scheduler_jobs`、`scheduler_job_runs`。
- `once`、`interval`、`cron` 三类计划表达式校验与 `next_run_at` 计算。
- `CreateJob`、`UpdateJob`、`PauseJob`、`ResumeJob`、`TriggerJob`、`GetJob`、`ListJobs` 服务能力。
- 管理端 HTTP API：`/api/v1/admin/scheduler/jobs`。
- gRPC 服务实现：`powerx.scheduler.v1.SchedulerService`。
- 标准触发事件：`powerx.runtime.scheduler.triggered.v1`。
- Capability Registry 暴露平台能力：`com.corex.scheduler.jobs`。
- 审计、指标、结构化日志、运行记录与事件关联。
- PowerXPlugin Framework host provider 通过 STS/delegated gateway 调用宿主 SchedulerService。

### Out of Scope

- 不扩展 `/admin/event-fabric/cron/jobs` 作为插件通用 Scheduler API。
- 不让 Scheduler 直接调用插件业务函数。
- 不允许 host provider 失败后静默降级到 local provider。
- 不在插件业务代码里维护本地内存 timer。
- 不把 workflow scheduler scope 复用为插件通用 scheduler scope。
- 不在首期支持业务插件自定义任意触发 topic。

## Clarifications

### Session 2026-05-20

- Q: 插件业务 schedule 是否应通过 Event Fabric Cron 创建？ → A: 否。Event Fabric Cron 只属于底座内部运维能力，插件业务 schedule 走 Runtime Scheduler。
- Q: HTTP 路径使用 `/api/v1/admin/runtime/scheduler/jobs` 还是 `/api/v1/admin/scheduler/jobs`？ → A: 使用 `/api/v1/admin/scheduler/jobs`，与 PowerX 管理端资源命名保持一致。
- Q: 插件 host 模式是否可以在宿主 SchedulerService 失败时回退本地 scheduler？ → A: 不可以。必须 fail-fast，避免重复触发、漏触发和审计断链。
- Q: API Key 和 STS 调用 Scheduler 时如何校验 `owner_id`？ → A: PowerX MUST 按请求凭证类型区分。`Authorization: ApiKey` 只证明租户与 API 权限，`owner_id` 是租户内声明的 plugin namespace，不证明插件身份；`Authorization: Bearer` 的插件 STS token MUST 校验 `aud=plugin:<owner_id>`。

## User Scenarios & Testing

### User Story 1 - 插件注册业务调度任务 (Priority: P1)

作为宿主模式插件，我希望通过 Framework scheduler facade 在 PowerX Core 创建业务计划任务，使付款后提醒、交付前提醒、延期检查等动作由宿主可靠触发。

**Independent Test**: 启动 PowerX Core 与一个 host 模式插件，插件通过 STS 调用 SchedulerService 创建 `owner_type=plugin`、`owner_id=<plugin_id>` 的 once job，并能查询到该 job。

**Acceptance Scenarios**:

1. **Given** 插件持有合法 STS 上下文，**When** 创建 `owner_type=plugin` 且 `owner_id` 等于当前插件 ID 的 job，**Then** PowerX 持久化 job 并返回 `job_id/next_run_at/status`。
2. **Given** 插件尝试创建 `owner_id` 为其他插件 ID 的 job，**When** 请求到达 SchedulerService，**Then** PowerX 返回 403 并写入审计。
3. **Given** local+proxy 使用租户级 API Key 且该 Key 已授权 Scheduler REST 权限，**When** 创建 `owner_type=plugin` 的 job，**Then** PowerX 接受请求，并将 `owner_id` 作为声明的 plugin namespace 记录，不执行插件身份真伪校验。

### User Story 2 - 宿主按计划触发并发布标准事件 (Priority: P1)

作为插件业务 handler，我希望到点后收到统一的调度触发事件，而不是 Scheduler 直接调用业务函数。

**Independent Test**: 创建一个到期 `once` job，运行 due scanner，验证生成 `scheduler_job_runs` 记录并发布 `powerx.runtime.scheduler.triggered.v1`。

**Acceptance Scenarios**:

1. **Given** job 到达 `next_run_at`，**When** due scanner 获取到任务，**Then** 系统创建 run 记录、发布标准事件并写入 `trace_id/event_id`。
2. **Given** 事件发布失败，**When** Scheduler 记录 run，**Then** run 标记为 failed，并按 retry policy 或补偿策略处理。

### User Story 3 - 管理员查询和手动处置任务 (Priority: P1)

作为 Root 或授权管理员，我希望能查看、暂停、恢复和手动触发调度任务，并能查看运行记录。

**Independent Test**: 使用 Admin token 调用 HTTP API list/get/pause/resume/trigger/runs，验证状态流转和审计记录。

**Acceptance Scenarios**:

1. **Given** 管理员访问 `/api/v1/admin/scheduler/jobs`，**When** 按 `tenant_uuid/owner_type/owner_id/status` 过滤，**Then** 返回符合权限边界的任务列表。
2. **Given** 管理员手动触发 job，**When** 调用 `POST /api/v1/admin/scheduler/jobs/:job_id/trigger`，**Then** 系统生成 `trigger_source=manual` 的 run 记录。

### User Story 4 - Framework 屏蔽 host/local 差异 (Priority: P2)

作为插件开发者，我希望业务代码只调用 Framework scheduler API，不在业务代码里判断 host/local。

**Independent Test**: 同一插件业务测试在 local provider 和 host provider 下运行；host provider 调用宿主 SchedulerService，local provider 使用本地实现。

**Acceptance Scenarios**:

1. **Given** 插件运行在 host 模式，**When** 调用 `scheduler.CreateJob`，**Then** Framework 使用 STS/delegated gateway 调宿主 SchedulerService。
2. **Given** host provider 请求失败，**When** Framework 返回错误，**Then** 不得自动降级到 local provider。

## Edge Cases

- `tenant_uuid` 与 token claims 不一致时必须拒绝。
- `owner_type=plugin` 且请求凭证为 Bearer/STS 插件 token 时，`owner_id` 不是调用方插件 ID 必须拒绝。
- `owner_type=plugin` 且请求凭证为 API Key 时，`owner_id` 是租户内声明 namespace；PowerX 只校验租户和 API Key Scheduler 权限，不校验插件身份真伪。
- `owner_type=core` 必须拒绝 API Key 和普通插件 token，只允许 root/system 管理。
- 非法 `schedule_expr` 必须 fail-fast，不得静默修正。
- 上一次 run 未完成且 `overlap_policy=skip` 时，本次触发必须记录 skipped。
- `misfire_policy=skip` 时错过窗口直接记录 skipped。
- `misfire_policy=run_catchup` 时最多补跑一次，不追赶所有历史窗口。
- 分布式部署中多个 scanner 同时发现到期 job 时必须通过锁或状态机避免重复触发。
- Consumer 处理事件失败属于 EventBus/TaskBus 语义，不由 Scheduler 直接重试业务函数。

## Requirements

### Functional Requirements

- **FR-001**: 系统 MUST 实现 `powerx.scheduler.v1.SchedulerService` 服务端并注册到 gRPC server。
- **FR-002**: 系统 MUST 提供 Admin HTTP API：`POST/PATCH/GET /api/v1/admin/scheduler/jobs`、`GET /api/v1/admin/scheduler/jobs/:job_id`、`POST /api/v1/admin/scheduler/jobs/:job_id/{pause|resume|trigger}`、`GET /api/v1/admin/scheduler/jobs/:job_id/runs`。
- **FR-003**: 系统 MUST 新增 `scheduler_jobs` 持久化模型，包含 `tenant_uuid/owner_type/owner_id/name/schedule_type/schedule_expr/timezone/topic/payload_json/status/next_run_at/last_run_at/misfire_policy/overlap_policy/idempotency_key` 等字段。
- **FR-004**: 系统 MUST 新增 `scheduler_job_runs` 持久化模型，包含 `run_id/job_id/tenant_uuid/owner_type/owner_id/trigger_source/scheduled_at/fired_at/status/event_id/trace_id/error_code/error_message` 等字段。
- **FR-005**: 系统 MUST 保证 `tenant_uuid + owner_type + owner_id + name` 唯一。
- **FR-006**: 系统 MUST 支持 `once`、`interval`、`cron` 三种 schedule type，并对表达式执行严格校验。
- **FR-007**: 系统 MUST 将 `timezone` 作为 IANA timezone 处理，默认 `UTC`。
- **FR-008**: 系统 MUST 在创建和更新 job 时计算并持久化 `next_run_at`。
- **FR-009**: 系统 MUST 实现 due scanner/dispatcher，到点后发布 `powerx.runtime.scheduler.triggered.v1`。
- **FR-010**: 标准触发事件 MUST 至少包含 `job_id/job_name/owner_type/owner_id/tenant_uuid/trigger_source/scheduled_at/fired_at/trace_id/idempotency_key/business_action/payload`。
- **FR-011**: 首期 `topic` MUST 固定为 `powerx.runtime.scheduler.triggered.v1`，不得接受插件自定义任意 topic。
- **FR-012**: 所有调用 MUST 使用 token claims、API Key 绑定租户或受信宿主上下文解析租户，不允许请求头覆盖租户。
- **FR-013**: `owner_type=plugin` 时，系统 MUST 按凭证类型授权：Bearer/STS 插件 token 必须校验调用方插件身份与 `owner_id` 一致；API Key 请求只校验租户与 Scheduler REST 权限，并将 `owner_id` 作为声明 namespace。
- **FR-013a**: API Key 请求 MUST NOT 操作 `owner_type=core` 的 Scheduler job。
- **FR-014**: SchedulerService MUST 暴露 Capability Registry 记录 `com.corex.scheduler.jobs`，并包含 `scheduler.job.read`、`scheduler.job.manage`、`scheduler.job.run` scope。
- **FR-015**: Framework host provider MUST 通过 STS/delegated gateway 调用宿主 SchedulerService。
- **FR-016**: Framework host provider 失败时 MUST 返回明确错误，不得自动切换 local provider。
- **FR-017**: Scheduler MUST 提供至少一次触发语义；插件 handler MUST 基于 `idempotency_key` 或业务主键幂等。
- **FR-018**: Scheduler MUST 写入审计动作：create、update、pause、resume、trigger、delete。
- **FR-019**: Scheduler 日志 MUST 包含 `job_id/job_name/tenant_uuid/owner_type/owner_id/schedule_type/schedule_expr/trigger_source/trace_id/event_id`。
- **FR-020**: Scheduler MUST 暴露指标：`scheduler_trigger_total`、`scheduler_trigger_failed_total`、`scheduler_misfire_total`、`scheduler_latency_ms`、`scheduler_active_jobs`。

### Key Entities

- **SchedulerJob**: 调度任务定义，描述归属主体、计划表达式、状态、触发 topic、业务 payload 与下一次触发时间。
- **SchedulerJobRun**: 一次触发事实，串联 `job_id/event_id/trace_id` 与触发结果。
- **SchedulerTriggerEvent**: 标准事件 `powerx.runtime.scheduler.triggered.v1` 的 payload。
- **SchedulerCapability**: Registry 中的 `com.corex.scheduler.jobs` 平台能力记录。
- **SchedulerProvider**: PowerXPlugin Framework 中的 host/local provider 抽象。

## Success Criteria

- **SC-001**: 插件 host 模式创建 job 后，95% 请求在 300ms 内返回 `job_id` 与 `next_run_at`。
- **SC-002**: 到期 job 在 60 秒扫描窗口内触发成功率达到 99%。
- **SC-003**: 100% 触发 run 能通过 `trace_id` 关联到 EventBus 投递记录或失败原因。
- **SC-004**: Bearer/STS 模式下，未授权跨插件 `owner_id` 创建或修改请求 100% 被拒绝并审计；API Key 模式下同租户 `owner_id` 作为声明 namespace 记录并审计。
- **SC-005**: Framework host provider 失败时 100% fail-fast，不产生本地静默调度。

## Assumptions

- EventBus/TaskBus 已提供标准发布、投递、Retry/DLQ 与观测能力。
- Integration Gateway 已支持 STS delegated bearer 调用底座能力。
- 插件通过 Framework 注册 handler，业务 handler 自行保证幂等。
- 首期不承诺秒级精确调度，默认按扫描窗口触发。
