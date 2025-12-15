# Tenant UUID-only Partner Tracking

| Partner/Customer | Tier | Contact | Status | Next Action | Notes |
| --- | --- | --- | --- | --- | --- |
| <Partner A> | ISV Tier-1 | alice@example.com | 🟡 Testing | 跟进 12/10 提供测试账号 | 收到 `tenant_id` header 报错，已指引升级 |
| <Partner B> | Channel | bob@example.com | 🟢 Ready | - | 已在 staging 验证成功 |

## 使用说明
1. Partner Eng (@claire) 负责每周更新此表，周五同步到 `reports/tenant-uuid-weekly.md#partners`。
2. 若发现仍依赖 `tenant_id` 的集成，创建 `tenant-uuid-ga` 标签 issue 并在本表注明链接。
3. 当所有 Tier-1/Tier-2 合作伙伴状态为 🟢 时，可将 T8.10「合作伙伴同步」标记为 ✅。
