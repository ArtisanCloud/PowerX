# 调度运行中心

> 状态：平台内置调度配置已可查看和手动触发；统一运行记录与插件业务调度尚未完整接入。  
> 平台入口：`docs/guides/async_runtime/README.md`  
> 插件通用调度契约：`docs/plan/integration/scheduler.md`

## 1. 产品概念

调度运行中心不是单纯的 Cron 页面，也不是单纯的队列页面。它按三层理解：

1. **调度配置**：计划什么时候触发，例如固定间隔、指定时间或 cron 规则。
2. **运行记录**：记录某一次计划是否真的触发、由谁触发、结果是什么。
3. **关联任务**：查看这次触发后产生的事件、队列任务、执行状态和任务历史。

关系如下：

```text
调度配置
    ↓ 触发
运行记录
    ↓ 发布事件 / 投递任务
关联任务
    ↓ 消费
业务 Handler
```

## 2. 当前页面状态

当前 `/monitor/task-cron` 页面按以下能力落地：

1. **调度配置**：已接入平台内置调度配置。
2. **运行记录**：仅提供空态说明，统一运行记录尚未接入。
3. **关联任务**：视图尚未接入；通用 Replay / Pipeline / Retry 联调保留在事件总线页面。

当前已展示的内置调度配置：

1. `event_fabric.retry_dispatch`：页面显示为“重试投递扫描”，按固定间隔扫描到期重试投递。
2. `event_fabric.authorization_challenge_timeout`：页面显示为“授权超时处理”，处理授权挑战超时队列。

这两项是平台内部固定调度配置，不是插件业务任务本身。

## 3. 当前边界

1. `/admin/event-fabric/cron/jobs` 是平台内部 Schedule 运维接口。
2. 该接口只服务平台内置调度配置，不接受插件创建任意业务 schedule。
3. 插件业务调度不能直接依赖 `/admin/event-fabric/cron/jobs`。
4. 插件业务应通过 PowerXPlugin Framework scheduler facade 注册 job。
5. 生产级插件调度应写入 `scheduler_jobs` 与 `scheduler_job_runs`，并通过标准事件触发插件 handler。

## 4. 页面与接口映射

### 4.1 调度配置

页面用途：

1. 查看平台内置调度配置。
2. 查看触发类型、触发规则、运行状态、下次触发时间。
3. 手动触发一次平台内置调度。
4. 暂停或恢复支持控制的内置调度。

接口：

1. `GET /admin/event-fabric/cron/jobs`
2. `POST /admin/event-fabric/cron/jobs/{job_id}/run-now`
3. `POST /admin/event-fabric/cron/jobs/{job_id}/pause`
4. `POST /admin/event-fabric/cron/jobs/{job_id}/resume`

注意：`run-now` 表示“触发一次调度检查”，不保证一定产生新任务。是否产生任务取决于当时是否存在到期数据。

### 4.2 运行记录

页面用途：

1. 展示每次调度触发的运行记录。
2. 串联 schedule_id、run_id、event_id、task_id、trace_id。
3. 判断触发成功、跳过、失败或投递失败。

当前状态：尚未完整接入。

目标接口应来自插件通用 SchedulerService 或管理端 Scheduler API，例如：

1. `GET /admin/scheduler/jobs/:job_id/runs`
2. `GET /admin/scheduler/runs?owner_type=&owner_id=&tenant_uuid=`

### 4.3 关联任务

页面用途：

1. 展示某次运行记录关联的 event_id 与 task_id。
2. 展示任务命中的 `tenant_key + subscriber_id` 队列分片。
3. 展示任务状态、失败原因、trace_id 与执行历史。
4. 跳转到事件总线和 Logs / Trace 做深入排查。

当前状态：尚未完整接入。通用 Replay / Pipeline / Retry 联调保留在事件总线页面，不在调度运行中心重复提供。

目标接口应基于 Scheduler Runs 与 Task History 聚合，例如：

1. `GET /admin/scheduler/jobs/:job_id/runs`
2. `GET /admin/scheduler/runs/:run_id/tasks`
3. `GET /admin/event-fabric/task-queue/messages`

## 5. 手动验证

### Step 1：查看调度配置

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs" | jq
```

预期：

1. 返回 `code=200`。
2. 能看到平台内置调度配置。
3. 状态通常为 `running`，若手动暂停过可能是 `paused`。

### Step 2：手动触发一次调度

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs/event_fabric.retry_dispatch/run-now" | jq
```

预期：

1. 返回 `code=200`。
2. 含义是立即触发一次重试投递扫描。
3. 如果没有到期重试数据，关联任务可能没有新增记录。

### Step 3：查看队列状态

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/stats" | jq
```

如需查看具体分片：

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/messages?tenant_key=global&subscriber_id=_subscriber.authorization.challenge_timeout&limit=30" | jq
```

## 6. 常见误解

1. 调度配置不是任务。调度配置只定义何时触发。
2. 运行记录不是队列。运行记录是一次触发事实，用来串联后续 event 与 task。
3. 关联任务是某次运行记录的执行视图，不是通用事件总线联调入口。
4. Event Fabric 是触发后的投递机制，不应该反过来当作调度运行中心的主语。
5. `/admin/event-fabric/cron/jobs` 不是插件通用 Scheduler API。
6. 插件通用调度的标准触发 topic 是 `powerx.runtime.scheduler.triggered.v1`。

## 7. 后续落地要求

1. 新增持久化 `scheduler_jobs` 与 `scheduler_job_runs`。
2. 接入 `powerx.scheduler.v1.SchedulerService` 实现。
3. 在运行记录视图展示每次触发记录。
4. 运行记录必须能关联 event_id、task_id、trace_id。
5. 插件业务调度必须通过 Framework scheduler facade 调用，不直接调 Admin Cron 接口。

## 8. 参考文档

1. `docs/plan/integration/scheduler.md`
2. `docs/guides/async_runtime/event_fabric/integration_playbook.md`
3. `docs/guides/async_runtime/task/README.md`
4. `scripts/cron/integration_playbook.sh`
