# 01. HTTP Dashboard（分层标签版）

## 1. 目标

用于快速回答：
- 请求是否异常（4xx/5xx）？
- 哪些接口在失败或变慢？
- 问题集中在哪个模块/实例？

## 2. 先配置变量（必须，逐项照填）

路径：`Dashboard -> Settings -> Variables -> Add variable`

### 2.1 system

- Variable type：`Query`
- Name：`system`
- Query options -> Data source：`loki-PowerX`
- Query type：`Label values`
- Label：`system`
- Stream selector：留空
- Multi-value：`Off`
- Include All option：`Off`
- Allow custom values：`Off`

### 2.2 service

- Variable type：`Query`
- Name：`service`
- Data source：`loki-PowerX`
- Query type：`Label values`
- Label：`service`
- Stream selector：`{system=~"$system"}`
- Multi-value：`On`
- Include All option：`On`
- Custom all value：`.+`
- Allow custom values：`Off`

### 2.3 env

- Variable type：`Query`
- Name：`env`
- Data source：`loki-PowerX`
- Query type：`Label values`
- Label：`env`
- Stream selector：`{system=~"$system",service=~"$service"}`
- Multi-value：`On`
- Include All option：`On`
- Custom all value：`.+`

### 2.4 instance

- Variable type：`Query`
- Name：`instance`
- Data source：`loki-PowerX`
- Query type：`Label values`
- Label：`instance`
- Stream selector：`{system=~"$system",service=~"$service",env=~"$env"}`
- Multi-value：`On`
- Include All option：`On`
- Custom all value：`.+`

### 2.5 module

- Variable type：`Query`
- Name：`module`
- Data source：`loki-PowerX`
- Query type：`Label values`
- Label：`module`
- Stream selector：`{system=~"$system",service=~"$service",env=~"$env",instance=~"$instance"}`
- Multi-value：`On`
- Include All option：`On`
- Custom all value：`.+`

### 2.6 tenant_uuid（可选，推荐）

- Variable type：`Query`
- Name：`tenant_uuid`
- Data source：`loki-PowerX`
- Query type：`Label values`
- Label：`tenant_uuid`
- Stream selector：`{system=~"$system",service=~"$service",env=~"$env"}`
- Multi-value：`On`
- Include All option：`On`
- Custom all value：`.+`
- Allow custom values：`On`（无下拉值时可手输）

### 2.7 plugin_id（可选，推荐）

- Variable type：`Query`
- Name：`plugin_id`
- Data source：`loki-PowerX`
- Query type：`Label values`
- Label：`plugin_id`
- Stream selector：`{system=~"$system",service=~"$service",env=~"$env"}`
- Multi-value：`On`
- Include All option：`On`
- Custom all value：`.+`
- Allow custom values：`On`

### 2.8 job（推荐文本变量）

- Variable type：`Text box`
- Name：`job`
- 默认值：留空

说明：
- 当前 job 形态未完全统一（可能是 `job_id`、`job=...`、`job ...`）。
- 用 `Text box` 比用 `Label values` 稳定。

### 2.9 level

- Variable type：`Custom`
- Name：`level`
- Values：`debug,info,warn,error`
- Multi-value：`On`
- Include All option：`On`
- Custom all value：`.*`

## 3. 基础查询模板

```logql
{system=~"$system",service=~"$service",env=~"$env",instance=~"$instance",module=~"$module"} |= "$tenant_uuid" |= "$plugin_id" |= "$job" | json | level=~"$level"
```

说明：
- 当 `tenant_uuid/plugin_id/job` 留空时，可临时删掉对应 `|= "$变量"` 片段避免误过滤。

## 4. 面板清单（建议最小集）

先说明：
- 本节每一项都对应一个独立 Panel。
- 不是在同一个 Panel 里塞 4 条查询。
- 建议先创建第 1 个 Panel，保存后再点 `Add panel` 继续创建第 2/3/4 个。

新建 Panel 基础步骤（每个面板都重复一次）：
1. Dashboard 右上角点 `Add` -> `Visualization`。
2. 数据源选择 `loki-PowerX`。
3. 粘贴本节对应查询，点 `Run query`。
4. 选择可视化类型（见下表，不要随意选）。
5. 填写 Panel 标题并保存：  
   - 请求吞吐面板：`HTTP Throughput (Logs/s)`  
   - 5xx 面板：`HTTP 5xx Trend`  
   - 4xx 面板：`HTTP 4xx Trend`  
   - 慢请求 TopN 面板：`Slow Requests TopN by Path`  
   - 错误接口 TopN 面板：`Error Paths TopN`

变量匹配约定（避免 All 导致空结果）：
- 本文所有变量统一使用正则匹配 `=~`。
- `system` 可固定为 `powerx`，也可变量化为 `$system`；变量化时也使用 `=~`。

1. 请求吞吐（Logs/s）

```logql
sum(rate({system=~"$system",service=~"$service",env=~"$env",instance=~"$instance",module=~"$module"}[1m]))
```
Panel 标题：`HTTP Throughput (Logs/s)`

2. 5xx 趋势

