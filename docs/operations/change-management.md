# PowerX 数据库变更/Migration 变更流程

> 适用于 Tenant UUID-only 迁移及其他高风险数据库操作。目标：在上线前明确审批、演练、执行、回滚步骤。

## 1. 变更类型

| 类型 | 示例 | 风险等级 |
| --- | --- | --- |
| Schema 批量修改 | 执行 `001_add_tenant_uuid_columns.sql`、`002_backfill_tenant_uuid.sql` | 高 |
| 数据删减/清理 | `999_drop_tenant_id_columns.sql` 删除列 | 高 |
| 紧急回滚 | 执行 backup SQL 或 `rollback` 脚本 | 中-高 |

## 2. 流程概览

1. **提出需求**（Owner）
   - 填写变更请求（CR）：目标、影响范围、执行窗口、回滚策略。
   - 附上 SQL、自动化脚本、日志路径。
2. **风险评审**（DBA + Ops + Owner）
   - 核对依赖（TENANT_TABLE、脚本版本、CI 结果）。
   - 确认 staging 已演练，包括 `tenant_uuid_consistency` 报告。
3. **审批**（Ops Lead + DB Infra）
   - 确认窗口、负责人与旁路策略。
   - 在变更看板（如 PagerDuty/ServiceNow）记录 “Approved/Rejected”。
4. **执行与监控**
   - 按手册运行 `scripts/ops/tenant-id-cleanup.sh run --drop` 或 `run-tenant-uuid.sh`。
   - 实时在 `#tenant-uuid-migration` 更新进度（开始、阶段、完成）。
5. **回滚准备**
   - 确保 `pg_dump --schema-only` 文件或 `rollback` 脚本可用。
   - 若执行失败 ≤15 分钟无法恢复，立即切换到回滚流程。
6. **复盘**
   - 24h 内在 `reports/tenant-uuid-weekly.md`、`postmortem/tenant-uuid-ga.md` 更新记录。

## 3. 执行 Checklist

| 阶段 | 事项 | 完成者 |
| --- | --- | --- |
| 前置 | 《变更申请》已提交并获批 | Owner |
| 前置 | `make check-tenant-migrations` / `scripts/ci/check-no-tenant-id.sh` / `scripts/ci/check-tenant-uuid-canonical.sh` 通过 | Dev |
| 前置 | Staging 演练 `run --drop` + `rollback` 通过，日志记录在 `tmp/reports/` | DBA |
| 前置 | 团队沟通（CS/Ops/Legal）同步时间窗口 | PM |
| 执行 | 运行 `scripts/ops/tenant-id-cleanup.sh run --drop`（含备份） | DBA/Ops |
| 执行 | `scripts/ops/checks/tenant_uuid_consistency.sql` 结果归档 | DBA |
| 执行 | Prometheus/Grafana 监控正常（schema drift=0） | Observability |
| 回滚 | `scripts/ops/tenant-id-cleanup.sh rollback <backup.sql>` 演练通过 | DBA |
| 收尾 | 在 change log / 周报登记 | Owner |

## 4. 变更申请模板

```
标题：Tenant UUID-only Stage → Prod 回填
日期：YYYY-MM-DD HH:MM ~ HH:MM
Owner：<name>
影响范围：数据库 schema（agent/workflow/...）
执行脚本：scripts/ops/tenant-id-cleanup.sh run --drop
验证方法：scripts/ops/checks/tenant_uuid_consistency.sql、Grafana dashboard
回滚方案：使用 <backup.sql> + tenant-id-cleanup.sh rollback
沟通计划：Slack #tenant-uuid-migration + email to <list>
```

## 5. 角色与责任

| 角色 | 职责 |
| --- | --- |
| Owner | 准备变更材料、协调各团队、最终复盘 |
| DBA | 技术评估、执行脚本、回滚 |
| Ops | 监控、告警、窗口协调 |
| CS/Legal | 客户/合规通知 |
| PM/Program | 记录变更、保持看板更新 |

## 6. 参考

- `scripts/ops/tenant-id-cleanup.sh`（计划/执行/回滚）
- `scripts/migrations/run-tenant-uuid.sh`
- `docs/operations/playbooks/tenant-uuid-risk-drills.md`
- `tmp/reports/tenant-cleanup-plan.md`、`tmp/reports/tenant-cleanup-staging.md`
