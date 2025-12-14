# Tenant UUID-only 切换 / 回滚 Playbook

> 适用对象：Ops / Support On-call。覆盖从打开 UUID-only 守卫、到发生异常时切回 legacy 模式的全流程。

## 1. 角色与前置条件

| 角色 | 职责 |
| --- | --- |
| Incident Commander（IC） | 宣布进入操作窗口、与 CS/PM 同步客户影响 |
| Database Engineer（DBE） | 执行数据校验、备份、`tenant_id` 列清理/恢复 |
| Application Engineer（AE） | 部署新版本、调整环境变量、观测 API |
| Support On-call（SOC） | 监控告警，按本手册排查客户问题 |

### 前置条件
1. `scripts/ops/tenant-id-cleanup.sh plan` 输出无 `tenant_id` 残留（日志记录到 `reports/tenant-uuid-ga-weekly.md#schema`）。
2. `scripts/ops/tenant-uuid-schema-drift.sh` 产出 `tenant_uuid_schema_drift = 0`、`tenant_uuid_tables_without_uuid = 0`，并由 node_exporter textfile collector 抓取。
3. Grafana 「Tenant UUID GA KPIs」仪表盘中 `tenant_header_reject_total`、`tenant_uuid_only_request_total` 正常，且最近 24h 没有 legacy header 请求。
4. Staging 已完成一次完整演练（参见第 4 节），并在 `reports/tenant-uuid-weekly.md#playbook` 打勾。

## 2. 切换到 UUID-only

1. **准备发布**  
   - AE 部署仅接受 `X-Tenant-UUID` 的版本，并确认 `PX_HEADER_UUID_ONLY=true`、`PX_ALLOW_TENANT_ID_HEADER=false`。  
   - DBE 在备份实例执行 `scripts/migrations/tenant-uuid/999_drop_tenant_id_columns.sql --direction=up`，输出归档。正式库在窗口期执行。
2. **执行 `tenant-id-cleanup`**  
   ```bash
   PX_ENV=prod scripts/ops/tenant-id-cleanup.sh run --drop
   ```
   - 如需跳过备份（仅限演练环境），可增加 `--skip-backup`。
3. **观测**  
   - 重点监控 Grafana 面板中 `Legacy Header 拒绝率`、API 4xx/5xx、队列 backlog。  
   - `PROM_URL=... scripts/ops/tenant-uuid-telemetry.sh --range 1h --output tmp/reports/tenant-uuid-telemetry.md`.
4. **沟通**  
   - IC 在 `#tenant-uuid-migration` 与 `#announcements` 发布“UUID-only 已启用”信息，附加回滚路径与联系人。  
   - Support 评估是否需要对重点客户逐一确认（见 `docs/operations/tenant-uuid-upgrade.md#沟通`）。

## 3. 紧急回滚

触发条件：Grafana 上 `Legacy Header 拒绝` 持续 > 0 且为关键客户、CLI/SDK 旧版本大规模失败、或 DB 校验未通过。

1. **流量侧**  
   - AE 立即回滚至上一稳定版本（仍接受 `tenant_id`）。  
   - 执行 `scripts/ops/tenant_header_switch.sh allow-legacy`，确认输出包含 `PX_ALLOW_TENANT_ID_HEADER=true`。
2. **数据库**  
   ```bash
   # 恢复 tenant_id 列
   psql "$DB_URL" -f scripts/migrations/tenant-uuid/999_drop_tenant_id_columns.sql --set direction=down --set ON_ERROR_STOP=1
   # 可选：重新运行 backfill
   psql "$DB_URL" -f scripts/migrations/tenant-uuid/002_backfill_tenant_uuid.sql --set backfill_direction=reverse
   # 校验
   psql "$DB_URL" -f scripts/ops/checks/tenant_uuid_consistency.sql
   ```
3. **监控**  
   - 确认 Grafana 中 `tenant_header_reject_total` 回落为 0。  
   - 记录 `tenant_uuid_only_request_total` 下降，以便后续复盘。
4. **沟通 & 记录**  
   - IC 向所有渠道说明“UUID-only 暂缓”，给出重新尝试时间；Support 更新工单模板。  
   - 在 `reports/tenant-uuid-weekly.md#rollback`、`postmortem/tenant-uuid-drill-<date>.md` 记录原因、影响、下一步行动。

## 4. Staging 演练模板

1. **模拟切换**：在 staging 运行 `scripts/ops/tenant-id-cleanup.sh run --drop --skip-backup`。  
2. **模拟告警**：通过注入旧版 CLI/Bridge 流量，验证 `tenant_header_reject_total` 告警能触发 PagerDuty。  
3. **模拟回滚**：执行第 3 节中的步骤（允许使用 `scripts/migrations/... --direction=down`），确保 30 分钟内恢复服务。  
4. **打分**：在 `reports/tenant-uuid-weekly.md#playbook` 填写演练日期、耗时、参与者、自评等级。

## 5. Support 排查清单

| 症状 | 处理步骤 |
| --- | --- |
| 客户 400：`legacy tenant header not allowed` | 检查其 CLI/SDK 版本，确认 `X-Tenant-UUID` 是否传入；必要时指向 `docs/operations/tenant-uuid-upgrade.md#faq` |
| 客户 403：`tenant uuid not found` | 使用 `px admin tenant lookup --tenant-uuid <uuid>` 确认目录是否存在；若无则在目录中回填 |
| Schema Drift 告警 | 运行 `scripts/ops/tenant-uuid-schema-drift.sh`，定位具体表；如确认误报，刷新 node_exporter textfile |
| 大量 `tenant_uuid_schema_drift > 0` | 停止变更，执行 `scripts/ops/tenant-id-cleanup.sh plan` 生成报告，再决定是否回滚 |

## 6. 变更记录

| 日期 | 内容 | 负责人 |
| --- | --- | --- |
| 2025-12-09 | 初版 Playbook：包含切换/回滚/演练/SOC checklist | Ops @zoe |

如需修改此文档，请同步在 `tmp/tenant-id-migration-plan.md` 更新对应条目状态。
