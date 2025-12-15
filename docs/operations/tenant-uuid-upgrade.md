# Tenant UUID 升级指南（外部通知版）

## 背景
PowerX 统一以 UUID 识别租户。旧的 `X-Tenant-ID`/`tenant_id` 将在 2025Q2 后完全下线，为避免客户端/插件出现 403/400，本指南提供升级步骤与时间表。

## 里程碑
| 时间 | 内容 | 影响 |
| --- | --- | --- |
| 2025-01 ~ 2025-02 | Web Admin、Bridge、CLI 切换到 `X-Tenant-UUID` | 客户端已默认使用 UUID，仅记录 fallback 指标 |
| 2025-03 | 关闭所有 fallback，观测面板聚焦 UUID-only 告警 | Ops 仅需监控指标，不再支持手动降级 |
| 2025-04 | 生产启用 UUID-only，拒绝旧 header | 继续出现 `X-Tenant-ID` 的请求将被 400 拒绝 |

## 必要操作
1. **Web Admin / 浏览器插件**：确保使用最新 `powerx-bridge-client.js`，监听 `auth-token` 消息中的 `tenant_uuid` 并写入 LocalStorage。无需再从 URL/旧 cookie 读取 `tenant_id`。
2. **PowerX CLI**：
   - `px plugin dev watch`, `px publish create`, `px plugin import`、`px version *` 仅接受 `--tenant-uuid`，若仍传入 `--tenant-id` 将立即报错；请更新脚本与自动化任务。
   - 自定义脚本可读取 `PX_TENANT_UUID` 环境变量或 `~/.powerx/credentials.json` 中的 `tenant_uuid` 字段。
3. **第三方集成/SDK**：
   - 所有 HTTP/gRPC 请求请改为 `X-Tenant-UUID`（Header）或 `tenant_uuid`（Body/Query/Proto）。
   - 需要数值 ID 时，通过新提供的 resolver API（`GET /api/admin/tenants/:tenant_uuid`）查询。
4. **插件开发者广播**：建议在插件 README、auto-updater 中加入以下提醒：
   > PowerX 平台将于 2025-04 起拒绝 `X-Tenant-ID` header，请升级到 `powerx-bridge-client.js@>=0.9` 或 `px-plugin@>=1.8`，并始终透传 `tenant_uuid`。

## 观测与回退
- 指标：`tenant_header_reject_total`（所有旧 header 被拒绝时累加）、`tenant_uuid_only_request_total`（JWT 中间件确认的合法请求率）、`tenant_uuid_schema_drift`/`tenant_uuid_tables_without_uuid`（由 Schema Drift 脚本生成的 gauge）。
- Grafana：导入 `grafana/powerx/tenant-uuid-ga.json` 得到「Tenant UUID GA KPIs」仪表盘，包含请求率、拒绝率、Drift 统计及 Markdown Runbook，建议绑定 `${DS_PROMETHEUS}` → `Prometheus` 数据源，刷新频率 1 分钟。
- Textfile Collector：将 `scripts/ops/tenant-uuid-schema-drift.sh --textfile /var/lib/node_exporter/textfile_collector/tenant-uuid.prom` 作为 cronjob（建议 5 分钟）运行，node_exporter 将采集并暴露 `tenant_uuid_schema_drift`/`tenant_uuid_tables_without_uuid`，Grafana Panel 会实时更新。
- Prometheus 报告：`PROM_URL=<prom> scripts/ops/tenant-uuid-telemetry.sh --range 7d --output tmp/reports/tenant-uuid-telemetry.md` 可一次性验证过去 7 天的 `tenant_header_reject_total`/`tenant_uuid_only_request_total`。
- Ingress 日志：`scripts/ops/tenant-uuid-traffic-logcheck.sh --path /var/log/nginx/access.log --summary-only` 可快速统计 `X-Tenant-ID`、`X-PowerX-Tenant` 仍是否出现，并把输出附在 `tmp/reports/tenant-uuid-ga-traffic.md`。
- OpenTelemetry：在 `otelcol` pipeline 中加入 attributes processor，若 trace/metric attribute 含 `tenant_id` 则 `action: delete` 并通过 `log` exporter写入告警，以保证下游 trace 仅携带 `tenant_uuid`。
- 由于 fallback 逻辑已删除，出现告警只能通过**回滚**到上一版本或修复客户端后重试，不再支持临时打开 `PX_ALLOW_TENANT_ID_HEADER` 之类的开关。

## 终态执行步骤
1. **发布前检查**
   - 在 staging 运行 `scripts/ops/tenant-id-cleanup.sh run --dry-run`，确认所有 `tenant_uuid` 列存在且 `tenant_uuid_consistency.sql` 无异常。
   - 通过 `scripts/ops/tenant-uuid-telemetry.sh` 或 Grafana 面板确认 `tenant_header_reject_total` 为 0。
   - 运行 `scripts/ops/tenant-uuid-schema-drift.sh` 生成 `tenant_uuid_schema_drift` Textfile 指标（默认写入 `backend/reports/tenant-uuid-schema-drift.prom`），并确保该 gauge 为 0，以便 Grafana Schema Drift 面板同步更新。
2. **生产启用**
   - 合并移除 `X-Tenant-ID` fallback 的版本，确认应用配置仅保留 `tenants.require_uuid = true`。
   - 在发布窗口密切关注 `tenant_header_reject_total`、API 4xx/5xx 指标以及 `ops/#tenant-uuid-migration` 告警。
   - 若需回滚，可使用 `scripts/ops/tenant-id-cleanup.sh rollback <backup.sql>` 恢复最近一次 `run` 命令生成的 schema 备份；数据库变更需遵循 `docs/operations/change-management.md` 流程。

