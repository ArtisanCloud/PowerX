# API 前缀统一管理与版本升级规范

适用范围：PowerX Core、Web Admin、Framework 调用方及已安装插件网关。本文是前缀来源、路径转换、版本迁移和相关验收要求的共享入口；API 命名规范、Gateway 契约和前端文档引用本文，避免各自维护一套转换规则。

记录日期：2026-10-13。插件公开路径匹配修复已落实到源码，并完成回归和本机已安装 AI Craft 的转发验收；全局前缀治理、v2 业务合同以及现有 Core 服务替换与公网验收仍需分别完成。

## 1. 前缀配置与 API 版本的区别

`server.api_prefix` 当前控制 Core HTTP 路由的挂载前缀。将配置从 `/api/v1` 改为 `/api/v2`，如果仍使用原处理器和 DTO，只改变访问地址，不构成新的 v2 契约，也不会自动提供 v1/v2 并存、调用方迁移或授权兼容。

API URL 主版本只在不兼容的合同变更时升级，例如删除字段、改变字段类型或必填要求、改变响应或分页结构、改变资源语义，或改变身份与授权合同导致旧调用无法继续成立。新增接口、兼容字段、性能优化和恢复既定行为的缺陷修复通常继续使用 v1。

HTTP API 版本、Core 软件版本、插件包版本和 capability 合同版本应分别管理，不能因某个组件发布新版本就自动替换所有 URL 中的 `v1`。正式 v1 合同中的 `/api/v1` 是版本承诺，可以明确保留；运行时地址构建与路径识别必须通过统一的前缀和映射规则完成。

## 2. 前缀来源与路径坐标

| 对象 | 配置或合同来源 | 示例 | 管理边界 |
|---|---|---|---|
| Core HTTP API | `server.api_prefix` / `cfg.Server.APIPrefix` | `/api/v1/admin/...` | Core 路由挂载与调用方 Core API 基址。 |
| Core 中的插件挂载入口 | `plugin.base_prefix` 加网关约定的 `/<pluginID>/api` | `/_p/<pluginID>/api/...` | 网关访问入口，不等于插件 HTTP base。 |
| 插件上游 HTTP API | 已安装插件 manifest 的 `endpoints.http_base_path` 及相关 route 声明 | `/api/v1/integration/...` | 每个插件自己的 HTTP 合同，不受 Core API 版本自动覆盖。 |
| Web Admin 的 Core API 基址 | `runtimeConfig.public.apiBase`；由 `NUXT_PUBLIC_API_BASE` 或 `POWERX_BACKEND` 的路径推断 | `/api/v1` | 必须与当前部署支持的 Core 入口一致；不能覆盖插件独立基址。 |
| 插件后端调用 Core | Host 提供的 Gateway origin 与 Core API prefix | `{PX_GATEWAY_BASE_URL}{PX_GATEWAY_API_PREFIX}/...` | Core 调用地址，不能与插件自身 HTTP base 混用。 |

Core 使用 v2、某个插件仍使用 v1 是合法组合。应按各自合同建立明确映射，不得使用 Core API prefix 全局替换插件版本。

### 2.1 统一构建与转换要求

- 每种前缀必须有明确的权威来源和集中定义的默认值。多个进程需要不同配置载体时，由部署配置或 Host 合同显式对齐；不能靠各模块重复写默认字符串保证一致。
- 业务路由和客户端服务优先声明相对资源路径，例如 `/admin/user/auth/login`；只在入口注册或地址构建边界拼接一次对应前缀。
- 路由注册、公开策略登记、权限路径匹配、认证重定向和实际转发必须共享明确的路径转换规则。各坐标可以不同，但不能由各模块独立猜测、剥离或补上版本。
- 前缀匹配必须按路径段边界处理。例如 `/api/v1` 不得误匹配 `/api/v10`。查询串及签名凭据不能参与路径匹配或被转换规则改写。
- 历史别名和固定入口必须集中声明来源、用途及兼容范围。合同文件、集中默认值和经过声明的兼容入口可以包含版本字面量；运行模块不得散落与配置无关的版本拼接或判断。
- 当前 `PolicyFromPlugin()` 优先选择 `routes.basePath`，而上游挂载读取 `endpoints.http_base_path`。公开策略生成已拒绝两者不一致的声明，公开请求检查还会核对策略与实际挂载 base；受保护策略的解析沿用原逻辑。共用函数但传入不同 base 不能证明一致，未来如支持不同 base 映射必须单独定义合同并验证整条链路。

