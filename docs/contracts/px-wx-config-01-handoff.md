# PX-WX-CONFIG-01 Mac 开发环境交接

电商 0.4.0 已通过正式上传安装完成迁移，并健康运行。用户重启 Core 后，官方 Framework v0.0.28 使用真实配置服务 Key 的 HTTP 409、业务错误码和上游 trace 透传复验已通过；本机 Core 侧 PX-WX-CONFIG-01 验收完成。

- Core：v1.0.0 / dev；当前插件：0.4.0 darwin/arm64。
- 测试租户：System（沿用 Lab 验收租户），`6b5d0240-9920-46da-b707-88200e0f51ea`。
- 包：`/private/var/www/html/ArtisanCloud/X/PowerX/Core/Plugins/com.powerx.plugin.ecommerce/dist/0.4.0-darwin-arm64-wx-config-01-20261030-162614/package.tar.gz`。
- 包 SHA256：`54276bacec320578fa06364e3e59c9f88ffd67472466ba7e07ea5826ebca167a`。
- 新配置 Key：`dev-workspacex-ecommerce-config-lab`，独立 Profile #12，owner 为 WorkspaceX。
- 私有 Key 文件：`/Users/michaelhu/.powerx/keys/dev-workspacex-ecommerce-config-lab.key`，权限 `0600`；本文不包含明文。
- 旧只读 Key 与共享 Profile 权限集合保持不变。新 Key 只具备四项电商业务授权和自身 grant-status 只读查询，无订单、支付、退款或管理端权限。

## 能力与 registration

| capability（均以 com.powerx.plugins.ecommerce. 开头） | 契约版本 | provider_version | registration / grant |
| --- | --- | --- | --- |
| product.configuration.read | 1.0.0 | 0.4.0 | published / granted |
| purchase.option.write | 1.0.0 | 0.4.0 | published / granted |
| purchase.operation.read | 1.0.0 | 0.4.0 | published / granted |
| product.spu.subscription.plan.read | 2.0.0 | 0.4.0 | published / granted |

各能力的 `capabilities_hash` 和 `protocol_hash` 见随附 JSON；原套餐读取 2.0.0 的两项 hash 与旧版本保持一致。

## 软件类目

- 名称：Lab 软件订阅；code：`lab-software-subscriptions`；状态：enabled。
- UUID：`5e12b6d9-2b1a-430e-b4af-214a2030eaf5`。
- path：`/5e12b6d9-2b1a-430e-b4af-214a2030eaf5/`。
- 当前权威接口返回无生效属性模板，因此没有类目附加必填属性，`attributes` 可传 `{}`。
- 新建商品仍需 code、name、category_id、category_path、description。类目以后绑定模板时，需重新获取其必填属性。

## 已创建的 Lab 草稿对象

| 配置 | SPU UUID | purchase option UUID | SKU UUID | 套餐 UUID |
| --- | --- | --- | --- | --- |
| one-time | 629d5c0b-fd09-482c-ba8b-5c8a3e187184 | 2e84eddc-b147-4b7e-b61a-999ffedd558b | fb4151f8-1d55-4f5e-af1d-bed9f7ceaee4 | — |
| annual | bee06d06-de49-4b47-9a3a-2d033f766d44 | f8b133a6-480a-417a-8b95-74996dc08c97 | d02e773a-dc8d-42af-a50a-696d15cbfb6f | 3bef023e-b72a-4776-8e28-fdb501ad48da |

一次性配置：CNY 1.00、initial_purchase、period_months=0。年度配置：CNY 2.00、annual_update、12 个日历月、auto_renew=false。地区均为 CN。商品、购买配置、SKU、套餐与价格版本均保持草稿；保存不触发上架。

## Framework 验收

- 使用公开模块 `github.com/ArtisanCloud/PowerXPlugin/framework/backend/go@v0.0.28`，无本地 replace。
- 通过官方 `gateway.NewClient`，携带新服务 Key，调用 `/api/v1/tenant/invocations`，传 capability_id、preferred_protocol=rest 和业务 payload。不要向该 Core DTO 添加 action 字段。
- 已验证商品查询、一次性与年度保存、同键同内容重放、成功响应丢失后的结果查询恢复，以及套餐读取 2.0.0。所有正常调用使用 rest，fallback_used=false。
- 幂等键必须在发送前持久化；响应丢失后使用原键查询／重试，不换键创建。回执保留原业务 trace_id。
- 此 SDK 版本的 Response.TraceID 是 HTTP 请求 trace；持久化的 Core invocation trace 取 Response.Data["trace_id"]，业务保存 trace 取结果回执 trace_id。

- 一次性保存 Core invocation trace：`cddfa647-b1cf-44d6-8c9d-ad9d5452d784`。
- 年度恢复 Core invocation trace：`e3b85665-57e3-4761-9472-a16b6398e54f`。
- 年度原业务 trace：`843936bc-ad59-4229-819c-77b3f0f30b79`。

## 重启后最终复验

- Core PID：`69564`；实际运行二进制 SHA256：`a5f5d805b0dbb2fe48fa72518284fa1d12750d334b528fb3a00eb11d0283770f`。
- 同一幂等键、不同内容：HTTP 409，`error_code=COMMERCE_CONFIGURATION_IDEMPOTENCY_CONFLICT`。
- 冲突请求 HTTP trace：`e736e7a0-1329-4ec2-9eff-95ad529b90d2`；上游业务 trace：`224a23da-9c5b-4cc6-b6bf-df7ca8329381`。
- 四项能力继续为 granted，registration 均为 published；新建商品和配置保持草稿，年度原保存回执可以恢复。
- 旧只读 Key、共享 Profile 权限集合和原 Lab 月度套餐读取均正常；没有重建 Key 或测试商品。
- 本地 Core 侧无剩余阻塞，可交给 WorkspaceX 进行应用内联调。

详细证据：`docs/contracts/evidence/px-wx-config-01-validation-20261030.json`。
