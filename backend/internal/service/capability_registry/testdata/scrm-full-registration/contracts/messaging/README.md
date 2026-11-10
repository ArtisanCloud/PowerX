# 消息管理合同与发布边界

管理接口以 [正式 OpenAPI](../../docs/contracts/messaging.openapi.yaml) 为单一来源；[总 API 索引](../../docs/contracts/openapi.yaml) 引用该合同。八个管理路由、四资源/六种成员权限与五项服务权限独立；声明权限不授予权限，正文、回复和执行读取不互相包含。真实消息设施仍受 T023 门禁约束。

五项正式服务 descriptor 位于根 `contracts/capabilities/com.powerx.plugins.scrm.messaging.*.yaml`，经生成器同步至 `plugin.d/capabilities.yaml`、`exposure.yaml`、`rbac.yaml`。服务使用完整固定路径、`core_service_ticket`、`service_actor`、租户范围及独立 API 权限；不能使用 Admin Bearer、用户自报身份或任意代理覆盖。服务 OpenAPI 单一来源是 [messaging-service.openapi.yaml](../../docs/contracts/messaging-service.openapi.yaml)。

[publication.yaml](publication.yaml) 仅供合同检查，**不是运行开关**：记录五项已纳入源码清单的服务能力，暂缓登记 `messaging.events.subscribe` 与四个版本化事件。状态 `registered_runtime_gated` 不表示已发布、安装、授予租户权限或真实收发可用。事件在 T046 单独登记和分权，不能因为已有 outbox/schema 就开放订阅。

Core 临时制品验证已确认五项新增能力登记与权限同步；完整能力目录仍有五项旧能力因适配器 ID 重复而报错，不能宣称完整安装同步通过。原因、复现和边界见 [T045 登记证据](../../specs/008-channel-provider-messaging/evidence/messaging-service-registration-20261102.md)。消息中心菜单已在 T043 实现，页面受控验收在 T044；本次不改实际宿主登记/grant，不启用消息设施，不迁移用户库或重启服务。
