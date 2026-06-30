# PowerX Customer Identity/Auth 权威数据模型规划

> Last Reviewed: 2026-06-25

## 1. 结论

PowerX Core 是生产环境 C 端 Customer Identity/Auth 的权威数据源。PowerXPlugin framework 只定义 customer 运行时契约、中间件、delegated adapter 和 local dev mirror 规则；业务插件只定义自己的领域模型。

因此：

1. 生产落库必须以 PowerX Core 的 customer 表为准。
2. 插件不得自定义生产 customer 主数据表。
3. 插件 local 模式可以在插件侧落一份本地表，但它只能用于开发调试，并且必须与 PowerX Core customer schema 保持兼容。
4. 如果 PowerX Core 缺少 customer 表，应该先补齐 Core，而不是让每个插件发明一套 customer schema。
5. `customer` 只表达 C 端客户/消费者/外部访问者，不表达家长、球员、学员、患者、粉丝等行业身份；这些属于插件领域模型。

## 2. 与现有 Member IAM 的边界

PowerX 现有 `iam_user`、`iam_member`、`iam_credential` 面向 B 端后台操作者：

1. `user` 是后台登录自然人账号。
2. `member` 是后台用户在租户内的员工/成员身份。
3. `role/permission/department` 支撑后台 RBAC、组织和管理权限。

Customer Identity/Auth 面向 C 端外部用户：

1. 典型入口为微信小程序、移动端 App、公开门户。
2. 核心判断是 customer 是否已登录、是否可以访问当前租户、token 是否匹配当前入口。
3. 不复用后台 member 的组织、部门和后台权限语义。

两套身份可以使用相似的 token、session、audit、tenant context 技术形态，但表和业务语义必须隔离。

## 3. 权威表范围

PowerX Core 至少应提供以下 customer 权威表。

### 3.1 `customer_accounts`

表达全局 C 端 customer 自然人账号。

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | customer 全局 UUID，唯一 |
| `status` | `active`、`disabled`、`deleted` |
| `primary_email` | 主邮箱，可空 |
| `primary_phone` | 主手机号，可空 |
| `display_name` | 通用展示名 |
| `nickname` | 昵称/社交昵称 |
| `given_name` | 名 |
| `family_name` | 姓 |
| `avatar_url` | 通用头像 |
| `locale` | 语言偏好 |
| `timezone` | 时区 |
| `metadata` | 通用扩展 JSON，不放行业字段 |
| `created_at` / `updated_at` / `deleted_at` | 生命周期字段 |

规则：

1. `customer_accounts` 不应包含 `tenant_uuid`，customer 是跨租户可复用的自然人账号。
2. 租户访问关系通过 `customer_tenant_memberships` 表达。
3. 至少一个可登录身份应落在 `customer_auth_identities`，不要求 `primary_email` 或 `primary_phone` 必填。

### 3.2 `customer_auth_identities`

表达 customer 的登录身份和第三方绑定。

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | identity UUID，唯一 |
| `customer_uuid` | 归属 customer |
| `provider` | `password`、`wechat`、`phone`、`email`、`apple`、`third_party` 等 |
| `provider_subject` | 第三方 openid/unionid/sub，可空 |
| `email` | 邮箱登录标识，可空 |
| `phone` | 手机登录标识，可空 |
| `password_hash` | 密码 hash，仅 password/provider 需要 |
| `status` | `active`、`disabled`、`deleted` |
| `verified_at` | 验证时间 |
| `metadata` | 通用扩展 JSON |
| `created_at` / `updated_at` / `deleted_at` | 生命周期字段 |

唯一性建议：

1. `provider + provider_subject` 唯一，适用于微信、Apple、第三方身份。
2. `provider + email` 在 email 非空时唯一。
3. `provider + phone` 在 phone 非空时唯一。

### 3.3 `customer_tenant_memberships`

表达 customer 在租户内的访问资格。

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | membership UUID，唯一 |
| `tenant_uuid` | 租户 UUID |
| `customer_uuid` | customer UUID |
| `status` | `active`、`pending`、`suspended`、`disabled`、`expired`、`deleted` |
| `roles` | customer 侧角色 JSON 数组 |
| `scopes` | customer 侧 scope JSON 数组 |
| `source` | `platform`、`delegated`、`third_party`、`local_dev`、`mock` |
| `expires_at` | 过期时间，可空 |
| `metadata` | 通用扩展 JSON |
| `created_at` / `updated_at` / `deleted_at` | 生命周期字段 |

规则：

1. `tenant_uuid + customer_uuid` 应唯一。
2. 只有 `active` 可以访问租户内 C 端业务。
3. `roles/scopes` 是 customer 侧语义，不复用后台 member IAM 的 permission。

### 3.4 `mini_app_entries`

表达 shared app SaaS 模式下的合法租户入口。

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | entry UUID，唯一 |
| `tenant_uuid` | 入口归属租户 |
| `entry_code` | scene、邀请码或机构码，唯一 |
| `entry_type` | `scene`、`invite_code`、`org_code`、`tenant_hint`、`direct` |
| `app_key` | shared app 标识 |
| `appid` | 小程序 appid，可空 |
| `channel` | 渠道 |
| `campaign` | 活动 |
| `brand_name` | 入口展示品牌名 |
| `org_name` | 入口展示机构名 |
| `theme` | 通用主题配置 JSON |
| `features` | 入口功能开关 JSON |
| `status` | `active`、`disabled`、`expired`、`revoked` |
| `expires_at` | 入口过期时间 |
| `metadata` | 通用扩展 JSON |
| `created_at` / `updated_at` / `deleted_at` | 生命周期字段 |

