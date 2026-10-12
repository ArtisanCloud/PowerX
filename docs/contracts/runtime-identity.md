# Core 环境与插件版本读取合同

## 公开健康接口

`GET /healthz`、`GET /api/v1/health` 保留原有 status/service/version/install_status/configured，增加 runtime_mode=powerx、core_version、deployment_env。部署环境来自已加载的 deployment.env，与租户 AI env 无关。

**公开接口不查询插件。** 只要传入 plugin_id（包括空值和重复参数），返回 400 / HEALTH_PLUGIN_QUERY_UNSUPPORTED。这是对原未合入匿名插件查询补丁的明确破坏性修正，调用方必须迁移。

## 已授权查询

Capability：`com.corex.runtime.identity.read`。
固定 service binding：`INVOKE core://runtime/identity`，经 `POST /api/v1/tenant/invocations` 调用。
用户管理接口：`GET /api/v1/admin/plugins/{id}/runtime-identity`，用户 JWT + 现有 root/tenant owner/admin guard；原 status 接口不变。

网关请求：

```json
{
  "capability_id":"com.corex.runtime.identity.read",
  "preferred_protocol":"core_internal",
  "payload":{
    "method":"INVOKE",
    "endpoint":"core://runtime/identity",
    "body":{"operation":"get","plugin_id":"com.powerx.plugin.ai-craft"}
  }
}
```

成功返回 `data.payload.item`（Admin 为 `data.item`）：

```json
{
  "runtime_mode":"powerx",
  "core_version":"v1.0.0",
  "deployment_env":"dev",
  "plugin_id":"com.powerx.plugin.ai-craft",
  "runtime_plugin_id":"com.powerx.plugin.ai-craft",
  "plugin_version":"0.1.75",
  "plugin_version_source":"registry",
  "plugin_state":"running"
}
```

plugin_version 为 Core 当前选中的注册版本，**不能宣称为进程实际运行版本**。plugin_state 来自 supervisor 成功查询；已确认 stopped 可以正常返回，缺少 supervisor 信息返回 503，不能把未知状态伪装为 stopped。runtime_plugin_id 是同一注册插件标识，不表示独立租户进程；不返回 PID、端口、日志、凭证。

## 授权边界

- Core 先验证正式能力发布、当前租户 registration 最新版本为 published，再查实时凭证 grant。
- STS 只允许 plugin_id 与可信 claims.PluginID 相同，且实例 auth.credentials 启用并包含本 capability。不能借此查询其他插件。
- API Key 必须有效且属于凭证租户，显式权限为 `_scope.runtime.identity.read` / `read` / `capability` / `runtime_identity_read`，权限行的 **PluginID 必须精确等于目标插件**。空 PluginID、`*` 或其他插件均不允许。
- 不自动给开发 Key 或安装实例新增 grant；Admin REST 权限不能替代 service grant。
- body 不允许 tenant_uuid、actor、额外字段；禁止自由 endpoint、method、query、headers 或 raw proxy。租户从可信凭证取得。
- grant-status=granted 表示 capability 权限；仍须调用目标插件验证 PluginID 限制，不代表可枚举所有插件。

## 错误与缓存

所有本合同健康响应、Admin 成功/业务错误及指定 capability 调用都设置 Cache-Control: no-store。

400 RUNTIME_IDENTITY_INVALID_ARGUMENT：非法插件 ID/operation/代理字段。
401 RUNTIME_IDENTITY_UNAUTHORIZED：无有效服务身份。
403 RUNTIME_IDENTITY_FORBIDDEN：未授权或插件/租户不匹配。
404 RUNTIME_IDENTITY_PLUGIN_NOT_FOUND：仅在管理器明确 CodeNotFound 时返回。
503 RUNTIME_IDENTITY_UNAVAILABLE：Core 身份或管理器依赖故障。
503 RUNTIME_IDENTITY_VERSION_UNAVAILABLE：注册版本缺失。
503 RUNTIME_IDENTITY_STATE_UNAVAILABLE：无法确认进程状态。

使用统一 error_code/reason_code，不仅在 message 填码；保留 Core Trace ID。网关外层 permission/version lock 错误仍遵循既有 registry 合同。

## Framework / Shopify 接入

Framework 在服务端加入 typed ReadRuntimeIdentity 方法及 capability 声明：Local Host 使用显式授权的服务端 API Key，Delegated 使用当前插件 STS。不能匿名调用 health?plugin_id，不能在前端暴露 API Key，不能回退到前端 package.json 或预置版本。
Shopify 页面通过插件后端取得结果，展示 Core 版本、部署环境、插件注册版本与已确认状态；401/403/404/503 保留码和 trace，不用未知状态冒充运行成功。

本轮只修改 Core，Framework/Shopify 消费者仍需按此合同迁移。无需数据库 migration；需 make capability-check、make capability-seed、正常重启 Core 加载代码，并由管理员设置精确 grant。不得为接入临时开放匿名访问。

## 本轮验证记录

相关 8 个 Go 包回归通过，pkg/dto 编译通过（无测试）；Core `cmd/app` 构建通过。验证覆盖匿名查询拒绝、原健康字段、Admin role guard、API Key 目标 PluginID 限制、STS 自身限制、授权撤销、租户不匹配、自由代理字段拒绝、管理器错误分类、supervisor 未知状态以及网关状态码/错误码/缓存策略。

make capability-check 成功：declared=1021；make capability-seed 成功，已发布正式能力和权限元数据，没有给现有 Key 或实例增加 grant，没有改签名密钥或执行 migration。
真实开发 Host Key grant-status HTTP 200，新能力 not_granted；X-Trace-ID：6ad7207d-46de-45d9-847c-51fa34eb0140，request_id：fbbc010c-7f22-4e24-9e0d-dc17f626a2c1。
8077 仍为 PID 65612、2026-10-06 14:44:09 启动的进程。本轮未重启用户服务，新的运行身份成功响应尚未在真实 API Key/STS 链路验收；Go 测试中的可信上下文不等同于真实 STS Exchange。
