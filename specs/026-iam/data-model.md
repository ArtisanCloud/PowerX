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
  - root 可以带 `system` 特殊租户的 `tenant_uuid + member_id/member_uuid`，该 member 是平台身份锚点，不参与普通业务租户授权。
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
  - `role` (`root` | `tenant_admin` | `member` | `vendor`)
  - `scope` (`cross_tenant` | `current_tenant` | `self`)
  - `allowed_actions` (array)
  - `denied_actions` (array)
- Validation Rules:
  - `root` 必须允许跨租户管理读写。
  - `tenant_admin` 必须拒绝跨租户读写。
  - `member` 必须拒绝租户级管理写操作。
  - `vendor` 是租户级供应商角色，只能获得显式授予的菜单与业务权限，不得继承租户管理权限。

## 3A. MenuPermission

- Purpose: 表达后台菜单入口的角色级可见性控制。
- Fields:
  - `module` = `menu`
  - `resource` (string，例如 `agent`, `agent.chat`, `skills`, `workflow`, `settings.users`)
  - `action` = `read`
  - `meta.type` = `menu`
  - `source` = `core` 或插件 ID
- Validation Rules:
  - 菜单权限必须绑定到角色，不直接绑定到用户。
  - `/api/v1/admin/menus` 只能返回当前 `tenant_uuid + member_id` 通过角色拥有的菜单项。
  - root 可通过平台身份看到 root 菜单，但 root 不应被自动视为业务租户 admin。
  - `role_user`、`role_readonly`、`role_vendor` 只能获得显式白名单菜单权限，不能因为 `action=read` 自动拥有全部菜单。
  - 菜单可见性只控制导航入口，不替代页面路由和 API 授权。

### 3A.1 PluginMenuPermission

- Purpose: 表达插件/App 后台菜单入口的角色级可见性控制。
- Source:
  - 插件 manifest 的 `frontend.admin.menus`
  - 插件安装、启用或 manifest 权限同步流程自动写入
  - 管理员不能人工创建或修改插件菜单资源本身，只能把已有菜单权限授予角色
- Fields:
  - `module` = `menu`
  - `resource` = `plugin.<plugin_id>.<menu_id>`
  - `action` = `read`
  - `source` = `plugin:<plugin_id>`
  - `meta.type` = `menu`
  - `meta.origin` = `plugin`
  - `meta.plugin_id` / `meta.plugin_name` / `meta.menu_id` / `meta.label`
- Validation Rules:
  - 插件菜单聚合返回的每个插件菜单项必须自动附加 `menu:plugin.<plugin_id>.<menu_id>:read`。
  - 插件菜单权限只控制菜单显示，不替代租户插件实例启用校验、插件代理入口校验和插件 API/能力权限校验。
  - 插件菜单子项必须递归生成独立菜单权限，父菜单在子项有权限时可作为容器保留。
  - 插件菜单资源 ID 必须由 manifest 的稳定 `id` 优先生成；缺少 `id` 时只能从 route/path 派生稳定段，不允许前端或管理员临时造菜单资源。

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
- `TenantMembership` N:M `Role` via `RoleBinding`
- `Role` N:M `MenuPermission` via `RolePermission`
- `Tenant` 1:N `TenantPluginInstance`
- `RootUser` 1:N `RootSupportSession`

## State/Consistency Rules

- 页面进入用户管理时：必须先刷新 `IdentityContext` 再决定视图分支。
- 本地缓存与服务端冲突时：服务端覆盖本地。
- 租户切换成功后：`current_tenant_uuid` 与页面可见操作集必须同时更新。
- 租户切换成功后：token 与 `me/context` 必须同时指向新的 `tenant_uuid + member_id/member_uuid`。
- 登录成功后：系统必须更新 `User.last_tenant_uuid` 为最终签发 token 指向的租户。
- `User.last_tenant_uuid` 只是最近使用偏好，不是授权凭证；登录时必须重新校验该 user 是否仍有目标租户 active member。
- root 默认状态：进入 Platform Console，不显示租户 AI Settings 和租户插件业务入口。
- 插件代理入口：必须校验当前租户已启用对应 `TenantPluginInstance`。

