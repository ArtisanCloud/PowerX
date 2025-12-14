# Tenant UUID-only Workshop（Ops / Support）

## 目标
1. 熟悉切换/回滚流程、关键脚本与告警。
2. 演练常见客户问题（旧 header、CLI 版本、Schema Drift）。
3. 确保 Support Portal / On-call Runbook 已更新，能在 15 分钟内定位问题。

## 课程大纲
| 模块 | 内容 | Demo/素材 |
| --- | --- | --- |
| Playbook 演示 | `docs/operations/playbooks/tenant-uuid-upgrade.md`：切换、回滚、演练清单 | staging 重演 `tenant-id-cleanup.sh run --drop` |
| 观测与指标 | Grafana `tenant_uuid_ga` Dashboard、`scripts/ops/tenant-uuid-telemetry.sh` 报告 | 直接在 demo Grafana 讲解 |
| Support 排障 | 工单模板、`docs/support/tenant-uuid-faq.md`、`scripts/ops/tenant-uuid-schema-drift.sh` | 现场演练 400/403/Schema drift 三个场景 |
| 沟通流程 | 借助 `docs/releases/tenant-uuid-announcement.md`、CRM note 追踪客户状态 | 讲解 Gainsight/HubSpot 记录要求 |
| 升级状态追踪 | `tenant-uuid-burndown` artifact + `projects/tenant-uuid/board.md` | 如何在值班时快速掌握剩余任务 |

## 关键动作
- On-call Runbook：将本课程 PPT/录屏链接添加到 `docs/support/tenant-uuid-faq.md` “内部资源”段落。
- Support Portal：发布 FAQ 并 pin 到“Known Issues / UUID-only” 分类。
- 值班演练：按照 Playbook 在 staging 完成一次完整切换 + 回滚（记录到 `reports/tenant-uuid-weekly.md#playbook`）。
