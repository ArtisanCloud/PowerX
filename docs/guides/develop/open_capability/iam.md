# IAM Provisioning 能力

本文面向插件后端和 PowerXPlugin framework。目标是让插件在 delegated 模式下，通过 PowerX Core 的服务态能力创建当前租户内的普通成员和插件托管角色。

## 能力边界

插件服务态调用使用 STS token，不代表某个后台管理员用户。因此插件不能直接调用 `/api/v1/admin/iam/*` 绕过用户 RBAC。

PowerX 提供两个受限服务态能力：

| 场景 | capability_id | REST endpoint |
| --- | --- | --- |
| 查询可绑定租户角色 | `com.corex.iam.roles.read` | `GET /api/v1/tenant/iam/roles` |
| 创建插件托管租户角色 | `com.corex.iam.roles.provision` | `POST /api/v1/tenant/iam/roles/provision` |
| 创建租户成员并绑定允许角色 | `com.corex.iam.members.provision` | `POST /api/v1/tenant/iam/members/provision` |

共同规则：

- 调用主体必须是 `powerx-sts` 签发、`aud=powerx:api` 的服务态 STS token。
- 租户只能来自 token claims，request body 不接受 `tenant_uuid`。
- 不能创建 root、system admin、tenant owner、tenant admin 或 builtin 角色。
- 角色 code 必须使用 `plugin_` 前缀，并且不能只有 `plugin_`。
- 成员只能绑定 `role_user` 或 `plugin_` 前缀角色。
- API 返回 `user_uuid`、`member_uuid`、`role_uuid`，插件不得保存或传播 numeric id。

## 直接 REST 调用

准备 STS token：

```bash
export API_ORIGIN="http://127.0.0.1:8077"
export POWERX_STS_TOKEN="<plugin-sts-token>"
```

查询当前租户可绑定角色：

```bash
curl -sS "$API_ORIGIN/api/v1/tenant/iam/roles?page=1&page_size=50&include_builtin=true" \
  -H "Authorization: Bearer $POWERX_STS_TOKEN" | jq .
```

成功响应只包含 `role_user` 和插件托管角色：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "items": [
      {
        "role_uuid": "2b2b1657-02d3-478a-8212-556636e22972",
        "code": "role_user",
        "name": "Tenant User",
        "builtin": true,
        "tenant_uuid": "9d52d6f1-2c15-4ab4-b82f-9dddfd4ef103"
      },
      {
        "role_uuid": "6b5d0240-9920-46da-b707-88200e0f51ea",
        "code": "plugin_crm_sales",
        "name": "CRM Sales",
        "builtin": false,
        "tenant_uuid": "9d52d6f1-2c15-4ab4-b82f-9dddfd4ef103"
      }
    ],
    "pagination": {
      "total": 2,
      "page": 1,
      "page_size": 50,
      "pages": 1
    }
  }
}
```

创建插件角色：

```bash
curl -sS -X POST "$API_ORIGIN/api/v1/tenant/iam/roles/provision" \
  -H "Authorization: Bearer $POWERX_STS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "code": "plugin_crm_sales",
        "name": "CRM Sales",
        "description": "CRM plugin sales role"
      }' | jq .
```

成功响应：

```json
{
  "code": 201,
  "message": "success",
  "data": {
    "payload": {
      "role_uuid": "6b5d0240-9920-46da-b707-88200e0f51ea",
      "code": "plugin_crm_sales",
      "name": "CRM Sales",
      "tenant_uuid": "9d52d6f1-2c15-4ab4-b82f-9dddfd4ef103"
    }
  }
}
```

创建成员：

```bash
curl -sS -X POST "$API_ORIGIN/api/v1/tenant/iam/members/provision" \
  -H "Authorization: Bearer $POWERX_STS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "username": "alice_crm",
        "email": "alice@example.com",
        "display_name": "Alice",
        "initial_password": "password123",
        "role_codes": ["role_user", "plugin_crm_sales"],
        "source_external_id": "crm-user-001",
        "metadata": {
          "source": "crm"
        }
      }' | jq .
