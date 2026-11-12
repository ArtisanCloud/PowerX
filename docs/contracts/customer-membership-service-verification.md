# 客户成员关系服务核验 v1

## 用途与边界

电商后端使用自己的正式服务身份，核验指定 Core Customer 是否在当前租户有效，及其主联系人是否有效。复用 Customer Account、TenantMembership、Contact；无新业务表、不写业务数据、不自动补联系人。Gateway 继续记录既有调用审计与 trace。

此能力不证明客户本人登录、下单者与客户的关系、付款同意或订单所有权。WorkspaceX／电商继续负责这些业务授权。每次交易操作重新核验；结果不能作为长期授权缓存。

既有 `com.corex.customer.memberships.delegated_read` 保持客户 JWT + STS 的客户本人合同，不放宽为按任意 UUID 查询。

## 冻结合同

| 项目 | 值 |
|---|---|
| Capability | `com.corex.customer.memberships.service_read` |
| permission_code | `corex.customer.memberships.service_read` |
| preferred_protocol | `core_internal` |
| 固定绑定 | `core://customer/memberships`，`INVOKE` |
| 主体 | `service_actor`，正式 STS 或 API Key |
| 范围 | 从认证凭据取得当前租户 |
| 操作 | `verify_customer` |
| schema_version | 整数 `1` |
| 请求／响应 Schema | [OpenAPI](customer-membership-service-verification.openapi.yaml) |

请求仅包含以下正文；字段名严格区分大小写，UUID 必须是规范小写、非零 UUID。拒绝额外字段、租户覆盖、任意 endpoint／method／headers／query，以及自由 context。

```json
{
  "capability_id": "com.corex.customer.memberships.service_read",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "operation": "verify_customer",
      "schema_version": 1,
      "customer_uuid": "11111111-1111-4111-8111-111111111111"
    }
  }
}
```

调用：`POST /api/v1/tenant/invocations`。HTTP `Authorization` 使用服务凭据。地址与方法由正式 binding 决定，请求中不再填写。Admin Bearer、Root 用户身份及用户角色不授予本能力。

Gateway 成功 envelope 的 `data.payload` 为：

```json
{
  "item": {
    "schema_version": 1,
    "tenant_uuid": "22222222-2222-4222-8222-222222222222",
    "customer_uuid": "11111111-1111-4111-8111-111111111111",
    "membership_uuid": "33333333-3333-4333-8333-333333333333",
    "primary_contact_uuid": "44444444-4444-4444-8444-444444444444",
    "status": "active",
    "expires_at": null,
    "checked_at": "2026-11-11T00:00:00Z",
    "revision": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }
}
```

示例 UUID、时间及摘要仅为格式示例，不代表任何实际客户。`data.trace_id` 为调用追踪号。

- `expires_at` 必须存在；null 表示未设置成员关系到期时间。
- `checked_at` 是本次查询完成后的 UTC 核验时间；`expires_at <= checked_at` 即拒绝。
- `revision` 是三对象的 UUID、状态、主联系人引用、到期时间和更新时间组成的 SHA-256 快照摘要。不包含核验时间和个人资料，不是递增序号，不是身份凭据。相同快照重复核验，摘要相同。

## 查询与错误

一条带租户条件的 SQL 联合读取 Account、TenantMembership 和 Contact，因此使用同一个数据库语句快照。排除已删除的 Account／Membership；Contact 使用精确租户、客户、引用 UUID 和软删条件的 LEFT JOIN，避免联系人失效被吞成客户不存在。

| HTTP | `details.reason_code` | 含义 |
|---|---|---|
| 400 | `CUSTOMER_VERIFICATION_INVALID_ARGUMENT` | 请求字段、版本、UUID 或固定绑定不合法 |
| 401 | `CUSTOMER_VERIFICATION_UNAUTHORIZED` | 非有效服务身份，包括 Admin 用户、失效令牌、主体错配 |
| 403 | `CUSTOMER_VERIFICATION_FORBIDDEN` | 服务未获授权，或内部租户上下文不一致 |
| 404 | `CUSTOMER_MEMBERSHIP_NOT_FOUND` | 当前租户中没有可读取的客户成员关系，包括已删除对象 |
| 409 | `CUSTOMER_ACCOUNT_INACTIVE` | 客户非 active |
| 409 | `CUSTOMER_MEMBERSHIP_INACTIVE` | 成员关系非 active |
| 409 | `CUSTOMER_MEMBERSHIP_EXPIRED` | 成员关系已到期 |
| 409 | `CUSTOMER_PRIMARY_CONTACT_INVALID` | 主联系人未绑定、缺失、停用、临时、已删除，或租户／客户不匹配 |
| 503 | `CUSTOMER_VERIFICATION_UNAVAILABLE` | 授权依赖、数据库或核验数据暂不可用 |

Gateway 在调用前拒绝凭据、未发布能力、未登记租户或撤权时，保留原 `CAPABILITY_*` 等公共机器码；不强制重写为客户业务码。SDK 应保留两类错误及 trace。服务错误不透传数据库原文。其他租户是否存在该客户不向调用者披露。

## 精确服务授权与部署

API Key 的正式映射：

```yaml
scope: _scope.customer.memberships.service_read
action: read
resource_type: capability
resource_pattern: customer_memberships_service_read
effect: allow
```

