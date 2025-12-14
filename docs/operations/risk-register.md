# Operations Risk Register

## Tenant UUID-only

| 风险 | 概率 | 影响 | 缓解 | Owner |
| --- | --- | --- | --- | --- |
| schema 迁移回填失败 | M | H | 先在 staging dry-run，持久化 backup，失败时执行 rollback | @kevin |
| legacy header 流量突增 | L | H | 启用告警 + 桌面演练，必要时临时开放 allow 列表 | @lily |
| CLI/SDK 升级滞后 | M | M | 联合 DevRel 发布公告 + 监控下载版本 | @nova |
| pg_checksums 权限不足导致验收延迟 | M | M | 预留 DBA 轮值窗口并在备份库执行校验，结果记录到 `reports/tenant-uuid-ga-weekly.md#schema` | @kevin |
| 观测指标被 OTEL/日志管线截断 | L | M | `tenant_uuid_only_request_total`、`tenant_header_reject_total` 通过 Prometheus Alertmanager 兜底，并在 `otelcol` attributes processor 中 drop `tenant_id` | @zoe |
| 外部伙伴升级计划缺失 | M | H | 建立 `crm/notes/tenant-uuid-ga.md` checklist + 周报，必要时提供临时映射文件并设定截止日期 | @claire |