### 2.2 插件公开路由的比较坐标

公开策略与请求应统一转换为相对于插件有效 HTTP base 的路径。例如插件 base 为 `/api/v1`：

```text
exposure：${POWERX_PLUGIN_HTTP_BASE:-/api/v1}/integration/example/callback
展开后：  /api/v1/integration/example/callback
比较路径：/integration/example/callback
```

对 `/_p/<pluginID>/api/v1/integration/example/callback` 的请求，Gin 在网关 `/:id/api/*filepath` 下捕获的是 `/v1/integration/example/callback`。请求归一化需理解挂载过程中的这一转换，并与策略归一化得到相同的比较路径。

router 包已提供 `NormalizePublicRoutePath(path, httpBase)`。策略生成先展开 HTTP base 占位符，再调用该函数；公开请求检查和公开转发使用同一规则及对应 base。根 base 和末尾斜杠在占位符展开时处理，空段、点路径和编码分隔符不会被折叠为公开入口。

公开转发按 `upstream target path + plugin HTTP base + relative resource path` 构建，默认 v1 的实际业务路径保持不变，自定义 base 不再带上由宿主 `/api` 挂载入口产生的多余段。受保护接口继续使用原授权与转发流程。

## 3. 公开入口与渠道认证职责

公开匹配只能来自已安装插件的 `type: rest`、`auth: public` exposure。公开 REST 声明必须提供 `security.verifier`。请求头、路径前缀或请求体不能自行声明一个公开入口。

路径匹配要求：静态段精确匹配；合同中的 `{参数}` 或 `:参数` 匹配一个非空路径段；路径段数与 HTTP 方法符合声明。不得把单段参数扩展为任意深度通配，也不得因存在公开 GET 就自动放行 HEAD、OPTIONS 或其他方法。

公开入口命中后，由插件验证渠道凭据及店铺、租户、客户和会话归属。Core 的公开路由标记不表示渠道认证成功，不能代替插件验签。Shopify 对接必须原样保留以下请求头：

- `X-AI-Craft-Shopify-App-Proxy-Query`：携带原始 App Proxy 签名查询。
- `X-Shopify-Session-Token`：携带 Shopify 会话令牌。

`X-PowerX-Plugin-Public-Route` 只表示 Core 的路由判定；客户端伪造该头不得让未声明路径绕过 Core 身份校验。不同渠道入口仍按各自 verifier 验证需要的凭据，不要求所有渠道同时携带上述两个头。

## 4. v1 → v2 的迁移要求

版本升级不能以全局字符串替换或单个配置变更作为完成依据。建议按以下顺序实施：

1. 列出不兼容变更、v1/v2 请求与响应合同、调用主体及兼容期限，明确哪些接口需要升级。
2. 实现 v2 处理器或适配层；迁移期保留 v1 入口及原合同。当前单个 `server.api_prefix` 配置不代表已具备双版本注册能力。
3. 核对并更新 OpenAPI、正式 capability protocol bindings、权限登记、STS/API Key 路由判定、认证重定向和插件策略。URL 改变不能自动扩大授权范围或重命名业务 capability。
4. 更新 Web Admin、Framework/SDK、插件后端 Core client，以及实际受影响的外部调用方和代理配置。Core 版本变化不自动要求插件自身 HTTP 合同升级。
5. 分别验收 v1 兼容、v2 行为、身份与租户边界、公开渠道认证、WS/SSE 和上传下载等受影响入口，记录本地与安装环境证据。
6. 按发布与兼容计划切换调用方；确认迁移完成后再撤下 v1，保留可恢复旧合同与配置的回滚路径。

如果正式 Core capability 合同发生变化，还必须执行相应 capability 检查、登记与授权验收；仅修复插件公开路径匹配不应借机改变 Core capability 或渠道授权合同。

