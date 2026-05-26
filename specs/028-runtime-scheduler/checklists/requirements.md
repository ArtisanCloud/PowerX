# Requirements Checklist: Runtime Scheduler

- [ ] Event Fabric Cron and Runtime Scheduler boundaries are explicitly separated.
- [ ] HTTP contract uses `/api/v1/admin/scheduler/jobs`.
- [ ] gRPC contract references `powerx.scheduler.v1.SchedulerService`.
- [ ] `owner_type=plugin` owner validation is specified.
- [ ] Tenant source is claims/context only; request header override is forbidden.
- [ ] Trigger event topic is fixed to `powerx.runtime.scheduler.triggered.v1`.
- [ ] `scheduler_jobs` and `scheduler_job_runs` are specified.
- [ ] Host provider fail-fast behavior is specified.
- [ ] Observability fields and metrics are specified.
