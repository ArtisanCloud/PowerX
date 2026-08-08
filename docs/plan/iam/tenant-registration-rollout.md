# 租户注册准入与灰度开放机制计划

> 状态：规划方案。目标是支撑 PowerX SaaS 上线前的内测、邀请制、逐步放量和最终开放注册。

## 1. 背景

PowerX 当前已有 SaaS 注册主链路：

1. 公开注册页 `/users/register`。
2. 公开接口 `POST /api/v1/public/saas/signup`。
3. 验证码接口 `POST /api/v1/public/saas/signup/verification-code`。
4. `SaaSSignupService` 负责创建租户、owner 用户、member、默认角色、租户密钥和租户固有对象。
5. `setup/status` 目前只向前端暴露 `saas_signup_enabled` 布尔值。

这个布尔开关不足以支撑 SaaS 上线过程。上线前需要按阶段控制谁可以注册、每天可以注册多少租户、是否需要邀请码、是否需要人工审核，以及灰度规则命中后如何审计。

## 2. 目标

1. setup 安装 PowerX 时可以初始化注册准入策略。
2. root 登录后可以在后台修改注册准入策略。
3. 公开注册页按策略展示可注册、关闭、邀请制、候补或审核态。
4. 后端必须在注册和验证码发送入口强制执行策略，前端展示不能作为安全边界。
5. 支持关闭、完全开放、邀请制、候补名单、人工审核、白名单和灰度注册。
6. 灰度规则必须可解释、可审计、可回滚，不使用不可追踪的隐式随机放行。
7. 不保留旧布尔语义作为兼容兜底；旧配置迁移后应转换为明确策略对象。

## 3. 非目标

1. 不在本阶段实现计费结算。
2. 不把 root 手工创建租户改造成注册策略的一部分；root 手工创建仍是平台治理能力。
3. 不把租户注册准入做成 STS 或 agent 可调用能力。
4. 不用前端隐藏按钮代替后端准入校验。
5. 不从自由文本中解析邀请码、渠道或灰度标识；这些值必须来自结构化字段。

## 4. 注册模式

### 4.1 `closed`

关闭新租户自助注册。

行为：

1. 注册页显示关闭状态。
2. 验证码发送接口拒绝。
3. 注册接口返回明确错误，例如 `registration_closed`。
4. root 仍可在后台手工创建租户。

适用阶段：

1. 生产环境刚部署。
2. 迁移巡检未完成。
3. 发现异常需要立即停止新增租户。

### 4.2 `open`

完全开放租户注册。

行为：

1. 任意符合基础校验的用户都可以创建租户。
2. 仍必须执行验证码、速率限制、租户 key/domain 唯一性、密码策略和风控策略。
3. 每次注册写入准入审计。

适用阶段：

1. SaaS 正式开放。
2. 风控、监控、客服和回滚链路已经准备好。

### 4.3 `invite_only`

邀请制注册。

行为：

1. 注册请求必须带有效 `invite_code`。
2. 邀请码必须属于 active 批次、未过期、未超配额、符合允许套餐和身份约束。
3. 邀请码消耗必须与租户创建在同一事务中完成。
4. 失败时不得消耗邀请码。

适用阶段：

1. 第一批内测客户。
2. 合作伙伴或指定客户导入。
3. 小规模可控验证。

### 4.4 `waitlist`

候补名单。

行为：

1. 用户提交注册申请，但不创建租户。
2. 系统创建 `registration_request`，状态为 `submitted`。
3. root 后台审核后可生成邀请码，或直接进入人工开通流程。
4. 申请提交、审核、拒绝和转邀请码都必须写审计。

适用阶段：

1. SaaS 官网入口已经打开，但暂不自动开户。
2. 希望先收集客户线索和需求规模。

### 4.5 `approval_required`

注册后待审核。

行为：

1. 用户提交完整注册资料。
2. 后端创建申请记录，不直接创建 active 租户。
3. root 审核通过后，系统执行租户创建事务。
4. 审核拒绝时保留申请记录和拒绝原因。

适用阶段：

1. 企业客户开户需要人工核验。
2. 特定地区、行业或套餐需要人工审批。

### 4.6 `allowlist`

白名单注册。

行为：

1. 仅允许指定邮箱、手机号、邮箱域名、企业域名或渠道标识注册。
2. 白名单规则必须结构化保存。
3. 命中规则写入审计事件，未命中明确拒绝。

适用阶段：

1. 内部员工试用。
2. 指定企业域名客户。
3. 指定渠道或合作伙伴批次。

### 4.7 `progressive_rollout`

