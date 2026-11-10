# 通用业务通知卡片 v1

本合同由 Core 提供通用展示、Gateway 读取、受控图片预览和成员级实时失效。业务标题、摘要、图片选择、可见内容版本与业务阅读游标归插件所有。Core 不理解大小单字段，也不保存订单摘要、成本、供应商 URL、签名下载地址或预览令牌。

合同：[OpenAPI](./business-notification-card.openapi.yaml)，[共享样例](./fixtures/business-notification-card.v1.json)。现有创建与阅读回执：[成员通知合同](./notification-member-delivery.openapi.yaml)。本轮源码和受控验收不等于 AI Craft 真实安装态联调。

## AI Craft 需要实现

1. 在 `plugin.yaml` 登记两个展示绑定，并在插件 API/RBAC 清单绑定相同 GET 路由。路径相对于插件 `endpoints.http_base_path`，不带 `/api/v1`；只允许固定 `/admin/` 路径及规定占位符。

```yaml
notification_presentations:
  - business_type: sample_track
    contract: business_notification_card.v1
    presentation_path: /admin/operations/order-content-notifications/sample-tracks/{delivery_key}/presentation
    preview_path: /admin/operations/order-content-notifications/sample-tracks/{delivery_key}/images/{image_key}
  - business_type: bulk_order
    contract: business_notification_card.v1
    presentation_path: /admin/operations/order-content-notifications/bulk-orders/{delivery_key}/presentation
    preview_path: /admin/operations/order-content-notifications/bulk-orders/{delivery_key}/images/{image_key}
```

2. 新通知通过现有 Create.Metadata 写入 `business_card`，字段为 `contract / source_plugin / business_type / delivery_key / source_revision`，精确类型见 Schema。继续提供现有 Navigation。标题和正文保持通用提醒，敏感业务内容仅在展示读取时产生。卡片必须面向具体成员，不支持租户广播。
3. 展示 GET 返回标准 `{data: Presentation}`。所有字段来自同一次当前成员可见快照：`contract / revision / title / summary / updated_at / unread_change_count`，有图时增加 `image: {image_key, alt}`。`Accept-Language` 决定业务文案语言。无图省略 image，不返回空 URL。revision 是**此投递成员可见状态**的单调整数（1 至 2^53−1），可包含权限变化；它不等于订单全局版本、Core revision 或未读次数。结果 revision 必须至少达到最近发送给 Core 的 source_revision，否则 Core 显示可重试失败。
4. 每次摘要、图片读取都检查当前 Gateway 身份、租户、通知接收人、订单存在、大小单读取权限和成本权限。delivery_key/image_key 均非凭证。图片读取还要检查图片属于该投递对应的当前可见订单；不能接受任意资产 UUID 直接取图。可以内部复用资产预览服务。图片只返回受控 PNG/JPEG/WebP/GIF/AVIF 字节，最大 10 MiB；摘要和图片均 `Cache-Control: private, no-store`。403 表示无权限，404/410 表示失效；不返回图片外链或重定向。
5. 保留聚合投递记录中的 Core 通知 UUID。已有 `order_content_changed` 信号需要转成下面的 Core typed invalidation；Core 不会自行订阅插件私有主题，也不会根据 business_uuid 猜测 delivery_key。只向实际受影响且仍为有效成员的接收人发出更新。普通成员看不到的成本变化不得推进他的摘要、时间、计数或可见版本。
6. 权限撤销、恢复，以及业务阅读游标变化也应推进对应投递的可见版本并发送 `reason: access_changed`。此原因保持 Core 通知读态，仅重新鉴权读取展示。订单删除同样触发失效，由展示接口返回404/410。完整订单阅读确认仍由插件自己的阅读接口完成；放大图片和 Core isRead 均不能推进订单阅读游标。

## Core typed 更新与旧通知迁移

正式能力 `com.corex.notifications.invalidate_card`，固定 `core://notifications`、`INVOKE`、service_actor，支持 STS 和显式 API Key。仅接受下列 DTO，不接受 HTTP method、endpoint、headers 或任意代理 body：

