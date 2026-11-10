# 插件 Event Fabric 安装、租户映射与 Framework 对齐

更新：2026-11-05。本合同接替最初将主题写入直接挂到所有 Enable 的方案。可靠事件传输继续使用 `event_fabric.service.v1`，参见 [服务合同](event-fabric-service-v1.md)。

## 职责与入口

| 场景 | Core 行为 |
|---|---|
| 正式安装后的自动启用、显式版本切换、Admin 插件启用 | 由内部控制位选择准备阶段：校验声明和归属，事务准备运行凭据租户的主题、ACL、绑定记录与映射 |
| 普通 Enable、Core 启动恢复、插件 restart、安装失败后的旧版本恢复 | 只读核验已经提交的映射及实时 ACL；缺少准备结果明确失败，不新增主题或授权 |
| 业务租户启用、Admin 显式主题重同步 | 以目标租户准备，并将该租户映射与实例启用状态在同一数据库事务中提交 |
| 插件服务调用 `resolve_topics` | 只读取得当前凭据租户、当前插件、当前能力方向下的映射；不准备资源 |

`WithEventFabricProvisioning` 是 Core 内部上下文控制位，不接受 HTTP 请求体、普通请求头或插件 manifest 传入。Admin 路由仍由原有用户 JWT / RBAC 授权；服务 API Key/STS 不能冒充该入口。没有选择固定主题声明/映射合同的直接能力消费者维持原流程。

全局插件进程的运行凭据当前属于 System。全局 host-values 的平面 `topic_bindings` 只供这一凭据租户使用，**不能把它当作全部业务租户映射**。业务租户映射保存在对应 `auth.credentials` 记录的 `config.runtime.event_bridge` 下，由租户启用事务生成；服务方通过下面的正式只读操作获取。

## 包内声明与归属

```yaml
runtime:
  event_bridge:
    topic_manifest: config/event_fabric_service.yaml
```

包内文件沿用 manifest v1：

```yaml
version: v1
topics:
- key: fixture.order.paid.v1
  namespace: powerx.fixture.order.paid
  name: v1
  acl:
  - principal_type: plugin
    principal_id: '{{plugin_id}}'
    actions: [publish]
```

- 路径必须是包内相对路径；空值、错误类型、两处声明冲突及越界符号链接都失败。不能声明第三方 principal、未要求的服务方向、未渲染成员/会话模板。
- 新主题默认只允许本插件的命名空间：插件短名（`-` 转 `_`）及其 `powerx.`、`_topic.` 前缀，或完整插件 ID。
- 非标准/旧共享命名由 **Core 配置所有者** 指定唯一归属。插件包和租户 Config 不能自行加入这项授权。例如确认 AI Craft 确实拥有该旧命名后，可在 Core 配置中使用：

```yaml
plugin:
  event_fabric_namespace_owners:
    powerx.channel.master: com.powerx.plugins.ai-craft
```

这个配置示例不表示本轮已批准或写入实际环境。AI Craft 当前 13 项声明中，`powerx.channel.master.credential_inspection` 使用了这个非标准根；全新环境需要明确归属，或由 AI Craft 将物理命名迁到其自有命名空间。业务 key 可以保持不变。已有主题仍须通过归属检查，配置不能接管别的插件主题。

- Core 给新主题写入保留的 `core.plugin_owner` 和 `core.plugin_topic_keys` 元数据；manifest 不能注入或覆盖这些字段。历史业务 key 标记保留在主题上，支持同 key 改名后的失败升级重试。
- 已有主题必须属于本插件。旧主题无标记时，必须有同租户、同插件、同 Topic UUID / FullTopic 的正式绑定，且不存在其他插件的归属声明；不能仅凭同租户或已有 ACL 自动认领公共主题。
- 同一或上下级命名空间已由其他插件占用时，准备失败。跨插件消费使用已经显式授权的映射，只读核验；不通过本插件 manifest 自行获得 ACL。
- 已有主题的 ACL 被撤销后，准备和恢复均失败，**不会自动补回撤销权限**。恢复授权使用既有管理员 ACL 管理入口。

