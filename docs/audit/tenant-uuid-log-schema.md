# Audit Log Schema（Tenant UUID-only）

> 用于 T8.12「审计记录」任务。描述 audit log 仅记录 `tenant_uuid` 的要求，以及导出/检索流程。

## 1. Schema 要求
- 所有 audit/event 记录必须包含 `tenant_uuid` 字段，类型为 `uuid` 或 `varchar`。
- 若历史表仍含 `tenant_id`，需保留但设置 `NULL` 或弃用；对外导出与 API 仅返回 `tenant_uuid`。
- 示例列：`tenant_uuid`, `actor_uuid`, `action`, `resource`, `payload`, `created_at`.

## 2. Migration 提示
- 在 `audit` 表上执行：
  ```sql
  ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS tenant_uuid uuid;
  UPDATE audit_events SET tenant_uuid = tenants.uuid
    FROM tenants WHERE audit_events.tenant_id = tenants.id AND audit_events.tenant_uuid IS NULL;
  ```
- 一旦确认填充完成，可考虑删除 `tenant_id` 列或在导出时隐藏。

## 3. 导出/检索
- 查询示例：
  ```sql
  SELECT * FROM audit_events WHERE tenant_uuid = $1 ORDER BY created_at DESC LIMIT 100;
  ```
- 若 UI 允许输入旧 `tenant_id`，需弹提示：“请使用 tenant_uuid，可通过 `scripts/ops/export-tenant-mapping.sh` 导出的映射表查询。”并提供脚本/文档链接。

## 4. 校验
- 在 CI/验收环境执行 `scripts/ci/check-no-tenant-id.sh` 与 `scripts/ci/check-tenant-uuid-canonical.sh`，确保既无新增 `tenant_id`，也不存在手动解析租户 UUID 的 handler/service。
- 在 `tmp/tenant-uuid-schema-completeness.md` spot-check audit 表。

## 5. 参考
- `tmp/tenant-id-migration-plan.md` T8.12
- `docs/operations/playbooks/tenant-uuid-upgrade.md`
