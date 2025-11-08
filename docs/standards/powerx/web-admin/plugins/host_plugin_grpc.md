# 宿主（PowerX） ↔ 插件 gRPC 控制通道设计

目的：在“首次启用/密钥轮换”等场景，宿主通过本地 gRPC 安全下发租户凭证给插件；同时保持统一的 gRPC 返回规范（common.v1.ResponseMeta + data）。

## 合同（Proto）

- 路径：`api/grpc/contracts/powerx/plugin/control/v1/control.proto`
- 服务：`powerx.plugin.control.v1.ControlService`
- 方法：`UpsertTenantCredentials(tenant_id, plugin_id, client_id, client_secret)`
- 统一返回：`common.v1.ResponseMeta` + `data.ok`

插件侧应实现该服务；宿主侧已内置客户端调用逻辑（见“宿主实现”）。

## 认证与信任链

- 内部 Bearer Token：宿主在启动插件子进程时，注入环境变量：
  - `POWERX_INTERNAL_TOKEN`（随机 48 字符，内存生成，不落盘）
  - `POWERX_PLUGIN_ID`（当前插件 ID）
- 调用习惯：宿主调用插件时，在 gRPC metadata 附带：`authorization: Bearer <POWERX_INTERNAL_TOKEN>`。
- 插件侧 gRPC 拦截器需校验：
  - 仅接受来自 127.0.0.1 的请求（可选）
  - 校验 `authorization` Bearer 值与 `os.Getenv("POWERX_INTERNAL_TOKEN")` 一致
  - 按需校验 `ctx.tenant_id` 与 `plugin_id` 格式/范围

注：首次下发前插件尚未具备 STS 能力，故不使用 STS 校验。

## 宿主实现（PowerX Core）

- 生成与注入内部令牌：在 `Enable` 启动子进程前，生成随机 token 并注入 `POWERX_INTERNAL_TOKEN`、`POWERX_PLUGIN_ID`；令牌仅保存在内存映射中（`pluginID -> token`）。
- 自动推送凭证：
  - 租户首次启用（EnsureCredentials 返回明文 secret）时：通过 gRPC 调用 `UpsertTenantCredentials` 下发 `client_id/secret`。
  - 轮换密钥成功后：同样触发一次下发。
- 代码位置：
  - 令牌注入：`internal/infra/plugin/manager/lifecycle.go`
  - 令牌内存存储与获取：`internal/infra/plugin/manager/manager.go`、`helpers.go`
  - gRPC 客户端调用：`internal/infra/plugin/manager/notify/notify.go` → `PushTenantCredentials`

## 插件实现示例（Go）

- 服务器：监听 `:PORT`（由宿主注入 PORT 环境变量）并注册 ControlService；建议允许 h2c（明文 gRPC）。
- 鉴权拦截器（示例伪码）：

```go
func unaryAuthInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
    md, _ := metadata.FromIncomingContext(ctx)
    got := ""
    if v := md.Get("authorization"); len(v) > 0 {
        got = strings.TrimSpace(v[0])
    }
    if strings.HasPrefix(strings.ToLower(got), "bearer ") {
        got = strings.TrimSpace(got[7:])
    }
    want := os.Getenv("POWERX_INTERNAL_TOKEN")
    if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
        return &pcv1.UpsertTenantCredentialsResponse{Meta: &commonv1.ResponseMeta{Code: 401, Message: "unauthorized", Timestamp: time.Now().Unix()}}, nil
    }
    return handler(ctx, req)
}
```

- 处理 UpsertTenantCredentials：将 `client_id/client_secret` 安全保存（本地配置/密钥库），并用于后续 STS Exchange。

## 调用时机与重试

- 宿主在以下时机调用：
  - 租户首次启用（EnsureCredentials 明文返回时）
  - 轮换密钥成功后
- 失败处理：宿主侧记录告警日志，不阻断启用/轮换；前端仍收到一次性明文 secret 以便手工配置兜底。

## 与 HTTP 推送的取舍

- 若已实现 HTTP 管理口，可同时保留；gRPC 与 HTTP 二选一即可。
- gRPC 通道具备统一元数据与更好扩展性（未来可加更多控制面 RPC）。

## 安全建议

- 内部令牌每次进程启动随机化；不要写日志与磁盘。
- 插件端限制仅本机访问，或进一步使用 UDS/mTLS 加固。
- 插件端持久化 `client_secret` 时注意加密与最小权限；避免通过前端接口回读。

