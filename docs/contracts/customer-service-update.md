# 客户资料更新：Core → PowerXPlugin Framework

日期：2026-09-26。Core 已新增类型化更新操作。Framework 需要新增 typed 更新方法，再接入 Delegated 编辑页面。本文只描述 Core 合同；不代表插件安装和页面链路已经验收。

## 调用合同

- Capability：`com.corex.customer.accounts.service_manage`（沿用原 grant）
- Binding：`core_internal`，`core://customer/accounts`，`INVOKE`
- Operation：`update`
- HTTP 入口：`POST /api/v1/tenant/invocations`
- 认证：开发 Host 的 `Authorization: ApiKey <key>`，或安装插件的 `Authorization: Bearer <STS access token>`。

完整请求：

```json
{
  "capability_id": "com.corex.customer.accounts.service_manage",
  "preferred_protocol": "core_internal",
  "payload": {
    "method": "INVOKE",
    "endpoint": "core://customer/accounts",
    "body": {
      "operation": "update",
      "customer_uuid": "<客户 UUID>",
      "display_name": "更新后的客户名称",
      "primary_email": "contact@example.com",
      "primary_phone": ""
    }
  }
}
```

`method`、`endpoint` 是固定网关 binding；Framework 公共 DTO 不得提供自由 endpoint、method、headers 或 raw payload。

## 部分更新语义

`customer_uuid` 必填且必须是非零 UUID。至少传一个可更新字段。字段值只能是字符串；`null`、未知字段、大小写别名会失败。字符串去除首尾空白，拒绝控制字符。

| 可选字段 | 校验 | 可传 `""` 清空 |
| --- | --- | --- |
| `display_name` | 非空，最多 128 字符 | 否 |
| `nickname`, `given_name`, `family_name` | 最多 128 字符 | 是 |
| `primary_email` | 最多 255，单一邮箱地址，不接受显示名 | 是 |
| `primary_phone` | 最多 32，仅数字、空格、`+ - ( )`，非空时至少含一个数字 | 是 |
| `avatar_url` | 最多 2048，HTTP(S) URL，不接受 URL 内凭证 | 是 |
| `locale` | 最多 32，有效语言标签，例如 `zh-CN` | 是 |
| `timezone` | 最多 64，有效时区，例如 `Asia/Shanghai`、`UTC`，不接受 `Local` | 是 |
| `status` | `active`, `pending`, `suspended`, `disabled`, `expired`, `deleted` | 否 |

- 未传字段保持原值。Framework Go DTO 应使用指针或等效 presence 类型，不能把 `""` 当成未传。
- 不支持修改 `type`、`primary_contact_uuid`。提交这些字段明确失败。
- 不同步已有 Contact 资料，也不修改邮箱登录、Shopify 或其他身份绑定。2026-09-26 补充：客户写入事务补存历史空类型为 person，并确保主要联系人存在：复用唯一有效 Contact，个人无 Contact 时从保存后的客户资料创建；明确 company 保留且无联系人时要求提供自然人。失效引用/多联系人歧义整体回滚。
- 客户资料存放于全局 Customer Account；先验证当前租户 membership，再更新该 Customer。状态更新沿用 Core 既有规则：更新 Account 状态及当前租户 membership 状态，不改其他租户 membership。`status=deleted` 是状态值，不执行软删除。
- 校验、归属检查、资料写入、当前 membership 状态写入与返回读取处于同一事务。

## 返回与错误

HTTP 200：统一 envelope 的 `data.payload.item`，沿用创建的 AccountRow，包含 `uuid`、`type`、`primary_contact_uuid`、客户资料及 membership 信息。`data.trace_id` 为真实网关调用 trace。

现有 AccountRow 的可选资料使用 `omitempty`：清空后的字段可能不出现在响应 JSON 中。Framework 应按空值解码，而不能拿旧资料覆盖。重新读取使用 `com.corex.customer.accounts.service_read` 的 `list` 操作。

| 情况 | HTTP | 错误 |
| --- | --- | --- |
| 非法 UUID、状态、格式、空 patch、禁止字段 | 400 | `registry.invalid_request`；details reason `CUSTOMER_ACCOUNT_INVALID_ARGUMENT` |
| 当前租户没有该客户或客户不存在 | 404 | `registry.not_found`；details reason `CUSTOMER_ACCOUNT_NOT_FOUND` |
| 没有凭证 | 401 | 网关认证错误 |
| 没有 service grant | 403 | 网关 capability 授权错误 |

