# Tenant UUID-only GA 公告模板

> 用于满足 T8.8 「发布公告」交付：在 `#announcements`、`PowerX Release Notes`、客户 Success 邮件模板中同步 UUID-only 上线信息。

## 1. 预告（GA 前 3 天）

**Slack（#announcements）**
```
🚧 PowerX Tenant UUID-only 预告
时间：<YYYY-MM-DD HH:mm TZ>
影响：所有 API/CLI/插件将仅接受 `X-Tenant-UUID`/`tenant_uuid` 字段，旧的 `tenant_id` 将被拒绝。
准备动作：
- 升级 CLI 至 px 1.8.0 / px-plugin 0.16.0
- 检查自动化脚本是否仍使用 `--tenant-id`，必要时改为 `--tenant-uuid`
- 参考升级指南：docs/operations/tenant-uuid-upgrade.md
回滚计划：docs/operations/playbooks/tenant-uuid-upgrade.md
如需协助，请联系 #tenant-uuid-migration 或 Ops On-call (@zoe)。
```

**PowerX Release Notes（Issue/Discussion）**
- 新建条目《Heads-up: Tenant UUID-only GA on <date>》。
- 内容包含：范围、升级步骤、回滚链接、Support 联系方式。

**客户 Success 邮件模板**
```
Subject: [Action Required] PowerX Tenant UUID-only Upgrade on <date>

Hi <Customer>,

PowerX will enforce Tenant UUID-only headers on <date>. After this change, any API/CLI call using `X-Tenant-ID` will return 400.

Required actions:
1. Update PowerX CLI to v1.8.0 or later.
2. Ensure integrations send `X-Tenant-UUID`.
3. Review the upgrade guide: https://.../docs/operations/tenant-uuid-upgrade

Rollback / Support:
If you are blocked, contact <support-email> or the on-call engineer listed here: docs/operations/playbooks/tenant-uuid-upgrade.md.

Thanks,
PowerX Team
```

## 2. GA 当日公告

**Slack（#announcements）**
```
✅ PowerX Tenant UUID-only 已正式上线
- Legacy `X-Tenant-ID` header 已被拒绝；如遇 400 `legacy tenant header not allowed`，请确认客户端版本。
- 观测指标：grafana/powerx/tenant-uuid-ga（legacy header = 0、UUID-only 请求率 ≈ 100%）
- CLI/SDK 版本：px 1.8.0、px-plugin 0.16.0、powerx-bridge-client.js 0.9.0
- Playbook & 回滚：docs/operations/playbooks/tenant-uuid-upgrade.md
- 详细 Release Notes：<link>
如需帮助，请联系 #tenant-uuid-migration 或 Support On-call。
```

**Release Notes**
- 在主 Release Notes 中添加正式条目，记录：上线日期、主要更改点、受影响接口、rollback 链接。

**客户 Success 邮件**
```
Subject: PowerX Tenant UUID-only GA Completed

Hi <Customer>,

The Tenant UUID-only upgrade has been completed. All requests must now include the `X-Tenant-UUID` header (or `tenant_uuid` fields). If you see errors referencing `legacy tenant header`, please upgrade your CLI/SDK or reach out to us.

Resources:
- Upgrade guide: https://.../docs/operations/tenant-uuid-upgrade
- Runbook: https://.../docs/operations/playbooks/tenant-uuid-upgrade
- Support: <support-email> / <CSA name>

Thanks for the collaboration!
PowerX Team
```

## 3. 验收记录
- 在 `reports/tenant-uuid-weekly.md#communications` 粘贴实际发送链接/截图。
- 在 `tmp/tenant-id-migration-plan.md` 的「发布公告」完成后更新为 ✅。