## 5. 回归与交付要求

### 5.1 路径配置矩阵

路由和网关变更应验证 `/api/v1`、`/api/v2`、自定义前缀及根 base（若声明支持）。这些测试证明路径配置适配能力，不代表业务 v2 合同已经交付。

同时覆盖：Core 与插件使用不同 base、base 的路径边界、受支持的挂载别名，以及 `routes.basePath` 与 `endpoints.http_base_path` 一致和不一致时的预期行为。重复斜杠、空参数、点路径和编码分隔符的处理应与上游实际路由一致，不能因为归一化意外扩大公开范围。

### 5.2 插件公开入口完整链路

回归测试必须串联真实策略生成，不能只在 manager 检查生成结果，再在 router 手工填另一种路径：

```text
已安装插件 exposure
→ PolicyFromPlugin()
→ InstallPolicy()
→ MountAPIProxy()
→ 实际 /_p 请求
→ 模拟插件上游
```

至少验证：

- 静态公开入口到达上游，并断言未调用 Core 用户身份校验、上游路径和请求体符合合同。
- `{sessionID}` 与 `:sessionID` 正确匹配一个非空段。
- 错误方法、未声明路径、空参数、额外层级及伪造公开标记继续受保护，不到达公开上游。
- 不同 HTTP base 下策略登记、请求判定和最终上游路径一致。
- 渠道请求头原样透传；模拟插件对缺失、错误凭据拒绝，对有效测试凭据通过。
- 受保护接口继续执行原身份和授权链路。

相关 Go 检查从 Core `backend` 目录运行：

```bash
go test ./internal/infra/plugin/manager ./internal/infra/plugin/manager/router
```

模拟上游只能证明网关职责和测试 verifier 行为。真实渠道验签、安装策略加载以及公网转发必须独立验收，不能用单元测试通过替代。

### 5.3 AI Craft 安装环境验收

运行修复后的 Core 并重新加载现有插件策略后，复测：

```text
POST /_p/com.powerx.plugins.ai-craft/api/v1/integration/ai-craft/shopify-storefront/runtime-identity/query
```

- POST 请求体为空，原始签名查询通过 `X-AI-Craft-Shopify-App-Proxy-Query` 传递。
- 有效签名经公网 Core 返回 200；缺失或无效签名到达插件后由插件拒绝。
- 未声明的管理接口继续要求 Core 用户身份。

本次路径匹配问题不需要数据库迁移、手工修改授权或重新打包 AI Craft。商家后台聊天消息接口的 exposure 属于独立接入任务。服务启停按仓库进程治理执行，不得擅自重启用户服务；临时调试服务须在交付前清理。

### 5.4 已安装插件的显式验收测试

`backend/internal/infra/plugin/manager/public_route_installed_test.go` 提供显式启用的安装验收。默认单元测试会跳过它；测试只读取安装 manifest 并向已运行插件发请求，不启停插件、不写入安装注册表或数据库。

| 环境变量 | 用途 |
|---|---|
| `POWERX_PUBLIC_GATEWAY_PLUGIN_ROOT` | 已安装 AI Craft 的版本目录，包含 `plugin.yaml` 与 catalogs。 |
| `POWERX_PUBLIC_GATEWAY_UPSTREAM` | 当前 Core 挂载的插件 HTTP origin，从 `__debug/plugins` 的对应 `apis.target` 读取；不能把同进程的 gRPC 端口作为 HTTP 上游。 |
| `POWERX_PUBLIC_GATEWAY_SIGNED_QUERY` | 当前有效的 Shopify App Proxy 原始签名查询。仅通过环境传给测试，不写入仓库或输出凭据；未提供时有效签名用例会跳过。 |

配置这些变量后，从 `backend` 目录运行：

```bash
go test ./internal/infra/plugin/manager -run '^TestPublicExposureInstalledPlugin$' -count=1 -v
```

该测试运行修复后的 DynamicRouter 和 Core JWT 中间件，重新从安装 exposure 生成策略，转发到已有插件。它不替代完整 Core bootstrap、用户实际服务重启或公网入口验收。

## 6. 2026-10-13 已确认现状与待办

