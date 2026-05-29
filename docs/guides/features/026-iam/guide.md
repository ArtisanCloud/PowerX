# SaaS IAM 注册、登录与租户切换指南

## 功能背景与目标

PowerX SaaS IAM 的核心边界是：`User` 是全局账号，`Member` 是用户在某个租户里的身份，登录 token 必须同时明确 `tenant_uuid + member_id/member_uuid`。本指南用于验证自助注册、多租户登录、租户切换、root 边界和历史数据迁移是否与 `specs/026-iam` 一致。

目标：

1. 新用户可以通过公开注册入口创建租户，并成为该租户 owner/admin/user。
2. 同一个 user 可以加入多个租户，登录后默认进入最近使用的有效租户。
3. 切换租户必须重新签发 token，不能只改前端状态。
4. 手机号注册用户不得显示伪造邮箱。
5. 历史数据上线前必须先迁移、再巡检、再开放 SaaS 能力。

## 角色与适用范围

适用角色：

1. SaaS 新用户：使用注册页创建租户。
2. 租户 owner/admin/member：登录、切换租户、使用租户业务能力。
3. Root：平台运维与全局插件包治理，不默认进入租户业务控制台。
4. QA/运维：执行迁移、巡检、回归测试。

不适用范围：

1. 外部身份源同步。
2. 租户计费结算。
3. 插件业务内部数据模型。

## 整体架构与模块关系

```mermaid
flowchart LR
  RegisterPage[Web Admin 注册页] --> SignupAPI[/POST /api/v1/public/saas/signup/]
  LoginPage[Web Admin 登录页] --> LoginAPI[/POST /api/v1/admin/user/auth/login/]
  TenantMenu[我的/租户切换] --> SwitchAPI[/POST /api/v1/admin/user/auth/me/switch-tenant/]
  SignupAPI --> SignupSvc[SaaSSignupService]
  LoginAPI --> AuthSvc[AuthService]
  SwitchAPI --> AuthSvc
  SignupSvc --> DB[(PostgreSQL IAM)]
  AuthSvc --> DB
  AuthSvc --> Token[Access/Refresh Token]
  Token --> Context[/GET /api/v1/admin/user/auth/me/context/]
```

关键数据：

1. `iam_user.email/phone/password credential` 是全局登录凭证。
2. `iam_user.last_tenant_uuid` 是最近租户偏好，不是权限。
3. `iam_member.tenant_uuid + user_id` 决定用户在租户里的身份。
4. `role_owner`、`role_admin`、`role_user` 决定租户内权限。

## 核心流程

```mermaid
flowchart TD
  A[输入租户名称、组织标识、邮箱或手机号、密码] --> B{验证码开关}
  B -->|关闭| C[提交 signup]
  B -->|开启| D[先发送并校验验证码]
  D --> C
  C --> E{tenant_key 是否显式填写}
  E -->|否| F[按 tenant_name 生成唯一 key]
  E -->|是| G[校验 key 唯一]
  G -->|冲突| X[返回 409 并回滚]
  F --> H[事务创建 tenant/user/member/roles]
  G --> H
  H --> I[更新 last_tenant_uuid]
  I --> J[签发 token + context]
```

失败分支：

1. 显式 `tenant_key` 已存在：返回冲突错误，不保留半成品 tenant/member。
2. 已有 user 但密码错误：拒绝创建第二租户。
3. 验证码关闭时仍不需要 `verification_code`。

## 跨角色协作流程

```mermaid
flowchart LR
  subgraph User[用户]
    U1[注册或登录]
    U2[切换租户]
  end
  subgraph Web[Web Admin]
    W1[提交表单]
    W2[保存新 token]
    W3[刷新 me/context]
  end
  subgraph Backend[PowerX Backend]
    B1[创建租户和成员]
    B2[选择默认租户]
    B3[重签 token]
    B4[更新 last_tenant_uuid]
  end
  subgraph Ops[运维/QA]
    O1[make db-migrate]
    O2[make iam-migration-report]
  end

  U1 --> W1 --> B1 --> B4
  U2 --> W1 --> B3 --> W2 --> W3
  O1 --> O2
```

## 前置条件与依赖

配置：

```yaml
feature_gate:
  enable_saas_signup_verification_code: false
```

前端环境：

```bash
NUXT_PUBLIC_SAAS_SIGNUP_VERIFICATION_ENABLED=false
```

发布前必须执行：

```bash
make db-migrate
make db-seed
make iam-migration-report
```

`make db-migrate` 必须包含：

1. `iam_tenant.domain` 历史回填。
2. `iam_member.username` 调整为租户内唯一。
3. API key profile key 调整为租户内唯一。
4. `iam_user.last_tenant_uuid` 字段。

## 操作步骤

### 页面操作步骤

注册：

1. 动作：打开注册页。
2. 入口：`/users/register`。
3. 填写：租户名称、组织标识、邮箱或手机号、密码、确认密码。
4. 预期结果：注册成功后进入工作台，当前 context 指向新租户和新 member。
5. 失败处理：如果组织标识重复，修改为新的 `tenant_key`；如果已有手机号/邮箱，确认密码是否为该全局账号密码。

登录：

