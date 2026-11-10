# Runtime Host Redis 装配

日期：2026-11-08。

## 问题与证据

本机 Core 主缓存为 Redis，但 `integration_gateway.redis_addr` 未配置。此前 `shared.NewDeps` 把可选的网关限流 Redis 客户端传给 Runtime Host；该客户端为 nil，导致已经发布和获授权的 `com.corex.runtime.realtime.stream` 在 cursor 阶段返回 `REALTIME_UNAVAILABLE`。

现场已观察到 AI Craft 的列表 HTTP 200、WebSocket HTTP 101、subscribe 请求，以及紧随其后的实时能力失败。截图中的 `REALTIME_*` 提示说明业务订阅未成功，不能用握手成功替代验收。证据收据见 [Runtime Redis 修复验证](evidence/runtime-host-redis-validation-20261108.json)。

## 配置与生命周期

Core 启动时复用 `cache.InitCache` 已创建的连接，经 `DepsOptions.RuntimeHostRedis` 显式传入 Runtime Host。主缓存的地址、凭据和 Redis DB 继续取有效 `cache` 配置；不新增一份配置，不从服务层创建客户端，也不通过网关限流地址推测 Runtime Host 地址。

`RedisCache.RedisClient` 只供 Core 内部启动装配使用，不是插件接口。客户端的生命周期仍归缓存提供器所有，Runtime Host 不重复关闭它。缺少客户端或 typed nil 继续明确不可用，不切换进程内缓存。现有网关限流和版本锁的 Redis 配置维持其职责。

Runtime Host 的租户、真实服务主体和命名空间构造、精确 grant、实时游标、重连、撤权及身份复核规则不变。历史实时缓冲是有界恢复数据；连接来源变化后旧游标可能过期，客户端按原合同读取业务状态并重新订阅，不能自动重放业务写入。

## 部署验收

本修复只修改 Core 依赖装配，不需要数据库迁移、目录重新发布或 Key 扩权。由服务所有者按原方式重新编译并重启 Core 后，使用 AI Craft 原身份验收：

1. cursor 能成功取得正式游标。
2. WebSocket subscribe 收到非空主题的 `WSBUS_SUBSCRIBED`，不再立即收到 `REALTIME_SUBSCRIPTION_FAILED`。
3. 正常流式重连、身份到期及撤权仍按合同处理。
4. 页面状态与已持久化业务状态一致；真实事件收发仍须插件侧验收，HTTP 101 不足以证明。

本轮不操作插件源码、重新打包插件、修改授权或重启用户日常进程。

## 2026-11-08 重启后复验

用户重启后的 Core PID 95939 已完成实时 cursor 调用，Redis 装配故障已经消除。后续 SSE 请求仍返回 HTTP 400，已通过官方 Framework 客户端与 Core 隔离 HTTP 入口对照，定位到 SDK 附加 `tenant_uuid` 请求头与可信租户合同冲突。后续处理见 [Framework 流式请求对齐](realtime-stream-tenant-framework-handoff-20261108.md)；不应再次用迁移或仅重启 Core 处理这个 SDK 合同问题。
