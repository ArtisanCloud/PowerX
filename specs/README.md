# Specs Index

本目录用于管理按编号归档的功能规格（spec kit 工作流产物）。

## 目录约定

- 每个功能使用独立目录：`specs/<编号>-<feature-name>/`
- 标准文件包括：`spec.md`、`plan.md`、`tasks.md`、`quickstart.md`
- 契约文件放在 `contracts/`，检查清单放在 `checklists/`

## 当前重点特性

- `026-iam`
  - 目标：统一 IAM 用户与角色 RBAC 能力
  - 重点：root/tenant admin/member 边界、`/settings/users` 交互语义、`me/context` 一致性
  - 入口：
    - `specs/026-iam/spec.md`
    - `specs/026-iam/quickstart.md`
    - `specs/026-iam/tasks.md`
- `028-runtime-scheduler`
  - 目标：统一插件与核心模块的运行时调度能力
  - 重点：`powerx.scheduler.v1.SchedulerService`、`/api/v1/admin/scheduler/jobs`、`scheduler_jobs`、`scheduler_job_runs`、`powerx.runtime.scheduler.triggered.v1`
  - 边界：`/admin/event-fabric/cron/jobs` 仅用于 Event Fabric 内部 Cron 运维，不作为插件业务 Scheduler API
  - 入口：
    - `specs/028-runtime-scheduler/spec.md`
    - `specs/028-runtime-scheduler/quickstart.md`
    - `specs/028-runtime-scheduler/tasks.md`