| 位置 | 已确认行为 | 后续要求 |
|---|---|---|
| `backend/config/defaults.go`、`config.ResolveAPIPrefix()` 与 `web-admin/nuxt.config.ts` | Core 回退为 `/api`，Nuxt API base 回退为 `/api/v1`。 | 部署显式对齐；默认值统一仍需代码工作，不因本文更新自动完成。 |
| `backend/internal/infra/plugin/manager/rbac.go`、`router/public_route.go` | 已共用公开路径归一化函数，展开占位符后登记 base 相对路径；矛盾 base 不生成公开策略。 | 源码与回归完成，实际 Core 替换后复测。 |
| `backend/internal/infra/plugin/manager/router/router.go` | 公开判定与公开转发已共用相对路径，并核对实际挂载 base；受保护转发不变。认证重定向仍固定拼接 `/api/v1`。 | 公开匹配已修复；Core 认证路径来源仍需独立治理。 |
| `backend/internal/infra/plugin/manager/router/authz_gate.go` | `IsPublicRoute()` 已严格按方法、静态段和单段动态参数匹配，不使用路径 glob 或层级折叠。 | 源码与回归完成，真实渠道会话归属仍由插件验证。 |
| `web-admin/server/middleware/00-proxy-plugins.ts` | 插件认证请求固定重写到 Core `/api/v1/admin/user/auth/...`。 | 目标地址使用 Core 的有效配置；历史匹配范围独立声明。 |
| `backend/internal/transport/http/openapi/media/router.go` | 媒体传输入口直接注册在 `/api/v1`。 | 核对固定合同、地址生成与配置迁移的兼容设计。 |
| `backend/internal/http/auth_subject_validator.go` | 部分路径规范化显式处理 `/api/v1` 和 `/api`。 | 按具体 binding 与授权规则审计不同前缀，不能以简单去掉任意版本代替授权。 |
| `public_route_proxy_test.go`、`router/public_route_path_test.go` | 已补齐 exposure 到真实 HTTP 模拟上游的链路；覆盖 7 种 base、两种动态参数、凭据透传、拒绝边界及冲突 base。 | 安装与公网证据分开记录。 |

本地已安装 AI Craft `0.1.76` 清单已声明运行身份查询和 `sessions/{sessionID}/events` 为公开入口。分析中的临时完整链路复现确认：原策略请求被 Core 以 `missing_bearer` / `IAM_CONTEXT_USER_SESSION_INVALID` 拒绝，未到达模拟上游；仅在临时策略中去掉 base 后，静态入口可转发，动态入口仍失败。该对照实验没有修改 Core 源码，不能作为修复或真实 Shopify 验签验收。

修复后的验证结果：

- `go test ./internal/infra/plugin/manager ./internal/infra/plugin/manager/router`、对应 `go vet` 及 Core `cmd/app` 构建通过。
- 本机安装验收通过：重新读取 AI Craft `0.1.76` exposure，使用新编译的临时 DynamicRouter 转发到当前 Core 记录的插件 HTTP 上游。POST 请求体和 URL 查询为空，签名查询只通过规定请求头传递。
- 现有租户 Shopify 配置仅通过只读查询用于生成新鲜测试签名，未输出或落盘 Client Secret。有效签名返回 200 / running；缺失与错误签名由插件返回 `401 / RUNTIME_IDENTITY_UNAUTHORIZED`，均未进入 Core 用户身份校验。
- 未声明管理接口仍返回 `401 / IAM_CONTEXT_USER_SESSION_INVALID`，未转发给插件。
- 临时测试网关随测试关闭；用户原有 Core 和插件进程未重启。现有 Core 服务及公网是否已经运行本补丁，需要操作者替换后另做验收。

## 7. 相关文档

- [Gateway 路由与认证契约](../../guides/develop/gateway_contract.md)
- [插件发布权限声明指南](../../guides/plugin_release/permission_declaration.md)
- [Web Admin API Client 与类型化 SDK](../powerx/web-admin/state-and-data/API_Client_and_Typed_SDK.md)
- [Web Admin 环境变量对照表](../powerx/web-admin/environment/Env_Variables_Schema.md)
