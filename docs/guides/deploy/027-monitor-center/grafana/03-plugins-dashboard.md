# 03. Plugins Dashboard（可直接配置版）

## 1. 目标

用于定位插件调用异常、受影响租户、单次调用链路。

## 2. 前置条件

- 先在 Explore 验证基础查询可出数：

```logql
{system="powerx",service="powerx-backend",env="dev"} |= "com.powerx.plugins."
```

- Dashboard 变量沿用全局：`system/service/env/instance/module`。
- 说明：当前插件链路常见日志是文本前缀（如 `[API-IN]`、`[GATE-DENY]`），不是每条都可 `| json`。

## 3. 新建面板通用步骤（每个面板重复）

1. 进入目标 Dashboard，右上角点 `Edit`。
2. 点 `Add` -> `Visualization`。
3. `Data source` 选择 `loki-PowerX`。
4. 粘贴本节对应查询，点 `Run query`。
5. 按本节建议设置可视化类型与标题。
6. 点 `Apply` 保存面板，最后点 `Save dashboard` 保存看板。

## 4. 面板清单（建议最小集）

1. Panel 标题：`Plugin Requests (API-IN)`
- 可视化：`Time series`

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} |= "[API-IN]" |= "plugin=com.powerx.plugins." [5m]))
```

2. Panel 标题：`Plugin Deny/Error (5m)`
- 可视化：`Time series`

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} |= "[GATE-DENY]" [5m]))
```

3. Panel 标题：`Denied Plugin Top 10 (15m)`
- 可视化：`Table`
- Query options：`Instant = On`

```logql
topk(
  10,
  sum by (plugin) (
    count_over_time(
      {system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"}
      |= "[GATE-DENY]"
      | regexp "plugin=(?P<plugin>[^ ]+)"
      [15m]
    )
  )
)
```

4. Panel 标题：`Plugin Products Route Deny (5m)`
- 可视化：`Time series`

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} |= "[GATE-DENY]" |= "/v1/agent/ai-craft/products" [5m]))
```

5. Panel 标题：`Plugin Realtime Logs`
- 可视化：`Logs`

```logql
{system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} |= "com.powerx.plugins."
```

## 5. Drilldown 查询模板（Explore）

按插件：

```logql
{system="$system",service="$service",env="$env"} |= "plugin=$plugin_id"
```

按插件 + 会话：

```logql
{system="$system",service="$service",env="$env"} | json | plugin_id="$plugin_id" | session_id="$session_id"
```

按消息全链路：

```logql
{system="$system",service="$service",env="$env"} | json | message_id="$message_id"
```

按 request_id（优先，最稳）：

```logql
{system="$system",service="$service",env="$env"} |= "request_id=$request_id"
```

按 trace_id：

```logql
{system="$system",service="$service",env="$env"} |= "trace_id=$trace_id"
```

## 6. 你当前问题的直接排障查询

插件 products 请求：

```logql
{system="$system",service="$service",env="$env"} |= "/api/v1/agent/ai-craft/products"
```

插件权限拒绝（403 线索）：

```logql
{system="$system",service="$service",env="$env"} |= "no permission rule for this route"
```

同一秒回放 API-IN + GATE-DENY：

```logql
{system="$system",service="$service",env="$env"} |= "ai-craft/products"
```

## 7. 验收

1. 执行一次插件调用，实时流可见插件日志。
2. 制造一次失败，错误趋势和 TopN 有变化。
3. 用 `plugin_id + message_id` 能回放完整调用日志链路。
4. 用单个 `request_id` 能命中 `API-IN -> GATE-* -> PROXY-*` 链路日志。
