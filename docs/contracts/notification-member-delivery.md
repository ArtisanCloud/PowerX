# Core 通知成员隔离与 AI Craft 接入

2026-10-18。Core 源码修复覆盖个人通知创建、所有系统通知入口的统一 WS 投递、发送时身份核验和宿主前端状态隔离。原 `POST <APIPrefix>/notifications`、`member_uuid` 和响应字段保持兼容，Framework 已有的 `Publisher.Create` 继续可用，不要求新增 SDK 方法。

## Core 行为

- 创建个人通知前，成员必须属于已认证租户，成员、关联用户和租户均处于有效状态。无效、跨租户、禁用或已删除的成员返回 `NOTIFICATION_RECIPIENT_INVALID`，不保存、不退化为广播。
- `_topic.system.notification` 的 `userId`（兼容 `member_uuid`）由 Hub 统一解析；定向消息只进入目标成员的同租户连接队列。OpenAPI、Admin 和 TaskBus 都复用此处。字段非法或冲突直接拒绝；空目标保留现有租户广播语义。
- WS 握手从已认证的租户、数字成员 ID、用户 ID 绑定真实成员 UUID；请求方不能自行选择收件身份。每次实际写帧前，重新核验 topic 授权、成员与用户状态、租户归属、JWT 期限和 Root 身份。排队期间成员被禁用后，连接关闭，不写出旧帧。Root 没有当前租户成员身份时不能读取／清空其他成员的个人通知。
- 创建服务通知需要 `com.corex.notifications.create` 已发布、目标租户登记及实际服务 grant。原 HTTP 路径支持 STS／API Key；Admin 用户操作仍走原用户 JWT／RBAC 路径。不能用 Admin 权限替代服务 grant。
- 同一能力提供固定 `core://notifications`、`INVOKE`、`operation=create` 的类型化服务绑定，拒绝任意 endpoint／method／headers／tenant override。API Key 使用显式 `_scope.notifications.create`、action `create`、resource_type `capability`、resource_pattern `notifications`，撤销即时影响新创建请求。HTTP 客户端无需改为调用该绑定。
- 宿主通知状态按用户／租户／成员切换清理，丢弃旧列表和旧更新响应。旧 WS 连接或旧登录 session 的帧不再进入当前订阅；最后一个订阅释放时关闭连接。重连后读取一次当前通知列表，不常驻轮询。

正式定义见 [OpenAPI 与类型化操作](notification-member-delivery.openapi.yaml)、[Capability](../../backend/config/platform_capabilities/notifications.yaml)。成员隔离与可选导航复用现有表；后续已读回执增加服务归属字段，见本文对应章节。业务内容仍由插件鉴权读取，Core 通知只保存通用提醒。通知创建尚未新增 delivery_key 幂等受理／查询保证；调用超时等结果未知场景仍由插件保留 uncertain，不得盲目重发。

## Root 成员的收件资格

`GET <APIPrefix>/tenant/iam/members` 枚举凭据租户内实际存在、未删除的成员，不能按 `username=root` 排除成员。分页总数与列表使用同一范围；没有当前租户成员身份的全局 Root 不会因此获得或被自动创建个人收件身份。管理端成员列表及导入显示名解析不在本次调整范围。

进入目录只是通知候选资格。插件仍按每个成员的正式业务授权、有效状态、读取／成本起点筛选。Core 复用系统内置角色编码 `root`，由实时 `User.IsRoot`、有效账号及当前租户有效成员派生正式来源；权限来自该系统角色的已登记目录，并与普通业务角色使用相同的权限映射、决策、连续区间和 `authorization_revision`。不会把 Root 标志直接当作允许，不自动加入业务租户，不继承其他成员的个人通知。