## 5. User

- Purpose: 表达全局自然人账号，承载登录凭证与跨租户成员关系。
- Fields:
  - `id` (number)
  - `uuid` (string)
  - `email` (string, optional)
  - `phone` (string, optional)
  - `display_name` (string)
  - `is_root` (bool)
  - `status` (number)
  - `last_tenant_uuid` (string, optional)
- Validation Rules:
  - `email` 与 `phone` 至少一个可作为登录 identifier。
  - 手机号注册不得写入伪造默认邮箱。
  - 登录密码跟随全局 user credential，不跟随单个 tenant member。
  - `last_tenant_uuid` 不得单独赋权，使用前必须校验 active membership。

## 6. SaaSSignupRequest

- Purpose: 表达 SaaS 自助注册新租户的公开请求。
- Fields:
  - `tenant_key` (string, optional)
  - `tenant_name` (string)
  - `plan` (string)
  - `owner_email` (string, optional)
  - `owner_phone` (string, optional)
  - `owner_password` (string)
  - `owner_display_name` (string)
  - `verification_code` (string, optional)
  - `invite_code` (string, optional)
  - `channel` (string, optional)
  - `campaign` (string, optional)
  - `registration_request_uuid` (string, optional)
- Validation Rules:
  - `tenant_key` 未填写时根据 `tenant_name` 自动生成唯一 key。
  - `tenant_key` 显式填写时必须 slug 规范化且全局唯一，冲突必须失败。
  - 租户名称可以重复；租户唯一性由 `tenant_key/domain` 保证。
  - `owner_email` 与 `owner_phone` 至少一个存在。
  - 已有 user 必须校验密码正确后才能创建新租户 member。
  - 验证码由 active `RegistrationPolicy.requires_verification` 控制。
  - `invite_code`、`channel`、`campaign` 必须是结构化字段，不得从自由文本解析。
  - `invite_only` 或策略要求邀请码时，`invite_code` 必填且必须命中 active invite code。
  - `waitlist` 模式不得创建 tenant/user/member，只能创建 `RegistrationRequest`。
  - `approval_required` 模式必须先创建 `RegistrationRequest`，root 审核通过后才执行租户创建。
  - 创建 tenant、member、role binding、默认设置必须具备事务语义。
  - 邀请码消耗与 tenant/member/role binding 创建必须处于同一事务；失败时不得留下半成品数据，也不得消耗邀请码。

## 7. RegistrationPolicy

- Purpose: 表达租户注册准入策略，是 setup、root 后台和公开注册入口共同使用的权威策略。
- Fields:
  - `id` (number)
  - `uuid` (string)
  - `version` (number)
  - `mode` (`closed` | `open` | `invite_only` | `waitlist` | `approval_required` | `allowlist` | `progressive_rollout`)
  - `status` (`draft` | `active` | `archived`)
  - `requires_verification` (bool)
  - `requires_invite_code` (bool)
  - `requires_root_approval` (bool)
  - `daily_tenant_quota` (number, optional)
  - `total_tenant_quota` (number, optional)
  - `start_at` (timestamp, optional)
  - `end_at` (timestamp, optional)
  - `rules` (array of RegistrationPolicyRule)
  - `activated_at` (timestamp, optional)
  - `created_by_user_uuid` (string)
  - `updated_by_user_uuid` (string)
- Validation Rules:
  - 同一时间只能有一个 active 策略。
  - 修改策略必须生成新版本，不能原地覆盖 active 策略。
  - 缺少 active 策略时，公开 effective policy、验证码发送和 signup 必须 fail fast。
  - 未识别的 `mode` 或 rule type 必须 fail fast。
  - 旧 `enable_saas_signup` 布尔开关只能在迁移时转换为策略对象，不得作为运行时 fallback。

## 8. RegistrationPolicyRule