租户及 actor 从可信凭证上下文推导。业务 payload 禁止提交 `tenant_uuid`、actor、plugin ID。不得转用 Admin REST、Admin 用户 JWT 或 `admin_manage` 绕过服务授权。

API Key 授权仍为 `_scope.customer.accounts.service_manage`，action `manage`，resource type `capability`，pattern `customer_accounts_service_manage`。同时满足正式 capability 发布、租户 registration、实时凭证 grant。STS 仍通过插件声明的 required capability 和租户启用授权。

## 部署与复验

本次不新增数据库字段，不需要 migration。已完成现有 Customer/Contact schema 迁移的环境，只需重启 Core 加载新代码，并同步能力声明：

```sh
make capability-check
make capability-seed
```

实际 PostgreSQL 事务回归（隔离 schema，全程回滚）：

```sh
cd backend
POWERX_CUSTOMER_TEST_CONFIG="$PWD/etc/config.yaml" go test ./internal/service/customer -run TestCustomerCreationPostgresNullablePrimaryContact -count=1
```

真实开发 Core API Key／STS 验收（创建临时客户、临时 API Key 与临时插件凭证；结束清理 fixture；不会改现有插件 grant）：

```sh
cd backend
POWERX_CUSTOMER_TEST_CONFIG="$PWD/etc/config.yaml" \
POWERX_LIVE_TEST_URL=http://127.0.0.1:8077 \
POWERX_LIVE_TEST_STS=127.0.0.1:9001 \
go test ./tests/contract/plugin_grants -run TestLiveCustomerProfileUpdateAPIKeyAndSTS -count=1 -v
```

脚本覆盖 API Key、STS 更新及重新读取、遗漏保留、空值清空、非法 UUID/状态、禁止修改类型与主联系人、拒绝 payload 租户覆盖、跨租户访问、撤销 grant 后拒绝。检查 Contact 和登录身份未变化，并输出服务端 trace ID。

## 本轮证据与交接边界

- 实际 PostgreSQL 回归已通过，覆盖 person/company 更新和事务边界。
- `make capability-check`、`make capability-seed` 已通过，发布注册同步到开发数据库。
- Customer service/repository、网关 transport、API Key permission 与 plugin grant 相关测试通过。
- 新 Core 二进制构建成功，受影响包 `go vet` 通过。
- 使用 PowerXPlugin skeleton 当前开发 Host API Key 实际调用 `grant-status` 返回 HTTP 200；客户 `service_manage` 和 `service_read` 均为 `granted`。这只证明现有凭证授权，不代表新版 update 已在运行进程生效。
- 2026-09-27 用户重启后，真实 HTTP API Key／STS 客户更新与重新读取验收 PASS。API Key update trace：`4a977cfb-ba6a-460f-a38d-ab2247a9bf95`；STS update trace：`9e542c17-258e-4830-abe4-ec3adf6c3884`。清空/保留字段、类型/主联系人不变、跨租户及撤销 grant 均通过；详见 [运行验收记录](customer-live-acceptance-2026-09-27.md)。
- PowerXPlugin 下一步：补 typed UpdateAccount DTO/方法（保留字段 presence），Delegated adapter 调用以上 binding，接入编辑页，并完成安装插件实际页面验收。本轮没有修改 PowerXPlugin。

## 实现位置

- `backend/internal/service/customer/account_update.go`
- `backend/internal/service/customer/capability_invoker.go`
- `backend/pkg/corex/db/persistence/repository/customer/account_update.go`
- `backend/internal/transport/http/openapi/capability_registry/tenant_handler.go`
- `backend/config/platform_capabilities/customer.yaml`
- `backend/tests/contract/plugin_grants/live_customer_update_test.go`

### 424 `registry.version_locked`

若客户更新在进入业务前返回 424，应由租户管理员确认 Core 能力版本，详见 [能力版本升级确认](capability-version-upgrade.md)。`grant-status=granted` 与版本升级确认是两项独立检查；插件无需传旧 hash，也无需为此迁移数据库。