## 提交、升级与恢复

准备在同一数据库事务中创建缺少主题、授予新主题声明 ACL、保存 Topic/ACL 归属记录、写审计记录。任何绑定记录写失败都阻断；事务中不写共享 ACL 缓存。PostgreSQL 使用租户级事务 advisory lock 串行准备，避免并发归属抢占。

全局 host-values 和 JSON registry 无法与数据库成为一个原子事务，因此使用 0600 检查点 `installed/{plugin_id}/.event-fabric-{version_hash}.pending.json`（版本包之外的宿主管理目录，安装包无法预置恢复内容）：

1. 保存原文件及原 HostConfig 检查点。
2. 在数据库准备事务内写新 host-values、更新并保存 registry。
3. 文件或 registry 失败，数据库回滚；恢复原文件、内存配置和 registry。
4. 数据库提交成功后删除检查点。异常中断留下检查点时，普通恢复明确返回 `EVENT_FABRIC_INSTALL_RECOVERY_REQUIRED`；显式 Admin 启用恢复检查点后重新准备。恢复失败保留检查点，不伪报成功。

准备阶段成功后，资源及归属已提交。后续进程启动/健康检查失败仍由原运行激活/drain 回滚处理；已提交的准备资源保留，便于显式重试，不能删除共享资源来模拟全局回滚。

旧生成映射通过数据库中同租户、同插件的正式绑定识别。新版删除/改名业务 key 时，只替换 Core 已生成的映射；不删除旧版本主题、ACL 或手工重试/超时策略。未知手工映射冲突明确拒绝。版本回退可以再次选择旧清单并复用已保留资源。

没有声明文件的预配置模式只核验目录与 ACL。若同时使用 publish/consume，需逐主题声明方向：

```yaml
runtime:
  event_bridge:
    tenant_uuid: <credential-tenant-uuid>
    topic_bindings:
      outbound.changed: <full-topic>
      inbound.changed: <full-topic>
    topic_actions:
      outbound.changed: [publish]
      inbound.changed: [subscribe]
```

不把两个服务能力并到每个主题上；缺少方向时返回 `EVENT_FABRIC_INSTALL_TOPIC_ACTIONS_REQUIRED`。

## Framework 对齐：按真实租户读取映射

Framework 当前 `NewHostRuntime` 接受单个租户和单套映射，禁止复用 System Runtime 处理其他租户。Core 新增以下 **只读** typed operation，不需要新的 grant、Admin 接口或 WSBus：

```json
{
  "capability_id": "com.corex.eventfabric.service_publish",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "contract_version": "event_fabric.service.v1",
      "operation": "resolve_topics",
      "tenant_uuid": "<当前服务凭据租户UUID>",
      "plugin_id": "com.powerx.plugins.ai-craft",
      "topic_keys": ["powerx.ai_craft.order.paid.v1"]
    }
  }
}
```

调用 `POST {api_prefix}/tenant/invocations`，使用目标租户实际 API Key 或 STS。REST 不接受自由 method/URL/headers/raw proxy。底层固定 binding 保持 `core://event-fabric/service` + `INVOKE`。

消费端用 `com.corex.eventfabric.service_consume`，并补充 `subscriber_id`，必须等于真实插件 ID。`resolve_topics` 返回：

```json
{
  "contract_version": "event_fabric.service.v1",
  "tenant_uuid": "<当前服务凭据租户UUID>",
  "plugin_id": "com.powerx.plugins.ai-craft",
  "topic_bindings": {
    "powerx.ai_craft.order.paid.v1": "<Core正式FullTopic>"
  }
}
```

响应采用现有 invocation 结果封装。服务会重新校验发布能力、租户 registration、实时凭据 grant、精确 plugin owner、实例启用及请求各主题的当前 ACL。某个 key 未准备、撤权或归属不符时明确失败，不拼接替代 Topic，也不返回其他租户映射。

