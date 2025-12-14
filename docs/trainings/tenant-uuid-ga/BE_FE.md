# Tenant UUID-only Workshop（Backend / Frontend）

## 目标
1. 复盘租户上下文新规范：只允许 `X-Tenant-UUID`/`tenant_uuid`。
2. 演示 CLI / Web Admin / Plugin Bridge 的升级步骤。
3. 提供排障脚本与测试建议，确保日常开发不再依赖 `tenant_id`。

## 课程大纲
| 模块 | 内容 | Demo/素材 |
| --- | --- | --- |
| 背景 | 为什么要切到 UUID-only、灰度阶段总结 | `docs/operations/tenant-uuid-upgrade.md` |
| CLI 演示 | `px plugin dev watch --tenant-uuid`、`px version scan`、`scripts/ci/check-no-tenant-id.sh`、`scripts/ci/check-tenant-uuid-canonical.sh` | 现场 demo + `docs/releases/tenant-uuid-ga.md` |
| Web Admin/Bridge | `powerx-bridge-client.js` 0.9 行为、iframe `postMessage` payload | `web-admin/public/powerx-bridge-client.js` |
| 测试策略 | 合约/集成测试如何注入 UUID、`tenant_uuid_testenv` helper | `backend/tests/.../http_helpers_test.go` |
| 常见问题 | resolver 删除后的影响、mock 数据如何写 UUID | `docs/support/tenant-uuid-faq.md#开发` |

## 作业 & 跟进
- 每个团队更新自测脚本，确保默认从 `.env`/`powerx.config` 读取 `TENANT_UUID`。
- 通过 `tmp/tenant-id-migration-plan.md` 的 T8.4/T8.5 checklist 自检是否仍引用 `tenant_id`。
- 将 workshop 录像上传到 LMS，并在 `docs/trainings/tenant-uuid-ga/attendance.csv` 标记参会人员。
