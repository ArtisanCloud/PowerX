# Tenant UUID Journey 分享会脚本

> 适用于 T8.24“分享会”交付。DevRel 主持，45 分钟，录制上传到 LMS。以下为大纲与准备事项。

## 0. 基础信息
- **主持人**：DevRel（@nova）
- **嘉宾**：Core Platform、Ops、CS、Partner Eng、Legal
- **目标**：回顾 `X-Tenant-ID → X-Tenant-UUID` 的旅程、经验与指标提升，形成跨团队共识。
- **输出**：Slides、录像、Q&A 文档。

## 1. Agenda（45 min）
| 时间 | 模块 | 负责人 | 说明 |
| --- | --- | --- | --- |
| 5 min | 开场 & Timeline | PM | 背景、里程碑、风险 |
| 10 min | 技术深潜 | Core Platform | 中间件、Schema、CI 经验 |
| 10 min | Ops/DB 视角 | Ops + DBA | 迁移脚本、监控、回滚演练 |
| 10 min | 客户成功 | CS + Partner Eng | 客户教育、FAQ、伙伴同步 |
| 5 min | 文化与激励 | People Ops | UUID Champion、培训数据 |
| 5 min | Q&A + Next Steps | DevRel | 收集问题，指向 FAQ |

## 2. 准备材料
- Slides 模板：`docs/trainings/tenant-uuid-ga/share-session-slides.pptx`（占位，可在同目录创建）
- Demo：
  - `px plugin dev watch --tenant-uuid`
  - Grafana `Tenant UUID GA KPIs` 面板
  - `scripts/ops/tenant-id-cleanup.sh plan/run/rollback` 命令演示
- 资料包：`docs/operations/tenant-uuid-upgrade.md`、`docs/support/tenant-uuid-faq.md`、`docs/culture/awards.md`。

## 3. 录制与回放
- 使用 Zoom/Meet 录制，命名为 `tenant-uuid-share-<YYYYMMDD>.mp4`。
- 上传到 LMS/Drive 后，在 `docs/trainings/tenant-uuid-ga/README.md`（可选）或周报中附链接。
- 参会名单记录在 `docs/trainings/tenant-uuid-ga/attendance.csv`，在 “session” 列标注 “share”.

## 4. Q&A 文档
- 创建 Google Doc 或 Markdown（建议：`docs/trainings/tenant-uuid-ga/share-session-qa.md`）收集问题。
- 每个问题需包含：提问人、类别（技术/客户/ops）、回答人与后续行动。

## 5. 复盘
- 分享会后一周在 `reports/tenant-uuid-weekly.md#communications` 填写总结。
- 若收集到新的需求/风险，映射到 `projects/tenant-uuid/board.md` 并在 `tmp/tenant-id-migration-plan.md` 对应条目更新状态。
