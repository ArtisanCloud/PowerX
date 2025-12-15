# Tenant UUID 季度巡检 Checklist

> 每季度第一个工作周由 Ops/Observability 执行，并在 `reports/tenant-uuid-weekly.md#quarterly-checks` 记录结果。

## 1. 环境配置审计
运行 `scripts/ops/env-audit.sh --env-file <path>`（或在目标环境中直接执行）检查以下变量：

| 变量 | 期望值 | 说明 | 结果 |
| --- | --- | --- | --- |
| `PX_HEADER_UUID_ONLY` | `true` / `1` | 必须启用 UUID-only | |
| `PX_ALLOW_TENANT_ID_HEADER` | unset / `false` | 不得重新打开旧 header | |
| `PX_TENANT_COMPAT_MODE` | unset | 任何兼容模式需被禁用 | |
| `TENANT_TABLE` | `public.iam_tenant` 或实际表 | 确认脚本指向正确目录 | |

## 2. 数据层巡检
| 检查项 | 命令 | 结果 |
| --- | --- | --- |
| Schema Drift | `scripts/ops/tenant-uuid-schema-drift.sh --output backend/reports/tenant-uuid-schema-drift.prom` | |
| Backfill 报告 | `scripts/ops/tenant-id-cleanup.sh status` | |
| 随机抽样 | `psql -c "SELECT tenant_uuid FROM <table> LIMIT 5"` | |

## 3. 流量 & 指标
| 检查项 | 命令/路径 | 结果 |
| --- | --- | --- |
| Legacy Header 日志 | `scripts/ops/tenant-uuid-traffic-logcheck.sh --path <logdir> --summary-only` | |
| Prometheus 报表 | `scripts/ops/tenant-uuid-telemetry.sh --range 90d --output tmp/reports/tenant-uuid-telemetry-q.md` | |
| Grafana 仪表盘截图 | `grafana/powerx/tenant-uuid-ga.json` | |

## 4. 文档/流程复核
| 文档 | 行动 | 结果 |
| --- | --- | --- |
| `docs/operations/tenant-uuid-upgrade.md` | 确认终态回滚章节与最新脚本一致 | |
| `docs/operations/change-management.md` | 确认流程/模板无缺项 | |
| `docs/projects/tenant-uuid-ga.md` | 更新“长期维护”段落中的负责人/日程 | |

## 5. 输出
- 更新 `reports/tenant-uuid-weekly.md` 中对应周的 #quarterly-checks。
- 若发现问题，在 `projects/tenant-uuid/board.md` 创建任务并附本 checklist。
