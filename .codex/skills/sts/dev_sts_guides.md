# PowerX STS & 插件对接规范

> 目标：定义插件以**租户维度**访问 PowerX 的鉴权方式（STS 令牌交换）、凭证生成/轮换、令牌使用与安全要求。  
> 传输层可为 gRPC/HTTP，拦截器/中间件与 Crypto KeyRing 统一。

## 1. 范围与术语
- STS（Security Token Service）：`Exchange(client_id, client_secret, aud, scope, ttl)` → 短期 JWT。
- 客户端：插件进程（per-tenant）。
- KeyRing：HS256 密钥集合（带 `kid`），STS 与 gRPC 拦截器共用。

## 2. 架构与目录
- STS 服务：`internal/transport/grpc/auth/sts_handler.go`（`Exchange`）
- KeyRing：`internal/transport/grpc/auth/key_ring.go`
- gRPC 拦截器：`internal/transport/grpc/auth/middleware/auth_interceptor.go`
- Proto：`api/grpc/contracts/powerx/auth/sts/v1/sts.proto`（Buf 生成到 `api/grpc/gen/go/...`）
- 插件-宿主关系与访问形态：详见《powerx_agent_plugin.md》。

## 3. 凭证模型（租户维度）
- 启用插件生成：`plugin_instance_configs(tenant_id, plugin_id, key="auth.credentials")`
    - 字段：`client_id`（`<pluginID>.<tenantID>`）、`client_secret_hash`（仅存 hash）。
    - 明文 `client_secret` 仅创建/轮换时展示一次，插件自行安全保存。
- 轮换：旧 secret 立即失效，插件更新后继续 Exchange。

## 4. 令牌交换（STS Exchange）
- 请求：`client_id`、`client_secret`、`audience=powerx:api`、`scope=access`、`ttl=300（秒）`。
- 返回：`access_token`（HS256，header.kid 写入）、`expires_in`、`aud`、`scope`、`iss`、`sub=client:<client_id>`。
- 验证：STS 验签与 gRPC 拦截器使用**同一 KeyRing**（kid 选择密钥）。

## 5. 令牌使用（插件 → PowerX）
- gRPC：在 metadata 设置 `authorization: Bearer <token>`；拦截器校验通过后带入 `tenant_id/actor` 上下文。
- HTTP（如需）：中间件与 STS 对齐验签策略（issuer/secret/kid）。
- 客户端缓存：仅内存缓存；若剩余寿命 <60s 先刷新；401/403 触发强制刷新再重试一次。

## 6. 安全与审计
- TTL 建议 2–10 分钟；`client_secret` 安全存储；校验 `aud/scope` 最小权限；
- 审计：记录 Exchange/业务调用的 `tenant/plugin/subject/trace_id`；异常 401/403 计数告警。
- 禁止在 `tenant_id=0` 上下文下生成租户凭证。

## 7. 与 Agent/插件关系（何时走 MCP）
- 插件直调 PowerX（gRPC/HTTP）或对外自暴露服务；
- 需要纳入统一“工具目录/市场”时，将插件能力包装为 MCP 工具（详见《powerx_agent_plugin.md》）。

## 8. 验收要点（Checklist）
- [ ] 存在 STS `Exchange` 实现与注册，Proto 契约落在 `powerx/auth/sts/v1`；
- [ ] KeyRing（HS256+`kid`）与拦截器复用，STS 签发的 token 可直接通过业务 RPC 鉴权；
- [ ] 插件凭证落在 `plugin_instance_configs`（仅存 hash），支持“轮换”；
- [ ] 客户端仅内存缓存 token，支持预刷新与 401/403 强制刷新；
- [ ] 审计与安全策略（TTL/aud/scope/告警）到位；
- [ ] 与 HTTP/gRPC 的错误语义一致（Unauthenticated/PermissionDenied）。