## 紧急回滚指南
若生产环境仍存在大量旧客户端导致 400：
1. **暂停流量**：通过流量管理或 Ingress 将受影响租户导流到旧版本（切换到上一稳定 tag）。
2. **恢复 Schema（如需）**：如果已执行 `scripts/migrations/tenant-uuid/999_drop_tenant_id_columns.sql`，可在独立连接中运行逆向操作：
   ```sql
   ALTER TABLE ... ADD COLUMN tenant_id bigint;
   UPDATE ... SET tenant_id = tenants.id FROM tenants WHERE ...;
   ```
   建议提前保留 `pg_dump --schema-only`，便于对照。
3. **回滚应用**：重新部署包含 `tenantresolver` 的上一版本，并在告警渠道同步“UUID-only 暂缓”消息。
4. **复盘**：定位仍发送 `X-Tenant-ID` 的客户端，要求其升级后再重新尝试 UUID-only。

## 终态回滚（Phase-3 专用）
当已经完成 schema 清理（执行 `999_drop_tenant_id_columns.sql`、移除 fallback）后，如需“全量回退”请按以下步骤操作：

1. **前置条件**  
   - 确认 Ops 已在 `#tenant-uuid-migration` 宣布进入回滚流程，阻断新的发布。  
   - 复核最近一次 `pg_dump --schema-only` 备份和 `tenant_uuid_consistency.sql` 报告均可用。
2. **数据库恢复**  
   ```bash
   # 1) 恢复 tenant_id 列
   psql "$DB_URL" -f scripts/migrations/tenant-uuid/999_drop_tenant_id_columns.sql --set ON_ERROR_STOP=1 --set direction=down

   # 2) 回填数值 ID（可按租户或全量）
   psql "$DB_URL" -f scripts/migrations/tenant-uuid/002_backfill_tenant_uuid.sql --set backfill_direction=reverse

   # 3) 校验
   psql "$DB_URL" -f scripts/ops/checks/tenant_uuid_consistency.sql
   ```
   所有操作需记录在 `reports/tenant-uuid-ga-weekly.md#rollback`，并附上耗时 / 影响范围。
3. **应用与配置**  
   - 回滚至上一稳定版本（含 `tenantresolver`），部署后立刻执行 `scripts/ops/tenant_header_switch.sh allow-legacy`。  
   - 重新启用 `PX_ALLOW_TENANT_ID_HEADER=true`、`PX_HEADER_UUID_ONLY=false`（若配置仍在），并重启 API / gRPC 入口。  
   - 验证 HTTP/gRPC 请求可携带 `X-Tenant-ID` 并正常解析。
4. **监控与沟通**  
   - 重点监控 `tenant_header_reject_total`、`tenant_uuid_only_request_total`，确保 legacy 请求数量下降。  
   - 在 `#announcements`、客户/合作伙伴渠道发布“UUID-only 暂缓”通知，明确预计恢复时间与联系人。
5. **复盘 & 再次推进**  
   - 收集触发原因、未升级客户名单，写入 `postmortem/tenant-uuid-drill-<date>.md`。  
   - 制定重新推进 UUID-only 的计划（补充培训/沟通、设置硬性截止），并在本指南“终态执行步骤”更新新的时间线。

## FAQ
**Q: 仍需兼容旧版本 CLI 吗？**  
A: 不再兼容。旧版本仍然使用 `--tenant-id`/`PX_TENANT_ID` 的命令会被 CLI 直接拒绝，必须升级到仅支持 UUID 的版本。

**Q: 是否支持多租户切换？**  
A: Web Admin 仍可在 Membership 列表中切换租户，Bridge 会在 `postMessage` 中附带最新 `tenant_uuid`。第三方 iframe 如需感知切换，监听 `powerx.auth-token` 事件。

**Q: gRPC proto 何时同步？**  
A: `knowledge`, `workflow`, `plugin_release`, `agent_model_hub` proto 已经使用 `tenant_uuid` 字段，发布 `buf` 包含 breaking change notifier。请重新生成客户端 stub。

## 终态变更记录
| 日期 | 变更 | 影响 |
| --- | --- | --- |
| 2025-02-15 | Web Admin Bridge/SDK 默认仅广播 `tenant_uuid`，移除了 `X-Tenant-ID` postMessage 降级 | 所有 iframe/插件需要监听 `auth-token.tenant_uuid`，否则将无法获租户上下文 |
| 2025-03-01 | CLI `px 1.8` / `px-plugin 0.16` 发布，删除 `--tenant-id` flag 与 `PX_TENANT_ID` 环境变量 | 自动化脚本需改为 `--tenant-uuid` 或 `PX_TENANT_UUID`，否则命令直接失败 |
| 2025-03-15 | OpenAPI/Buf 契约（Event Fabric、Workflow、Knowledge、Agent Model Hub）统一 `tenant_uuid` 字段 | 重新生成 SDK / 客户端 stub，注意字段 rename 属 breaking change |
| 2025-04-05 | CoreX Admin/API 移除所有 Header fallback，Ingress 仅接受 `X-Tenant-UUID` | 任意残留 `X-Tenant-ID` 请求都会触发 `tenant_header_reject_total` 告警并返回 400 |
| 2025-04-12 | `scripts/ops/tenant-id-cleanup.sh` 执行 `999_drop_tenant_id_columns.sql`，完成 schema layer 清理 | 旧数据仅存 `tenant_uuid` 列，如需回滚必须按本指南“恢复 Schema” 步骤执行 |