- Purpose: 表达注册策略中的结构化准入规则。
- Fields:
  - `type` (`email_domain_allowlist` | `contact_allowlist` | `invite_batch` | `channel_allowlist` | `percentage` | `time_window` | `daily_quota` | `total_quota`)
  - `values` (array, optional)
  - `batch_uuid` (string, optional)
  - `value` (number, optional)
  - `seed` (`contact` | `email` | `phone`, optional)
- Validation Rules:
  - `percentage.value` 必须在 0 到 100 之间。
  - `percentage.seed` 必须使用稳定输入，例如 contact hash；不得使用每次请求随机值。
  - `invite_batch.batch_uuid` 必须引用 active `RegistrationInviteBatch`。
  - allowlist 规则只接受结构化 contact、domain 或 channel，不解析自由文本。

## 9. RegistrationInviteBatch

- Purpose: 表达邀请码批次，用于内测、合作伙伴、灰度批次和限量放号。
- Fields:
  - `id` (number)
  - `uuid` (string)
  - `name` (string)
  - `status` (`draft` | `active` | `paused` | `expired` | `revoked`)
  - `max_codes` (number)
  - `max_uses_per_code` (number)
  - `allowed_plan` (string, optional)
  - `allowed_email_domains` (array, optional)
  - `allowed_channels` (array, optional)
  - `starts_at` (timestamp, optional)
  - `expires_at` (timestamp, optional)
  - `created_by_user_uuid` (string)
- Validation Rules:
  - 只有 `active` 且处于有效期内的批次可用于注册。
  - 批次暂停或撤销后，所有关联邀请码都不得继续用于新注册。
  - 批次规则必须参与准入审计。

## 10. RegistrationInviteCode

- Purpose: 表达单个邀请码。
- Fields:
  - `id` (number)
  - `uuid` (string)
  - `batch_uuid` (string)
  - `code_hash` (string)
  - `status` (`active` | `used` | `paused` | `expired` | `revoked`)
  - `max_uses` (number)
  - `used_count` (number)
  - `bound_contact_hash` (string, optional)
  - `expires_at` (timestamp, optional)
- Validation Rules:
  - 邀请码明文不得落库。
  - 校验时使用 hash 查询。
  - 使用次数更新必须与 SaaS signup 租户创建在同一事务内完成。
  - 失败注册不得增加 `used_count`。

## 11. RegistrationRequest

- Purpose: 表达候补名单或人工审核注册申请。
- Fields:
  - `id` (number)
  - `uuid` (string)
  - `mode` (`waitlist` | `approval_required`)
  - `status` (`submitted` | `approved` | `rejected` | `converted` | `cancelled`)
  - `tenant_name` (string)
  - `tenant_key` (string, optional)
  - `owner_email` (string, optional)
  - `owner_phone` (string, optional)
  - `plan` (string)
  - `invite_code_uuid` (string, optional)
  - `policy_uuid` (string)
  - `policy_version` (number)
  - `reviewed_by_user_uuid` (string, optional)
  - `reviewed_at` (timestamp, optional)
  - `reject_reason_code` (string, optional)
  - `created_tenant_uuid` (string, optional)
- Validation Rules:
  - `waitlist` 提交不得创建 tenant/user/member。
  - `approval_required` 只有审核通过后才能转换为租户创建事务。
  - 审核拒绝必须保留机器可读拒绝原因。
  - `created_tenant_uuid` 只能在转换成功后写入。

## 12. RegistrationPolicyAuditEvent

- Purpose: 表达注册准入判定、策略变更、邀请码消耗和审核动作审计。
- Fields:
  - `id` (number)
  - `uuid` (string)
  - `event_type` (`policy_changed` | `evaluate_allowed` | `evaluate_denied` | `evaluate_pending` | `invite_consumed` | `request_approved` | `request_rejected`)
  - `policy_uuid` (string)
  - `policy_version` (number)
  - `contact_hash` (string, optional)
  - `tenant_uuid` (string, optional)
  - `request_uuid` (string, optional)
  - `invite_code_uuid` (string, optional)
  - `decision` (`allow` | `deny` | `pending`)
  - `reason_code` (string)
  - `matched_rules` (array)
  - `ip` (string, optional)
  - `user_agent` (string, optional)
  - `trace_id` (string, optional)
