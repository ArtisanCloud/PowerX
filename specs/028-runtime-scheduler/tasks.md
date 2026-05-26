# Runtime Scheduler Tasks

## Phase 1 - Contract And Persistence

- [ ] T001 确认 `backend/api/grpc/contracts/powerx/scheduler/v1/scheduler.proto` 字段是否需要补充 runs、misfire、overlap、topic。
- [ ] T002 生成并校验 gRPC 代码。
- [ ] T003 新增 `scheduler_jobs` GORM model。
- [ ] T004 新增 `scheduler_job_runs` GORM model。
- [ ] T005 新增数据库迁移。
- [ ] T006 新增 repository 与基础单测。

## Phase 2 - Service

- [ ] T007 实现 schedule expression 校验。
- [ ] T008 实现 `next_run_at` 计算。
- [ ] T009 实现 CreateJob。
- [ ] T010 实现 UpdateJob。
- [ ] T011 实现 PauseJob。
- [ ] T012 实现 ResumeJob。
- [ ] T013 实现 TriggerJob。
- [ ] T014 实现 GetJob/ListJobs/ListRuns。
- [ ] T015 实现租户与 owner 校验。

## Phase 3 - Dispatcher

- [ ] T016 实现 due scanner。
- [ ] T017 实现分布式锁或状态 CAS 防重。
- [ ] T018 实现标准事件发布 `powerx.runtime.scheduler.triggered.v1`。
- [ ] T019 实现 run 记录状态流转。
- [ ] T020 实现 misfire/overlap policy。

## Phase 4 - Transports And Registry

- [ ] T021 注册 `powerx.scheduler.v1.SchedulerService`。
- [ ] T022 新增 Admin HTTP handler。
- [ ] T023 新增 HTTP OpenAPI 契约。
- [ ] T024 注册 Capability `com.corex.scheduler.jobs`。
- [ ] T025 配置 scopes：`scheduler.job.read/manage/run`。

## Phase 5 - Observability And Framework

- [ ] T026 接入审计日志。
- [ ] T027 接入 metrics。
- [ ] T028 接入结构化日志字段。
- [ ] T029 提供 PowerXPlugin Framework host provider 联调用例。
- [ ] T030 提供 AI Craft 业务迁移验证用例。
