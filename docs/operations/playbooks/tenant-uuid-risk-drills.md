# Tenant UUID-only 风险演练手册

> 目的：为 T8.21 “风险事件响应演练” 提供统一操作指南，保证桌面演练、技术演练、沟通演练可重复执行，并沉淀标准模板。

## 1. 演练概览

| 演练类型 | 目标 | 频率 | 负责人 |
| --- | --- | --- | --- |
| 桌面演练（Tabletop） | 验证跨团队沟通与回滚决策流程 | 每季度一次 | SRE（@mia） |
| 技术演练 | 实际触发 `tenant_uuid_schema_drift` 告警并执行回滚脚本 | 每半年一次 | DBA（@kevin）+ Ops（@zoe） |
| 沟通演练 | 验证对外公告/客户触达流程 | 每半年一次，与技术演练同周 | CS Ops（@claire）+ PR（@dawn） |

### 前置条件

- `PX_ENV`、`DB_URL`、`PROM_URL`、`SLACK_WEBHOOK` 等环境变量在 staging 均已配置。
- `scripts/ops/tenant_header_switch.sh`、`scripts/ops/tenant-uuid-schema-drift.sh`、`scripts/ops/tenant-id-cleanup.sh` 经过最新版本测试。
- Grafana Dashboard `Tenant UUID-only Execution` 已导入（`grafana/powerx/tenant-uuid-ga.json`）。
- 复盘模板：`postmortem/tenant-uuid-drill-template.md`。

## 2. 桌面演练脚本

**场景**：大量 legacy 客户突然发送 `X-Tenant-ID` 导致请求被拒绝，需要在 30 分钟内决策是否回滚。

1. **准备阶段（T-3 天）**
   - SRE 发送日历邀请（SRE、Ops、DBA、CS、Legal、Product、Comms）。
   - Ops 预热脚本，确认 staging 具备 `uuid-only` 配置。
   - CS/Legal 预读 FAQ（`docs/releases/tenant-uuid-announcement.md`）并带来最新客户名单。
2. **演练当天**
   - SRE 展示触发条件截图（Grafana `tenant_header_reject_total` > 0、Alertmanager 告警链接）。
   - 依照以下决策树讨论：
     1. 评估影响范围（CS 提供客户列表 + SLA）。
     2. 确认技术手段（Ops 是否可在 15 分钟内执行 `allow-legacy`）。
     3. 决定是否回滚或继续观察（Product + Legal）。
   - 记录所有决策、风险、后续动作。
3. **验收**
   - 15 分钟内完成信息收集与初步决策。
   - 30 分钟内给出沟通草案（Slack 模板 + 邮件），并明确执行人。
4. **输出**
   - 使用 `postmortem/tenant-uuid-drill-template.md` 填写场景、时间线、跟进项。
   - 在 `reports/tenant-uuid-weekly.md#drills` 登记演练结果。

## 3. 技术演练（Schema Drift）

**目标**：验证 `tenant_uuid_schema_drift` 告警链路与回滚脚本。

1. **注入异常**
   - 在 staging DB 执行：
     ```sql
     ALTER TABLE workflow_definitions ADD COLUMN tenant_id bigint;
     ```
   - 运行 `scripts/ops/tenant-uuid-schema-drift.sh --output tmp/tenant-uuid-schema-drift.prom` 并 push 至 Prometheus textfile 目录。
2. **验证告警**
   - 等待 `tenant_uuid_schema_drift > 0` 告警触发（≤5 分钟）。
   - 截图 Alertmanager 与 Grafana 面板，附加日志输出。
3. **回滚操作**
   - 执行 `scripts/migrations/tenant-uuid/999_drop_tenant_id_columns.sql --down=false --tables=workflow_definitions` 恢复现场。
   - 再次运行 drift 脚本确保指标归零。
   - 运行 `scripts/ops/tenant-id-cleanup.sh status` 验证。
4. **验收标准**
   - 全程 ≤45 分钟。
   - 所有命令日志上传到 `tmp/reports/tenant-uuid-drill-<date>.log`。
   - Postmortem 中记录执行耗时、阻塞点、改进项。

## 4. 沟通演练

**目标**：在模拟事故中 20 分钟内完成客户/内部公告草案。

1. **触发条件**：沿用桌面演练场景或技术演练输出，由 CS/PR 接棒。
2. **步骤**
   - CS 根据 `docs/releases/tenant-uuid-announcement.md` 选择合适模板（预告/正式）。
   - PR 在 `#tenant-uuid-migration` 发布内部通告草稿，并准备 press FAQ。
   - Legal 审核内容后通过 Slack Thread 确认（≤10 分钟）。
   - CS 在 Gainsight/HubSpot 创建“模拟通知”记录。
3. **验收**
   - 模板字段（受影响接口、回滚方式、支持联系人）填写完整。
   - Gainsight/HubSpot 中有演练记录并链接至 Postmortem。

## 5. 复盘与文档

- 每次演练后必须填写 `postmortem/tenant-uuid-drill-template.md`，命名为 `postmortem/tenant-uuid-drill-YYYYMMDD.md`。
- 在 `reports/tenant-uuid-weekly.md` 的 **#drills** 章节引用复盘链接。
- 若演练暴露脚本缺陷，立即在 `projects/tenant-uuid/board.md` 创建条目并标记状态。

## 6. Checklist

| 项目 | 描述 | 负责人 | 状态 |
| --- | --- | --- | --- |
| ✅ 手册准备 | 本文档发布，链接写入 `tmp/tenant-id-migration-plan.md#T8.21` | Core Platform | 完成 |
| ☐ 桌面演练记录 | 最近一次演练的 Postmortem 链接 | SRE | 待执行 |
| ☐ 技术演练记录 | Schema drift 演练 Postmortem | DBA | 待执行 |
| ☐ 沟通演练记录 | 客户沟通演练 Postmortem | CS Ops | 待执行 |
