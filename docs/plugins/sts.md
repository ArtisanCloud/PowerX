# gRPC STS 令牌交换机制（Plugin → PowerX）

本文描述插件服务以租户维度访问宿主 PowerX 时的鉴权机制：插件使用租户侧的 client_id/client_secret 调用 STS（Security Token Service）换取短期访问令牌（JWT），并用该令牌调用 PowerX 的 gRPC/HTTP 接口。

## 核心目标

- 每租户独立凭证：同一插件对不同租户拥有不同 `client_id/client_secret`。
- 短期令牌（STS）：插件以 client_credentials 流程换取短期 JWT，减少长期秘钥暴露面。
- 与服务端验签对齐：STS 签发与 gRPC 拦截器使用同一 KeyRing（HS256 + kid），调用方携带 `Authorization: Bearer <token>` 即可通过验证。

## 概念与数据

- 安装态（系统维度）：通过注册表（`plugins/registry.json`）表示插件包是否安装/启用，和租户无关。
- 启用态（租户维度）：`plugin_instance_configs(tenant_id, plugin_id, key)` 表记录每个租户对某插件的启用与配置。
  - `key = "auth.credentials"` 用于存放 `{client_id, client_secret_hash, ...}`（只存 hash）。
  - `client_id` 约定格式：`<pluginID>.<tenantID>`（例：`com.powerx.demo.hello_world.123`）。
  - 明文 `client_secret` 只在创建/轮换时展示一次，需要插件安全保存。

## 服务端组件

- STS 服务实现：`internal/transport/grpc/auth/sts_handler.go`
  - 方法 `Exchange(ExchangeRequest) -> ExchangeResponse`
  - 校验 client_id/client_secret → 颁发短期 JWT（HS256，带 `kid`）。
- KeyRing：`internal/transport/grpc/auth/key_ring.go`
  - 维护多套 HS256 密钥（区分 user/customer/default 等），拦截器与 STS 共用。
- gRPC 鉴权拦截器：`internal/transport/grpc/auth/middleware/auth_interceptor.go`
  - 从 metadata 读取 `authorization: Bearer <token>` 并用 KeyRing 验签（优先使用 token header 中的 `kid`）。

## 令牌交换（Exchange）

请求要点（client_credentials 流程）：
- `client_id`: 形如 `<pluginID>.<tenantID>`；
- `client_secret`: 最新轮换得到的明文；
- `audience`（可选）：默认 `powerx:api`（或按服务端约定）；
- `scope`（可选）：默认 `access`；
- `ttl_seconds`（可选）：默认 `300`（建议 120–600 秒）。

服务端校验流程：
1. 解析 `client_id` → 拆出 `pluginID` 与 `tenantID`；
2. 调 `PluginInstanceConfigService.VerifyClient(...)` 比对密钥与能力约束（aud/scope）；
3. 选取 KeyRing HS 密钥，构造 `CoreXClaims`，签发 HS256 JWT，header 写入 `kid`；
4. 返回 `access_token/token_type/expires_in/audience/scope/issuer/subject/issued_at`。

令牌内容（要点）：
- 算法/密钥：HS256（KeyRing），header.kid = 所用密钥的标识；
- Claims：`tenant_id`（如有）、`scope`、`aud`、`iss`、`sub=client:<client_id>`、`iat/exp`；
- 仅短期有效（`exp = iat + ttl`）。

## 使用令牌访问 PowerX

- gRPC：在 metadata 设置 `authorization: Bearer <access_token>`，即通过服务端拦截器校验；
- HTTP（可选）：若需要用 STS 令牌打 HTTP，需要将 HTTP 中间件的验签策略与 STS 对齐（可单独对齐 issuer/secret 或挂载专用中间件）。

## 插件侧令牌生命周期（推荐）

- 内存缓存：进程内保存 `access_token` 与 `expires_at`；不要长久化。
- 预刷新：调用前若剩余寿命 < 60s，则先调用 STS 交换刷新；
- 失败兜底：业务调用返回 401/403 时，立即刷新一次再重试；
- 进程重启：启动时用持久化的 `client_id/secret` 重新换取 STS 令牌。

伪代码（Go）示例：

```go
func (c *Client) getToken(ctx context.Context) (string, error) {
    if c.token != "" && time.Now().Before(c.expiry.Add(-60*time.Second)) {
        return c.token, nil
    }
    req := &stsv1.ExchangeRequest{
        ClientId:     c.ClientID,
        ClientSecret: c.ClientSecret,
        Audience:     "powerx:api",
        Scope:        "access",
        TtlSeconds:   300,
    }
    resp, err := c.sts.Exchange(ctx, req)
    if err != nil || resp.GetMeta().GetCode() != 200 {
        return "", fmt.Errorf("sts exchange failed: %v", err)
    }
    c.token = resp.GetData().GetAccessToken()
    c.expiry = time.Now().Add(time.Second * time.Duration(resp.GetData().GetExpiresIn()))
    return c.token, nil
}

func (c *Client) CallPowerX(ctx context.Context, req *pb.SomeRequest) (*pb.SomeReply, error) {
    tok, err := c.getToken(ctx)
    if err != nil { return nil, err }
    md := metadata.Pairs("authorization", "Bearer "+tok)
    ctx = metadata.NewOutgoingContext(ctx, md)
    resp, err := c.corex.SomeRPC(ctx, req)
    if status.Code(err) == codes.Unauthenticated || status.Code(err) == codes.PermissionDenied {
        c.token = "" // 强制刷新
        tok, rerr := c.getToken(ctx)
        if rerr != nil { return nil, rerr }
        md := metadata.Pairs("authorization", "Bearer "+tok)
        ctx = metadata.NewOutgoingContext(ctx, md)
        return c.corex.SomeRPC(ctx, req)
    }
    return resp, err
}
```

## 凭证生成与轮换

- 生成：租户启用插件（PostEnable）时，宿主为该“租户-插件”生成 `auth.credentials`（只存 hash），首次明文 secret 只此一次；
- 轮换：管理员通过“轮换凭证”接口触发 `RotateSecret`，得到新明文 secret（旧 secret 立即失效）；
- 插件需保存 `client_id` 与最新 `client_secret`（安全存储），用以随时换取 STS 令牌；
- 宿主侧 `enabled` 字段可控制该租户对插件的启用/禁用状态。

## 安全建议

- 最小化 TTL：建议 2–10 分钟范围，过短会增加交换频次，过长增加泄露风险；
- 安全存储 client_secret：避免写入日志/前端；轮换后尽快分发并替换；
- 校验 audience/scope：按场景限制可用范围；
- 审计：记录 Exchange/业务调用的 tenant/plugin/subject/trace_id；异常（401/403）计数告警；
- 租户上下文：仅在有 `tenant_id>0` 的上下文中为该租户生成凭证，避免产生 `tenant_id=0` 记录。

---

参考实现位置：
- STS 服务：`internal/transport/grpc/auth/sts_handler.go`
- KeyRing：`internal/transport/grpc/auth/key_ring.go`
- gRPC 拦截器：`internal/transport/grpc/auth/middleware/auth_interceptor.go`
- 插件凭证：`internal/service/setting/plugin_instance_config_service.go`
- 插件启用流程：`internal/infra/plugin/manager/lifecycle.go`

