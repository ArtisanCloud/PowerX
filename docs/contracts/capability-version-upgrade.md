# 租户能力版本升级确认

日期：2026-09-26。

## 本次故障

trace `1afdd409-ed99-4b0b-94e7-ce27d34b5d99` 于 22:04:03 到达 Core `POST /api/v1/tenant/invocations` 并返回 424。客户更新尚未进入业务执行。

- 租户：`6b5d0240-9920-46da-b707-88200e0f51ea`
- 能力：`com.corex.customer.accounts.service_manage`
- 租户原确认 hash：`6e296da4604bb577182d34d1738f3f94ac2578724cfe554086574f867d49e667`
- 当前发布 hash：`db9aa42525ee0f23dd8c8456405a24a5590c2bd6b8598e1e4a4ec7e8f63eb825`

两个版本来自 Core。发布同步更新 CapabilityRecord，而租户版本锁仍要求原版本。API Key 的 service grant 有效也不能代替管理员确认。此次与插件 migration 无关。

## 管理入口

```text
POST /api/v1/admin/capability-registry/capabilities/:capabilityId/tenants/:tenant_uuid/upgrade
Authorization: Bearer <管理员用户 JWT>
Content-Type: application/json
```

```json
{
  "capabilities_hash": "db9aa42525ee0f23dd8c8456405a24a5590c2bd6b8598e1e4a4ec7e8f63eb825",
  "reason": "确认客户资料 update 服务合同；关联故障 trace 1afdd409-ed99-4b0b-94e7-ce27d34b5d99"
}
```

- 当前租户 `role_admin` / `system_admin` 或平台 root 用户可操作；普通用户、其他租户管理员、插件 STS 和 API Key 被拒绝。
- actor 从可信用户凭证取得，不接受 payload 传 actor 或 tenant。
- hash 必须为当前已发布的完整 SHA-256；过期/错误目标返回 409。
- 要求当前租户最新 registration 为 published。没有注册或最新已停用返回 404，不使用历史 published registration。
- 成功返回旧/新 hash、租户、能力及审计记录 ID。
- 先持久化 `CONFIRM_UPGRADE` 审批事件，再应用运行时版本锁，最后标记审计 SUCCESS。审计写入失败不得改变锁。运行时写入失败保留 APPROVED 审批记录，可用同一目标重试。审计与运行时 store 不宣称跨存储原子事务。
- 重复确认同一 hash 安全；不影响其他租户、能力或 API Key/STS grant。

正式能力为 `com.corex.capabilities.version_upgrade.confirm`，仅 Admin 用户 JWT；无插件服务授权面，因为插件不能自批准治理变更。

先通过 `GET /api/v1/admin/capabilities/:capabilityId` 核对最新 `capabilities_hash`。不得删除 Redis key、禁用版本锁或自动批准全部租户来替代确认。

## 部署与验收状态

本次新增 HTTP/service 入口，复用已有审计表，不新增 schema migration。

1. `make capability-check`，`make capability-seed` 同步正式声明。
2. 重启 Core 加载新入口。
3. 使用管理员用户 JWT 对上述租户和能力提交确认。
4. 核对成功响应与审计事件，再按 [客户更新验收命令](customer-service-update.md) 分别验证 API Key/STS 更新及重新读取，并记录真实 trace ID。

本轮已复现旧运行进程的 424，确认数据库目标发布 hash，补齐源码和权限/版本/审计回归。目标租户的升级确认及后续真实 API Key/STS 验收尚待新进程和管理员调用，不能视作已解锁。
