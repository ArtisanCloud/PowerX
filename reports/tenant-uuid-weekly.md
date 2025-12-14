# Tenant UUID Weekly Report

- 周期：2025-W49
- 撰写人：Codex
- 日期：2025-12-05

## 1. 本周进展
- [x] T8.1 数据层收尾：`scripts/migrations/tenant-uuid/*` + `scripts/ops/tenant-id-cleanup.sh` ready，等待 staging dry-run
- [x] T8.2 代码层清理：完成 `tools/tenant-uuid-scan` CLI 并生成 `tmp/tenant-id-report.md`（TOP 30 命中已出），待指派 Owner 批量整改
- [ ] T8.3 中间件/配置：未开始（下周聚焦 PX header flag 清理）
- [ ] T8.4 CLI/文档：DevRel 仍在准备 CLI 帮助截图
- [ ] T8.5 测试体系：tenantuuid helper landed，resolver stub 移除计划排期中
- [x] 其他：`pkg/testing/tenantuuid` helper、Prom telemetry 脚本落地
- [x] 其他：CI `scripts/ci/check-no-tenant-id.sh` 新增 `tenant_id IS NOT DISTINCT FROM`/`IN()/<>` 检查，diff 中出现立即拒绝

## 2. 风险与阻塞
| 风险 | 影响 | Owner | 状态 |
| --- | --- | --- | --- |
| staging 尚未验证 tenant-id-cleanup.sh | 数据迁移无法进入生产窗口 | @kevin | 需排期 |
| IAM/Agent 等文件仍存在 40+ `tenant_id`（见 tmp/tenant-id-report.md） | 代码清理周期延长 | @backend-apps | 待拆分任务 |
| Prom 指标查询需网络/凭证 | 无法生成 telemetry 报告 | @ops | 需提供 token |

## 3. 指标快照
- `tenant_header_reject_total` (本周): N/A（等待 telemetry 脚本在 Prom 上运行）
- `tenant_uuid_schema_drift` 告警: 未采集（需 `PROM_URL`）
- CLI/Web Admin 升级率: N/A（DevRel 未反馈）

## 4. 下周计划
- 在 staging 运行 `tenant-id-cleanup.sh run --skip-backup` 并记录输出
- 指派 TOP 10 `tenant_id` 命中文件给各域 owner，安排清理时间表

## 5. 需要协助
- Prom/Grafana 凭证（@ops）
- IAM & Agent 团队提供清理 ETA（@backend-apps）

## 6. Communications / Feedback（模板）
- **公告/邮件链接**：_待填_（参见 `docs/releases/tenant-uuid-announcement.md`）
- **Partner 进度**：详见 `crm/notes/tenant-uuid-ga.md`
- **客户/伙伴反馈摘要**：
  | 来源 | 级别 | 问题/建议 | Owner | 状态 |
  | --- | --- | --- | --- | --- |
  |  |  |  |  |  |
