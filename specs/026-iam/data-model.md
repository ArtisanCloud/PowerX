# Phase 1 Data Model: IAM 用户与角色 RBAC 统一能力

## 1. IdentityContext

- Purpose: 表达当前会话身份与租户上下文，是页面角色分流与权限判定的上游输入。
- Fields:
  - `is_root` (bool)
  - `current_tenant_uuid` (string)
  - `current_member_id` (number)
  - `user` (object: `id/email/display_name/status`)
  - `members` (array of MembershipSummary)
- Validation Rules:
  - `is_root=true` 表示平台 root 身份，不等于任意业务租户 admin。
  - `current_tenant_uuid` 必须在 `members` 中可解析，除非 root 走平台上下文或 Support Session。
  - 当前 token/context 必须包含可用于审计的 `member_id/member_uuid`，普通租户业务写操作不得只依赖 `user_id`。

## 2. MembershipSummary

- Purpose: 表达用户在某租户内的成员视图，用于前端判定租户管理员权限与切换候选。
- Fields:
  - `tenant_uuid` (string)
  - `tenant_name` (string)
  - `member_id` (number)
  - `member_uuid` (string)
  - `is_admin` (bool)
  - `is_owner` (bool)
- Validation Rules:
  - 同一 `tenant_uuid` 只能存在一条有效成员摘要。
  - 被禁用成员不应被当作可管理身份。
  - `is_admin` 来自 `role_admin`，`is_owner` 来自 `role_owner`，不能由 `is_root` 推导。

## 3. RoleCapabilityBoundary

- Purpose: 角色与可执行动作的映射边界。
- Fields:
  - `role` (`root` | `tenant_admin` | `member`)
  - `scope` (`cross_tenant` | `current_tenant` | `self`)
  - `allowed_actions` (array)
  - `denied_actions` (array)
- Validation Rules:
  - `root` 必须允许跨租户管理读写。
  - `tenant_admin` 必须拒绝跨租户读写。
  - `member` 必须拒绝租户级管理写操作。

## 4. UserManagementAction

- Purpose: 用户管理页面动作语义实体，避免复合副作用。
- Fields:
  - `action_type` (`view_detail` | `switch_tenant` | `navigate_dashboard`)
  - `target_tenant_uuid` (optional)
  - `trigger_source` (`row_click` | `button_click` | `menu_click`)
- Validation Rules:
  - 单次触发仅允许一个 `action_type`。
  - `view_detail` 不得隐式触发 `switch_tenant`。
  - `switch_tenant` 不得隐式触发 `navigate_dashboard`。

## Relationships

- `IdentityContext` 1:N `MembershipSummary`
- `RoleCapabilityBoundary` 1:N `UserManagementAction`（按 role 限定可执行动作）
- `User` 1:N `TenantMembership`
- `Tenant` 1:N `TenantMembership`
- `Tenant` 1:N `TenantPluginInstance`
- `RootUser` 1:N `RootSupportSession`

## State/Consistency Rules

- 页面进入用户管理时：必须先刷新 `IdentityContext` 再决定视图分支。
- 本地缓存与服务端冲突时：服务端覆盖本地。
- 租户切换成功后：`current_tenant_uuid` 与页面可见操作集必须同时更新。
- 租户切换成功后：token 与 `me/context` 必须同时指向新的 `tenant_uuid + member_id/member_uuid`。
- root 默认状态：进入 Platform Console，不显示租户 AI Settings 和租户插件业务入口。
- 插件代理入口：必须校验当前租户已启用对应 `TenantPluginInstance`。

## 5. SaaSSignupRequest

- Purpose: 表达 SaaS 自助注册新租户的公开请求。
- Fields:
  - `tenant_key` (string)
  - `tenant_name` (string)
  - `plan` (string)
  - `owner_email` (string, optional)
  - `owner_phone` (string, optional)
  - `owner_password` (string)
  - `owner_display_name` (string)
- Validation Rules:
  - `tenant_key` 全局唯一。
  - `owner_email` 与 `owner_phone` 至少一个存在。
  - 已有 user 必须校验密码正确后才能创建新租户 member。
  - 创建 tenant、member、role binding、默认设置必须具备事务语义。
  - 失败时不得留下半成品 tenant/member/role binding。

## 6. RootSupportSession

- Purpose: 表达 root 显式进入业务租户上下文的支持会话。
- Fields:
  - `id` (number)
  - `root_user_id` (number)
  - `target_tenant_uuid` (string)
  - `reason` (string)
  - `mode` (`read_only` | `write_enabled`)
  - `started_at` (timestamp)
  - `ended_at` (timestamp, optional)
  - `status` (`active` | `ended` | `revoked`)
- Validation Rules:
  - 只能由 `is_root=true` 的 user 创建。
  - 必须有非空 `reason`。
  - 默认 `mode=read_only`。
  - 写操作必须记录 root actor、target tenant、support session id。

## 7. TenantPluginInstance

- Purpose: 表达某租户对某全局插件包的启用状态和配置。
- Fields:
  - `tenant_uuid` (string)
  - `plugin_id` (string)
  - `version` (string)
  - `enabled` (bool)
  - `config` (object)
  - `created_by_member_id` (number)
  - `updated_by_member_id` (number)
- Validation Rules:
  - 全局插件包存在不代表当前租户已启用。
  - 菜单聚合、插件 admin 代理、插件 api 代理都必须校验 `enabled=true`。
  - 租户停用插件不得删除全局插件包，也不得影响其他租户实例。
  - 租户启用/停用插件不得直接启动或停止全局插件进程。
  - 同一 PowerX 节点内，同一 `plugin_id` 的运行时进程由 `PluginRuntimeProcess` 表达，不按租户复制。

## 8. PluginRuntimeProcess

- Purpose: 表达 PowerX 节点内存中的插件全局运行时进程。
- Fields:
  - `plugin_id` (string)
  - `version` (string)
  - `process_id` (string: `plugin_id` 或 `plugin_id_admin`)
  - `pid` (number)
  - `port` (number)
  - `state` (`starting` | `running` | `unhealthy` | `stopped` | `exited` | `crashed`)
  - `started_at` (timestamp)
  - `health_path` (string)
- Validation Rules:
  - `process_id` 由全局 `plugin_id` 派生，不包含 `tenant_uuid`。
  - 一个 PowerX 节点内同一 `plugin_id` 只能有一组后端/admin 运行进程。
  - 多租户共享同一运行进程，租户隔离必须发生在请求上下文、事件 payload、租户配置和数据访问层。
  - 进程启动环境只能包含平台级 runtime 配置，不应绑定某一个业务租户的 STS client 或 tenant secret。

## 9. IAMMigrationReport

- Purpose: 表达 SaaS IAM 语义上线前后的历史数据巡检结果。
- Fields:
  - `root_users` (array)
  - `system_tenant_status` (`ok` | `missing` | `invalid`)
  - `root_system_member_status` (`ok` | `missing` | `invalid`)
  - `tenant_owner_missing` (array)
  - `tenant_admin_missing` (array)
  - `auto_fix_candidates` (array)
  - `manual_fix_required` (array)
- Validation Rules:
  - 巡检默认只读。
  - 缺少 owner 但有 active admin 的租户可以进入 `auto_fix_candidates`。
  - 缺少 active admin 的租户必须进入 `manual_fix_required`。
  - 自动补齐必须写审计，禁止静默修改。