权限同步后必须有 `allow_api_key=true`、`api_key_explicit=true`。不通过 Admin 管理权限、默认 Profile 扩权或用户角色授予。STS 按电商自身运行凭据的精确 `allowed_capabilities` 判断。发布、租户登记和授权均实时核对，同一个旧令牌／Key 在撤权后应立即拒绝下一次调用。

发布步骤：

1. `make capability-check` 检查正式目录。
2. 对目标实例配置执行限定发布：`make capability-seed CAPABILITY_SEED_CONFIG=/absolute/path/to/config.yaml CAPABILITY_SEED_FLAGS='-capability-ids com.corex.customer.memberships.service_read'`。
3. Core 所有者构建、重启其日常服务。这次没有业务 schema 迁移。
4. 管理员只给电商自身的服务凭据添加上面的精确只读 grant；STS 则按正式插件能力申请／审批流程加入 required capability。不改其他 Key、默认角色或 Profile。
5. 用电商实际身份查询 `POST /api/v1/tenant/capabilities:grant-status`，确认本能力为 `granted`。查询本身也需要目录查询能力的既有授权，不为了验收追加宽权限。
6. Framework 新客户端发布后，使用真实 SDK 验证有效客户、跨租户、停用、过期、无效主联系人及撤权；随后由电商绑定 `Deps.CheckoutCustomers`。

发布命令同步正式权限与目录，不给现有 Key 自动附加 grant。重启本身不会代替能力发布和授权。

## Framework 对齐清单

改动归 Framework 仓库，本轮 Core 不修改 SDK 或版本号。位置为 `framework/backend/go/runtime/customerfw/`，复用公开 `gateway.Client`／既有 `AdminGatewayInvoker` 的 Invoke 方法，但新增独立服务客户端，不改变 AdminClient 或 DelegatedMembershipResolver。

建议公开：

```go
verifier, err := customerfw.NewServiceMembershipVerifier(gatewayClient)
result, err := verifier.Verify(ctx, customerUUID)
```

实现要求：

1. 固定 capability、`core_internal` 和上述 `payload.body`；不沿用 AccountSelectorClient 中手填 method／endpoint 的旧形式。
2. 每次调用使用当前服务凭据；不需要客户 JWT，不转发 Admin Token，不允许传入 tenant／plugin／actor 覆盖身份。
3. 强类型校验 `data.payload.item`：版本 1、四个规范非零 UUID、目标 customer 完全一致、当前 Gateway 租户一致、active、`expires_at` 字段存在且格式合法、非空合法 checked_at／revision。成员到期时拒绝。
4. 返回 Gateway trace；若返回类型携带 TraceID，应从 envelope 取得，不能用请求 ID 冒充。未知版本、缺字段、UUID 错配和空结果应明确失败，不能生成“有效客户”占位值。
5. 保留 HTTP 状态、Core `details.reason_code`、公共 Gateway 错误码和 trace，不把授权错误或平台故障改成客户不存在。
6. 不跨交易缓存核验结果、不对写交易做隐式重试、不回退到客户列表、插件本地表或伪造客户 JWT。
7. 发布可消费 SDK 版本，并回传成功、错误映射、取消、跨租户响应、版本／字段损坏和撤权测试。电商再升级依赖并实现其 CheckoutCustomerVerifier 适配器。

## Core 回归与现场边界

| 文件 | 职责 |
|---|---|
| `backend/internal/service/customer/membership_verification_service.go` | 服务身份实时授权、状态／到期判断、最小结果和快照摘要 |
| `backend/pkg/corex/db/persistence/repository/customer/membership_verification.go` | 精确租户和客户的单条联合查询 |
| `backend/internal/service/customer/membership_verification_invoker.go` | 固定绑定和封闭的强类型请求 |
| `backend/internal/service/customer/capability_invoker.go` | 在现有 customer invoker 中装配；shared.Deps 和 tenant handler 的现有构造路径均自动取得新服务 |
| `backend/internal/transport/http/openapi/capability_registry/customer_verification_errors.go` | HTTP 状态及业务机器码分类 |
| `backend/internal/transport/http/openapi/capability_registry/tenant_handler.go` | 错误映射、保留 trace 并屏蔽内部依赖原文 |
| `backend/config/platform_capabilities/customer.yaml` | 正式能力及独立服务权限，不附加默认授权 |
| `backend/config/capability_audit_required.yaml` | 防止治理检查遗漏该正式能力 |

```bash
cd backend && go test ./internal/service/customer ./internal/transport/http/openapi/capability_registry ./internal/service/integration_gateway/apikeypermissions ./internal/service/capability_registry
```

真实 PostgreSQL 回归仅允许专用 `px_customer_verify_` 测试库；`POWERX_MEMBERSHIP_VERIFY_TEST_DSN` 指向隔离实例后运行 `go test ./internal/service/customer -run '^TestMembershipVerificationPostgres$' -count=1 -v`。测试会创建模型和测试数据，禁止把日常配置转成这个环境变量。覆盖 UUID／时间类型及原子切换主联系人时的并发快照。

本轮验收收据另列源码、隔离 PostgreSQL、隔离 capability seed 和受控 HTTP grant-status 结果。正式实例尚须按上面的发布步骤执行；源码通过不表示电商真实交易已经联通。