灰度注册。

行为：

1. 系统按配置的规则组合判断是否允许注册。
2. 支持时间窗口、批次、渠道、邮箱域、邀请码、每日额度、总额度和百分比放量。
3. 灰度结果必须可解释：每次准入结果都记录命中的规则、拒绝原因和策略版本。
4. 百分比规则必须使用稳定种子，例如 contact hash，不得每次请求随机变动。

适用阶段：

1. 从邀请制过渡到公开入口。
2. 按周或按批次逐步扩大内测。
3. 出现异常时快速降级到更严格模式。

## 5. 策略模型

系统设置保存权威策略对象：

```text
platform.registration.policy
```

建议结构：

```json
{
  "version": 1,
  "mode": "progressive_rollout",
  "status": "active",
  "requires_verification": true,
  "requires_invite_code": false,
  "requires_root_approval": false,
  "daily_tenant_quota": 100,
  "total_tenant_quota": 1000,
  "start_at": "2026-08-10T00:00:00+08:00",
  "end_at": "2026-09-01T00:00:00+08:00",
  "rules": [
    {
      "type": "email_domain_allowlist",
      "values": ["artisancloud.cn", "powerx.ai"]
    },
    {
      "type": "invite_batch",
      "batch_uuid": "00000000-0000-0000-0000-000000000000"
    },
    {
      "type": "percentage",
      "value": 10,
      "seed": "contact"
    }
  ]
}
```

规则：

1. `mode` 是权威注册模式。
2. `requires_verification` 控制验证码是否为注册前置条件。
3. `requires_invite_code` 为 true 时，请求必须提交结构化 `invite_code`。
4. `requires_root_approval` 为 true 时，不直接创建 active 租户。
5. `daily_tenant_quota` 和 `total_tenant_quota` 必须按策略版本和自然日统计。
6. `start_at/end_at` 不匹配时明确拒绝，不自动回退到 open。
7. 未识别的规则类型必须 fail fast，不能跳过。

## 6. 数据模型

### 6.1 `registration_policies`

保存策略版本和当前状态。

字段建议：

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | 策略 UUID |
| `version` | 策略版本号 |
| `mode` | 注册模式 |
| `status` | `draft`、`active`、`archived` |
| `policy_json` | 完整策略 JSON |
| `activated_at` | 生效时间 |
| `created_by_user_uuid` | 创建人 |
| `updated_by_user_uuid` | 更新人 |
| `created_at` / `updated_at` | 生命周期字段 |

规则：

1. 同一时间只能有一个 active 策略。
2. 修改策略必须生成新版本，旧版本归档。
3. 注册审计必须记录命中的策略 UUID 和版本。

如果短期内不建独立表，也可以先放入 `system_settings`，但需要保留同样的版本、状态和审计字段语义。

### 6.2 `registration_invite_batches`

保存邀请码批次。

字段建议：

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | 批次 UUID |
| `name` | 批次名称 |
| `status` | `draft`、`active`、`paused`、`expired`、`revoked` |
| `max_codes` | 最大邀请码数量 |
| `max_uses_per_code` | 单码可用次数 |
| `allowed_plan` | 允许套餐 |
| `allowed_email_domains` | 允许邮箱域名 JSON |
| `allowed_channels` | 允许渠道 JSON |
| `starts_at` / `expires_at` | 有效期 |
| `created_by_user_uuid` | 创建人 |
| `created_at` / `updated_at` | 生命周期字段 |

### 6.3 `registration_invite_codes`

保存单个邀请码。

字段建议：

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | 邀请码 UUID |
| `batch_uuid` | 批次 UUID |
| `code_hash` | 邀请码 hash，不保存明文 |
| `status` | `active`、`used`、`paused`、`expired`、`revoked` |
| `max_uses` | 最大使用次数 |
| `used_count` | 已使用次数 |
| `bound_contact` | 可选绑定邮箱或手机号 hash |
| `expires_at` | 过期时间 |
| `created_at` / `updated_at` | 生命周期字段 |

规则：

1. 明文邀请码只在生成时展示一次。
2. 校验时按 hash 查询。
3. 消耗次数与租户创建必须在同一事务内更新。

### 6.4 `registration_requests`

保存候补和审核申请。

