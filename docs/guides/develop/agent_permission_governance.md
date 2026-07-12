# Agent 使用授权开发说明

## 权限模型

Agent 运行时权限由“成员/角色是否可使用该 Agent”和能力级交集共同决定：

```text
effective_allowed = agent_access_allowed && user_allowed && agent_allowed && tenant_enabled && policy_allowed
```

字段语义：

- `agent_access_allowed`：当前成员或其角色是否被授权使用该 Agent。
- `user_allowed`：成员通过 IAM 角色绑定获得的权限，由 `RBACService.Enforce` 判断。
- `agent_allowed`：Agent 自身允许调用该能力。底座能力和 Agent 所属插件能力是基线能力，默认允许；其他插件能力通过 Agent grants 增量授权。
- `tenant_enabled`：当前租户是否启用该 capability。
- `policy_allowed`：能力策略是否允许 Agent 使用。

## 生效权限计算规则

生效权限预览不落库为结果快照。接口每次读取配置、IAM、Capability Registry 后动态计算，并使用 PowerX Cache 做短 TTL 缓存。

计算入口：

- 指定成员预览：`GET /api/v1/admin/agents/{agent_uuid}/effective-permissions?member_uuid={member_uuid}&env=dev`
- 当前登录成员预览：`GET /api/v1/admin/agents/{agent_uuid}/my-effective-permissions?env=dev`
- 服务实现：`backend/internal/service/agent_authz/service.go`

核心步骤：

1. 读取 Agent，确认 `env + tenant_uuid + agent_uuid` 下 Agent 存在。
2. 读取可授权能力目录 `ListGrantableCapabilities`。
3. 读取 Agent 能力授权 `agent_capability_grants`。
4. 读取 Agent 使用授权 `agent_access_grants`，判断成员或其角色是否允许使用该 Agent。
5. 对每条 capability 计算 `user_allowed`、`agent_allowed`、`tenant_enabled`、`policy_allowed`。
6. 按 `effective_allowed = agent_access_allowed && user_allowed && agent_allowed && tenant_enabled && policy_allowed` 输出最终结果。

### 用户 IAM 判断

`user_allowed` 由 `RBACService.Enforce` 判断。成员有效权限必须包含：

- 成员直绑角色：`iam_role_binding.subject_type = MEMBER`
- 成员通过组织/团队/岗位/用户组 assignment 继承的角色

维度映射是显式规则：

| `iam_member_assignment.dim_type` | `iam_role_binding.subject_type` |
| --- | --- |
| `ORG` | `ORG_UNIT` |
| `TEAM` | `TEAM` |
| `POSITION` | `POSITION` |
| `GROUP` | `GROUP` |

### Capability 到 IAM 的映射

插件能力和底座 REST 能力不能用同一种 `permission_code` 解释。

插件能力使用插件声明的结构化 `permission_code`。例如：

```text
com.powerx.plugins.base.template:create
=> module=com.powerx.plugins.base
=> resource=template
=> action=create
```

插件 capability sync 会把 `permission_code` 注册成 IAM permission，并自动授给当前租户的 `role_owner` / `role_admin`。普通成员角色不默认继承插件能力。

如果某个插件能力本来就是普通用户可用能力，插件 descriptor 必须显式声明：

```yaml
default_role_grants:
  - role_user
```

支持位置：

- 顶层 `default_role_grants`
- `security.default_role_grants`
- `rbac.default_role_grants`

允许角色码：`role_owner`、`role_admin`、`role_user`、`role_readonly`、`role_vendor`。非法角色码会导致能力同步失败，不能静默忽略。

底座 REST capability 不直接用 `corex.rest:*` 作为 IAM 三元组，必须从 capability `protocols` 里的 REST binding 读取 `method + endpoint`，并复用平台权限生成规则转换成 IAM 三元组。

转换规则来自 `backend/internal/service/integration_gateway/apikeypermissions/platform_capabilities.go` 的 `RESTPermissionTriple`。

示例：

```text
GET /api/v1/admin/agents
=> module=admin
=> resource=admin_agents
=> action=list
```

```text
GET /api/v1/admin/agents/:uuid
=> module=admin
=> resource=admin_agents_uuid
=> action=read
```

HTTP action 规则：

| Method | Action |
| --- | --- |
| `GET` 无路径参数 | `list` |
| `GET` 有路径参数 | `read` |
| `POST` | `create` |
| `PUT` / `PATCH` | `update` |
| `DELETE` | `delete` |

### Agent 能力授权判断

`agent_allowed` 的基线规则：

