# Regulator Interfaces（Tenant UUID-only）

> 用于记录任何仍向监管机构报送 `tenant_id` 的接口、频次与联系人，并说明如何提供 UUID 映射或函件。

## 1. 清单
| 接口 / 报送名称 | 当前字段 | 状态 | 联系人 | 备注 |
| --- | --- | --- | --- | --- |
| *(示例)* 金融监管报送 | `tenant_id` | 待改为 UUID | Compliance @dawn | 正在起草函件 |

## 2. 行动要求
1. Compliance 团队每季度复核此表，确认是否仍有报送需要 `tenant_id`。
2. 若监管表单无法即时改名，需提供官方函件或映射文件（由 `scripts/ops/export-tenant-mapping.sh` 导出），并记录有效期。
3. 所有变化需在 `tmp/tenant-id-migration-plan.md` T8.12 条目更新状态。

## 3. 参考
- Playbook：`docs/operations/playbooks/tenant-uuid-upgrade.md`
- 数据导出脚本：`scripts/ops/export-tenant-mapping.sh`
- 合同/法务：`docs/legal/tenant-uuid-opinion.md`
