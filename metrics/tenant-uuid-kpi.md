# Tenant UUID KPI 定义

## KPI 表

| KPI | 指标说明 | PromQL | 目标 | 告警阈值 | Owner | 频率 |
| --- | --- | --- | --- | --- | --- | --- |
| UUID-only 请求占比 | 入口仅接受 `X-Tenant-UUID`，拒绝 legacy header | `sum(rate(tenant_uuid_only_request_total[5m])) / (sum(rate(tenant_uuid_only_request_total[5m])) + sum(rate(tenant_header_reject_total[5m])))` | 100% | < 99.5% 连续 5 分钟 | Observability @zoe | 7x24 |
| Legacy header 拒绝 | 监控旧 header 被拒次数 | `sum(rate(tenant_header_reject_total[5m]))` | 0 | > 0 | Observability @zoe | 7x24 |
| Schema Drift | 仍包含 `tenant_id` 列的表数 | `sum(tenant_uuid_schema_drift)` | 0 | > 0 | DB Infra @kevin | 每日 |
| 缺失 `tenant_uuid` 列 | 未创建 `tenant_uuid` 列的表数 | `sum(tenant_uuid_tables_without_uuid)` | 0 | > 0 | DB Infra @kevin | 每日 |
| CLI/Web Admin 升级率 | 客户端自检，仅统计 UUID-only 版本 | `sum(clients_uuid_only) / sum(clients_total)` | ≥ 95% | < 90% 持续 24h | DevRel @nova | 每日 |
| 回填缺失行 | 回填报告中 `missing_rows` 累计 | `sum(tenant_uuid_backfill_missing_rows)` | 0 | > 0 | DB Infra @kevin | 各运行窗口 |

> 说明：`tenant_uuid_backfill_missing_rows` 由 `scripts/migrations/tenant-uuid/002_backfill_tenant_uuid.sql` 写入 `tenant_uuid_backfill_report` 后同步到 Prometheus textfile（参考 `scripts/ops/tenant-uuid-schema-drift.sh`）。客户端升级率的基础指标由下载/自检日志上报至 Prometheus `clients_*` 系列。

## 告警与仪表盘

- Grafana Dashboard：`grafana/powerx/tenant-uuid-ga.json` 导入后即可获得「Tenant UUID GA KPIs」面板，包含核心 Panel、Top tenants、操作提示等组件，数据源指向 `${DS_PROMETHEUS}`。
- Alertmanager：为表格中每个 KPI 配置告警规则（示例）：
  - UUID-only 请求占比 `< 0.995` 连续 `5m` 触发 `tenant-uuid/uuid-only-low`，通知 `#tenant-uuid-alerts` + PagerDuty。
  - Schema Drift / 缺失列 > 0 立即触发，跳转到 `docs/operations/tenant-uuid-upgrade.md#观测性` 的回滚流程。
  - Legacy header 拒绝 > 0 时附带 top tenants 标签，方便 Support 介入。
- Textfile metrics：运行 `scripts/ops/tenant-uuid-schema-drift.sh --textfile /var/lib/node_exporter/textfile_collector/tenant-uuid.prom`，由 node_exporter 抓取 `tenant_uuid_schema_drift` 与 `tenant_uuid_tables_without_uuid`。

## 自动化报告

- 周/专项报表：`scripts/ops/tenant-uuid-kpi-report.sh` 会从 Prometheus 拉取 KPI，默认生成 `reports/tenant-uuid-kpi/tenant-uuid-kpi-<date>.md`，可通过 `PROM_URL`、`--range`、`--rate-window` 自定义统计范围。
- 临时 telemetry：`scripts/ops/tenant-uuid-telemetry.sh --range 24h --output reports/tenant-uuid-telemetry.md` 可输出原始时间序列，用于 `reports/tenant-uuid-weekly.md` 附录。
- 建议结合 `cron` 或 CI 每周运行 KPI Report 与 Telemetry 脚本，结果链接填入 `reports/tenant-uuid-weekly.md`，并在 `#tenant-uuid-migration` 频道同步。

## 告警规则

- PrometheusRule 示例：`metrics/tenant-uuid-alerts.yaml`，覆盖 UUID-only 比例、legacy header、schema drift、缺失 `tenant_uuid` 列、客户端升级率等指标，默认 `namespace=tenant-uuid` 并指向 `#tenant-uuid-alerts` + PagerDuty。
- 使用说明：
  1. 将 YAML 载入 Prometheus Operator（或直接由 Prometheus `rule_files` 引用）。
  2. Alertmanager route 应匹配 `namespace=tenant-uuid`，目标通知渠道：`#tenant-uuid-alerts`（Slack）与 `Tenant UUID GA` PagerDuty service。
  3. `TenantUUIDClientUpgradeLow` 依赖 `clients_uuid_only`/`clients_total` 指标，如暂未接入可在 YAML 中注释该规则。