```json
{
  "operation": "invalidate_card",
  "member_uuid": "22222222-2222-4222-8222-222222222222",
  "notification_uuid": "33333333-3333-4333-8333-333333333333",
  "source_revision": 2,
  "reason": "content_changed"
}
```

Core 校验发布状态、租户 capability registration、实时服务 grant、创建服务所有权、租户与有效成员。只有更高 source_revision 才更新原行、推进 Core `revision` 并发送消息；重复/乱序无写入、无新通知、无新消息。`content_changed`（默认）重置 Core isRead/readAt；`access_changed` 保留读态。返回标准 `{result: {notification_uuid, revision, source_revision, changed}}`。

旧记录原位迁移：同一 DTO 可附带 `card_reference`，其结构等于 metadata.business_card，source_revision 必须与请求一致。Core 只允许原创建服务为没有卡片的通知绑定已登记插件来源，保留其他 metadata，推进 Core revision，使用原通知 UUID。重复相同绑定无效应；已绑定的 source_plugin/business_type/delivery_key 不可修改（409 `NOTIFICATION_CARD_BINDING_CONFLICT`）。STS 来源必须等于调用插件 ID。迁移通常使用 `reason: access_changed`，以保留已有通知读态。没有引用的历史通知继续按普通通知展示。

API Key 创建的通知与 STS 创建的通知具有不同不可变服务所有者；迁移必须使用原所有者通道，不静默换通道重试。

## 展示请求与实时恢复

Core 三处共用 `NotificationBusinessCard.vue`：铃铛、列表、详情。浏览器先 GET `/api/v1/admin/notifications/{uuid}/presentation-source`，取得 Core 根据启用插件 manifest 生成的 Gateway 路径，再携当前登录 JWT 请求展示和图片。Core 不使用插件服务凭据代查业务，也不允许通知 metadata 注入 URL。

`_topic.system.notification` 延续现有租户和成员隔离。更新事件是完整通知视图加 `eventType: notification_updated`、`payload_version: 1`、`tenant_uuid`。创建响应、列表、详情、TaskBus、WS 均包含同一 metadata、Core revision 和 sourceRevision。metadata.business_card.source_revision 保留最初绑定的值；实时源水位读取通知顶层 sourceRevision。相同 ID 按 Core revision 更新；旧版本不覆盖新版本，重复消息不增加条数。页面没有周期列表/详情查询。

WS 重连后补读一次列表，同时当前卡片重新读取展示和图片；读接口是权威状态。当前实现没有逐条 WS replay/ack 承诺，插件需要可靠保存待发送更新，失败重试必须使用相同 source_revision。断线、Core 重启后通过持久化通知与重新读取收敛。

图片用带鉴权 GET 获取 Blob，48×48 保持比例，放大与导航是两个按钮。放大不触发通知已读或订单已阅。进入通知详情保留 Core 原有自动已读行为；点击业务详情沿用 Navigation。切换账号/租户/登录会话、刷新权限上下文、收到新版本或断线重连，会取消旧请求、清空旧内容/预览并释放 Blob URL。组件卸载也释放资源。

## Framework 需要对齐

- Go/Nuxt/.NET 的创建 DTO 可继续使用 Metadata/Navigation；增加可选卡片引用类型和 contract 常量便于校验，不要求改原创建入口。
- manifest 模型、Schema 校验、Skeleton/生成器应保留并生成 `notification_presentations`，不可打包时丢弃绑定。
- 增加 typed `InvalidateCard` 方法，固定调用上述 Core binding；支持 reason 和一次性 card_reference。完整保留上游 code、HTTP status、trace_id。API Key 和安装态 STS 分别走启动确定的通道。
- 现有 ReadReceipt DTO 增加可选 `revisions: {notification_uuid: Core_revision}`。动态卡片每个 ID 都必须带当前 Core revision，缺失/旧版本409，批量操作原子失败；普通旧通知仍兼容未带 revisions 的调用。
- 共享样例覆盖新建、展示、失效、阅读回执；不得将 Core revision、source_revision、插件业务阅读游标互相代用。

## 授权与验收边界

