# Tenant UUID GA 资源索引

本页聚合 T8 终态清理的全部交付物、脚本与报告，便于跨团队自助查阅。

## 快速链接
- 迁移计划：`tmp/tenant-id-migration-plan.md`
- 项目看板：`projects/tenant-uuid/board.md`
- 迁移脚本：`scripts/migrations/tenant-uuid/`
- 运维脚本：`scripts/ops/tenant-id-cleanup.sh`
- 校验 SQL：`scripts/ops/checks/tenant_uuid_consistency.sql`
- Legacy 扫描工具：`go run ./tools/tenant-uuid-scan`
- Telemetry 报告：`scripts/ops/tenant-uuid-telemetry.sh`
- CI 检查：`scripts/ci/check-no-tenant-id.sh` / `scripts/ci/check-tenant-uuid-canonical.sh`（配合 Danger/CI 阻断新增 tenant_id & 手动解析）
- 风险登记：`docs/operations/risk-register.md`
- 周报：`reports/tenant-uuid-weekly.md`
- 里程碑：`reports/tenant-uuid-ga-milestones.csv`

## 操作手册摘要
1. **数据层**：按 README 指南运行 `tenant-id-cleanup.sh plan` → `run`，在 staging 验证无误后再于生产执行。
2. **代码层**：CI 新增的 `scripts/migrations/check-tenant-migrations` 以及 Danger 规则负责阻止新 `tenant_id` 字段。
3. **观测性**：Grafana Dashboard `Tenant UUID GA KPIs` 聚合 `tenant_header_reject_total`、`tenant_uuid_only_request_total` 等指标。
4. **回滚**：`tenant-id-cleanup.sh rollback <schema.sql>` + `docs/operations/tenant-uuid-upgrade.md#回滚` 提供完整步骤。

## 文档/报告留档
| 类型 | 路径 | 说明 |
| --- | --- | --- |
| Playbook | `docs/operations/tenant-uuid-upgrade.md` | 灰度、切换、回滚操作 |
| KPI | `metrics/tenant-uuid-kpi.md` | 成功指标定义与告警阈值 |
| Drill Postmortem | `postmortem/tenant-uuid-drill-*.md` | 演练复盘 |
| GA Postmortem | `postmortem/tenant-uuid-ga.md` | 正式 GA 复盘 |

如需新增资料，请在 PR 中同步更新本页。

## 长期维护 / T8.23

| 主题 | 交付物 | 说明 |
| --- | --- | --- |
| 年度审计 | `audit/tenant-uuid/annual-review-template.md` | Q1 审计模板，涵盖 CI/Schema/Telemetry/Traffic 检查与整改表 |
| 季度巡检 | `docs/operations/checklists/tenant-uuid-quarterly.md` + `scripts/ops/env-audit.sh` | Ops 使用 checklist + env audit 脚本验证环境变量、Schema Drift、指标与文档 |
| 技术债评审 | 本页（段落） | 每半年 Architecture Review Board 复盘 `tenant_uuid` 相关技术债，更新 `projects/tenant-uuid/board.md` backlog 并在此记录决议 |
| 参考实现 | `examples/tenant-uuid-only/README.md` | 最小实践/伪代码示例，供新服务/脚手架遵循 |
| 策略监控 | 记录在 `docs/operations/change-management.md` + 发布周报 | Product Strategy 每季度同步政策变化，将新的要求写入 change 管理流程并更新 `tmp/tenant-id-migration-plan.md` |

> **技术债评审流程**：每年 Q2/Q4 的 Architecture Review 会议添加 “Tenant Identity” 议题，对照上述审计/巡检结果决定是否需要新增 schema 改造或文档更新；会议纪要请链接回本页。

## Issue 标签与 Burndown

- 标签创建（一次性）：
  ```bash
  gh label create tenant-uuid-ga \
    --color 0E8A16 \
    --description "Tracking for Tenant UUID GA tasks"
  ```
- CI 守卫：`.github/workflows/tenant-uuid-label-check.yml` 会在 PR 修改任意 `tenant*` 文件时强制要求 `tenant-uuid-ga` 标签，无需人工逐个检查。
- Burndown 快照：`scripts/ops/tenant-uuid-burndown.mjs` 支持本地或 CI 统计带标签 Issue 总量；`.github/workflows/tenant-uuid-burndown.yml` 每周一 01:00 UTC 运行并在 artifacts 中输出 `tmp/reports/tenant-uuid-burndown.json`，可粘贴到 `tmp/reports/tenant-uuid-ga-weekly.md` 或导入 Grafana Synthetic 数据源。