规则：

1. shared app 标准版不要求租户配置 appid/secret。
2. 租户后台生成 entry，用户必须通过合法 entry 或已有 active membership 进入 C 端业务。
3. 无合法 entry、无历史 membership、无有效机构码时，C 端入口应被拒绝。

### 3.5 `customer_sessions`

表达 customer access/refresh 生命周期。

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `uuid` | session UUID，唯一 |
| `customer_uuid` | customer UUID |
| `tenant_uuid` | 当前租户，可空 |
| `membership_uuid` | 当前 membership，可空 |
| `refresh_token_hash` | refresh token hash |
| `source` | `platform`、`delegated`、`third_party`、`local_dev`、`mock` |
| `issued_at` | 签发时间 |
| `expires_at` | 过期时间 |
| `revoked_at` | 吊销时间 |
| `metadata` | 通用扩展 JSON |
| `created_at` / `updated_at` | 生命周期字段 |

### 3.6 `customer_login_events`

表达注册、登录、校验、刷新、登出等安全审计。

| 字段 | 说明 |
| --- | --- |
| `id` | 内部主键 |
| `tenant_uuid` | 租户 UUID，可空 |
| `customer_uuid` | customer UUID，可空 |
| `identity_provider` | 登录身份来源 |
| `event_type` | `register`、`login`、`validate`、`refresh`、`logout` |
| `ok` | 是否成功 |
| `error_code` | 失败错误码 |
| `ip` | 客户端 IP |
| `user_agent` | 客户端 UA |
| `trace_id` | 链路 ID |
| `metadata` | 脱敏扩展 JSON |
| `created_at` | 创建时间 |

规则：

1. 禁止记录 raw token、明文密码、微信 secret、短信验证码。
2. 失败路径也应记录脱敏审计，方便风控和排障。

## 4. 与 PowerXPlugin Framework 的映射

PowerXPlugin framework 的 `customerfw` 不拥有生产表，只映射 Core 权威表到运行时合同。

| `customerfw` 运行时合同 | PowerX Core 权威来源 |
| --- | --- |
| `CustomerContext.customer_uuid` | `customer_accounts.uuid` |
| `CustomerContext.profile.display_name` | `customer_accounts.display_name` |
| `CustomerContext.profile.nickname` | `customer_accounts.nickname` |
| `CustomerContext.profile.given_name` | `customer_accounts.given_name` |
| `CustomerContext.profile.family_name` | `customer_accounts.family_name` |
| `CustomerContext.profile.avatar_url` | `customer_accounts.avatar_url` |
| `CustomerContext.profile.locale/timezone` | `customer_accounts.locale/timezone` |
| `CustomerContext.tenant_uuid` | `customer_tenant_memberships.tenant_uuid` 或 `mini_app_entries.tenant_uuid` |
| `CustomerContext.membership_uuid` | `customer_tenant_memberships.uuid` |
| `CustomerContext.roles/scopes` | `customer_tenant_memberships.roles/scopes` |
| `CustomerContext.source` | token validator / membership resolver 决策来源 |
| `CustomerMembership` | `customer_tenant_memberships` |
| `BootstrapContext` | `mini_app_entries` |
| `CustomerAuthResult` | `customer_accounts` + `customer_auth_identities` + `customer_sessions` |

## 5. 插件 local 模式规则

插件 local 模式只服务开发调试：

1. 插件本地 customer 表必须是 Core schema 的兼容镜像，不是生产权威设计。
2. 字段命名、UUID 语义、状态枚举、membership 规则必须与 Core 保持一致。
3. PowerX Core customer schema 变更后，PowerXPlugin skeleton/scaffold 和插件 local mirror 必须同步更新。
4. 插件 local 模式可以用更少的表实现最小闭环，但不得改变对外 contract 语义。
5. 插件业务表只能引用 `customer_uuid`、`tenant_uuid`、`membership_uuid`，不得把 customer 主数据复制成插件权威表。

## 6. Shared App 租户识别策略

SaaS 标准版采用 shared app：

1. 平台统一维护小程序 appid/secret/发布审核。
2. 租户后台生成 `mini_app_entries`。
3. 用户扫码或输入机构码后，客户端提交 `scene/invite_code/org_code`。
4. Core 解析 entry 得到 `tenant_uuid` 和入口展示配置。
5. 登录后创建或复用 `customer_tenant_memberships`。
6. 后续请求必须由 customer token、entry context 或 membership 推导出同一个 `tenant_uuid`。

无合法租户上下文时，客户端必须显示无效入口，不允许进入登录后主业务。

## 7. 后续实现顺序

1. 在 PowerX Core 增加 customer GORM models、migration、repository 和 service。
2. 对齐 `customer-auth.openapi.yaml` 的注册、登录、validate、bootstrap、membership 合同。
3. PowerXPlugin framework delegated client 调用 Core customer API。
4. 更新 PowerXPlugin skeleton/scaffold 的 local mirror schema。
5. 更新业务插件 local 模式表结构并补充 migration 校验。
6. 最后再接入具体插件的 C 端业务模型，例如球员、家长关系、训练档案等。
