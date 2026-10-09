# API Key 插件 owner 配置与持久化

日期：2026-10-08。分支：`024-ai-engineering-skills`。

## 授权关系

Profile 定义功能权限；具体 API Key 定义可操作的插件 owner。调度服务同时校验能力 grant、租户和目标任务 owner。`grant-status=granted` 只确认能力权限，不能替代目标 owner 校验。

`com.powerx.plugins.crm` 与 `com.powerx.plugins.scrm` 是不同的 owner。不能把 Key 名称、Profile 名称、API Key 主体或请求中的 owner 字符串当作可信插件身份。

## 管理接口

正式 OpenAPI：[api-key-owners.openapi.yaml](../../specs/007-integration-gateway-and-mcp/contracts/api-key-owners.openapi.yaml)。

```http
GET /api/v1/admin/integration/api-keys/{key_id}/plugin-owners
PUT /api/v1/admin/integration/api-keys/{key_id}/plugin-owners
Authorization: Bearer <管理员用户 JWT>
```

只允许 root 或当前租户管理员用户；API Key 和 STS 服务身份均返回 403。租户来自可信请求上下文，body 不接受 `tenant_uuid`、headers、endpoint 或任意代理字段。跨租户 Key 返回 404。

请求为精确替换集合：

```json
{"plugin_ids":["com.powerx.plugins.crm","com.powerx.plugins.scrm"]}
```

- 最多 32 个 ID，每个最长 128 字符，格式 `^[a-z0-9][a-z0-9._-]{0,127}$`；去重、排序。
- 不支持空 ID、通配符、路径和大写字符，返回 400。
- `plugin_ids: []` 明确解除插件 owner 授权；字段缺失或 null 返回 400。
- Profile 中含固定插件的权限与所选 owner 不一致时返回 `API_KEY_PLUGIN_OWNER_CONFLICT`，不静默忽略。
- 已失效 Key 或已停用 Profile 返回 409。

成功 `data` 示例：

```json
{
  "key_id":"6db849ca-e003-4fcb-8163-c0572678ea99",
  "tenant_uuid":"6b5d0240-9920-46da-b707-88200e0f51ea",
  "plugin_ids":["com.powerx.plugins.crm"],
  "binding_mode":"key",
  "updated_at":"2026-10-08T15:00:00Z"
}
```

这里的时间是格式示例；当前修复的 CRM Key 使用下述精确调度权限模式，回读 `binding_mode=legacy_permissions`。

新建 Key 请求支持可选 `plugin_ids`；Key 列表、详情、创建和轮换响应均返回 `plugin_ids` 与 `binding_mode`。轮换继承原 Key 的完整 owner 策略。

## 保存与兼容

- `integration_gateway_api_keys.plugin_owner_policy` 为独立 JSONB 列，由集中 `migrateIntegrationGatewayModels` 的既有 AutoMigrate 注册迁移。
- `key` 模式存储管理员明确选定的 owner 集合，并应用到当前 Profile 的授权。后续新增功能权限仍须经 Profile 授权。
- `legacy_permissions` 模式冻结原授权的 scope/action/resource/effect/plugin_id 组合。再次保存或追加 Profile、默认 Profile 同步、权限清空后恢复、Key 轮换均保留原有细分范围。旧的“read=SCRM、delete=CRM”不会被扩大成两者都能读写。
- NULL 的旧策略首次同步时从精确现有授权冻结。空或通配 `plugin_id` 不会变成指定插件的授权。
- 管理员在 owner 表单保存精确集合后，模式改为 `key`；页面先展示原有细分范围，明确告知新的范围。
- 更新 owner 和 Profile 同步按相同顺序锁定 Profile、Key，避免同步覆盖；owner、生成授权和变更审计在同一事务中提交。

管理能力 `com.corex.integration.api_key_owners.admin_manage` 为 `admin_user/user_jwt`、`agent_usable=false`，不提供 Framework 服务绑定。它不是 Framework scheduler selector 的服务权限。

## 当前 CRM Key 修复

- Key：`CRM Devleopment Key` / `6db849ca-e003-4fcb-8163-c0572678ea99`，租户 `6b5d0240-9920-46da-b707-88200e0f51ea`。
- 修复前 4 条 scheduler service grant 的 `plugin_id` 均为空。已在本机 Core 维护事务中仅给这 4 条授权补 `com.powerx.plugins.crm`，并保存独立、按权限的 owner 快照。其他授权范围保留。
- 备份：`/tmp/powerx-api-key-owners/crm-key-before.json`，权限 0600；未包含 Key 明文或哈希。
- 本机维护审计 Trace：`7e5766f2-2fa6-4c41-861c-ad86523eed6e`。
- 真实开发 PostgreSQL / Core 服务层检查：两项能力 granted，CRM 预览成功、4 个任务；SCRM owner 返回 `403 / SCHEDULER_PLUGIN_OWNER_MISMATCH`。没有删除或触发任务。
- 本轮没有使用此 CRM Key 的 HTTP 明文凭据；上述证据是实际持久记录及服务层检查，不替代 .NET/浏览器端到端验收。

## 验证与运行步骤

1. `make migrate` 已完成新增列。迁移 CLI 的 JWT 配置校验使用进程内临时强密钥，未写入配置、未修改运行服务或登录令牌。
2. SQLite HTTP 生命周期与 Service 测试、`-race` 通过；真实 PostgreSQL 独立 schema 的清空/恢复、旧细分授权、跨租户/权限拒绝、并发 owner 更新/Profile 同步测试通过并清理 schema。
3. 前端 6 项输入测试通过，Vue script/template 与中英文 JSON 检查通过。
4. `make capability-check` 通过，`declared=1086, referenced=62, rest_routes=1007, candidates=1010`。本次 owner 管理合同已用正式 seed 单独登记。
5. 完整目录的 `make capability-seed` 被另一处新生成能力 ID 的 130 字符与 Registry `varchar(128)` 限制阻断；这不属于 owner 合同，记录于 `/tmp/powerx-api-key-owners/capability-seed.log`。集成网关包的两项 builtin Media 声明测试亦已有失败，生产 YAML 中存在 Media 能力而 builtin fallback 列表不包含它。本次未修改这些无关实现。
6. **重启本机 8077 后台加载新代码**，再使用页面：设置 → API Key 管理 → 对应 Key → 配置插件 owner。插件 ID 从 manifest 取值；现有授权先核对，再保存。
7. 用 CRM 当前 Key 重试 .NET 任务预览；再次保存 Profile、刷新 owner 回读和重复预览。新保存链路只有新后台代码加载后才生效，旧进程再次保存 Profile 仍会重建旧的无 owner 授权。

没有遗留临时服务。所有代码变更保持在当前分支，提交、推送、合并由用户执行。
