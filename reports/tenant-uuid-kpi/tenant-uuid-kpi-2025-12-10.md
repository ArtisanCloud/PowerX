# Tenant UUID KPI 报告 (${timestamp})

- Prometheus：${PROM_URL}
- increase 统计范围：${RANGE}
- rate 统计窗口：${RATE_WINDOW}

## 指标速览
| 指标 | 当前值 | ${RANGE} 累计 | 目标 | 告警阈值 |
| --- | --- | --- | --- | --- |
| UUID-only 请求占比 | ${uuid_ratio_display} （有效 ${uuid_rate_fmt}/s） | 成功 ${uuid_increase_fmt} 次 | 100% | < 99.5% 连续 5 分钟 |
| Legacy header 拒绝 | ${reject_rate_fmt}/s | ${reject_increase_fmt} 次 | 0 | > 0 |
| Schema Drift 表计数 | ${schema_drift_fmt} | - | 0 | > 0 |
| 缺少 tenant_uuid 的表 | ${tables_without_uuid_fmt} | - | 0 | > 0 |

## PromQL 参考
- UUID-only rate：`sum(rate(tenant_uuid_only_request_total[${RATE_WINDOW}]))`
- Legacy header rejects：`sum(rate(tenant_header_reject_total[${RATE_WINDOW}]))`
- Schema drift：`sum(tenant_uuid_schema_drift)`
- Tables without tenant_uuid：`sum(tenant_uuid_tables_without_uuid)`
- Legacy header累计：`increase(tenant_header_reject_total[${RANGE}])`

> 生成脚本：`scripts/ops/tenant-uuid-kpi-report.sh`，支持 `PROM_URL`、`PROM_BEARER`、`PROM_USER` 等参数。若需周报可结合 `cron`，把输出保存到 `reports/tenant-uuid-kpi/` 并归档。
