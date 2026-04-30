# 03. Plugins Dashboard（分层标签版）

## 1. 目标

用于定位插件调用异常、受影响租户、单次调用链路。

## 2. 变量（沿用全局）

- `system/service/env/instance/module`
- `level` 使用 `Custom`：`debug,info,warn,error`（多选 + All=`.*`）

## 3. 面板清单（建议最小集）

1. 插件错误趋势

```logql
sum(count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | level=~"warn|error" | plugin_id!="" [5m]))
```

2. 插件错误 TopN

```logql
topk(10, sum by (plugin_id) (count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | level=~"warn|error" | plugin_id!="" [15m])))
```

3. 受影响租户 TopN

```logql
topk(10, sum by (tenant_uuid) (count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | level=~"warn|error" | plugin_id!="" | tenant_uuid!="" [15m])))
```

4. 插件状态码分布

```logql
sum by (plugin_id, status_code) (count_over_time({system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | plugin_id!="" | status_code!="" [10m]))
```

5. 插件实时日志流

```logql
{system="$system",service="$service",env="$env",instance=~"$instance",module=~"$module"} | json | plugin_id!=""
```

## 4. Drilldown 模板

按插件：

```logql
{system="$system",service="$service",env="$env"} | json | plugin_id="$plugin_id"
```

按插件 + 会话：

```logql
{system="$system",service="$service",env="$env"} | json | plugin_id="$plugin_id" | session_id="$session_id"
```

按消息全链路：

```logql
{system="$system",service="$service",env="$env"} | json | message_id="$message_id"
```

## 5. 验收

1. 执行一次插件调用，实时流可见插件日志。
2. 制造一次失败，错误趋势和 TopN 有变化。
3. 用 `plugin_id + message_id` 能回放完整调用日志链路。