```

成功响应：

```json
{
  "code": 201,
  "message": "success",
  "data": {
    "payload": {
      "user_uuid": "4e20cb1b-6b72-4a24-bcb4-6c80c7c26e94",
      "member_uuid": "8c4d8a7c-3b36-439d-8065-e6f84ecfe333",
      "username": "alice_crm",
      "email": "alice@example.com",
      "role_codes": ["plugin_crm_sales", "role_user"],
      "tenant_uuid": "9d52d6f1-2c15-4ab4-b82f-9dddfd4ef103"
    }
  }
}
```

## 通过能力调度调用

插件也可以通过统一能力入口调用：

```bash
curl -sS -X POST "$API_ORIGIN/api/v1/tenant/invocations" \
  -H "Authorization: Bearer $POWERX_STS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "capability_id": "com.corex.iam.members.provision",
        "preferred_protocol": "rest",
        "payload": {
          "method": "POST",
          "endpoint": "/api/v1/tenant/iam/members/provision",
          "body": {
            "username": "alice_crm",
            "email": "alice@example.com",
            "display_name": "Alice",
            "initial_password": "password123",
            "role_codes": ["role_user", "plugin_crm_sales"]
          }
        }
      }' | jq .
```

直接 REST 适合 framework 内部封装和简单调试；`/tenant/invocations` 适合统一 Selector、trace、授权和协议适配。

推荐作业顺序：

1. 调用 `com.corex.iam.roles.read` 查询当前租户已有 `role_user` 和 `plugin_` 角色。
2. 如果缺少插件业务角色，调用 `com.corex.iam.roles.provision` 创建。
3. 调用 `com.corex.iam.members.provision` 创建成员并绑定 `role_user` 与插件角色。

## 常见失败

| 错误 | 含义 | 处理 |
| --- | --- | --- |
| `sts token not allowed for this route` | 当前运行库还没有同步 capability，或旧进程没有加载新代码 | 执行 `make capability-seed` 并重启 Core |
| `iam.provision.service_actor_required` | 使用了用户 JWT 或错误 audience 的 token | 插件后端先通过 STS exchange 获取 `powerx:api` token |
| `iam.provision.include_builtin_invalid` | `include_builtin` 不是布尔值 | 使用 `true` 或 `false` |
| `iam.provision.role_code_prefix_required` | 角色 code 没有 `plugin_` 前缀 | 改成插件托管角色，例如 `plugin_crm_sales` |
| `iam.provision.role_code_suffix_required` | 角色 code 只有 `plugin_` | 补充插件自己的稳定后缀 |
| `iam.provision.role_code_conflict` | 当前租户已有同名角色 | 换稳定 code，或先查询已有角色后复用 |
| `iam.provision.role_code_not_allowed` | 成员绑定了非 `role_user` 或非 `plugin_` 角色 | 只传 `role_user` 和插件托管角色 |
| `iam.provision.initial_password_required` | 创建新用户时没有初始密码 | 新用户必须提供 `initial_password` |
| `iam.provision.username_conflict` | 当前租户中该 username 已被其他成员占用 | 换用符合规则且租户内唯一的 username |

重复调用 `com.corex.iam.members.provision` 时，如果邮箱对应的用户已经是当前租户成员，PowerX 会返回已有 `member_uuid`，不会覆盖原有 `username` 或 `display_name`。

## 发布和验证

代码发布后，需要同步能力注册表：

```bash
make capability-seed
```

远程 systemd dev 环境要指定运行时配置：

```bash
cd /opt/powerx-dev/backend
sudo -u powerx ./platform_capability_seed -config /etc/powerx-dev/config.yaml
sudo systemctl restart powerx-dev-backend
```

验证当前租户是否能解析 IAM provisioning 能力：

```bash
curl -sS "$API_ORIGIN/api/v1/tenant/capabilities/resolve?source=corex&method=POST&endpoint=/api/v1/tenant/iam/members/provision" \
  -H "Authorization: Bearer $POWERX_STS_TOKEN" | jq .
```

PowerX Core 会写入 `iam.provisioning` 审计事件，并在应用日志中输出 `[iam-provision]` 结构化记录，排障时可以按 `tenant_uuid`、`actor_subject`、`member_uuid` 或 `role_uuid` 检索。
