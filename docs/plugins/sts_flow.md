# STS 短期令牌 Flow（Plugin → PowerX）

本文件给出插件以租户维度访问 PowerX 的 STS（Security Token Service）鉴权流程，含接口位置与最小实现要点，便于在其他项目中复用。

## 总览

- 安装态（系统）：宿主仅需安装一次插件（与租户无关）。
- 启用态（租户）：每个租户启用插件时，为“租户-插件”生成一对长期凭证 `client_id/client_secret`（落在表 `plugin_instance_configs`，key=`auth.credentials`，只存 hash）。
- 访问宿主：插件用长期凭证向 STS 交换“短期令牌”（JWT，带 exp/ttl），用该 token 调用 PowerX 的 gRPC/HTTP 接口。
- Token 只缓存内存，按需获取/预刷新；凭证泄露可轮换 secret 立即失效旧值。

## 相关 RPC 与代码位置

- 令牌交换（STS）
  - RPC: `/powerx.auth.sts.v1.STSService/Exchange`
  - Proto: `api/grpc/contracts/powerx/auth/sts/v1/sts.proto`
  - 生成代码: `api/grpc/gen/go/powerx/auth/sts/v1`
  - 实现: `internal/transport/grpc/auth/sts_handler.go`（方法 `Exchange`）
  - 注册: `internal/server/grpc/server.go`（`RegisterSTSServiceServer`）
  - 说明: 同服务还包含可选 `Introspect` 调试接口。

- 业务调用（示例：成员服务）
  - RPC: `/powerx.iam.v1.MemberService/ListMembers`（或 `GetMember/BatchGetMembers`）
  - Proto: `api/grpc/contracts/powerx/iam/v1/member.proto`
  - 生成代码: `api/grpc/gen/go/powerx/iam/v1`
  - 实现: `internal/transport/grpc/iam`
  - 注册: `internal/server/grpc/server.go`（`RegisterMemberServiceServer`）

- 统一验签
  - 鉴权拦截器: `internal/transport/grpc/auth/middleware/auth_interceptor.go`
  - KeyRing（HS256 + kid）: `internal/transport/grpc/auth/key_ring.go`
  - STS 与拦截器共用 KeyRing → Exchange 签发的 token 能直接通过业务 RPC 的拦截器校验。

## 生命周期与调度顺序

1) 启用租户插件（一次性）
- 管理员为某租户启用插件 → 宿主生成/确保该“租户-插件”的 `client_id/client_secret_hash`，保存于 `plugin_instance_configs`。
- 明文 `client_secret` 仅在首次或“轮换”时展示一次，插件需安全保存。

2) 插件启动
- 内存中“没有 token”。
- 配置中“有长期凭证”：`client_id/client_secret`（从宿主获取后保存）。

3) 首次业务调用前（需要 token）
- 调用 STS `Exchange(client_id, client_secret, audience, scope, ttl)` 获取短期 token。
- STS 校验凭证/能力（aud/scope），用 KeyRing 签发 HS256 JWT（header.kid 指示密钥）。
- 插件缓存 token 与过期时间于内存（不落盘）。

4) 后续业务调用
- 每次调用前：若 token 未临期（例如剩余>60s）→ 复用；若将近过期或已过期 → 先 `Exchange` 刷新，再调用。
- 失败兜底：业务返回 401/403 → 清空本地 token → 重新 Exchange → 重试一次。

5) 重启与轮换
- 插件重启：内存 token 丢失，首次调用再 Exchange 即可。
- 凭证轮换：宿主“轮换 secret”后旧 secret 立即失效；插件更新 `client_secret` 后继续 Exchange。

## 最小客户端逻辑（伪代码）

```
getToken():
  if token != "" and now < expiry-60s: return token
  resp = STS.Exchange(client_id, client_secret, aud="powerx:api", scope="access", ttl=300)
  token = resp.access_token; expiry = now + resp.expires_in; return token

callCoreX(req):
  tok = getToken(); ctx = withMetadata("authorization", "Bearer "+tok)
  resp, err = GRPC.Call(ctx, req)
  if err in {Unauthenticated, PermissionDenied}:
    token = ""  # force refresh
    tok = getToken(); ctx = withMetadata(...)
    resp, err = GRPC.Call(ctx, req)
  return resp, err
```

## 配置约定（建议）
- `COREX_GRPC_ADDR`：PowerX gRPC 地址（例如 `127.0.0.1:9001`）
- `COREX_PLUGIN_CLIENT_ID`：形如 `<pluginID>.<tenantID>`（例如 `com.powerx.demo.hello_world.123`）
- `COREX_PLUGIN_CLIENT_SECRET`：明文 secret（从宿主启用/轮换接口安全获取）
- `STS_AUDIENCE`（可选，默认 `powerx:api`）
- `STS_SCOPE`（可选，默认 `access`）
- `STS_TTL`（可选，默认 `300` 秒）

## 安全要点
- Token 仅存内存；TTL 建议 2–10 分钟；预留 60s 余量做预刷新。
- `client_secret` 需安全存储；轮换后旧 secret 立即失效。
- 校验 audience/scope 限定用途；记录 Exchange 与业务调用审计日志。
- 启动自恢复避免生成 `tenant_id=0` 的凭证（仅在上下文含有效租户时创建）。

---
参考实现：
- STS：`internal/transport/grpc/auth/sts_handler.go`
- KeyRing：`internal/transport/grpc/auth/key_ring.go`
- gRPC 拦截器：`internal/transport/grpc/auth/middleware/auth_interceptor.go`
- 插件凭证服务：`internal/service/setting/plugin_instance_config_service.go`
- 启用流程（PostEnable）：`internal/infra/plugin/manager/lifecycle.go`