字段建议：

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | 申请 UUID |
| `mode` | 来源模式：`waitlist` 或 `approval_required` |
| `status` | `submitted`、`approved`、`rejected`、`converted`、`cancelled` |
| `tenant_name` | 申请租户名称 |
| `tenant_key` | 申请租户 key |
| `owner_email` / `owner_phone` | 申请人联系信息 |
| `plan` | 申请套餐 |
| `invite_code_uuid` | 可选邀请码 UUID |
| `policy_uuid` | 策略 UUID |
| `policy_version` | 策略版本 |
| `reviewed_by_user_uuid` | 审核人 |
| `reviewed_at` | 审核时间 |
| `reject_reason_code` | 拒绝原因码 |
| `created_tenant_uuid` | 转换成功后的租户 UUID |
| `created_at` / `updated_at` | 生命周期字段 |

### 6.5 `registration_policy_audit_events`

保存准入判定和配置变更审计。

字段建议：

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | 审计 UUID |
| `event_type` | `policy_changed`、`evaluate_allowed`、`evaluate_denied`、`invite_consumed`、`request_approved` |
| `policy_uuid` | 策略 UUID |
| `policy_version` | 策略版本 |
| `contact_hash` | 邮箱或手机号 hash |
| `tenant_uuid` | 创建成功后的租户 UUID |
| `request_uuid` | 关联申请 UUID |
| `invite_code_uuid` | 关联邀请码 UUID |
| `decision` | `allow`、`deny`、`pending` |
| `reason_code` | 机器可读原因 |
| `matched_rules` | 命中规则 JSON |
| `ip` / `user_agent` / `trace_id` | 请求上下文 |
| `created_at` | 创建时间 |

## 7. 后端服务设计

新增服务建议：

```text
RegistrationPolicyService
InviteCodeService
RegistrationRequestService
```

核心方法：

```go
Evaluate(ctx, RegistrationPolicyEvaluateInput) (RegistrationPolicyDecision, error)
ConsumeInviteInTx(ctx, tx, inviteCode, decision) error
CreateRegistrationRequest(ctx, input, decision) error
ActivatePolicy(ctx, input) error
```

准入决策结构：

```json
{
  "decision": "allow",
  "mode": "progressive_rollout",
  "policy_uuid": "...",
  "policy_version": 3,
  "reason_code": "matched_email_domain_allowlist",
  "matched_rules": ["email_domain_allowlist", "daily_quota"],
  "requires_verification": true,
  "requires_invite_code": false,
  "requires_root_approval": false
}
```

规则：

1. `SaaSSignupService.Signup` 事务前必须先执行策略判定。
2. `invite_only` 和带邀请码的灰度策略必须在租户创建事务内消耗邀请码。
3. `approval_required` 不直接调用租户创建逻辑，而是创建申请。
4. `waitlist` 只创建候补记录，不创建用户、租户或 member。
5. 验证码发送也必须执行策略判定，避免关闭或邀请制时泄露可用入口。
6. 所有拒绝必须返回机器可读错误码，不返回模糊字符串。

## 8. API 设计

### 8.1 公开接口

查询注册状态：

```text
GET /api/v1/public/saas/registration-policy/effective
```

用途：

1. 注册页展示当前模式。
2. 显示是否需要邀请码、验证码、候补或审核。
3. 不返回敏感规则细节，例如完整白名单、配额剩余内部细节。

发送验证码：

```text
POST /api/v1/public/saas/signup/verification-code
```

变更：

1. 发送前执行注册策略判定。
2. 关闭、候补、审核待提交前置不满足时拒绝。
3. 邀请制可要求先提交邀请码再发送验证码。

注册租户：

```text
POST /api/v1/public/saas/signup
```

新增字段：

```json
{
  "invite_code": "PX-XXXX-XXXX",
  "channel": "launch_partner",
  "campaign": "private_beta_2026",
  "registration_request_uuid": ""
}
```

规则：

1. `invite_code/channel/campaign` 是结构化字段。
2. 不从 `tenant_name`、备注或 URL 自由文本中解析。
3. 不满足策略时不创建租户。

提交候补申请：

```text
POST /api/v1/public/saas/registration-requests
```

### 8.2 root 后台接口

策略读取与更新：

```text
GET /api/v1/admin/registration-policy
PUT /api/v1/admin/registration-policy
POST /api/v1/admin/registration-policy/activate
```

邀请码批次：

```text
GET /api/v1/admin/registration-invite-batches
POST /api/v1/admin/registration-invite-batches
POST /api/v1/admin/registration-invite-batches/:batchUuid/codes
POST /api/v1/admin/registration-invite-batches/:batchUuid/pause
POST /api/v1/admin/registration-invite-batches/:batchUuid/revoke
```

审核申请：

```text
GET /api/v1/admin/registration-requests
POST /api/v1/admin/registration-requests/:requestUuid/approve
POST /api/v1/admin/registration-requests/:requestUuid/reject
POST /api/v1/admin/registration-requests/:requestUuid/issue-invite
```

