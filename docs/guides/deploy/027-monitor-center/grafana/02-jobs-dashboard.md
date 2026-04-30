# 02. Jobs Dashboard（可直接配置版）

## 1. 目标

用于监控异步作业、调度任务、重试与失败扩散。

## 1.1 Job 是什么（先看这个）

在 PowerX 里，`job` 指“异步后台执行单元”，不是 HTTP 请求本身。  
典型特征是：由接口触发或系统调度后，在后台独立执行，拥有自己的生命周期（创建、运行、成功/失败、重试）。

当前主要包括：

1. 备份作业（backup job）
- 例子：`backup.api.trigger_job`
- 常见字段：`job_id`（数值）

2. 知识库导入作业（ingestion job）
- 例子：`[ingestion] async start ... job=<uuid>`
- 常见字段：`job`（uuid 文本）

3. 能力目录同步作业（capability sync job）
- 例子：`[capability_sync] update job ...`
- 常见字段：`job <uuid>`（文本中出现）

说明：

1. 目前并非所有 job 日志都统一成 `job_id` JSON 字段。
2. 所以 Jobs Dashboard 里会同时存在“文本匹配查询”和“JSON 字段查询”两类写法。

## 2. 前置条件

- 先确认 Explore 可查到作业日志（至少能查到一条包含 `job_id` 的记录）。
- Dashboard 变量沿用全局：`system/service/env/instance/module`。
- 建议 `system` 固定默认值 `powerx`，避免空值导致查询无结果。
- 新增一个 Drilldown 变量：`job_query`（`Text box` 类型，默认空字符串）。

`job_query` 用法示例：

1. ingestion job：`job=3f2d...`
2. backup job：`"job_id":123`
3. capability sync job：`job 3f2d...`

## 3. 新建面板通用步骤（每个面板重复）

1. 进入目标 Dashboard，右上角点 `Edit`。
2. 点 `Add` -> `Visualization`。
3. `Data source` 选择 `loki-PowerX`。
4. 粘贴本节对应查询，点 `Run query`。
5. 按本节建议设置可视化类型与标题。
6. 点 `Apply` 保存面板，最后点 `Save dashboard` 保存看板。

## 4. 面板清单（建议最小集）

1. Panel 标题：`Job Throughput (5m)`
- 可视化：`Time series`

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | job_id!="" [5m]))
```

2. Panel 标题：`Job Failures (5m)`
- 可视化：`Time series`

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | level=~"warn|error" | job_id!="" [5m]))
```

3. Panel 标题：`Failed Jobs Top 10 (15m)`
- 可视化：`Table`
- Query options：`Instant = On`

```logql
topk(10, sum by (job_id) (count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | level=~"warn|error" | job_id!="" [15m])))
```

4. Panel 标题：`Job Retries (attempt > 1)`
- 可视化：`Time series`

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | attempt > 1 [5m]))
```

5. Panel 标题：`Job P95 Latency (ms)`
- 可视化：`Time series`

```logql
quantile_over_time(0.95, {system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | unwrap latency_ms [10m])
```

6. Panel 标题：`Job Logs (Drilldown)`
- 可视化：`Logs`

```logql
{system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} |= "$job_query"
```

说明：

1. 这个面板用于“单个 job 回放”。
2. 不依赖统一 `job_id` 字段，兼容当前文本型 job 日志。

## 5. Drilldown 查询模板（Explore）

按 job_id：

```logql
{system="$system",service="$service",env="$env"} | json | job_id="$job_id"
```

按 tenant_uuid + job_id：

```logql
{system="$system",service="$service",env="$env"} | json | tenant_uuid="$tenant_uuid" | job_id="$job_id"
```

## 6. 验收

1. 触发一批任务，吞吐和延迟有变化。
2. 触发至少一次失败，失败数/TopN 有记录。
3. 使用 `job_id` 能追溯到完整任务生命周期日志。
