# Runtime Cache 固定类型服务合同

`cache.service.v1` 使用 `core://runtime/cache`、方法 `INVOKE`。只允许 `get / set / delete`；拒绝调用方提供任意 HTTP method、endpoint、headers、query、context 或未知字段。`tenant_key` 仅是租户一致性断言，必须等于已验证凭据和请求上下文，不用于决定授权主体。

服务能力：`com.corex.runtime.cache.service_read` 对应 get；`com.corex.runtime.cache.service_manage` 对应 set／delete。两者均显式声明 `api_key_or_sts / service_actor`，API Key 权限分别为 `_scope.runtime.cache.read|manage`、动作 read|manage、资源 capability／`runtime_cache_read|manage`。这两个服务能力不授予管理员 REST 权限。现有 STS 专用 REST Cache 与 TaskCenter 合同保留，未新增 Admin 页面。

每次调用重新核对活动租户、正式 capability、最新租户 registration 和当前凭据授权。API Key 必须是已认证 key hash 且活动；STS 必须匹配当前插件服务凭据。不得以用户 JWT 代替服务授权。撤权、停用、过期及最新 registration disabled 均立即失败。调用方不能指定其他 service subject。

请求 body 使用 [OpenAPI](cache-service-v1.openapi.yaml)。set 必须提供 canonical Base64 值及 1～86400000 毫秒 TTL；get／delete 不得携带 set 字段。值最大 1 MiB。无 tenant／subject 选择器或旧自由代理格式。

返回包含 `contract_version / operation / namespace / key`；get 另含 `found / value_base64 / expires_at`。未命中为 found=false、空值、null expiry；命中空值仍为 found=true，并有实际 expiry。set／delete 返回确认后的上述身份字段，delete 不存在的键成功。错误明确区分 400 合同、401 凭据、403 scope／grant 和 503 存储／依赖，不能降级为 miss。

新服务入口与现有 REST 共用 `Service.getCache / setCache / deleteCache`、Redis Lua 存储及操作审计。物理缓存键按 Core environment、租户、服务主体、namespace、key 隔离；API Key 与 STS 是不同服务主体。更换服务凭据不读取另一主体的缓存。过期值不搬运，TTL 不扩大到 24 小时以上。

`shared.NewDeps` 只创建一个 Runtime Host Service，REST 与 CacheInvoker 共用它。新能力需通过正式 capability seed 发布／登记；seed 不会自动替 API Key 增加 grant。实际旧服务重启、目标库 seed 和真实 grant 变更需按部署安排执行，不以隔离测试代替。

验证入口：`internal/service/runtime_host/cache_invoker_test.go`，覆盖 API Key／STS、实时撤销、跨主体／环境隔离、严格 DTO、租户不一致和上限。`TestCacheFrameworkBridgeFixture` 只在 `X05_CORE_BRIDGE_FILE` 显式给定时启动受控 HTTP 接入；身份验证由夹具提供，实际 grant、Core invoker、数据库和 Redis 按测试设置执行，不能称完整 Gateway／STS 签发验收。
