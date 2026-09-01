# Delegated Gateway Contract v1（精准版，不做兼容）

> Token 边界以 `docs/guides/auth/plugin_auth_token_model.md` 为准。

## 1. 目标

在宿主模式（delegated）下，插件访问 PowerX Capability Gateway 的认证链路只保留一套契约，避免多变量、多策略并存导致排障困难。

业务调用和启动期平台调用主凭证统一为 STS access token；`PX_PLUGIN_TOOL_TOKEN` 不再属于 delegated 契约。

## 2. 强约束（MUST）

1. 认证方案固定为 `bearer`。
2. 禁止注入、读取或依赖 `PX_PLUGIN_TOOL_TOKEN`。
3. 插件仅接受 `PX_GATEWAY_BASE_URL` 作为 Gateway 入口地址。
4. delegated 模式下，任一关键变量缺失都必须启动失败（fail-fast），禁止运行中软降级成 503。
5. delegated 模式下禁止使用 `PX_GATEWAY_API_KEY`。
6. delegated 模式下禁止读取 `PX_TOOL_TOKEN`（不保留别名兼容）。

## 3. PowerX（宿主）责任边界

1. 在插件进程启动前注入：
   - `PX_GATEWAY_BASE_URL`
   - `PX_GATEWAY_AUTH_SCHEME=bearer`
   - `POWERX_STS_CLIENT_ID`
   - `POWERX_STS_CLIENT_SECRET`
   - `POWERX_STS_AUDIENCE=powerx:api`
   - `POWERX_STS_SCOPE=access`
   - `POWERX_GRPC_UPSTREAM_ADDRESS`
   - `POWERX_GRPC_UPSTREAM_TENANT_UUID`
2. 注入后执行启动前检查，缺失即拒绝启用插件，并返回结构化错误码。
3. 插件启用成功后执行凭证探活（health + dry-run），探活目标按接口元数据（`auth_required`、`tenant_scoped`）选择，失败则将插件状态标记为启用失败并附原因。
4. 托管插件进程是全局运行态，不按业务租户复制。启动期 STS 凭证使用 `system` tenant 作为平台运行身份锚点，不得绑定任一普通业务租户。

## 4. PowerXPlugin（插件框架/运行时）责任边界

1. delegated 模式初始化 Gateway Client 时必须强校验：
   - `PX_GATEWAY_BASE_URL` 非空
   - `PX_GATEWAY_AUTH_SCHEME == bearer`
   - `POWERX_STS_CLIENT_ID` 非空
   - `POWERX_STS_CLIENT_SECRET` 非空
   - `POWERX_GRPC_UPSTREAM_ADDRESS` 非空
   - `POWERX_GRPC_UPSTREAM_TENANT_UUID` 非空
2. 删除 delegated 分支中的以下读取逻辑：
   - `PX_TOOL_TOKEN`
   - `PX_PLUGIN_TOOL_TOKEN`
   - `PX_GATEWAY_API_KEY`
3. 所有 capability 调用入口统一复用同一个 Gateway Guard，输出统一错误结构与错误码；业务请求需按当前请求上下文执行 STS exchange，禁止按 URL 前缀硬编码鉴权策略。
4. 启动日志必须输出（脱敏）：
   - `iam_mode`
   - `gateway_auth_scheme`
   - `gateway_base_url_present`
   - `sts_client_present`

## 5. 统一错误码（建议）

- `GW_CFG_MISSING_BASE_URL`
- `GW_CFG_INVALID_AUTH_SCHEME`
- `GW_CFG_MISSING_STS_CLIENT`
- `GW_CFG_MISSING_GRPC_UPSTREAM`
- `GW_CFG_APIKEY_FORBIDDEN_IN_DELEGATED`
- `GW_BOOTSTRAP_CONTRACT_BROKEN`
- `GW_AUTHZ_PERMISSION_CLAIMS_MISSING`
- `GW_AUTHZ_POLICY_VERSION_EXPIRED`
- `GW_AUTHZ_PERMISSION_DENIED`

## 6. 授权传递契约（MUST）

delegated 模式下，PowerX 是权限源，插件只声明并执行权限结果。PowerX 下发给插件前端、插件后端或网关的授权快照必须包含：

```json
{
  "subject": {
    "tenant_uuid": "tenant_uuid",
    "user_uuid": "user_uuid",
    "member_uuid": "member_uuid"
  },
  "permission_codes": [
    "example.record:read",
    "example.record:approve"
  ],
  "policy_version": "2026-08-10T10:00:00Z",
  "perms_hash": "sha256:..."
}
```

强约束：

1. `permission_codes` 必须来自插件注册到 PowerX 的 `menu/page/action/api` 权限声明，不得由 URL、前端路由或自由文本临时推导。
2. `policy_version` 与 `perms_hash` 必须同时存在；缺失时插件后端和 Gateway 均按 `GW_AUTHZ_PERMISSION_CLAIMS_MISSING` 拒绝。
3. PowerX 角色授权变更后必须推进权限策略版本；插件后端发现版本过期时返回 `GW_AUTHZ_POLICY_VERSION_EXPIRED`，不得继续使用旧快照。版本过期和 hash mismatch 的判断必须来自 PowerX signed context、短期 signed claims 或 authz/introspection 响应；插件无法验证新鲜度时必须 introspection 或拒绝。
4. 插件服务态 STS token 的 `aud` 固定表达目标受众，例如 `powerx:api`；插件身份固定来自 `plugin_id` claim。底座和插件不得把 `plugin:<plugin_id>` 放进 audience，也不得从 audience 推断插件 owner。
5. Gateway 转发插件 API 前按 `plugin_id + method + path` 映射到注册的 effective permission 并先行拦截；插件后端仍需按同一 effective permission 做二次校验。effective 规则固定为：API 有 `business_permission_code` 时使用业务权限；API 显式 `independent: true` 时才使用 raw API `permission_code`；否则声明无效。
6. local 模式只能用同一份权限声明模拟 PowerX 的授权快照，字段名、hash/version 语义必须与 delegated 模式一致，不得维护另一份正式授权定义。
7. delegated 插件后端使用 STS 调用 runtime ws-bus/taskbus 发布事件时，Event Fabric ACL 必须授权 `plugin:<plugin_id>` publish。`member:system` 与 `role:role_admin` 不代表插件服务态 principal。
8. `runtime.contract:*` 是 PowerX 与插件之间的基础设施合同权限，不是业务角色授权项。Gateway 解析到已注册的 `runtime.contract:*` route binding 后，只校验登录态、tenant context、插件租户启用状态和 route binding 存在；不得要求管理员把 `runtime.contract:*` 勾给普通业务角色。Gateway 下发给插件的 delegated token 仍必须包含该 contract permission code、`policy_version` 和 `perms_hash`，供插件后端验证快照来源。

如果采用 introspection 而不是 token claims，响应结构必须与上述快照等价：

```json
{
  "allowed": true,
  "permission_code": "example.record:approve",
  "permission_codes": [
    "example.record:read",
    "example.record:approve"
  ],
  "policy_version": "2026-08-10T10:00:00Z",
  "perms_hash": "sha256:..."
}
```

`allowed=false` 时必须返回稳定错误码和被拒绝的 `permission_code`，不能只返回布尔值。

## 7. 版本策略

本方案为 breaking change，不提供兼容期。实施顺序：

1. 先落地 PowerX 注入与启用前检查。
2. 再落地 PowerXPlugin 的严格校验与旧变量删除。
3. 最后切换文档和 CI 检查规则，阻止旧变量回流。
