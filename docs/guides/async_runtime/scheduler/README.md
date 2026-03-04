# Scheduler / Cron 管理（async_runtime）

> 状态：已实现（可运维）  
> 平台入口：`docs/guides/async_runtime/README.md`

## 1. 这份文档解决什么问题

1. 如何确认 Cron 作业已注册且可运行
2. 如何手动触发 `run-now`
3. 如何安全执行 `pause/resume`
4. 如何判断调度是否驱动了 Task 变化

## 2. 前置条件

```bash
export POWERX_BASE_URL="http://127.0.0.1:8077/api/v1"
export ADMIN_TOKEN="<root-admin-jwt>"
```

要求：

1. backend 已启动
2. Root 管理权限可用
3. Task 子系统已可读（`task-queue/stats` 接口正常）

## 3. 核心接口

1. `GET /admin/event-fabric/cron/jobs`
2. `POST /admin/event-fabric/cron/jobs/{job_id}/run-now`
3. `POST /admin/event-fabric/cron/jobs/{job_id}/pause`
4. `POST /admin/event-fabric/cron/jobs/{job_id}/resume`
5. `POST /admin/event-fabric/retry/tasks`（制造 Retry 样本）

## 4. 内置作业（当前）

1. `event_fabric.retry_dispatch`
2. `event_fabric.authorization_challenge_timeout`

### 4.1 调度类型与 UI 字段对照

1. `kind=interval`：固定周期调度（UI 显示 `interval=Ns`）
2. `kind=queue`：队列触发调度（UI 显示 `trigger=queue`，不展示固定秒数）
3. `batch`：单次处理批量上限（两类作业都会显示）
4. `run-now`：立刻执行当前作业一次，不会“随机创建任务”
5. `event_fabric.retry_dispatch`：run-now 会扫描“到期可重试”任务并重新投递
6. `event_fabric.authorization_challenge_timeout`：run-now 会扫描授权超时队列并执行超时处理

## 5. 手动验证（最短路径）

### Step 1：查询作业列表

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs" | jq
```

预期：

1. 返回 `code=200`
2. 能看到上面两个内置作业
3. 状态通常为 `running`（若你手动暂停过可能是 `paused`）

### Step 2：触发 run-now

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs/event_fabric.retry_dispatch/run-now" | jq
```

预期：

1. 返回 `code=200`
2. 返回体 `data.id=event_fabric.retry_dispatch`
3. 含义是“立即触发 retry_dispatch 扫描重试队列”，不是直接插入一条业务任务

### Step 3：暂停并恢复

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs/event_fabric.retry_dispatch/pause" | jq
```

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/cron/jobs/event_fabric.retry_dispatch/resume" | jq
```

预期：

1. 两次都返回 `code=200`
2. pause 后状态为 `paused`
3. resume 后状态回到 `running`

### Step 4：确认调度和任务联动

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/stats" | jq
```

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$POWERX_BASE_URL/admin/event-fabric/task-queue/messages?tenant_key=global&subscriber_id=_subscriber.authorization.challenge_timeout&limit=30" | jq
```

说明：

1. `run-now` 成功不等于一定有新任务（取决于是否存在到期数据）
2. 若当前无到期项，`stats/history` 可无明显增量，这属于正常
3. 这两类作业是常驻 worker，`status` 通常持续为 `running`，不会出现一次性 `completed`

## 6. 常见误解

1. Scheduler 不直接替代 Task；它只负责“什么时候触发”
2. Task 是执行与历史层；Scheduler 是调度与控制层
3. `run-now` 是触发检查，不是强制制造业务数据

## 7. 自动化脚本

1. 只读：`scripts/cron/integration_playbook.sh`
2. 读写：`scripts/cron/integration_playbook.sh --with-write`

## 8. 参考文档

1. `docs/guides/async_runtime/event_fabric/integration_playbook.md`
2. `scripts/cron/integration_playbook.sh`
