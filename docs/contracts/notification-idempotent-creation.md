# 成员通知幂等创建 v1

`POST /api/v1/notifications/idempotent` 使用正式 `com.corex.notifications.create` 服务授权；固定 `core_internal` 绑定为 `core://notifications`、`INVOKE`、`operation=create_idempotent`。仅接受类型化字段，不接受租户、身份、HTTP 方法、地址或 headers 代理字段。普通创建及广播契约保持独立；新接口必须明确规范成员 UUID。

输入和响应见 [OpenAPI](notification-member-delivery.openapi.yaml)。输入必须有 `idempotency_key`、`member_uuid`、非空 title/content；键为1–256个可见 ASCII 字符。响应 data 为 `{contract: notification_create.v1, idempotency_key, replayed, notification}`。SDK 必须核对契约、键、成员、通知 UUID、正文、元数据与导航。不能将普通创建响应作为幂等证明。

去重范围为**认证租户 + 实际服务 owner + 键**。STS owner 为插件身份，API-key owner 为该 key 的稳定标识。其他发布者可以使用同名键，但不能读取或占用原发布者凭据。凭据 owner 变化属于不同去重范围；轮换身份前须核对未决业务，不能把旧身份下响应未知的请求换身份盲目重试。

`system_notification_creation_receipts` 是独立、无软删除的空新表。普通 schema 迁移只创建表及唯一索引，不回填旧通知、不清理或重写历史。预占键、通知和原始创建响应在同一数据库事务提交。并发相同请求只产生一条通知；不同正文、成员、导航或元数据返回409 `NOTIFICATION_IDEMPOTENCY_CONFLICT`。元数据对象键排序后核对；数值的词法表示保持精确，`1` 与 `1.00` 作为不同请求处理。不得使用浮点舍入来消除冲突。

每次创建或重放均核实实时 capability 发布、租户 registration、服务 grant 和有效成员/账户/租户。已读、卡片更新或通知删除不清除创建凭据；重放返回最初创建快照，不复活已删除通知，不再次发布 created 事件，不改变已读状态。任一事务步骤失败均回滚键及通知。新表缺失时明确不可用，不退回普通创建。

创建完成只说明**持久化成功**。提交后的既有 WS 发布仍须运行时授权过滤；进程可能在提交后、发布前中断。幂等重放不补造第二条通知，也不承诺浏览器已收到或业务已读。客户端不自行重试；业务调用方必须在每次有限重试前重新检查来源状态/接收人，并使用同一 owner、同一键与同一内容。网络超时后的结果未知可以原样重试；409需核实差异，401/403需恢复真实授权，404/501需升级 Host，不能降级到普通发送。

2026-11-12 隔离验证：通知 service/实际签名 STS HTTP、显式 API-key 权限、固定 capability invoker、SQLite 回归以及临时 PostgreSQL 的10并发/事务回滚/重读/删除后重放通过；相关 API-key及repository回归通过。`make capability-check`、隔离`make capability-seed`、实际fixture key的GrantStatus和`go vet`通过。实际.NET SDK→Core HTTP→PostgreSQL验证默认null metadata、精确金额、导航、冲突和重放；另运行CRM本轮24h流程→SDK→Core创建后丢响应再重试，返回同一持久通知。临时进程、端口和目录均已清理，未启用日常schema/grant、重启服务或发送真实业务消息。

日常升级须由用户明确执行schema迁移、保留安装状态的定向capability seed，并核对实际Host key grant-status；本次隔离证据不代表日常环境升级或实际送达。组织契约见[组织关系依据](iam-member-authority.md)；CRM当前级调度与持久有界重试已完成源码/隔离验收，真实客户/浏览器收到仍待启用验收。可复用的合成HTTP服务见[隔离fixture](../../backend/internal/testutil/notification_contract_fixture/README.md)，不启动完整Core或连接日常库。
