# 02. Jobs Dashboard 设计与配置

## 0. 先说清楚：Jobs 看板看什么

Jobs 看板不是看 HTTP 接口本身，而是看异步任务执行状态。

- `service`（Loki label）= 日志来源（通常 `powerx-backend`）
- `job_id`（日志字段）= 具体任务实例
- `outcome/status/level`（日志字段）= 任务成功/失败信号

## 1. 设计目标

回答三个问题：
- 哪些任务正在失败？
- 失败是否持续扩大？
- 哪个 `job_id` 需要优先排障？

## 2. 覆盖场景

- 定时任务失败
- 队列任务重试
- 某批任务持续 error/warn

## 3. 配置前提

日志里建议包含：
- `job_id`
- `outcome` 或 `status`
- `trace_id`
- `tenant_uuid`

## 4. Grafana 里怎么配置（一步一步）

### 4.1 新建看板

1. Grafana -> `Dashboards` -> `New` -> `New dashboard`
2. `Add visualization`，数据源选 Loki
3. 保存看板名：`PowerX-Jobs-Overview`

### 4.2 配置变量

在 `Dashboard settings` -> `Variables` 添加：

1. `service`
- Type: `Query`
- Query: `label_values(service)`

2. `env`
- Type: `Query`
- Query: `label_values(env)`

## 5. 面板清单（推荐最小集）

1) 任务吞吐（按 job_id）
```logql
sum by (job_id) (count_over_time({service=~"$service",env=~"$env"} | json | job_id!="" [5m]))
```

2) 任务失败数（按 job_id）
```logql
sum by (job_id) (count_over_time({service=~"$service",env=~"$env"} | json | job_id!="" | outcome=~"FAILED|failed" [15m]))
```

3) 任务失败率
```logql
sum(count_over_time({service=~"$service",env=~"$env"} | json | outcome=~"FAILED|failed" [15m]))
/
sum(count_over_time({service=~"$service",env=~"$env"} [15m]))
```

4) Top 失败任务
```logql
topk(10, sum by (job_id) (count_over_time({service=~"$service",env=~"$env"} | json | job_id!="" | level=~"error|warn" [30m])))
```

## 6. Drilldown 查询模板

- 指定任务实例：
```logql
{service=~"$service",env=~"$env"} | json | job_id="YOUR_JOB_ID"
```

- 仅看失败任务：
```logql
{service=~"$service",env=~"$env"} | json | outcome=~"FAILED|failed"
```

- 指定租户任务故障：
```logql
{service=~"$service",env=~"$env"} | json | tenant_uuid="YOUR_TENANT_UUID" | outcome=~"FAILED|failed"
```

## 7. 验证步骤

1. 触发一次可观测任务（定时/手动）。
2. Explore 执行：
```logql
{service=~".+"} | json | job_id!=""
```
3. 看板应出现 `job_id` 曲线。
4. 人工制造失败，确认失败数与失败率上升。
