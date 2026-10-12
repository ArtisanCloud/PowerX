# 2026-10-08 插件启动的 Gateway 地址故障

## 远程只读核对结果

- 服务器：`47.116.10.200`，Core 版本 `prod-20261008-1355`，PID `779440`，正式 HTTP 健康检查返回 200。
- 插件：`com.powerx.plugins.ai-craft@0.1.75`，PID `780062` 在 `2026-10-08 14:52:23 +08:00` 退出，错误为 `FRAMEWORK_REQUIRED_CAPABILITY_PREFLIGHT_FAILED`，内部错误 `CAPABILITY_UPSTREAM_DEPENDENCY / 503`。
- 安装关联 trace：`88c1c894-9ecf-4d8e-b110-7c538a1dee95`，request ID：`1ada89ee-cc46-4bea-b522-cc979a4f74fe`。
- 当前插件 STS Client ID 和 secret 已保存；本次未报告凭证缺失。
- Core 活跃进程环境：`POWERX_HTTP_PROXY_BASE=http://127.0.0.1:8080`，同时 `POWERX_GATEWAY_BASE_URL=https://agent.xpersonatoy.com`。
- 插件注册表中的 `PX_GATEWAY_BASE_URL` 被设为 `https://agent.xpersonatoy.com`。
- 这个域名的 TLS 证书 `notAfter=Sep 15 07:48:29 2026 GMT`；服务器上的 HTTPS 客户端实际返回 `CERTIFICATE_VERIFY_FAILED: certificate has expired`。
- 当前管理域名 `https://agent.starrtoylumi.com/api/v1/health` TLS 校验正常，返回本机 Core 的 200。

插件实际依赖 Framework `v0.0.23`。其 capability HTTP client 在 token 获取失败或 HTTP 连接失败时均返回泛化的 `503 / CAPABILITY_UPSTREAM_DEPENDENCY`；不能把这个错误直接解释为 Core 返回 503 或数据库未迁移。上述地址／证书是已核实的调用阻塞项，修正后仍需重新验收插件启动和授权结果。

## Core 修复

Host 配置生成已经优先使用内部地址，但 `ensurePluginRuntimeCredential` 的 Gateway URL 解析此前只优先读取 `POWERX_GATEWAY_BASE_URL`；随后 `applyRuntimeCredentialToEnv` 用它覆盖生成的 `PX_GATEWAY_BASE_URL`。

现在凭证注入的优先级与 Host 内部地址解析一致：

1. `POWERX_INTERNAL_GATEWAY_BASE_URL`
2. `POWERX_HTTP_PROXY_BASE`
3. `POWERX_GATEWAY_BASE_URL`
4. Core 配置的监听地址和端口

回归测试覆盖内部代理地址优先于公网地址、显式内部覆盖和无覆盖时使用监听地址。修改位于 `backend/internal/bootstrap/plugin.go`，测试位于 `plugin_gateway_base_test.go`。

## 当前服务器临时处理

由操作者修改 `/etc/powerx/powerx.env` 中的两项 URL：

```dotenv
POWERX_GATEWAY_BASE_URL=http://127.0.0.1:8080
POWERX_PUBLIC_BASE_URL=https://agent.starrtoylumi.com
```

已正确设置的 `POWERX_HTTP_PROXY_BASE=http://127.0.0.1:8080` 保留。只调整 URL，不修改 JWT、数据库或 STS secret。旧 Core helper 由此也能选中内部地址；新的公网变量保持浏览器 URL 指向当前有效管理域名。

更新环境后重启 Core 和管理前端，再在插件管理页启用已经安装的 AI Craft 版本。Core 启用路径会重新写入 Host 配置和注册表的 Gateway URL；无需再次删除插件目录。重启会短暂中断正式 API／管理前端。

验收：插件后端 URL 为 `http://127.0.0.1:8080`，启动健康检查通过，Framework 必需能力检查返回明确结果；如果仍失败，读取新 trace 下的完整致命日志继续定位。

本轮远程只进行了读取、TLS 证书检查和健康查询，没有修改服务器配置、数据库、凭证或重启服务。上述 Core 代码补丁尚需提交发布。