Capability 边界：

1. root 后台策略和邀请码治理属于 admin user 能力。
2. 不设置 `sts_direct: true`。
3. 不作为 agent 可选能力。
4. 新增路由必须补正式 platform capability REST binding 或明确 ignore；产品后台能力优先补声明，不用 ignore 逃避配置完整性。

## 9. 前端设计

### 9.1 setup 安装页

setup 增加“租户注册策略”配置块：

1. 默认关闭注册。
2. 可选择关闭、邀请制、候补、审核、灰度、开放。
3. 可配置验证码是否必需。
4. 可配置初始每日额度和总额度。
5. 保存后写入 `platform.registration.policy` 初始版本。

### 9.2 root 后台

建议入口：

```text
设置 > 平台设置 > 租户注册
```

页面能力：

1. 查看当前策略状态和版本。
2. 切换注册模式。
3. 配置灰度规则、时间窗口和配额。
4. 管理邀请码批次和单码状态。
5. 查看候补和审核申请。
6. 查看准入审计事件。
7. 一键暂停注册，生成新的 `closed` active 策略版本。

### 9.3 公开注册页

注册页根据 effective policy 展示：

1. `closed`：显示关闭状态和联系入口。
2. `open`：显示完整注册表单。
3. `invite_only`：要求邀请码。
4. `waitlist`：显示候补申请表。
5. `approval_required`：显示注册申请表和审核说明。
6. `allowlist`：显示注册表单，但后端严格校验联系人。
7. `progressive_rollout`：按 effective policy 返回的公开字段展示注册表单或等待状态。

所有用户可见文案必须写入 locale，不在 Vue 组件中硬编码。

## 10. 灰度上线阶段

### 阶段 0：关闭注册

策略：

```text
mode = closed
```

目标：

1. 完成生产安装和基础巡检。
2. 验证 root 手工建租户、登录、租户切换、固有对象初始化。

验收：

1. 公开注册和验证码发送均返回关闭错误。
2. root 可手工创建租户。
3. 审计有关闭策略命中记录。

### 阶段 1：邀请制内测

策略：

```text
mode = invite_only
requires_verification = true
daily_tenant_quota = 20
```

目标：

1. 第一批内测客户可开户。
2. 每个邀请码批次可独立暂停和撤销。
3. 邀请码使用情况可追踪。

验收：

1. 无邀请码无法注册。
2. 邀请码过期、暂停、超次数时明确拒绝。
3. 租户创建成功和邀请码消耗在同一事务。

### 阶段 2：白名单灰度

策略：

```text
mode = progressive_rollout
rules = email_domain_allowlist + daily_quota
```

目标：

1. 开放指定企业域名或合作伙伴渠道。
2. 控制每日新增租户数量。

验收：

1. 命中域名白名单才允许注册。
2. 超出每日额度后明确拒绝。
3. 审计展示命中规则和额度原因。

### 阶段 3：百分比放量

策略：

```text
mode = progressive_rollout
rules = percentage + quota + time_window
```

目标：

1. 从 5% 到 10%、25%、50% 逐步扩大。
2. 同一联系人每次评估结果稳定。

验收：

1. 百分比命中基于稳定 contact hash。
2. 同一联系人不会在 allow/deny 之间随机跳变。
3. root 可快速切回 `invite_only` 或 `closed`。

### 阶段 4：候补或审核开放

策略：

```text
mode = waitlist
```

或：

```text
mode = approval_required
```

目标：

1. 对外开放入口。
2. 仍然控制真实开户。

验收：

1. 提交申请不创建租户。
2. root 审核通过后才创建租户。
3. 审核拒绝保留原因和审计。

### 阶段 5：完全开放

策略：

```text
mode = open
requires_verification = true
```

目标：

1. SaaS 正式开放。
2. 保留验证码、速率限制、审计和一键关闭能力。

验收：

1. 普通用户可以自助创建租户。
2. 异常时 root 可立即切换到 `closed`。

## 11. 迁移计划

当前配置：

```yaml
feature_gate:
  enable_saas_signup: false
  enable_saas_signup_verification_code: false
```

迁移规则：

1. `enable_saas_signup=false` 转为 `mode=closed`。
2. `enable_saas_signup=true` 转为 `mode=open`。
3. `enable_saas_signup_verification_code` 转为 `requires_verification`。
4. 迁移后公开接口只读取策略对象。
5. 旧布尔配置不作为运行时 fallback；缺策略时后端 fail fast，并提示执行 setup 或 root 后台初始化策略。