1. 动作：使用邮箱或手机号登录。
2. 入口：`/users/login`。
3. 预期结果：单租户用户直接进入该租户；多租户用户进入 `last_tenant_uuid` 指向的最近有效租户。
4. 失败处理：检查该 user 是否仍有 active member。

切换租户：

1. 动作：在“我的/租户切换”选择目标租户。
2. 入口：前端调用 `/api/v1/admin/user/auth/me/switch-tenant`。
3. 预期结果：前端保存返回的新 `access_token/refresh_token`，随后 `me/context` 与 token claims 指向同一租户成员。
4. 失败处理：403 表示当前 user 没有目标租户 active member。

### 接口调用步骤

SaaS 注册：

```bash
curl --noproxy '*' -sS http://127.0.0.1:8080/api/v1/public/saas/signup \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_name": "Acme Inc",
    "tenant_key": "acme-inc",
    "plan": "free",
    "owner_phone": "13800000000",
    "owner_password": "secret123",
    "owner_display_name": "Owner"
  }'
```

预期响应片段：

```json
{
  "data": {
    "access_token": "...",
    "refresh_token": "...",
    "context": {
      "current_tenant_uuid": "...",
      "current_member_id": 1
    }
  }
}
```

切换租户：

```bash
curl --noproxy '*' -sS http://127.0.0.1:8080/api/v1/admin/user/auth/me/switch-tenant \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"tenant_uuid":"<target-tenant-uuid>"}'
```

预期响应必须包含：

```json
{
  "data": {
    "token_type": "Bearer",
    "access_token": "...",
    "refresh_token": "...",
    "context": {}
  }
}
```

### 本地联调步骤

```bash
cd backend
make db-migrate
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache go test ./tests/integration/iam ./internal/service/auth ./internal/transport/http/admin/user/auth ./pkg/corex/db/migration -count=1

cd ../web-admin
npm run test:unit -- tests/unit/iam
npm run build
```

## 预期结果与验收标准

1. 新租户注册后，首个 member 同时拥有 `role_owner`、`role_admin`、`role_user`。
2. `tenant_name` 可以重复；未显式填写 `tenant_key` 时系统自动生成唯一 key。
3. 显式 `tenant_key` 冲突返回错误，并且没有半成品 tenant/member。
4. 手机号注册用户不显示默认邮箱。
5. 登录不要求先选组织；多租户用户进入最近有效租户。
6. 切换租户后 token、context、页面当前租户一致。
7. root 默认不被前端判定为租户 admin。
8. IAM migration report 不破坏 root、system tenant、组织架构和已有角色绑定。

## 代码实现映射

| 能力 | 路径 |
|---|---|
| SaaS signup service | `backend/internal/service/auth/saas_signup_service.go` |
| 验证码 service | `backend/internal/service/auth/signup_verification_service.go` |
| SaaS signup handler | `backend/internal/transport/http/public/saas/signup_handler.go` |
| 登录和租户切换 service | `backend/internal/service/auth/auth_service.go` |
| me/switch-tenant handler | `backend/internal/transport/http/admin/user/auth/me_extra_handler.go` |
| User last tenant repository | `backend/pkg/corex/db/persistence/repository/iam/user_repo.go` |
| IAM migrations | `backend/pkg/corex/db/migration/202605270001_*.go` 到 `202605270004_*.go` |
| 注册页 | `web-admin/app/pages/users/register.vue` |
| 登录页 | `web-admin/app/pages/users/login.vue` |
| me service | `web-admin/app/composables/api/services/meService.ts` |
| user store | `web-admin/app/stores/user.ts` |

## 常见问题与排障

注册时报 `tenant key already exists`：

1. 显式组织标识已被占用。
2. 修改组织标识后重试。
3. 不要手动删除已有 tenant 规避唯一约束。

手机号注册后显示邮箱：

1. 检查 `iam_user.email` 是否被写入伪造默认邮箱。
2. 检查 Header/Sidebar 是否按 email、phone、`-` 顺序展示。

切换租户后接口仍打旧租户：

1. 检查前端是否保存 `switch-tenant` 返回的新 token。
2. 检查新 token claims 是否包含目标 `tenant_uuid + member_id/member_uuid`。
3. 检查 `me/context` 是否强制刷新。

登录进入错误租户：

1. 检查 `iam_user.last_tenant_uuid`。
2. 检查该 user 在目标租户是否仍有 active member。
3. 最近租户无效时应回退到第一个 active member。

迁移失败：

1. 先看 `make db-migrate` 输出的具体约束或索引名。
2. 不手动删 root/setup/组织架构数据。
3. 修复后重新执行 `make iam-migration-report`。

## 回滚与风险控制

1. 发布前备份 PostgreSQL。
2. 先执行迁移，再执行 `make iam-migration-report`。
3. 若 `manual_fix_required` 非空，不开放 SaaS signup。
4. 验证码开关默认关闭；未接入 SMTP/短信前不要在生产开启。
5. 出现 token/context 不一致时，优先回滚前端切换租户入口或禁用多租户切换入口，保留登录基础能力。

## 变更记录

| 日期 | 变更 | 责任 |
|---|---|---|
| 2026-05-27 | 对齐 SaaS signup、登录默认租户、switch-tenant 重签 token、验证码开关和 IAM 迁移验收 | PowerX Core |
