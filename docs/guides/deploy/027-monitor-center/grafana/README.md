# Grafana 监控看板（PowerX）

本目录按排障链路拆分为三份：

1. `01-http-dashboard.md`：接口请求与错误定位
2. `02-jobs-dashboard.md`：任务执行与失败定位
3. `03-plugins-dashboard.md`：插件运行与故障定位
4. `04-log-label-fields-spec.md`：日志标签与字段埋点规范（PowerX/插件统一）

建议阅读顺序：
- 先看 HTTP（最常见故障入口）
- 再看 Jobs（异步任务链路）
- 最后看 Plugins（插件能力链路）

统一前提：
- Loki 已有数据（例如 `{service=~".+"}` 能查到）
- 日志建议包含字段：`trace_id/request_id/tenant_uuid/plugin_id/job_id/status/latency_ms/level/message`
