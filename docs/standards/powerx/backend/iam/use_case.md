# 登录与权限模型

本文定义 PowerX SaaS IAM 的登录身份、租户上下文和菜单/API 边界。当前标准以 `specs/026-iam` 为准。

## 核心结论

PowerX root 可以带 `system` 特殊租户的 `member/admin` 上下文，但这只是平台身份锚点，不等于普通业务租户 admin。

这个设定是正确且必须保留的：

1. root 是平台级账号，来源于 `iam_users.is_root = true`。
2. root 登录 token 仍需要明确的 `tenant_uuid + member_id/member_uuid`，用于鉴权上下文、审计、STS、API Key Profile、历史 setup 和初始化兼容。
3. root 的默认上下文应锚定在 `system` 特殊租户，而不是任意业务租户。
4. `system` tenant member/admin 不参与普通业务租户成员语义，不代表 root 是所有业务租户的 owner/admin。
5. root 默认进入 Platform Console；只有显式 Support Session 或显式租户视角切换，才能进入目标业务租户上下文。

因此不能把 root 简化理解为“某个业务租户 admin”，也不能把 root 的 `system` member 删除。

## 身份边界

| 登录身份 | 默认上下文 | 权限范围 / 能力 | 典型菜单 | 数据可见性 / 备注 |
| --- | --- | --- | --- | --- |
| Root（平台超级账号） | `system` 特殊租户身份锚点 + Platform Console | 平台初始化、租户治理、全局用户治理、插件包安装/版本治理、系统审计、安全、监控、备份 | 平台总览、租户管理、全局用户、插件市场管理、插件能力、发布候选、系统审计 | 不默认查看业务租户数据；不默认展示租户 AI Settings 和租户插件业务菜单 |
| Tenant Owner/Admin | 当前业务租户 `Member` | 管理本租户组织、成员、角色、AI Settings、插件订阅/实例配置、API Key | 成员管理、部门/团队、角色与授权、AI 设置、插件订阅、业务配置 | 仅本租户数据；不可跨租户 |
| Tenant Member | 当前业务租户 `Member` | 按角色和数据域使用业务功能 | 工作台、插件业务功能 | 仅当前租户内自己权限范围的数据；可在自己加入的租户间切换 |

## 登录后决策

1. 如果 `is_root=true`，默认进入 Platform Console。
2. 如果不是 root，列出该 `User` 关联的 active tenant members。
3. 如果只有一个可用 member，直接进入该租户。
4. 如果有多个可用 member，进入默认租户；用户可在“我的/租户切换”中切换。
5. 进入租户后，必须通过当前 `member_id/member_uuid` 和角色绑定判断 owner/admin/member，不能通过 `user_id` 或 `is_root` 推导。

## 路由与菜单护栏

1. 平台级菜单只对 root 可见，例如租户治理、全局插件包管理、发布候选、系统审计。
2. 租户级管理菜单只对当前租户 owner/admin 可见，例如用户、角色、AI Settings、插件订阅。
3. 插件业务菜单只在当前租户已订阅/启用对应插件实例时展示。
4. root 默认不加载租户插件业务菜单；Support Session 中按目标租户实例过滤。
5. 租户侧成员列表不展示 root 的 `system` member；业务租户成员列表只展示该业务租户内真实 member。

## Root Support Session

root 如需进入业务租户上下文，必须通过显式 Support Session：

1. 必须选择目标租户。
2. 必须填写原因。
3. token/context 必须区分普通租户登录与 root support。
4. 写操作必须记录 root actor、target tenant、support session id 和 reason。
5. Support Session 不应让 root 永久成为目标租户 member，也不应污染目标租户成员列表。

## 实现约束

1. 业务 API 必须依赖当前 `tenant_uuid + member_id/member_uuid`。
2. 普通用户只能切换到自己有 active member 的租户。
3. root 的 `system` member 只服务平台身份锚点，不服务普通租户业务授权。
4. `isCurrentTenantAdmin` 不得因为 `is_root=true` 返回 true。
5. 租户 owner/admin 权限必须来自当前租户的角色绑定，例如 `role_owner`、`role_admin`。