- PowerX/CoreX 底座能力默认允许。
- Agent 所属插件能力默认允许。
- 其他插件能力必须通过 `agent_capability_grants` 显式授权。

注意：`com.powerx.plugins.base.local` 和 `com.powerx.plugins.base` 是两个独立插件实体，不能合并判断。

### 缓存规则

生效权限结果使用 PowerX `pkg/cache.ICache` 缓存，缓存只用于加速，不是权限事实源。

缓存 key 包含：

- `env`
- `tenant_uuid`
- `agent_uuid`
- `member_uuid`
- `member_id`
- `user_uuid`
- `is_root`
- Agent 授权配置版本
- 租户 IAM 版本
- 算法版本

默认 TTL：`30s`。

失效规则：

- 修改 `agent_access_grants` 或 `agent_capability_grants` 后，递增该 Agent 的授权配置版本。
- 修改 IAM 角色权限、成员角色绑定、角色权限集合后，递增该租户 IAM 版本。
- 算法规则变化时必须提升算法版本，避免旧缓存继续命中。

## 页面职责

- `设置 > 用户管理 > 权限管理`：维护 IAM 权限、角色、成员绑定。
- `设置 > AI 设置 > 智能体管理`：维护 Agent 基础信息和能力边界。
- `设置 > AI 设置 > 智能体授权`：配置成员/角色能否使用某个 Agent，并预览成员最终生效权限。

“生效权限预览”是授权配置页的辅助信息，不是独立的诊断主入口。

## 后端接口

### Agent 使用授权

```http
GET /api/v1/admin/agents/{agent_uuid}/access-grants?subject_type=member|role&env=dev
PATCH /api/v1/admin/agents/{agent_uuid}/access-grants?env=dev
```

PATCH 请求体：

```json
{
  "grants": [
    {
      "subject_type": "member",
      "subject_uuid": "member-uuid",
      "enabled": true
    }
  ]
}
```

约束：

- `agent_uuid`、`subject_uuid` 必须使用 UUID。
- `subject_type` 只能是 `member` 或 `role`。
- PATCH 是增量更新，不全量覆盖当前 Agent 的其他授权对象。
- subject 必须属于当前租户；缺失或跨租户直接失败。

### 生效权限预览

```http
GET /api/v1/admin/agents/{agent_uuid}/effective-permissions?member_uuid={member_uuid}&env=dev
GET /api/v1/admin/agents/{agent_uuid}/my-effective-permissions?env=dev
```

用途：

- 管理页预览指定成员使用该 Agent 时的最终交集。
- 聊天工作台查看当前登录成员对当前 Agent 的生效权限。

约束：

- 指定成员预览必须显式传 `member_uuid`。
- 不允许在缺失 `member_uuid` 时回退到当前用户。

## 关键代码

- Agent 使用授权模型：`backend/internal/server/agent/persistence/model/agent_gorm.go`
- Agent 使用授权仓储：`backend/internal/server/agent/persistence/repository/agent_access_grant.go`
- 路由：`backend/internal/transport/http/admin/agent/api.go`
- Handler：`backend/internal/transport/http/admin/agent/agent_authz_handler.go`
- 计算：`backend/internal/service/agent_authz/service.go`
- 生效权限缓存：`backend/internal/service/agent_authz/effective_permissions_cache.go`
- 底座 REST capability 到 IAM 映射：`backend/internal/service/integration_gateway/apikeypermissions/platform_capabilities.go`
- IAM 有效角色/权限：`backend/internal/service/iam/rbac_service.go`、`backend/pkg/corex/db/persistence/repository/iam/permission_repo.go`
- 前端 API：`web-admin/app/composables/agent/useAgentManager.ts`
- 授权页面：`web-admin/app/pages/settings/ai/agent-access-grants.vue`
- 入口菜单：`backend/internal/transport/http/admin/menu/system_menus_handler.go`

## 前端实现规则

- 所有用户可见文案必须走 `web-admin/i18n/locales/*.json`。
- 成员、角色、Agent 选择和列表展示名称，不把 UUID 作为主标签。
- UUID 只作为请求参数、隐藏值或调试元数据。
- 成员/角色授权开关必须调用增量 PATCH，不做全量覆盖。
- 角色授权依赖 `iam.roles.uuid`，旧数据由迁移补齐 UUID。

## 验证命令

```bash
cd backend
go test ./internal/service/agent_authz ./internal/service/integration_gateway/apikeypermissions ./pkg/corex/db/persistence/repository/iam ./pkg/cache
```

```bash
cd web-admin
npm run build
```
