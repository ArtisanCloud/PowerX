# UUID-only 常见问题（Support Portal）

## 客户常见报错

### 1. `legacy tenant header not allowed`
- **原因**：请求仍发送 `X-Tenant-ID` 或 body 中包含 `tenant_id`。
- **排查步骤**：
  1. 确认 CLI/SDK 版本：`px --version` 应为 ≥ 1.8.0；`px-plugin`/`powerx-bridge-client.js` 查看 package 版本。
  2. 检查请求日志/Browser DevTools，确认 header 是否含 `X-Tenant-UUID`。
  3. 提供升级指南链接：`docs/operations/tenant-uuid-upgrade.md`。

### 2. `tenant uuid not found`
- **原因**：客户端传入的 UUID 不存在或已注销。
- **排查步骤**：
  1. 使用 `px admin tenant lookup --tenant-uuid <uuid>` 或 Admin Console 搜索。
  2. 如为新租户，确认目录回填是否完成（参见 `scripts/migrations/tenant-uuid/002_backfill_tenant_uuid.sql`）。
  3. 若客户误传旧 ID，可在回复中提供 `tenant_uuid` 的映射方式。

### 3. Schema Drift 告警
- **症状**：Grafana 报 `tenant_uuid_schema_drift > 0` 或 `tenant_uuid_tables_without_uuid > 0`。
- **处理**：
  1. 执行 `scripts/ops/tenant-uuid-schema-drift.sh --textfile /tmp/tenant-uuid.prom` 获取具体表。
  2. 若为迁移脚本未覆盖，联系 DBA (@kevin) 安排 `tenant-id-cleanup.sh plan/run`。
  3. 告知客户这是内部告警，不影响 API；若确有影响，走 Playbook 回滚。

## 内部资源
- Playbook：`docs/operations/playbooks/tenant-uuid-upgrade.md`
- 观测面板：Grafana `Tenant UUID GA KPIs`
- 培训材料：`docs/trainings/tenant-uuid-ga/BE_FE.md`, `docs/trainings/tenant-uuid-ga/OPS_SUPPORT.md`
- 伙伴同步：`crm/notes/tenant-uuid-ga.md`

## 工单模板（建议）
```
标题：[Tenant UUID] <客户名称> <错误码>
内容：
- 环境/Region：
- CLI/SDK 版本：
- 请求 ID / Trace ID：
- 是否按升级指南操作：
- 附件：日志/截图
```

收到工单后，Support 请在 `docs/trainings/tenant-uuid-ga/attendance.csv` 中的“status”列标记 “Follow-up” 或 “Closed”，便于追踪培训效果。
