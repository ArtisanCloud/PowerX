# 02. Jobs Dashboard（分层标签版）

## 1. 目标

用于监控异步作业、调度任务、重试与失败扩散。

## 2. 变量（沿用全局）

- `system/service/env/instance/module`
- `level` 使用 `Custom`：`debug,info,warn,error`（多选 + All=`.*`）

## 3. 面板清单（建议最小集）

1. Job 总吞吐

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | job_id!="" [5m]))
```

2. Job 失败数

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | level=~"warn|error" | job_id!="" [5m]))
```

3. 失败 Job TopN

```logql
topk(10, sum by (job_id) (count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | level=~"warn|error" | job_id!="" [15m])))
```

4. 重试趋势

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | attempt > 1 [5m]))
```

5. 任务延迟分布

```logql
quantile_over_time(0.95, {system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | unwrap latency_ms [10m])
```

## 4. Drilldown 模板

按 job_id：

```logql
{system="$system",service="$service",env="$env"} | json | job_id="$job_id"
```

按 tenant_uuid + job_id：

```logql
{system="$system",service="$service",env="$env"} | json | tenant_uuid="$tenant_uuid" | job_id="$job_id"
```

## 5. 验收

1. 触发一批任务，吞吐和延迟有变化。
2. 触发至少一次失败，失败数/TopN 有记录。
3. 使用 `job_id` 能追溯到完整任务生命周期日志。
