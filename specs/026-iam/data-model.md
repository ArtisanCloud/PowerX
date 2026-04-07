# Phase 1 Data Model: IAM 用户与角色 RBAC 统一能力

## 1. IdentityContext

- Purpose: 表达当前会话身份与租户上下文，是页面角色分流与权限判定的上游输入。
- Fields:
  - `is_root` (bool)
  - `current_tenant_uuid` (string)
  - `current_member_id` (number)
  - `user` (object: `id/email/display_name/status`)
  - `members` (array of MembershipSummary)
  - `ctx` / `ctx_sig` / `ctx_jwt` (string, optional)
- Validation Rules:
  - `is_root=true` 时允许无当前租户成员也能读取跨租户管理入口。
  - `current_tenant_uuid` 必须在 `members` 中可解析，除非 root 走平台上下文。

## 2. MembershipSummary

- Purpose: 表达用户在某租户内的成员视图，用于前端判定租户管理员权限与切换候选。
- Fields:
  - `tenant_uuid` (string)
  - `tenant_name` (string)
  - `member_id` (number)
  - `is_admin` (bool)
- Validation Rules:
  - 同一 `tenant_uuid` 只能存在一条有效成员摘要。
  - 被禁用成员不应被当作可管理身份。

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

## State/Consistency Rules

- 页面进入用户管理时：必须先刷新 `IdentityContext` 再决定视图分支。
- 本地缓存与服务端冲突时：服务端覆盖本地。
- 租户切换成功后：`current_tenant_uuid` 与页面可见操作集必须同时更新。
