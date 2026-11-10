# AI Craft 实时订阅：Framework 流式请求租户合同对齐

日期：2026-11-08。范围：本机已安装 AI Craft 0.1.78 的实时连接诊断、Core 边界回归及 SDK 修复交接。

## 现场结论

用户重启后的 Core 已加载 Runtime Host Redis 装配修复。实时能力的 cursor 调用已经完成，但后续 `POST /api/v1/tenant/invocations/stream` 返回 400，插件随即关闭浏览器 WebSocket，所以列表正常显示仍会出现“实时更新已中断”。

这与之前的 `REALTIME_UNAVAILABLE` 是连续暴露的两个故障。再次迁移数据库、修改 Key/Profile 或仅重启 Core 均不能修复当前请求合同。

- Core PID：95939，端口：8077；运行 build ID：`73bc053a2ad545fba0c5eae6d34b0e02c7d35f03f06bc97d45cd8fece4e05c36`。
- cursor 完成 trace：`104019e4-c980-46db-8548-f398213c3261`，时间 14:09:12 +08:00。
- SSE HTTP 400 trace：`3a08e6f4-ebcd-436f-9812-fc2a761ee3e5`，时间 14:10:30 +08:00。
- 已安装插件二进制的模块信息为 Framework `v0.0.28`，但有本地源码 `replace`，不能把该二进制等同于纯发布包版本。

完整回执：[现场与隔离对照证据](evidence/realtime-stream-tenant-contract-20261108.json)。

## 合同冲突与归属

Framework 当前 `runtime/wsbus/realtime_service.go` 的 `SubscribeEvents` 将 `TenantUUID` 传给共享 `runtime/powerx/hostcontract/invocation_stream.go`。后者第 155 行把它写成 `tenant_uuid` 请求头。

Core 的 `backend/internal/transport/http/openapi/capability_registry/tenant_handler.go` 中，普通及流式能力入口共用 `tenantUUIDFromRequest`，拒绝 URL 的 `tenant_uuid` 和调用方租户请求头。租户只从认证中间件建立的可信身份上下文取得；同租户字符串也不能成为覆盖入口。

因此当前 SDK 请求在进入能力选择、grant 校验和 SSE 执行之前就失败。受控 Core 回包包含 `capability.tenant_override`；当前 SDK 将该失败解码为 HTTP 400 / `CAPABILITY_STREAM_UPSTREAM_FAILED`。现场访问日志只能证明 HTTP 400，受控联调进一步复现了具体拒绝原因，二者不混作同一种证据。

Core 不应通过放开租户请求头、转用 Admin 身份或扩大授权来兼容此请求。实时 DTO 的 `tenant_key` 是与可信租户比较的业务范围断言，必须保留；它不能决定调用身份。

## Framework 需要修改

修改仓库 `../Plugins/PowerXPlugin` 中的共享流式实现：

`framework/backend/go/runtime/powerx/hostcontract/invocation_stream.go`

1. 保留 `StreamInput.TenantUUID` 的 UUID 格式检查，停止执行 `request.Header.Set("tenant_uuid", input.TenantUUID)`。不要改成其他 `X-Tenant-*` 请求头。
2. 保留现有服务凭据、固定能力入口、`preferred_protocol=core_internal`、请求与 trace 标识、严格响应解析。租户由 Core 验证凭据后确定。
3. 保留 `runtime/wsbus/realtime_service.go` 的类型化 `tenant_key`、namespace、cursor 及返回范围校验。跨租户正文断言必须继续被 Core 拒绝。
4. 检查两个委托入口同时使用修复后的共享实现：`internal/integration/gateway/stream.go`、`runtime/powerx/capability/stream.go`。不只修其中一条调用路径。
5. 补充真实 HTTP 边界回归：官方客户端发出的请求没有租户覆盖头；服务凭据和 tenant_key 正确时取得 SSE ready；缺失/错误凭据、跨租户断言、缺 grant、撤权继续拒绝；断线重连保留正式 cursor 规则。
6. 发布可消费 SDK 版本并同步相应模板与合同。不能只依赖本地 replace 证明插件安装包已修复。

Core 本轮新增 `tenant_stream_tenant_contract_test.go`，验证凭据租户成功、同租户覆盖头拒绝、跨租户覆盖头拒绝。Core 的生产租户校验规则未改。

## AI Craft 需要对齐与验收

升级到修复后的 Framework，更新依赖锁定并重新构建、打包，再通过正式插件安装/升级流程部署。不要手改 `plugins/installed` 内的二进制或配置；不需要重置已存在的事件 ACL。

使用同一实际插件服务身份验收：

1. 初始业务列表可读，cursor 调用完成；正式 SSE 返回 200、ready，并持续收到 heartbeat。
2. 浏览器收到非空 topics 的订阅确认；插件本地“主题已订阅”日志或 WebSocket 101 不能单独证明上游 SSE 已接受。
3. 经已有安全场景产生变更事件，确认页面收到并与持久化状态一致。不得为验收重提订单、付费生成等业务写入。
4. 重连后补齐状态；权限撤回、登录身份变化和离开页面时释放旧连接。
5. 连接仍失败时保留错误反馈及 trace，不能通过隐藏 toast、常驻轮询或切换 Admin/本地通道代替修复。

本轮没有修改 Framework/AI Craft 源码、安装包、Key/grant 或用户进程。完整已部署插件验收仍待 SDK 与安装包升级。

## 本轮验证及复现

Core 固定能力 HTTP 回归（在 `backend` 目录）：

```sh
GOWORK=off go test -json -race ./internal/transport/http/openapi/capability_registry -run 'Test(CoreInternalStreamCredentialTenantRejectsLegacyTenantHeader|InvokeCapabilityStream)'
```

6 项顶层测试、28 条含父子场景记录通过；指定包 vet 通过。

额外使用当前官方 Framework 共享客户端，借助独立 modfile 和 Go overlay 调用 Core 的隔离 HTTP fixture，不修改两个仓库的依赖或 SDK 源码：原客户端得到 400；诊断对照仅在临时 HTTP transport 去掉该头，同一凭据、相同正文与授权成功收到 SSE ready。两项子场景通过。这个对照证明请求头冲突，不代表已修复已安装插件，也不代表真实业务事件端到端通过。

隔离测试使用合成 Key、内存 SQLite 与自动关闭的 httptest 端点，不读取实际服务 Key，不调用业务生成，不向日常数据库或 Redis 写入测试数据。临时文件及命令记录见证据 JSON。
