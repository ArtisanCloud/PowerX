# PowerX RBAC & STS 设计说明

本说明阐述 PowerX/ CoreX 的权限模型（RBAC），以及直连 HTTP 接口的鉴权流程、与插件（Plugin）的网关鉴权与 STS（短期令牌）机制。本文基于现有实现梳理，并补充建议与最佳实践。

## 目标与原则

- 统一：CoreX 与 Plugin 使用一致的“资源/动作”判定模型（resource/action），最小化歧义。
- 解耦：路由与权限映射可显式声明（策略）或自动推导，业务层可复用服务封装与中间件。
- 最小开销：上游只做一次身份校验；到 Plugin 的二跳走 STS 短期令牌，减少重复解析与权限查询压力。
- 可观测：权限拒绝/通过均可审计，健康检查与业务调用在语义与可用性上区分清晰。

## 核心概念

- 三元权限（Triple）：`plugin/resource/action`。示例：`iam/role/read`。
- 作用域（Scope）：系统级（`system`）/ 租户级（`tenant`）。角色与权限绑定遵循作用域约束。
- 主体（Subject）：成员（member）、用户（user）等；绑定时以成员为主（目前实现）。
- 身份与声明（Claims）：`env, tenant_id/uuid, user_id, member_id, is_root, roles, platforms, scope` 等，注入于 `request.Context()`。

## 数据模型与仓储

- Permission（`iam_permission`）：唯一键 `(plugin, resource, action)`；含 `effect/status/meta/introduced/deprecated_at/source` 等。
- Role（`iam_role`）、RolePermission（`iam_role_permission`）、RoleBinding（`iam_role_binding`）。
- 仓储与服务：在 `pkg/corex/db/persistence/repository/iam/*` 与 `internal/service/iam/rbac_service.go` 提供 Upsert/授予/撤销/绑定/鉴权。
- 权限同步：支持从 OpenAPI/Swagger 生成权限并落库（`cmd/perm_gen`、`permission_repo.Sync`），支持系统默认权限与默认角色赋权（`cmd/database/seed/*`）。

## 身份认证（JWT）与上下文

- 入站统一使用 `pkg/auth/middleware/jwt.go`：校验 `issuer/audience/scope/exp/nbf/signature`，支持撤销（黑名单）、Root 代理租户、环境注入、Trace 透传。
- 中间件将声明注入 `request.Context()`，通过 `pkg/corex/iam/reqctx/*` 读取（`GetTenantID/GetUserID/IsRoot/...`）。
- 管理端“仅管理员”接口可复用 `AdminOnlyMiddleware`（允许 `system_admin/role_admin/is_root`）。

## 直连 RBAC（CoreX HTTP）

- 业务 Handler 在需要“资源-动作”粒度时调用服务层：
  - `RBACService.Enforce(ctx, tenantID, memberID, plugin, resource, action)` → 返回允许/拒绝。
  - 角色/权限/绑定相关管理操作走 `RBACService` 封装（授予/撤销/绑定/解绑/列举）。
- 可选（建议）：在路由层增加“RBAC 授权中间件”，将 `METHOD:/path` 自动映射为 `resource/action`：
  - 优先显式策略映射；否则按 HTTP 方法推导动作（GET/HEAD→read，POST→create，PUT/PATCH→update，DELETE→delete），按路径首段推导资源（必要时做单复数归一）。
  - 参考 Plugin 网关策略的实现方式，保持一致的推导规则，减少重复。

## Plugin 网关与 STS

- 动态反代：宿主将 `/_p/:id/api/*` 反向代理到插件后端（见 `internal/infra/plugin/manager/router/router.go`）。
- 路由策略（Policy）：
  - 来自插件 manifest：`endpoints.http_base_path` 与 `rbac.resources[*].actions`；健康检查 `GET|HEAD:/healthz` 固定放行。
  - 先匹配显式规则（`METHOD:/pattern`），否则自动推导（方法→动作；路径→资源，移除 `http_base_path`）。