## 12. 观测与审计

指标建议：

1. `powerx_registration_evaluate_total{mode,decision,reason_code}`
2. `powerx_registration_signup_total{mode,result}`
3. `powerx_registration_invite_consume_total{batch_uuid,result}`
4. `powerx_registration_request_total{mode,status}`
5. `powerx_registration_policy_change_total{mode}`

日志字段：

1. `trace_id`
2. `policy_uuid`
3. `policy_version`
4. `mode`
5. `decision`
6. `reason_code`
7. `matched_rules`
8. `invite_batch_uuid`
9. `registration_request_uuid`
10. `tenant_uuid`

审计要求：

1. 策略创建、激活、暂停、回滚都写审计。
2. 邀请码生成只记录 hash 和批次，不记录明文。
3. 注册拒绝也写准入审计，但敏感信息脱敏。
4. root 审核动作记录审核人、理由和结果。

## 13. 验收清单

后端：

1. `RegistrationPolicyService.Evaluate` 覆盖所有模式。
2. 注册和验证码发送入口都执行策略判定。
3. 邀请码消耗和租户创建同事务。
4. 缺策略、未知模式、未知规则类型均 fail fast。
5. `make capability-check` 通过。
6. Service 测试覆盖 closed、open、invite_only、waitlist、approval_required、allowlist、progressive_rollout。

前端：

1. setup 可初始化注册策略。
2. root 后台可查看和激活策略版本。
3. 注册页根据 effective policy 展示对应状态。
4. 用户可见文案走 i18n。
5. 不把 UUID 作为业务对象主展示文本。

数据：

1. 所有业务对象表有稳定 UUID。
2. 跨表引用使用 `*_uuid`。
3. 邀请码明文不落库。
4. 策略和准入审计可按策略版本追溯。

运维：

1. 支持一键切换 `closed`。
2. 支持查看实时注册拒绝原因分布。
3. 支持按邀请码批次暂停放量。
4. 支持导出内测批次使用情况。

## 14. 实施顺序

1. 新增策略对象、DTO、service 和 service tests。
2. 将 setup 初始化和 root 后台配置写入同一策略对象。
3. 改造公开 effective policy、验证码和注册接口。
4. 新增邀请码批次和邀请码消耗事务。
5. 新增候补/审核申请模型和 root 审核接口。
6. 改造注册页、setup 页和 root 后台页。
7. 补 platform capability 声明、OpenAPI 合同和 i18n。
8. 补审计、指标和运行指南。
9. 执行灰度阶段验收。

## 15. 代码映射

当前相关入口：

| 模块 | 路径 |
| --- | --- |
| SaaS 注册服务 | `backend/internal/service/auth/saas_signup_service.go` |
| 验证码服务 | `backend/internal/service/auth/signup_verification_service.go` |
| SaaS 注册 HTTP | `backend/internal/transport/http/public/saas/signup_handler.go` |
| setup 状态 | `backend/internal/transport/http/admin/system/setup_handler.go` |
| 系统设置服务 | `backend/internal/service/system/setting_service.go` |
| 系统设置模型 | `backend/pkg/corex/db/persistence/model/setting/system_setting_gorm.go` |
| 租户模型 | `backend/pkg/corex/db/persistence/model/tenant/tenant_gorm.go` |
| 注册页 | `web-admin/app/pages/users/register.vue` |
| setup 页 | `web-admin/app/pages/setup/index.vue` |
| setup 状态 composable | `web-admin/app/composables/useSetupStatus.ts` |
| IAM 使用指南 | `docs/guides/features/026-iam/guide.md` |

建议新增：

| 模块 | 路径 |
| --- | --- |
| 注册策略 service | `backend/internal/service/auth/registration_policy_service.go` |
| 邀请码 service | `backend/internal/service/auth/registration_invite_service.go` |
| 注册申请 service | `backend/internal/service/auth/registration_request_service.go` |
| 注册策略模型 | `backend/pkg/corex/db/persistence/model/iam/registration_policy_gorm.go` |
| 邀请码模型 | `backend/pkg/corex/db/persistence/model/iam/registration_invite_gorm.go` |
| 注册申请模型 | `backend/pkg/corex/db/persistence/model/iam/registration_request_gorm.go` |
| root 后台 HTTP | `backend/internal/transport/http/admin/iam/registration_policy_handler.go` |
| public effective policy HTTP | `backend/internal/transport/http/public/saas/registration_policy_handler.go` |
| root 后台页面 | `web-admin/app/pages/settings/registration/index.vue` |

