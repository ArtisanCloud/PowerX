# 01. HTTP Dashboard 设计与配置

## 0. 先说清楚：为什么查询里有 `service`，但这是 HTTP 看板？

- `service` 是 Loki 的**流标签（stream label）**，用于先圈定日志来源（例如 `powerx-backend`）。
- `request`（method/path/status/latency_ms）通常在日志 JSON 正文里，不是顶层 label。
- 所以 HTTP 看板的正确查询方式是：
  1) 先用 `{service="powerx-backend"}` 选流；
  2) 再用 `| json` 解析请求字段做统计。

一句话：`service` 是“从哪来”，`request` 字段是“发生了什么”。

## 1. 设计目标

HTTP 看板用于回答三个问题：
- 现在流量是否正常？（QPS）
- 是否出现大面积失败？（4xx/5xx、错误率）
- 哪些接口慢或异常？（路径级定位）

## 2. 覆盖场景

- 管理端 API 请求失败
- Web 页面调用后端返回 4xx/5xx
- 单接口耗时突然升高
- 某租户接口报错但全局看起来正常

## 3. 建议变量

在 Dashboard -> Settings -> Variables 中添加：
- `system`: `label_values(system)`
- `service`: `label_values(service)`
- `env`: `label_values(env)`

## 4. Grafana 里怎么配置（一步一步）

### 4.1 新建看板

1. 打开 Grafana -> `Dashboards` -> `New` -> `New dashboard`  
2. 点 `Add visualization`，数据源选择你的 Loki（例如 `loki-PowerX`）  
3. 保存看板名：`PowerX-HTTP-Overview`

### 4.2 配置变量（避免每个面板都改查询）

1. 右上角齿轮 `Dashboard settings` -> `Variables` -> `Add variable`  
2. 新建变量 `system`
   - Type: `Query`
   - Data source: Loki
   - Query: `label_values(system)`
3. 新建变量 `service`
   - Type: `Query`
   - Data source: Loki
   - Query: `label_values(service)`
4. 新建变量 `env`
   - Type: `Query`
   - Data source: Loki
   - Query: `label_values(env)`
4. 点击 `Apply` 保存变量。

### 4.3 添加面板

每个面板都用 Loki 数据源，时间范围建议先用 `Last 24 hours` 校验，再收敛到 `Last 1 hour`。

## 5. 面板清单（推荐最小集）

1) QPS（按服务）
```logql
sum by (service) (rate({system=~"$system",service=~"$service",env=~"$env"}[1m]))
```

2) 错误率（4xx+5xx）
```logql
sum(rate({system=~"$system",service=~"$service",env=~"$env"} | json | status=~"4..|5.." [5m]))
/
sum(rate({system=~"$system",service=~"$service",env=~"$env"}[5m]))
```

3) HTTP 状态码分布
```logql
sum by (status) (count_over_time({system=~"$system",service=~"$service",env=~"$env"} | json | status!="" [5m]))
```

4) 慢请求 TopN（latency_ms）
```logql
topk(10, max_over_time({system=~"$system",service=~"$service",env=~"$env"} | json | unwrap latency_ms [5m]))
```

5) 接口失败排行（按 path）
```logql
topk(10, sum by (path) (count_over_time({system=~"$system",service=~"$service",env=~"$env"} | json | path!="" | status=~"4..|5.." [15m])))
```

## 6. Drilldown 查询模板

- 只看失败请求：
```logql
{system=~"$system",service=~"$service",env=~"$env"} | json | status=~"4..|5.."
```

- 指定 trace_id：
```logql
{system=~"$system",service=~"$service",env=~"$env"} |= "YOUR_TRACE_ID"
```

- 指定租户：
```logql
{system=~"$system",service=~"$service",env=~"$env"} | json | tenant_uuid="YOUR_TENANT_UUID"
```

## 7. 验证步骤

1. 打开 Explore，执行：
```logql
{service=~".+"}
```
2. 触发一次后台 API 请求。
3. 在 HTTP 看板确认 QPS 和日志有同步波动。
4. 人工构造一个 404/500，确认错误率和状态码分布变化。

## 8. 常见误区

1) “HTTP 看板不应该有 service”  
- 错。Loki 必须先用 label 选流，`service` 是最基础入口。

2) “有日志但面板为空”  
- 先把时间范围调大到 `24h`。  
- 再在 Explore 用最宽查询 ` {service=~".+"} ` 验证数据源。  
- 然后再加 `| json` 和字段过滤。
