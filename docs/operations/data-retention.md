# Data Retention & Security（Tenant UUID-only）

> 描述 `tenant_uuid` 字段的保留、加密、脱敏与备份策略，供 Security/Ops 在 GA 验收与后续审计中引用。

## 1. 目标
- 所有涉及租户标识的数据仅保存 `tenant_uuid`。
- 备份/日志/导出中的租户标识需加密或掩码，防止泄露。
- 在删除流程中按 UUID 定位并清理。

## 2. 加密 / 脱敏
| 场景 | 要求 |
| --- | --- |
| 数据库备份 | 备份文件通过 KMS 或数据库原生加密；如需导出用于审计，需对此字段做掩码（例如 `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxx12345`）。 |
| 日志 / Metrics | 禁止将 `tenant_id` 写入日志；`tenant_uuid` 仅在必要时写入，并在敏感日志 pipeline 中加掩码。 |
| 导出文件 | 若对第三方（监管/合作伙伴）导出，需提供 `tenant_uuid` 与 `tenant_id` 映射文件（见 `scripts/ops/export-tenant-mapping.sh`），并设置过期时间。 |

## 3. 备份与销毁
- 备份恢复演练：按照 Playbook（`docs/operations/playbooks/tenant-uuid-upgrade.md`）执行，确认恢复脚本根据 `tenant_uuid` 查找。
- 删除流程：当租户注销时，通过 `tenant_uuid` 调用各服务的清理接口；禁止依赖数值 ID。

## 4. 验证清单
- 在 `tmp/tenant-uuid-schema-completeness.md` Spot-check `tenant_uuid` 列。
- Security 团队每季度运行一次备份恢复演练，并将结果附在 `reports/tenant-uuid-weekly.md#security`.

## 5. 参考
- `scripts/ops/export-tenant-mapping.sh`
- `docs/legal/tenant-uuid-opinion.md`
- `tmp/tenant-id-migration-plan.md` T8.12 条目
