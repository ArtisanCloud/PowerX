# 客户外部身份管理合同（Core → Framework → 插件）

日期：2026-09-27。本合同管理 Customer 的外部登录身份，不是 ContactIdentity。不提供解绑、转移归属或客户合并。

## 固定服务合同

HTTP：`POST /api/v1/tenant/invocations`。固定 binding：`core_internal`、`core://customer/external-identities`、`INVOKE`。

| Capability | operation | 行为 |
| --- | --- | --- |
| `com.corex.customer.external_identities.service_read` | `lookup` | 只读。当前租户、调用插件命名空间内未绑定返回 `found:false`，不会创建或补写 |
| 同上 | `list_by_customer` | 只读，按 customer_uuid 列出当前插件关联身份；默认 page=1/page_size=20，最大 100 |
| `com.corex.customer.external_identities.service_manage` | `bind` | 显式指定已有 customer_uuid；重复绑定同一客户幂等，其他归属 409；不修改客户或 Contact 资料 |
| 同上 | `create_and_bind` | 客户、membership、primary Contact、外部身份、写入审计同一事务；任一步失败全部回滚；已有同租户关联返回原 UUID，不覆盖资料 |

完整 Lookup 请求：

```json
{
  "capability_id": "com.corex.customer.external_identities.service_read",
  "preferred_protocol": "core_internal",
  "payload": {
    "method": "INVOKE",
    "endpoint": "core://customer/external-identities",
    "body": {
      "operation": "lookup",
      "provider_subject": "shop:example.myshopify.com:customer:gid://shopify/Customer/42"
    }
  }
}
```

其余业务 body：

```json
{"operation":"list_by_customer","customer_uuid":"<UUID>","page":1,"page_size":20}
```

```json
{"operation":"bind","customer_uuid":"<已有客户 UUID>","provider_subject":"shop:example.myshopify.com:customer:gid://shopify/Customer/42"}
```

```json
{
  "operation":"create_and_bind",
  "provider_subject":"shop:example.myshopify.com:customer:gid://shopify/Customer/42",
  "customer":{"primary_email":"person@example.com"}
}
```

`customer` 可包含 type、display_name、nickname、given_name、family_name、primary_email、primary_phone、primary_contact。`primary_contact` 包含 display_name、given_name、family_name、email、phone。创建 type 缺省为 person；company 必须显式提供自然人联系人资料。以上两项写入使用 `service_manage`，只读 capability 不能执行它们。其他字段（包括 tenant_uuid、plugin_id、provider、actor、自由 headers/endpoint）明确拒绝。

## 返回

统一 envelope 的 `data.payload`：

- Lookup：`{"found":false}`，或 `{"found":true,"item":{...}}`。
- List：`{"items":[...],"total":1,"page":1,"page_size":20}`。
- Bind/CreateAndBind：`{"item":{...}}`。

item 字段：identity_uuid、customer_uuid、provider_subject、status、type、primary_contact_uuid。只读/Bind 对历史空类型、空主联系人按原值返回，不补写。CreateAndBind 遇到已有但类型/主联系人缺失或引用失效的关联会明确失败，不擅自修复，也不报告完整成功；由客户资料写入或已验证登录规则处理。

## 唯一性与范围

- provider 从可信插件身份派生；调用方不能自选 provider。
- 使用既有全局 `(provider, provider_subject)` 唯一性，不按邮箱合并。租户通过 membership 校验。
- 身份管理不会因为另一个租户已拥有该身份而自动新建 membership。Lookup 对当前租户不可见的关联返回 found=false；写入遇到其他归属返回 409，不泄漏客户 UUID。
- PostgreSQL 上显式 Bind/CreateAndBind 与登录 Resolve 使用同一个 provider+subject advisory transaction lock。已有数据库唯一约束继续生效。并发同一身份只创建一个聚合。
- 已删除或停用身份不能通过管理写入自动恢复、转移或重新分配。
- provider_subject 必须包含实例范围，格式 `<channel>:<instance>:<entity>:<id>`，最多 255 字符。Core 不能替插件猜店铺。
- Shopify 登录当前使用**配置中的店铺域名 + 完整 Shopify 客户 ID/GID**：`shop:example.myshopify.com:customer:gid://shopify/Customer/42`。不得在管理页面改用数字 shopID 或裁掉 GID。Core 公共构造函数为 `ShopifyExternalIdentitySubject`。

## 姓名和展示标签

真实 given_name/family_name 可以为空。display_name 是识别标签，不是“真实姓名已验证”的声明。

没有显式 display_name 时，按 nickname → 已提供的 given_name/family_name → primary_email 选择标签。邮箱只写 primary_email 与 display_name，不填充 given_name、family_name 或 nickname。只有邮箱的 person 创建成功，并同事务创建主 Contact；Contact 的展示标签可以是邮箱，其真实姓名字段仍为空。没有任何可用标签时返回参数错误。

已有联系人对象只缺 display_name 时，也可使用其明确姓名或 email 作标签。company 仍要求明确自然人联系人对象。查询和 Bind 不覆写任何人工资料。重复 CreateAndBind 返回原关联，不把这次传入的 customer 覆盖到已有资料。

Framework 提供共享 `NormalizeBasicAccountLabels`；Skeleton Local 基础客户创建页和 delegated 基础创建客户端已接入。其他插件 Local 持久化 adapter 必须使用同样规则并运行同一业务断言；本轮不宣称所有插件的 Local 业务链路均已验收。

