# thumbnail_64 / 通知卡片 v2 对齐任务

Core 契约基线：standard-image-v2、thumbnail_64、business_notification_card.v2。本文件供 Framework 与 AI Craft 对齐；不表示它们已完成接入，也不表示真实安装态已验收。

## Framework：媒体 DTO 与消费

1. 原 CreateAsset / upload / CompleteUpload 链路不变。新图与命中原图去重都会在 Core 自动检查派生，不增加普通上传方的 ensure 授权。不得用本地额外编码补 delegated 缺失状态。
2. `derivatives.recipe` 透传 standard-image-v2；items 允许三个以上条目，按 variant 名查找 thumbnail_64，不用数量或数组下标推断就绪。Go 现有字符串 recipe/variant 和 slice 可继续复用。
3. 透传每项 status、variant_uuid、可选 error_code/retryable；原图 status/upload_state、整体派生 status、项目状态分别保留。整体 queued/processing/failed 时，已经 ready 的 preview/thumbnail 仍可消费。
4. 读取：查找 thumbnail_64 → 确认 ready + variant_uuid → 现有 GetVariant 查询实际 MIME/size → 现有 parent asset_uuid + variant_uuid 的 presign-download。缺少时返回明确状态，不回填 preview/origin，不拼接存储地址。
5. 显式上传 Variant 的类型/枚举允许 thumbnail_64。Core 要求 checksum、JPEG/PNG、实际文件≤32768 bytes、宽高≤64；ready 响应可能复用 Core 已生成结果，尊重返回 MIME/size，不假定等于本地编码。
6. 原状态/ensure 固定类型化能力与 SSE 路径不变。自动执行不需要新增 grant；主动状态/事件需要 image_derivatives.read，人工 ensure/retry 才需要 image_derivatives.ensure。保持启动时运行通道、状态码、上游错误码和 trace，不静默 Local/Delegated 切换。
7. 状态推送沿用现有 SSE/WS 桥接；首次/手动刷新/事件/重连读取快照，不增加常驻状态轮询。SSE 不保证逐事件回放，以当前快照恢复。
8. 把 skeleton、生成模板和实际客户端校验器中 preview/thumbnail 枚举一起更新；避免只改主 SDK。Go/Nuxt 先受控验收，其余语言明确版本与发版状态。

共享样例：[media-image-derivatives.v2.json](fixtures/media-image-derivatives.v2.json)。正式合同：[image-derivatives.openapi.yaml](../../specs/001-media-storage/contracts/image-derivatives.openapi.yaml)、[Host HTTP](../../specs/001-media-storage/contracts/http-openapi.yaml)。

## Framework：通知卡片 v2

1. Create.Metadata、历史通知、实时更新完整保留 business_card.contract=v2；v1 继续支持，不能在 v1 单 image_key 中追加大小图字段。
2. 安装清单 notification_presentations 支持同一 business_type 分别登记 v1/v2；唯一键是 business_type + contract，重复同一组合仍拒绝。manifest、descriptor、SDK、skeleton/模板、安装序列化一起检查。
3. v2 image 类型：alt、thumbnail、preview；thumbnail.variant 固定 thumbnail_64，preview.variant 固定 preview。两者独立 status；只有 ready 才有 opaque image_key。未 ready 不携带可读 key，无图省略 image。
4. 继续使用登记的只读 presentation_path 和 preview_path（后者含 delivery_key、image_key）。每种 key 表示插件受控引用，均不是凭证。不要把任意 URL、存储凭据或任意 asset UUID 放进 Core 代理调用。
5. InvalidateCard 新增 reason=presentation_changed；图片就绪/失败、展示协议升级使用此原因，保留 Core isRead/readAt。content_changed 仍重开通知，access_changed 仍用于权限/业务阅读游标刷新。不能把资源完成计成业务更新。
6. 已有 v1 原位升级：原创建服务提供 v2 card_reference、相同 source_plugin/business_type/delivery_key、更高 source_revision、reason=presentation_changed；禁止倒退、换绑或创建重复通知。409 读取当前版本后处理。

共享样例：[business-notification-card.v2.json](fixtures/business-notification-card.v2.json)。Schema：[business-notification-card.openapi.yaml](business-notification-card.openapi.yaml)。Core 已处理可见卡片加载、点击预览、取消、身份隔离及 Blob 释放，Framework 不重做 Core 组件。

## AI Craft：业务接入

- 列表持久关联素材/变体 UUID，响应提供受鉴权 thumbnail_url、preview_url 时明确代理路径或短时票据的有效期；不把签名 URL 当永久素材引用。
- delegated 小图消费 thumbnail_64，大图消费 preview；列表只加载可见行，点击才获取 preview，不请求 origin。缺失、排队、失败独立显示，不做大图兜底。
- Local 使用相同 JPEG/PNG/GIF/静态 WebP 输入、方向/透明度/首帧规则，缩略图不放大、不裁切、两边≤64、输出≤32768 bytes，实际 MIME 与大小一致。
- v2 presentation 的两个 key 分别绑定当前投递、成员、素材、具体变体。每次摘要/大小图 GET 检查租户、接收人、订单及成本权限，保持 no-store 与无外链重定向。
- 消费媒体状态事件，将资源变化映射到受影响成员投递，推进成员可见 source_revision 并发送 presentation_changed。业务内容版本、业务更新时间和未读次数不因派生完成而增加。
- 放大只打开预览；订单已阅继续由插件完整阅读确认推进。

## 共同验收与发布

受控测试先覆盖样例乱序、缺规格、部分失败、旧版兼容、ready 项独立读取、重复及乱序失效、取消与迟到响应。真实安装态分别验证 API Key、安装 STS、普通成员与成本裁剪、撤权、跨租户。20/50 行记录实际图片响应尺寸和下载字节，点击前无 preview/origin，点击后才下载 preview。

历史补图由部署负责人执行 Core media_tool derivatives-backfill，先 dry-run 审阅范围和统计，再显式执行。submitted 只是登记成功，最终生成通过任务状态/SSE 确认。运行前与通用 Redis 隔离任务核对环境管理已有的环境 ID（installation.json.id / active_environment_id）；Core 与维护命令读取同一条环境记录，正式/开发各自使用其环境 ID，无需额外 namespace 配置。不要自行清空旧队列或重启用户服务。

Core 源码与受控证据：[media-thumbnail64-core-validation-20261022.json](evidence/media-thumbnail64-core-validation-20261022.json)。当前改动尚未提交或发版，Framework 可以按合同开始源码对齐；真实联调前需锁定 Core 提交及 SDK 版本，并完成另一任务负责的通用 Redis 环境隔离。
