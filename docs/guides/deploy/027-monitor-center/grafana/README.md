# Grafana 监控看板（PowerX）

本目录已按新的分层标签策略统一更新。

## 1. 全局标签策略（唯一标准）

- 只用以下 Loki labels 做聚合和变量：
  - `system`
  - `service`
  - `env`
  - `instance`
  - `module`
- 可选业务维度（若日志中已做 label）：
  - `tenant_uuid`
  - `plugin_id`
- `level` 在当前实现中按日志字段过滤，不作为 label 下拉变量查询。
- `job` 当前不作为统一 label；建议用 `Text box` 变量做模糊/关键字筛选。
- 禁止再用旧口径 `service_name` 作为看板主变量。

## 2. 文档清单

1. `01-http-dashboard.md`：HTTP/API 请求质量与错误定位
2. `02-jobs-dashboard.md`：异步任务与调度运行监控
3. `03-plugins-dashboard.md`：插件调用链路与故障定位
4. `04-log-label-fields-spec.md`：日志标签与字段埋点规范

## 3. 推荐实施顺序

1. 先完成变量统一（`system/service/env/instance/module/tenant_uuid/plugin_id/job` + `level` 自定义变量）。
2. 再按 01/02/03 三份文档创建看板与面板。
3. 最后用 `trace_id/request_id/tenant_uuid/session_id/message_id` 做 Drilldown 验证。

## 4. 最小自检

在 Grafana Explore 先执行：

```logql
{system="powerx"}
```

能查到日志后，再开始配置看板。