`root` 由 Core 维护，不能通过角色管理或 RBAC 接口创建、编辑、删除、授予、撤销或人工绑定。角色列表标明“由 Core 维护”，业务权限编辑器排除该系统角色。`role_admin` 继续是租户管理员角色，权限和绑定不因本修复扩大；Framework 和插件不需要创建 Root 专用业务角色。

2026-10-18 的 `role_ai_craft_cost_reader` 是一次临时业务授权补丁，不是部署规范。其 [历史操作收据](evidence/notification-root-membership-20261018.json) 保留审计用途，不能按其内容在服务器复制角色或绑定。本修复复用已有 `root` 的 UUID/ID，不批量重命名或删除未知自建角色，也不重置已有业务授权。

正常 `database migrate -config <实例配置>` 和 Core 正常启动都会维护系统 root 目录、补齐所有适用租户的基线。权限登记及 root 目录同步在同一源事务完成，错误使事务回滚，不再异步丢弃错误。既有 Root 流只有在旧事实指纹精确匹配时，才从本次升级实际时间追加正式来源；原 `known_since` 和历史决策保留，已连续允许的窗口继续保留。其他未知漂移继续返回不可用，不能通过启动猜测历史。Root 身份撤销、账号／成员／租户禁用、插件停用和权限状态变更均经统一源事务影响基线。

部署新 Core 后，无需 `make seed`、手工建角色或逐租户初始化；正常启动本身可以完成这次无新增表的修复。更新 web-admin 才能看到角色管理保护。随后重新启用插件，使用实际服务凭据复核三项基线及通知开关，再由另一个成员修改内容验证 Root 收件。源码、隔离迁移和受控页面测试不替代远程安装验收，见 [本次修复证据](evidence/iam-managed-root-validation-20261019.json)。

## 插件侧对齐顺序

