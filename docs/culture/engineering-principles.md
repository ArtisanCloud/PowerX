# Engineering Principles（摘录）

> 如需提案/变更，请在 PR 中更新本文件并通知 Architecture Council。新增原则：**租户标识统一**。

## 核心原则

1. **Tenant Identity First（租户标识统一）**
   - 所有对外/对内接口必须仅使用 `tenant_uuid`。任何 `tenant_id`/`tenantId`/`X-PowerX-Tenant` 的字段或 Header 都视为违反规范。
   - 新服务创建时，从 ReqCtx/上下文获取 UUID，并在 repo 层加入唯一索引 `tenant_uuid`.
   - 代码评审需检查：Handler/Service/Repo/测试是否有 `tenant_id` 出现；如因兼容必须临时保留，需附 TODO 和 sunset 日期。
   - 参考资料：`tmp/tenant-id-migration-plan.md`、`docs/operations/tenant-uuid-upgrade.md`、`examples/tenant-uuid-only/README.md`.

2. *(保留位置，以便未来扩展其他原则)*

## 实施
- Architecture Council 在新项目审查 checklist 中增加项：“是否只依赖 `tenant_uuid`？”。
- CI `scripts/ci/check-no-tenant-id.sh` / `scripts/ci/check-tenant-uuid-canonical.sh` 执行结果需在 PR 评论中呈现，若命中 `tenant_id` 或手动解析租户 UUID 则阻断合并。
