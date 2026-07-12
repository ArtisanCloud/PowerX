---
name: sts
description: PowerX STS 与插件鉴权规范（Exchange、KeyRing、拦截器、审计）。
---

# PowerX STS

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### dev_sts_guides.md

````markdown
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

### 5.1 HTTP direct route 边界
- 插件调用底座能力的推荐主路径是 `/api/v1/tenant/invocations`。
- 插件 STS token 直接访问 Core HTTP 时，允许集合由 capability governance 管理：
  `static plugin runtime contracts + formal platform_capabilities REST endpoints - STS blocklist`。
- 普通开放 REST 能力必须先进入正式 `backend/config/platform_capabilities/*.yaml` 的 REST protocol；不得通过手工改 STS validator 代替能力登记。
- `/api/v1/admin/*` 是后台用户态 API 命名空间。插件 Admin 页面、PowerX Admin 页面、以及任何携带用户 JWT 的后台请求，仍然由用户鉴权、租户成员、RBAC 和业务权限判定，不受服务态 STS direct blocklist 影响。
- 普通 STS token 是插件服务态身份，不携带 `uid/mid`，不能代表登录用户调用 `/api/v1/admin/*` 绕过用户 RBAC。插件后端如果要代表当前用户调用底座后台 API，必须引入 delegated/on-behalf-of 机制。
- 对服务态 STS direct call，`/admin/*`、`/internal/*`、`/public/*`、`/auth/*`、`/setup/*`、debug、migration、root、drain、bootstrap、mock、health、根级动态路径默认不允许。确认为插件服务运行时合同的少量入口必须进入 static allow 并补测试。

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
````