部署前增量发布正式 capability/permission metadata，为目标租户登记并授予插件 `com.corex.notifications.create`、`com.corex.notifications.invalidate_card`，需要发送 Core 阅读回执时另授予 `com.corex.notifications.read_receipts`。开发 Host API Key 需要分别显式授权对应最小 scope/action，不能借 Admin 权限代替服务授权；Admin 展示来源是 `com.corex.notifications.presentation_source`（JWT、当前成员）。插件展示/图片读取另由插件 RBAC 和业务权限校验。

Core 自动验证涵盖拥有者/成员/租户/授权隔离、旧通知原位绑定、版本更新去重、旧回执冲突、重启读取、受控图片无已读副作用、撤权清空、重连和桌面/390px展示。API Key 与 STS 服务授权在受控数据库验证。

最终安装态验收由 Core + AI Craft 完成：真实大小单两类事件，普通成员成本裁剪、成本隐藏变化、撤权和恢复权限、图片与订单权限绑定、完整订单阅读游标、已有通知迁移，刷新/实时结果一致。受控浏览器 fixture 不证明上述插件业务授权逻辑；本轮没有修改 AI Craft、调用真实供应商或操作生产租户。


## v2 大小图合同（2026-10-22）

新接入使用 business_notification_card.v2；v1 的单 image_key 继续按原合同处理，不往 v1 塞大小图字段。插件可为同一 business_type 分别登记 v1/v2，唯一键是 business_type + contract；历史 v1 引用需要旧绑定继续存在。v2 仍复用已登记 preview_path 的 {delivery_key}/{image_key} 受控 GET，无新任意代理入口或额外 Core capability。

```json
{"contract":"business_notification_card.v2","revision":2,"title":"小单 A-102","summary":"设计图已更新","updated_at":"2026-10-22T00:00:00Z","unread_change_count":1,"image":{"alt":"设计图","thumbnail":{"variant":"thumbnail_64","status":"ready","image_key":"opaque-small"},"preview":{"variant":"preview","status":"ready","image_key":"opaque-large"}}}
```

有 image 时同时声明 thumbnail/preview 及其独立状态；只有 ready 才携带 image_key。每个 opaque key 由插件绑定当前成员投递、资产和指定变体；禁止任意资产 UUID 直取、路径猜测、持久保存签名 URL。缩略图必须 JPEG/PNG、两边≤64、≤32,768 bytes；预览必须真正的 preview，不能原图，也不能把64图放大。读取前重新检查租户、接收人、当前订单与成本权限；沿用 no-store、不重定向、不公开存储凭据。无 thumbnail 不回填大图；ready 的 preview 即使 thumbnail 失败仍可点击。

Core 只为可见卡片加载摘要和缩略图，点击才加载大预览；关闭预览、中止请求、离开可视区域/组件、身份/权限/语言变化均释放对应 Blob。大图失败不清空仍有效的小图；401/403 清空旧展示。现有 v1 只保证旧行为，不承诺64px下载量。

图片排队/完成/失败属于展示资源变化。AI Craft 消费对应 Media 事件，将其映射到受影响成员投递、推进成员可见 source_revision，再调用既有 invalidate_card，reason=presentation_changed。Core 更新同一通知并实时失效刷新，保留 isRead/readAt；插件不得因派生就绪递增业务未读次数、更新时间或订单内容版本。Core 不知道不透明 key 对应哪个业务资产，不能替插件做这层映射。事件可合并；客户端不轮询或为每张小图常驻创建独立连接。

原位升级已有 v1 卡片：原创建服务发送新的 card_reference.contract=v2、同 source_plugin/business_type/delivery_key、更高 source_revision，reason=presentation_changed。Core 校验新登记绑定后按旧 source watermark 做 CAS 更新，保留通知 UUID/读态与其他 metadata；禁止降级或换绑业务对象。重复请求无副作用，并发冲突明确409后读取重试。未升级记录继续 v1。

正式 Schema：[business-notification-card.openapi.yaml](business-notification-card.openapi.yaml)；完整共享样例：[business-notification-card.v2.json](fixtures/business-notification-card.v2.json)。

卡片 v2 的桌面/窄屏、按需预览和可见区域下载检查见 [Core 受控验收](evidence/media-thumbnail64-core-validation-20261022.json)。真实 AI Craft 成员权限、成本裁剪与安装态联调仍由跨仓库验收完成。