```logql
sum(count_over_time({system=~"$system",service=~"$service",env=~"$env",instance=~"$instance",module=~"$module"} | json | status >= 500 [5m]))
```
Panel 标题：`HTTP 5xx Trend`

3. 4xx 趋势

```logql
sum(count_over_time({system=~"$system",service=~"$service",env=~"$env",instance=~"$instance",module=~"$module"} | json | status >= 400 | status < 500 [5m]))
```
Panel 标题：`HTTP 4xx Trend`

4. 慢请求 TopN（按 path）

```logql
topk(
  10,
  max by (path) (
    max_over_time(
      {system=~"$system",service=~"$service",env=~"$env",instance=~"$instance",module=~"$module"}
      |= "http_request"
      | regexp "(?P<json>\\{.*\\})"
      | line_format "{{.json}}"
      | json
      | __error__=""
      | path!=""
      | unwrap latency_ms
      [5m]
    )
  )
)
```
Panel 标题：`Slow Requests TopN by Path`

5. 错误接口 TopN（按 path）

```logql
topk(
  10,
  sum by (path) (
    count_over_time(
      {system=~"$system",service=~"$service",env=~"$env",instance=~"$instance",module=~"$module"}
      |= "http_request"
      | regexp "(?P<json>\\{.*\\})"
      | line_format "{{.json}}"
      | json
      | __error__=""
      | path!=""
      | status >= 400
      [15m]
    )
  )
)
```
Panel 标题：`Error Paths TopN`

可视化类型建议：
- 请求吞吐：`Time series`
- 5xx 趋势：`Time series`
- 4xx 趋势：`Time series`
- 慢请求 TopN：`Table`（`Instant` 查询）
- 错误接口 TopN：`Table`（`Instant` 查询）

## 4.1 面板标题与可视化类型（直接照抄）

| 面板用途 | Panel 标题（建议） | 可视化类型 |
|---|---|---|
| 请求吞吐（Logs/s） | `HTTP Throughput (Logs/s)` | `Time series` |
| 5xx 趋势 | `HTTP 5xx Trend` | `Time series` |
| 4xx 趋势 | `HTTP 4xx Trend` | `Time series` |
| 慢请求 TopN（按 path） | `Slow Requests TopN by Path` | `Table` |
| 错误接口 TopN（按 path） | `Error Paths TopN` | `Table` |

## 5. Drilldown 模板

按租户：

```logql
{system="$system",service="$service",env="$env"} | json | tenant_uuid="$tenant_uuid"
```

按链路：

```logql
{system="$system",service="$service",env="$env"} | json | trace_id="$trace_id"
```

## 6. 验收

1. 执行一次正常 API 请求，看吞吐有变化。
2. 人工触发一次 4xx 或 5xx，看错误趋势上涨。
3. 用 `trace_id` 能回放完整日志链路。

## 7. 怎么解读这 5 个面板（重点）

### 7.1 HTTP Throughput (Logs/s)

看什么：
- 当前请求量是否稳定。
- 是否出现突增/突降（突降常见于服务异常或流量切换）。

异常信号：
- 请求量突然归零：优先检查服务健康、反代、网关。
- 突增并伴随 4xx/5xx 上升：优先看限流、鉴权、上游超时。

### 7.2 HTTP 5xx Trend

看什么：
- 服务端错误（5xx）是否出现、是否持续。

异常信号：
- 连续多个时间窗 > 0：需要告警。
- 当前窗口为 `No data`：通常表示该窗口没有 5xx（正常）。

### 7.3 HTTP 4xx Trend

看什么：
- 客户端/调用参数类错误是否在扩大。

异常信号：
- 长期高位：多为调用方参数、鉴权、租户上下文问题。
- 与吞吐同向上升但 5xx 不升：优先排查请求构造而非服务稳定性。

### 7.4 Slow Requests TopN by Path

看什么：
- 哪些 path 最慢（峰值延迟最高）。

异常信号：
- 某 path 持续位于 Top1/Top3 且显著高于其它路径：优先做接口级 profiling。
- 突然出现极端大值：先核查日志解析和单位（ms/s）是否一致。

建议：
- 本面板用 `Table + Instant`，便于直接看到“当前最慢前 N 个路径”。

### 7.5 Error Paths TopN

看什么：
- 哪些 path 在当前窗口错误最多（4xx/5xx 聚合）。

异常信号：
- 某 path 长期 Top1：优先排查该接口依赖和入参来源。
- 4xx TopN 高、5xx 低：优先排查调用方和权限上下文。
- 5xx TopN 上升：优先排查服务内部错误与下游依赖。

建议：
- 本面板用 `Table + Instant`，按值降序。

## 8. 从图到排障的最短路径

1. 先看 `HTTP 5xx Trend` 是否大于 0。  
2. 若 5xx 有值，立即看 `Error Paths TopN` 拿到具体 path。  
3. 用该 path 去日志面板检索 `trace_id/request_id`。  
4. 回放链路并定位到具体模块/依赖。  
5. 若 5xx 无值但 4xx 高，转向调用方参数/鉴权/租户上下文排查。