- Validation Rules:
  - 注册允许、拒绝、候补、审核和策略变更都必须写审计。
  - 审计字段必须脱敏，不能记录邀请码明文或完整敏感 contact。
  - 每条 signup 相关审计必须可追溯到策略 UUID 和版本。

## 13. RootSupportSession

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

## 14. TenantPluginInstance

- Purpose: 表达某租户对某全局插件包的启用状态和配置。
- Fields:
  - `tenant_uuid` (string)
  - `plugin_id` (string)
  - `version` (string)
  - `enabled` (bool)
  - `status` (`available` | `subscribed` | `enabled` | `disabled` | `draining_requested` | `disabled_by_platform` | `drained` | `expired`)
  - `config` (object)
  - `drain_job_id` (string, optional)
  - `drain_requested_at` (timestamp, optional)
  - `drained_at` (timestamp, optional)
  - `created_by_member_id` (number)
  - `updated_by_member_id` (number)
- Validation Rules:
  - 全局插件包存在不代表当前租户已启用。
  - 菜单聚合、插件 admin 代理、插件 api 代理都必须校验 `enabled=true`。
  - 租户停用插件不得删除全局插件包，也不得影响其他租户实例。
  - 租户启用/停用插件不得直接启动或停止全局插件进程。
  - 同一 PowerX 节点内，同一 `plugin_id` 的运行时进程由 `PluginRuntimeProcess` 表达，不按租户复制。
  - `status=draining_requested` 后必须禁止该租户目标插件新增业务写入、scheduler job、queue task、workflow run、webhook/event delivery。
  - `status=drained` 必须表示入口已关闭且存量 session/request/task/job/event 全部清零；单纯 idle 不得标记为 drained。
  - `disabled_by_platform` 表示 Root/Platform 对目标插件实例执行了平台级禁用，不等于租户主动停用。

## 15. PluginRuntimeProcess

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

## 16. PluginDrainJob

- Purpose: 表达 Root/Platform 对某个插件或插件版本发起的下架、删除或安全禁用计划。
- Fields:
  - `job_id` (string)
  - `plugin_id` (string)
  - `version` (string, optional)
  - `scope` (`plugin` | `plugin_version`)
  - `status` (`requested` | `blocking_new_usage` | `draining` | `ready_to_uninstall` | `completed` | `failed` | `cancelled`)
  - `reason` (string)
  - `requested_by_root_user_id` (number)
  - `requested_at` (timestamp)
  - `completed_at` (timestamp, optional)
  - `affected_tenant_count` (number)
  - `drained_tenant_count` (number)
  - `last_blocker` (object, optional)
- Validation Rules:
  - 只能由 Root/Platform 创建。
  - 作用范围必须精确限定为目标 `plugin_id` 或 `plugin_id + version`。
  - 启动后必须关闭目标插件新增使用入口，但不得影响其他插件或同租户其他业务。
  - 只有所有目标 `TenantPluginInstance` 都进入 `drained` 后，才能进入 `ready_to_uninstall`。
  - 取消 drain job 必须保留审计记录，并明确是否恢复目标插件新增入口。

## 17. PluginReplaceOperation

- Purpose: 表达同一个 `plugin_id + version` 的物理包替换动作，主要用于本地开发或受控热修。
- Fields:
  - `operation_id` (string)
  - `plugin_id` (string)
  - `version` (string)
  - `status` (`requested` | `stopping_runtime` | `copying_artifact` | `healthchecking` | `completed` | `failed`)
  - `requested_by_root_user_id` (number)
  - `requested_at` (timestamp)
  - `completed_at` (timestamp, optional)
- Validation Rules:
  - replace 只能影响目标插件版本的物理目录、registry 和运行时。
  - replace 不得删除 TenantPluginInstance、订阅、权限、租户配置、凭证引用或业务数据。
  - 生产常规升级应使用新版本安装与 current version 切换，不应使用同版本 replace 作为发布路径。

## 18. IAMMigrationReport

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
