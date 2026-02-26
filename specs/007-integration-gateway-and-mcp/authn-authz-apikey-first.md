# API Key / JWT 分流鉴权方案（Integration Gateway 全域）

## 1. 目标与范围

- 目标：统一 Integration Gateway 全部入口的鉴权行为，采用 **单请求单凭证分流**。
- 范围：`/api/v1` 下所有 OpenAPI Gateway 路由（含 tenant/admin/internal 入口），`/internal/ws-bus/register|publish` 只是其中一个子场景。
- 约束：不再出现“某些接口只认 JWT、某些接口只认 API Key”的分裂实现。

## 2. 鉴权规则（强约束）

1. `Authorization: ApiKey <key>`：仅走 API Key 链路。
2. `Authorization: Bearer <token>`：仅走 JWT 链路。
3. 不支持 `X-API-Key`。
4. 不采用“API Key 失败回退 JWT”的混合兜底策略。

## 3. 统一授权模型

API Key 与 JWT 最终都收敛到同一 `AuthContext`：

- `tenant_uuid`
- `principal_type`（`user` / `api_key`）
- `principal_id`
- `roles[]`
- `scopes[]`
- `actions[]`
- `plugin_id`（可选）

授权判断只看统一上下文，不区分来源实现。

## 4. API Key 数据模型（含主体分组）

建议维护四类实体（tenant 级可管理）：

1. `api_key_profile`
   - 字段：`id`, `tenant_uuid`, `name`, `status`, `description`, `created_by`, `created_at`
   - 语义：API Key 的用途主体（如 `notify.prod` / `notify.staging`）。
2. `gateway_api_keys`
   - 字段：`id`, `tenant_uuid`, `profile_id`, `name`, `key_prefix`, `key_hash`, `status`, `expires_at`, `last_used_at`, `created_by`, `created_at`
   - 规则：明文 key 只在创建时返回一次；库内仅存 hash + prefix。
3. `gateway_api_key_permissions`
   - 字段：`profile_id`（或 `api_key_id` 扩展位）, `scope`, `action`, `resource_type`, `resource_pattern`, `plugin_id`, `effect`
   - 语义：默认以 `profile` 为权限模板，key 继承该模板。
4. `gateway_api_key_audit_logs`
   - 字段：`api_key_id`, `tenant_uuid`, `path`, `method`, `result`, `reason`, `trace_id`, `requested_at`
   - 用于审计、排障、风控。

## 5. 多租户与角色策略

- 每个租户都可创建并管理本租户 `api_key_profile` 与 API Key。
- 普通租户只可授予本租户范围内 scope。
- `root/admin` 仅做跨租户“有效性治理”（禁用/吊销/审计），不查看其他租户明文 key。

## 6. Scope 体系（统一命名）

建议统一前缀化 scope：

- `_scope.gateway.invoke`
- `_scope.gateway.capability.read`
- `_scope.gateway.capability.manage`
- `_scope.event.topic.publish`
- `_scope.event.topic.subscribe`
- `_scope.event.topic.replay`
- `_scope.ws.topic.subscribe`
- `_scope.ws.topic.publish`
- `_scope.scheduler.job.run`
- `_scope.scheduler.job.manage`

> topic / subscriber / kind 仍沿 `_topic.*`、`_subscriber.*`、`_kind.*` 命名；scope 不混用 topic 名称。

## 7. 管理端（Web Admin）能力

新增“租户 API Key 管理”页面（tenant root / role_admin 可管理）：

- `api_key_profile` 列表：名称、状态、描述、创建人。
- profile 权限矩阵：按 scope/action/resource 配置模板权限。
- key 列表：名称、prefix、状态、过期时间、最近使用、创建人。
- key 生命周期：创建、轮换、吊销、审计。

## 8. 与插件 Host / Standalone 对齐

- Host 模式：使用 JWT 调用 Gateway/OpenAPI。
- Standalone Proxy 与外部平台：使用 API Key 调用 Gateway/OpenAPI。
- 插件自定义 topic 不要求普通用户运行时动态注册；应在插件安装/升级时由平台完成 topic + ACL 预注册。

## 9. 迁移与收敛

- 新中间件统一注入 `AuthContext`，所有 handler 鉴权逻辑收敛到统一授权器。
- 内部接口（ws-bus 等）不保留入口特化鉴权分支。

## 10. 验收标准

1. Gateway 全入口支持 `ApiKey` 与 `Bearer` 两种 scheme，且行为一致。
2. 任一路由都不再依赖 `X-API-Key`，且不存在失败回退 JWT。
3. 租户管理员可在 UI 完成 `api_key_profile` 与 key 全生命周期管理。
4. 审计可按 `api_key_id`、`tenant_uuid`、`trace_id` 检索。
5. Host（JWT）/Standalone（API Key）两种插件模式都能跑通联调。
