# 03. Plugins Dashboard 设计与配置

## 0. 先说清楚：Plugins 看板看什么

Plugins 看板用于定位“哪个插件、影响哪个租户、哪条链路失败”。

- `service`（Loki label）= 日志来源
- `plugin_id`（日志字段）= 插件维度
- `tenant_uuid`（日志字段）= 影响范围
- `trace_id`（日志字段）= 精确排障

## 1. 设计目标

回答三个问题：
- 哪个插件错误最多？
- 哪些租户被插件故障影响？
- 能否快速回放到单条 trace？

## 2. 覆盖场景

- 插件能力调用失败（403/404/502）
- 插件 probe 失败
- 插件策略下发失败

## 3. 配置前提

日志里建议包含：
- `plugin_id`
- `tenant_uuid`
- `trace_id`
- `status`/`outcome`
- `message`

## 4. Grafana 里怎么配置（一步一步）

### 4.1 新建看板

1. Grafana -> `Dashboards` -> `New` -> `New dashboard`
2. `Add visualization`，数据源选 Loki
3. 保存看板名：`PowerX-Plugins-Overview`

### 4.2 配置变量

在 `Dashboard settings` -> `Variables` 添加：

1. `service`
- Type: `Query`
- Query: `label_values(service)`

2. `env`
- Type: `Query`
- Query: `label_values(env)`

## 5. 面板清单（推荐最小集）

1) 插件错误排行（按 plugin_id）
```logql
topk(10, sum by (plugin_id) (count_over_time({service=~"$service",env=~"$env"} | json | plugin_id!="" | level=~"error|warn" [15m])))
```

2) 插件请求状态分布
```logql
sum by (plugin_id, status) (count_over_time({service=~"$service",env=~"$env"} | json | plugin_id!="" | status!="" [10m]))
```

3) 受影响租户排行
```logql
topk(10, sum by (tenant_uuid) (count_over_time({service=~"$service",env=~"$env"} | json | plugin_id!="" | level=~"error|warn" [30m])))
```

4) 插件日志实时流
```logql
{service=~"$service",env=~"$env"} | json | plugin_id!=""
```

## 6. Drilldown 查询模板

- 指定插件：
```logql
{service=~"$service",env=~"$env"} | json | plugin_id="com.powerx.plugins.ai-craft"
```

- 插件失败事件：
```logql
{service=~"$service",env=~"$env"} | json | plugin_id!="" | outcome=~"FAILED|failed"
```

- 插件 + trace：
```logql
{service=~"$service",env=~"$env"} | json | plugin_id="com.powerx.plugins.ai-craft" |= "YOUR_TRACE_ID"
```

## 7. 验证步骤

1. 在 PowerX 页面执行一次插件 Probe/策略下发。
2. Explore 执行：
```logql
{service=~".+"} | json | plugin_id!=""
```
3. 看板中插件排行应出现目标插件。
4. 复制一条 `trace_id` 回放单链路，验证可定位。