- Authorizer：网关调用 `Authorizer.Permissions(ctx, tenantID, userID, pluginID)` 获取当前用户在该插件下拥有的权限集合（如 `note:read`）。
- 通过则颁发 STS 短期令牌（aud=`plugin:<id>`，TTL 默认 60s），替换下游 `Authorization` 头；同时透传签名上下文 `X-PowerX-CTX` 与 `X-PowerX-CTX-SIG`（HMAC）。
- 建议生产实现：
  - 用 DB/缓存实现 `Authorizer`，复用 `PermissionRepository.MemberHasPermissionViaBinding` 或等价查询；
  - 在 STS Claims 中可包含 `policy_version`/`perms_hash` 以便插件侧做细化校验与缓存版本控制。

## Health（健康检查）与差异

- CoreX 健康：`GET /api/health` 公开无鉴权，用于宿主存活/就绪检测。
- 插件健康：`/_p/:id/api/healthz` 特殊路径直达插件的 `runtime.health.http`（默认 `/healthz`），且被策略白名单放行，不参与业务权限判定。
- 区分点：
  - healthz 仅用于可用性探测，不携带业务语义与权限；
  - 普通业务路由需走 RBAC 判定 +（到插件）STS 下发。

## 权限生命周期与同步

- 自动化：
  - 从 OpenAPI 生成三元权限（按路径/方法推导）并 `UpsertBatch`；
  - 标记废弃：不在导入集且来源一致的权限置 `deprecated`（保留历史，可回滚）。
- 角色与默认赋权：

  - 系统默认角色（如 `system_admin/role_admin/role_user`）通过 Seed 确保存在并赋权；
  - 平台级 API（`module=system`）与租户级 API 区分赋权边界。

## 缓存与性能建议

- 入站 JWT 中间件已支持用户/成员/租户快照校验；
- Authorizer 建议在网关侧做权限集缓存（key=`tenant:user:plugin`，TTL=短，命中率高），并暴露失效通知（变更后刷新）。
- STS TTL 维持在 30–120 秒区间，减少下游签名解析与权限查询频率；插件应校验 `aud=plugin:<id>` 与 `iss`。

## 审计与可观测性

- 建议在：
  - 入站 JWT 拒绝、RBAC 拒绝/通过时落审计（含 `tenant/subject/resource/action/trace_id`）。
  - Plugin 网关 `CheckAndMint` 的拒绝原因（`permission required`）可聚合导出监控指标。

## 示例（片段）

1) CoreX 直连接口（在 Handler 中判定）

```go
ok, err := rbacSvc.Enforce(c.Request.Context(), 0, 0, "iam", "role", "read")
if err != nil || !ok { c.AbortWithStatus(403); return }
```

2) Plugin 路由策略（自动推导）

```
GET /v1/notes      -> note:read
POST /v1/notes     -> note:create
PATCH /v1/notes/1  -> note:update
DELETE /v1/notes/1 -> note:delete
```

3) Plugin 端校验 STS（建议）

```go
claims, err := auth.ParseAndValidate(bearer, secret, "powerx-auth", "plugin:<id>")
// 校验通过后可按需读取 X-PowerX-CTX 并验签作审计
```

## 安全注意事项

- 严格区分 Audience：CoreX 入站 JWT（如 `aud=user`）与下发至插件的 STS（`aud=plugin:<id>`）。
- 短期令牌 TTL 要短，并考虑时钟偏移；必要时支持撤销（黑名单）。
- 插件侧尽量校验 `X-PowerX-CTX-SIG`，避免上游上下文被伪造。
- 警惕通配权限（`*`/`res:*`）的滥用，最小授权原则。

## 建议的微调（保持现实现状）

- 引入通用的“路由 RBAC 中间件”（可选）：将现有 `RBACService.Enforce` 封装成 Gin 中间件，统一在路由层声明所需权限（或自动推导）。
- 在 STS Claims 中加入 `policy_version`（来自 Authorizer），便于插件快速判定策略是否需刷新。
- 开放 Authorizer 的缓存与失效接口（如 `Invalidate(tenant,user,plugin)`），绑定在角色/权限变更处触发。

---

参考实现位置（部分）：

- JWT 中间件：`pkg/auth/middleware/jwt.go`
- RBAC 服务：`internal/service/iam/rbac_service.go`
- 权限仓储：`pkg/corex/db/persistence/repository/iam/*`
- Plugin 网关与策略：`internal/infra/plugin/manager/router/*`、`internal/infra/plugin/manager/rbac.go`
- 健康检查：`internal/transport/http/admin/health_handler.go`、`internal/infra/plugin/manager/supervisor/*`

