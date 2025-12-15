# 法务意见书模版：Tenant UUID-only 方案

> 适用于 T8.12「法务审查」交付。请 Legal/Compliance 在 GA 前填写，确认是否影响隐私政策、跨境条款等。

## 基本信息
- **项目名称**：PowerX Tenant UUID-only（移除 `tenant_id` 兼容层）
- **撰写人**：`<Legal Owner>`（@dawn）
- **审阅日期**：YYYY-MM-DD
- **生效日期**：YYYY-MM-DD

## 1. 背景
- 说明：从 `X-Tenant-ID`/`tenant_id` 切换到 `X-Tenant-UUID`，所有接口/日志仅保留 UUID。
- 相关文档：`docs/operations/tenant-uuid-upgrade.md`, `tmp/tenant-id-migration-plan.md`, `docs/operations/playbooks/tenant-uuid-upgrade.md`.

## 2. 风险与评估
| 事项 | 判定 | 说明/要求 |
| --- | --- | --- |
| 隐私合规（个人数据定义） | 无影响 / 需更新 | |
| 数据驻留/跨境 | 无影响 / 需更新 | |
| 合同/条款引用 `tenant_id` | 无影响 / 需更新 | |
| 外部监管（如金融/医疗） | 无影响 / 需附说明 | |

## 3. 结论
- 是否允许上线：✅ / ⚠️（需满足以下条件）
- 需要更新的文档/条款：列出合同模板、FAQ、隐私条款等。
- 对外沟通建议：例如在 release note、客户公告中加入法务声明。

## 4. 附件
- 支持材料：邮件记录、法律条款 diff、外部咨询意见等。
- 联系人：Legal Owner、Compliance Partner、Product PM。

> 完成后，请将结论链接附在 `tmp/tenant-id-migration-plan.md` 的 T8.12 条目，同时在 `reports/tenant-uuid-weekly.md#legal` 更新状态。