Framework 需要完成：

1. 在 Event Fabric Client 增加 `ResolveTopics` typed 方法及 DTO；新增 `topic_keys` 字段仅限此 operation，其他操作携带时应拒绝。
2. 当前凭据租户确定后，以该租户凭据查询需要的业务 key，校验响应 contract / tenant / plugin / keys / FullTopic 租户前缀及重复映射。
3. 用返回映射装配该租户 `NewHostRuntime`。多个租户分别持有凭据提供器和 Runtime；租户切换关闭旧订阅并释放旧 Runtime。
4. System 的静态 host-values 只能服务 System。租户实例配置或 `resolve_topics` 结果不能与别的租户 Runtime 混用。
5. 缺映射、未注册、撤权、查询失败保持明确错误；不得回退 Admin Bearer、WSBus、本地事件器、拼接 topic 或其他租户凭据。
6. 原信封、稳定 event/idempotency ID、业务提交后 ACK、ACK 不确定状态、相同订阅身份重连等可靠性规则保持。[详细合同](event-fabric-service-v1.md)
7. 补 Client/Runtime/模板/生成包回归，覆盖两租户、同 owner 不同凭据、撤权、缺 key、错误响应和重连；源码通过后再交付可消费版本。

本轮 Core 没有修改 Framework 或 AI Craft 源码，没有更改实际 Namespace owner 配置、Key grant、Profile、默认角色或数据库 schema。全新 AI Craft 安装前应确认上述非标准 namespace 的归属。真实安装、服务重启、业务事件联调与受控回归分别验收。

## 错误与排查

| 错误 | 操作 |
|---|---|
| `EVENT_FABRIC_INSTALL_NAMESPACE_DENIED` | 检查物理 namespace 是否属于插件；非标准根由 Core 配置所有者明确归属 |
| `EVENT_FABRIC_INSTALL_TOPIC_OWNER_REQUIRED/CONFLICT` | 核对目录 owner 与正式绑定；不能靠删除检查或重新授予 ACL 接管 |
| `EVENT_FABRIC_INSTALL_BINDING_CONFLICT` | 查具体业务 key；只有可由权威绑定证明的生成映射可随升级替换 |
| `EVENT_FABRIC_INSTALL_PREPARATION_REQUIRED` | 目标租户尚未通过正式启用/重同步提交映射；普通启动不会补建 |
| `EVENT_FABRIC_INSTALL_RECOVERY_REQUIRED` | 保留检查点，通过显式 Admin 启用恢复；不要手改安装目录 |
| `EVENT_FABRIC_INSTALL_ACL_REQUIRED` / `EVENT_FABRIC_TOPIC_FORBIDDEN` | 核对该租户、插件与方向的实时 ACL；撤权不会因重启失效 |
| `EVENT_FABRIC_GRANT_REQUIRED` | 核对实际服务 Key/STS、published capability 与 registration；Admin JWT 成功不算验收 |

## 代码与验证

- `internal/service/event_fabric/pluginbinding/`：公共声明、归属检查、事务、审计与映射准备。
- `internal/service/event_fabric/manifest/seed_service.go`：严格归属准备、绑定失败阻断、保留旧资源和策略。
- `internal/infra/plugin/manager/event_fabric_runtime.go`：只读恢复、文件/registry 检查点及回滚。
- `internal/service/plugin/tenant_instance_service.go`：租户准备与实例状态同事务。
- `internal/service/event_fabric/service_channel/topic_bindings.go`：正式只读服务身份映射查询。
- `internal/bootstrap/plugin.go`、Admin plugin handlers：显式准备与普通恢复的装配边界。

测试包括绑定写失败、文件/registry/数据库提交失败、检查点恢复、归属/namespace 冲突、双租户、升级/回退、撤权、分方向映射、API Key / STS owner 和实时 grant；并在专用 PostgreSQL 验证并发准备及可靠投递。结果见本轮 evidence 收据。