1. 使用包含本修复的新 Core；按现有发布流程同步能力目录与显式服务权限元数据。既有用户角色权限不批量新增，实际调用方需要单独核验 grant。
2. 对齐并重新安装插件。继续使用既有 Framework `NotificationsRuntime.Publisher().Create` 和明确的目标 `MemberUUID`，沿用 `com.corex.notifications.create`。发布 SDK 或本地 replace 的选择以已有 IAM 基线合同交付为准，创建路径未新增 Framework 方法；下文的服务已读回执另用独立接口。
3. 保留插件的真实 diff、个人订单阅读游标、接收人业务权限过滤、成本隔离、操作人排除，以及未准备完成时的写入门禁。宿主铃铛已读不能推进订单阅读游标。
4. 重装并启用租户插件后，核验实际凭据的 authorization_baseline.service_read、iam.directory.read、iam.identity_context.service_read、capabilities.grant_status.read 和 notifications.create 授权。不能以 Root 旁路或隔离测试凭据替代。
5. Core 正常迁移／正常启动及后续授权源事务，会为所有适用租户自动建立已声明业务权限的缺失追踪。已有 guard 与 known_since 保留。Core 起点不依赖插件 `notifications-prepare`，普通部署无需逐租户 apply 或 `make seed`。完整规则见 [权威指导第 7.2 节](../guides/features/iam-authorization-baseline/guide.md#72-初始化与启用场景)。
6. 三项各自 ready 后，插件读取各 known_since，并在自己的受控边界内检查事件／个人游标／投递历史，自动保存业务启用点。`initialize_on_install: true` 的插件仍按其安装启动／重新启用流程执行；Core 不替它清理历史或开放写入门禁。无需用户手填时间、转交 guard 或重新打包来初始化 Core。
7. 用三个普通账号做实际投递与阅读验收，通过后才启用 `host_notifications_enabled`。源代码测试不替代这一项。

## 基线只读核验与历史处理

在 Core backend 目录、使用目标实例的实际配置执行：

```sh
go run ./cmd/authorization_baseline \
  -config '<实际Core配置绝对路径>' \
  -tenant '<当前实例的实际租户UUID>' \
  -plugin 'com.powerx.plugins.ai-craft' \
  -permissions 'production.sample_track:read,production.bulk_order:read,operations.business_dashboard.cost:read'
```

该命令没有 `-apply`，仅核验，不会触发初始化。正常部署应由 Core 生命周期自动补齐所有适用租户；若仍缺失，检查新 Core 是否实际运行、schema、插件启用状态及权限标记，不转交插件 guard 补单个租户。known_since 表示 Core 开始可靠记录授权事实的时间，不伪装成历史首次授权时间；初始化不新增角色授予、不清理插件事件。旧 apply 仅保留受控维护兼容，规则见权威指导。

以下为 2026-10-18 旧版手工处理的历史证据，已由 2026-10-19 全租户生命周期取代，不是新的部署前置步骤：

早先只读预检观察到本机 8077 对应目标插件没有 auth.credentials／tracking 记录，CLI 返回 `IAM_BASELINE_INITIALIZATION_SUBJECT_INACTIVE`。用户确认当时刚删除插件，因此按卸载状态处理，当时未补造凭据或初始化起点。

后续插件已重新安装。2026-10-18 修复 Selector 调用上下文冲突后，核对安装实例的真实 pending guard、租户 RLS 下的空变更／游标／相关投递记录，再初始化三项缺失基线；只读复查均 ready，known_since 均为 `2026-10-18T11:07:30.719208Z`。原始结果 JSON、guard 和回归证据见 [本次收据](evidence/iam-baseline-selector-context-20261018.json)。插件自身启用状态及宿主铃铛开关未由 Core 修改，仍需运行修复后的 Core 并重新启用插件后验收。

## 验证与限制

源码回归包含：三个成员与双设备真实 WS、跨租户阻断、无效目标拒绝、排队后禁用阻断、租户广播、实际 JWT 签名中间件 → HTTP 创建 → 数据库 → 成员 WS、服务 grant 撤销，以及前端旧身份响应／旧连接帧拒绝。测试中的 IAM、ACL、凭据为隔离 fixture；安装实例三账号验收仍待重装后执行。

通知专项测试、race、vet、前端状态回归和隔离正式 seed 证据见 [验证记录](evidence/notification-member-delivery-20261018.json)；其中记录的是该轮检查时的四条备份路由声明缺口。后续 Selector 修复的最新 `capability-check` 已通过，见上述本次收据。没有停止或重启用户服务。


## 可选业务导航

服务创建请求和固定 `create` typed operation 支持可选 `navigation: {label, path}`。`label` 由发布方按语言文件本地化；`path` 仅允许同源绝对路径，不接受 URL scheme、authority、fragment、反斜线、控制字符或路径穿越。校验失败返回 `NOTIFICATION_NAVIGATION_INVALID` / HTTP 400。不要把用户权限或业务读取凭证放入导航对象。

Core 使用现有 `system_notifications.actions` 存储，无新增表。创建响应、typed operation 结果和 `system.notification` 事件均返回 `actions`，管理员列表／详情保留既有动作字段。主动作合同为 `{id:"view_business", label, type:"primary", action:"navigate", params:{path}}`；铃铛菜单优先执行该显式主动作，未声明动作的普通通知继续进入消息详情。通知详情和列表按钮执行相同路径校验。非法历史动作显示本地化错误，不跳到外域；执行非法动作本身不提交通知已读。打开通知详情仍按既有规则标记提醒已读，与插件订单内容阅读进度无关。

业务入口负责复查当前账号、租户、权限和业务对象。Core 通知已读仅确认提醒处理，不能更新插件的订单阅读游标。AI Craft 使用只保存在插件内的成员投递映射，不将业务订单信息暴露给宿主长期存储。旧通知无动作时不自动修补；真实验收使用更新后创建的新通知。

运行变更：需同时更新 Core 后端与 web-admin，之后重新构建安装插件。SDK 源码对齐不等于 SDK 已发布；本轮不执行发布、安装或重启。合同用例覆盖 REST → DB → 定向 WS、typed invocation、非法导航拒绝和通知详情独立实例；插件隔离浏览器证据不替代安装环境验收。

## 服务已读回执（2026-10-18）

业务确认已阅后，发布该通知的服务可以确认对应提醒已读；普通用户在 Core 标记通知已读不产生业务已阅回执。此合同不读取或更新插件订单游标，也不根据通知元数据猜业务范围。

- 能力：`com.corex.notifications.read_receipts`；权限 `corex.notifications:read_receipts`，仅服务调用。直接 HTTP 为 `POST <APIPrefix>/notifications/read-receipts`；类型化绑定仍为 `core://notifications`、`INVOKE`，固定 `operation=acknowledge_read`。API Key 需要独立的 `_scope.notifications.read_receipts`、action `read_receipts`、resource `capability/notifications`；创建能力不能替代此授权。
- 输入仅 `member_uuid` 与 `notification_uuids`，每次 1–100 个唯一、规范、非零 UUID。租户及发布者从实际服务凭据取得。响应为 `{contract:"notification_read_receipt.v1", member_uuid, notification_uuids}`；SDK 必须核对成员及完整 ID 集合，不接受扩大或缩小范围的收据。
- 正常 schema migration 给现有通知模型增加 `service_owner`，默认空。服务创建时由已验证凭据写入：STS 为 `plugin:<plugin_id>`，API Key 为实际 `api_key:<key_uuid>`。用户请求或 metadata 不能指定发布者归属；普通内部／管理员创建保留空值，不冒充服务创建。
- 每次调用重新核验服务 grant 和目标成员有效状态；事务先校验全部通知属于同租户、同发布者、同成员，再更新未读项。混入其他成员／其他服务／其他租户的 ID 整批拒绝，不部分更新。重复回执不重写 `read_at`；已由目标成员删除的同归属通知视为已处理。旧通知缺少可信归属时拒绝，不根据旧 metadata 回填。
- 提交后在既有 `_topic.system.notification` 发送 `eventType=notification_read`、`payload_version=1`、`contract=notification_read_receipt.v1`、`tenant_uuid`、`userId` 和 `notification_uuids`。复用发送时实时授权及成员过滤；它不是新通知，不增加未读数。用户常规单条标为已读也发送同一成员事件。
- Core web-admin 的铃铛、列表、详情通过同一 Pinia 已读回执 Store 应用结果，各自保留分页和筛选。成员／租户／登录 session 切换清空回执并丢弃迟到结果；先到已读、后到创建或旧列表响应也不能恢复该通知未读。通知详情选中对象同步反映状态。断线重连按已有事件触发列表刷新，不新增轮询。

运行要求：服务所有者执行正常 migration 和 `make capability-seed` 后运行新版 Core 后端、前端，再由插件重装声明并获取新增服务 grant。旧版本服务无法识别此能力，不能以插件单元测试替代实际 grant 检查。本轮正式 seed 只在可丢弃 PostgreSQL 中执行；当前安装实例的新增能力只读检查仍为 `CAPABILITY_UNKNOWN`，未重启服务、未修改真实通知。真实部署验收使用新版本创建的通知。

验证覆盖真实签名服务中间件 → HTTP → DB → 定向成员 WS、类型化调用、API Key 独立权限、其他发布者／成员阻断、批次原子拒绝、重放保持时间、删除后重放及前端跨实例／迟到结果隔离。Core 通知专项 race／vet、前端 11 项测试、能力校验及隔离正式 seed 均通过；与真实安装账号的铃铛联测分开验收。

## Core review 落实（2026-10-18）

保留独立 `notifications.read_receipts` 能力和现有通知表的 `service_owner` 字段。服务批量确认其他成员的提醒，与用户确认自己的提醒是不同的授权范围，创建权限不隐含回执权限。服务归属必须在创建时从可信凭据写入，不能由请求字段或 metadata 补造。

归属按租户分别隔离：STS 使用逻辑插件 ID，不随插件版本或短期 token 更换；API Key 使用 Key UUID。保持同一 UUID 的密钥轮换不改变归属，新建另一 UUID 的 Key、切换 API Key／STS 不自动继承原归属。旧通知没有可信归属时保留原数据，由接收成员在 Core 普通通知入口标为已读；不通过新服务接口批量“认领”，不追溯回填。

本次修正以下运行边界：

- `useNotifications` 同时监听本标签登录事件和跨标签 `storage` 事件。相同成员重新登录也会清空旧回执、列表和选中详情；同一 session 的 token refresh 保留状态。组件作用域释放时移除监听。
- 已读回执立即更新共享 Store。已读／未读筛选列表在实际回执变更后合并读取一次，处理不在当前页中的通知；重复回执不重复读取。列表请求使用序号和登录上下文阻断迟到响应，已确认的旧未读记录不再计入响应总数。没有变更时不周期查询。
- Header 由完整登录上下文负责首次加载和重新订阅，同一上下文重连只读取一次；移除重复连接监听和 mounted 读取。通知详情在桌面／窄屏布局中都随登录上下文清空。
- Core 安装／启用配置和实际进程环境统一从 `config.ResolveAPIPrefix(CoreConfig)` 写入 `PX_GATEWAY_API_PREFIX`，同时同步 `host-values.yaml` 的 `gateway.api_prefix`，覆盖安装产物中的旧前缀。这不修改插件自己的 HTTP base。
- Framework 创建和回执接口使用同一地址构建边界；资源路径中不再拼接固定版本。Go 本地回执参数错误为 400、响应合同错误为 502、未配置客户端为 503。具体 SDK 配置见 [Framework 对齐合同](../../../Plugins/PowerXPlugin/docs/contracts/notification-business-navigation.md)。

插件自有启动装配仍需把已加载的 `Gateway.APIPrefix` 传入 `notifications.Config.APIPrefix`（.NET 对应 `apiPrefix`），不能只升级 SDK 后继续遗漏配置。AI Craft 对齐位置为 `backend/internal/bootstrap/framework_notifications.go`；其 `backend/internal/config/config.go` 的 `normalizeGatewayAPIPrefix()` 也需保留显式 `/`，不能退回 v1。Framework Skeleton 和生成模板已传入该配置、保留根前缀，并通过配置加载到通知客户端的回归。未设置时仅使用 SDK 声明的 v1 默认值，不猜测 Gateway URL 中的前缀。

源码验证及本次边界见 [Core review 回归收据](evidence/notification-core-review-alignment-20261018.json)。本轮未运行真实数据库迁移、能力 seed、插件重装或用户服务重启；之前安装检查的 `CAPABILITY_UNKNOWN` 不能视为已解决。安装验收应先完成正式能力登记、目标租户注册和实际服务 grant，再使用新创建的通知验证导航、业务已阅后的回执，以及其他成员／服务／租户拒绝。

## 本机环境落地（2026-10-18，后续操作）

以下更新上节源码阶段尚未执行的环境事项，结果见 [本机交付收据](evidence/notification-core-local-readiness-20261018.json)：

- 全应用 `cmd/app` 和正式数据库迁移工具均已构建。构建期间修正恢复模块的一处字段名误用：`Component.SizeBytes` 改为实际存在、序列化为 `size_bytes` 的 `Component.Size`；不据此宣称恢复模块已完成功能验收。
- 先生成并验证当前数据库的受限访问备份，再按用户明确授权向 Core PID `73285` 发送 SIGTERM。Core、其四个直接子进程均退出，8077 端口释放；未替用户启动日常服务。
- 使用正式 `database migrate -config etc/config.yaml` 完成迁移。`public.system_notifications.service_owner` 已存在，为非空 `varchar(512)`、默认空字符串。原有 101 条通知、92 条未读保持一致；排除新增字段后的完整通知记录摘要与迁移前一致，旧通知归属仍为空，没有猜测回填。
- `make capability-check` 和保留现有授权状态的 `make capability-seed` 均成功。通知创建／回执能力及目标租户 registration 均为 `published`；回执服务权限已带 `allow_api_key=true`、`api_key_explicit=true`，类型化端点为 `core://notifications`。这是数据库与正式目录核验，Core 重启后的真实 HTTP grant-status 尚待读取。
- 当前已安装 AI Craft 为 `0.1.77`，其安装 manifest 尚未声明 `com.corex.notifications.read_receipts`；现有服务凭据继续保留创建授权，没有回执授权。Core 未直接修改凭据白名单。新版插件完成自有前缀装配、声明新增能力后，应由正常安装／启用流程派生授权，再进行实际通知联测。

Core 侧构建、迁移和目录登记的准备工作已完成，可以启动包含本次源码的新 Core 及最新 web-admin。插件重装、实际 HTTP grant-status 和新通知导航／回执联测仍是后续安装验收，不能用数据库登记结果代替。

## 普通成员订阅与 Header 生命周期修复（2026-10-19）

本机只读核查确认：通知 `3c0616d3-8ef9-46b2-8f26-644f3dc67bb2` 已在 `2026-10-18T23:57:40.821851+08:00` 保存为已读；目标成员的两条有效通知中剩一条未读。该成员与用户均有效，但没有个人通知 Topic 的 `member:<id>` subscribe ACL。生产 WS authorizer 之前对普通成员要求这项通用 ACL，Root 则旁路通过；因此即使前端已经建立连接，普通成员也会在 subscribe 时收到 `permission_denied`。仅看回执 HTTP 200 或 WebSocket open 不能证明订阅成功。

修复将已登记的精确 `_topic.system.notification` 的 subscribe 定义为个人通知接收操作：从已验证 JWT 与实时 IAM 解析租户、用户、成员，必须与连接绑定的 MemberUUID 一致。接收本人提醒无需逐成员配置 Event Fabric ACL。Hub 与写帧前校验仍限制租户和收件成员、检查 JWT 期限与成员有效状态；Root 不继承其他成员的个人提醒。通知 publish/replay 和其他 Topic 继续遵守既有 ACL，不为此新增或扩大角色／服务授权。此处不改变通知内容对应的业务权限或插件阅读游标。

Header 先等待当前登录 session 的成员上下文就绪，再读取通知并订阅。账号、租户或 session 变化时释放旧订阅，拒绝旧连接与旧登录的迟到消息。共享 WS 不再以模块全局 initialized 标记挂靠首个组件的 watcher；每个调用方持有自己的身份与网络监听，释放时只清理自己的订阅，最后一个订阅离开后关闭连接。监听本页登录事件与跨页 storage 事件，包括 token 字符串不变而 session 改变的情况。共享连接已经打开、成员上下文稍后才就绪时，也会正确记录重连上下文。

已读事件通过现有共享 Store 同步铃铛、列表和详情，保留其他旧未读通知。断线重连仅补读一次列表；事件、身份切换、初次载入以外没有周期查询。没有新增接口、表或迁移，不要求 Framework 更新或插件重新打包。

回归包括生产 authorizer 拒绝通用 ACL 的真实成员 WS、两位普通成员与 Root、同成员双设备、异租户阻断、禁用后的排队消息阻断，以及真实 Header → useWSBus → useNotifications 的“2 → 1”、旧登录消息丢弃和重连补读。证据与验证边界见 [订阅修复收据](evidence/notification-ws-subscription-validation-20261019.json)。本次未停启用户 Core，也没有修改真实通知或 ACL。用户更新 Core／web-admin 后刷新各账号页面，再验证真实 Safari 与三账号设备；源码及受控回归不代替该安装验收。

## 铃铛只展示未读（2026-10-19）

铃铛下拉菜单使用既有列表接口的 `is_read=false`，只展示最近 5 条未读；红点使用服务端返回的未读总数，不受当前页大小限制。已读回执通过共享 Store 立即移除对应菜单项，并沿用已有事件触发的列表补读，不增加轮询。

全部读完时显示“暂无未读通知”，仍可进入通知中心查看完整历史。铃铛移除清空全部消息入口，避免在快捷提醒入口删除历史；没有按标题合并、删除或补造通知。通知中心继续使用独立的分页与筛选实例，默认包含已读和未读。`useNotifications` 的可选初始筛选只影响调用方实例，既有调用保持兼容。

本次仅调整 web-admin，无需修改后端接口、执行迁移或重新打包插件。通知专项 32 项回归通过，包括全部已读但历史保留、跨页未读总数、WS 回执移除、重连补读与身份隔离；实际浏览器页面仍需加载最新前端后复测。证据见 [铃铛未读展示收据](evidence/notification-bell-unread-validation-20261019.json)。

## 备份结果通知与详情直达修复（2026-10-19）

旧的数据库备份、恢复验证结果仅广播 `_topic.system.notification`，未保存 `system_notifications`。前端又为缺少 ID 的事件生成随机 UUID，造成铃铛可见、通知中心查无记录；通知中心同时未处理 URL 的 `id`。此类旧随机 ID 没有对应的历史记录，不能凭标题或任务号猜测回填。

当前流程为：任务保存终态 → 复用 Notifications Service 保存正式成员通知 → WS 推送持久化 ID、成员及导航动作。成功和失败使用不同标题，失败为重要通知；不把文件路径、连接信息或原始脚本错误广播到提醒中。通知写入失败只记录错误，不重跑已完成任务、不降级为临时通知。进度仍沿用现有备份事件，不写进铃铛。

手动任务绑定经过入口鉴权的发起成员，按租户、数字成员／用户身份解析，创建时再次校验租户、成员、账号有效性。内部上下文的显式 MemberUUID 也必须通过同一收件人校验。没有交互成员的定时任务仅通知执行租户内有效的 Root 成员；没有收件人就不广播。个人通知仍由既有通知 API 和成员 WS 隔离，通知不会赋予备份操作权限。

导航动作使用已有合同：备份结果进入 `/ops/backup?job_id=<ID>` 的执行记录，恢复验证进入 `/ops/backup?drill_id=<ID>` 并展开报告。页面直接查询指定记录，不依赖首页、可用备份过滤或最近 20 条历史。记录不存在／无权查看、网络失败均有明确反馈，可返回全部记录或手动刷新。

通知中心按 `?id=<UUID>` 加载正式详情，等待登录上下文就绪；切换路由、账号、租户或 session 后丢弃旧响应。不存在、已删除或无权限的通知显示明确提示，可重试或返回列表。窄屏详情与列表切换及窗口缩放保留筛选／分页上下文。详情已读失败不会阻挡内容展示。全部状态筛选保持包含已读和未读。

前端不再为缺少服务端 ID 的任务事件制造提醒。初次加载、身份就绪、已读事件与重连按既有机制同步，不增加轮询。新增提示已同步中英文。

部署需要同时更新 Core 和 web-admin，然后刷新浏览器；无需数据库迁移、修改 Framework、重装插件或升级 Redis。旧的临时提醒刷新后消失；已复制的旧链接显示不可用。验收请以更新后产生的新备份／恢复验证结果为准：铃铛点击能定位实际记录，通知中心刷新后保留历史，其他成员不收到该手动任务的个人提醒。

验证命令与源码／受控浏览器验收边界见 [结果通知修复收据](evidence/notification-persisted-task-validation-20261019.json)。本轮未修改远程数据或停启用户服务；真实服务器部署后验收仍需执行。