## API Key、STS 与 grant

STS 需要启用实例的 `auth.credentials.allowed_capabilities` 明确包含所需新 capability，且当前 published/tenant registration 有效。租户、plugin_id、client_id 均来自已认证上下文，并重新检查 live instance grant。

API Key 需要正式 permission 元数据 `allow_api_key=true`、`api_key_explicit=true`，以及该 key 的精确授权：

| scope | action | resource_type | resource_pattern |
| --- | --- | --- | --- |
| `_scope.customer.external_identities.service_read` | read | capability | customer_external_identities_service_read |
| `_scope.customer.external_identities.service_manage` | manage | capability | customer_external_identities_service_manage |

**每项精确 grant 的 `plugin_id` 必须由管理员绑定为唯一插件 ID。** 没有 plugin_id、多个相互矛盾的 plugin_id、只有 wildcard grant，均不能推导身份命名空间，因此身份操作被拒绝。不要从 X-Plugin-ID 或 payload 取值。管理员在现有 API Key permission 管理入口设置该字段。仅 capability-seed 不会替已有 Key 选定插件，也不会自动给安装实例增权。

管理页面使用插件服务合同；不把 Admin REST 权限当成 service grant。

## 错误与审计

| 情况 | HTTP / reason |
| --- | --- |
| 非法 DTO、操作越界、UUID/格式/缺少实例范围 | 400 / CUSTOMER_ACCOUNT_INVALID_ARGUMENT |
| 已关联客户类型/主要联系人缺失或引用失效 | 400 / CUSTOMER_PRIMARY_CONTACT_REQUIRED |
| 客户不属于当前租户 | 404 / CUSTOMER_ACCOUNT_NOT_FOUND |
| 身份归属冲突或不可重新分配 | 409 / CUSTOMER_EXTERNAL_IDENTITY_CONFLICT |
| 不可信服务 actor／API Key 缺失插件归属 | 401 / CUSTOMER_EXTERNAL_IDENTITY_SERVICE_ACTOR_INVALID |
| 缺 grant、注册失效、旧 STS grant 被撤销 | 403 / 网关授权错误 |
| 能力发布 hash 尚未确认 | 424 / registry.version_locked |

写入审计与业务同事务，记录身份 UUID、客户 UUID、可信 actor/plugin 和 trace；不记录凭证或原始邮箱。只读调用沿用网关请求审计。重复写入无数据变化时不重复写业务变更审计。

## 既有登录 Resolve

`com.corex.customer.external_identities.resolve` / `core://customer/external-identities/resolve` 保留现有 **STS 登录用 resolve-or-create** 语义，未绑定时可创建客户和 membership；已验证登录可按规则修复空类型/主联系人。它不是 Lookup，不能供管理页面检查关联时调用。本轮新四项操作不更换既有登录语义。

## 实现、测试和运行边界

Core：`backend/internal/service/customer/external_identity_management.go`；typed binding 在 `capability_invoker.go`；序列化锁位于 repository `external_identity_lock.go`。

Framework：`runtime/customerfw.ExternalIdentityStore` 四方法；`NewExternalIdentityClient` 为 delegated 实现，不回退到 Local 写库。插件 UI 需接入该客户端；本轮未新增管理 UI、解绑或合并。

实际 PostgreSQL 回归（独立临时 schema，结束删除）：

```sh
cd backend
POWERX_CUSTOMER_TEST_CONFIG="$PWD/etc/config.yaml" \
go test ./internal/service/customer -run TestExternalIdentityManagementPostgres -count=1 -v
```

覆盖 8 个并发事务、四项操作、不同插件/租户隔离、同邮箱不合并、主联系人/真实姓名规则、API Key 精确 plugin_id 映射、撤销 grant、只读不补写及失败完整回滚。凭证上下文在测试中构造；这不是已安装插件的真实 STS Exchange 验收。

部署需 `make capability-check`、`make capability-seed`、重启 Core/插件加载新代码，然后设置明确 grant，检查真实 grant-status 并从插件调用四项操作，记录网关 trace ID。无新增数据库表/列，不需要本轮 schema migration。已有环境仍需具备此前 Customer/Contact schema。

源码与数据库回归已完成。2026-09-27 用户重启后，真实 HTTP API Key／STS 四操作验收也已通过，详见 [运行验收记录](customer-live-acceptance-2026-09-27.md)。插件 UI 与安装生命周期仍需消费者验收。

### 开发 Host grant 验证记录

2026-09-27，经用户明确选择，将当前 skeleton 开发 Host Key 的两项精确身份 grant 绑定为 `com.powerx.plugins.base`，并写入 `GRANT_CUSTOMER_EXTERNAL_IDENTITY` 操作审计。真实调用 grant-status 返回 HTTP 200，两项均为 `granted`。没有修改其他 Key、其他 scope 或既有 STS 实例 grant。

用户已重启 Core；`TestLiveCustomerProfileUpdateAPIKeyAndSTS` 实际运行 PASS，覆盖临时 API Key 和真实 STS Exchange 凭证的四操作及负例。额外使用原有 skeleton 开发 Key 完成只读 Lookup，返回 HTTP 200、found=false。完整 trace ID 见上述运行验收记录。
