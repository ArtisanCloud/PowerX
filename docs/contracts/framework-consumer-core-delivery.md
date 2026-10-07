# Framework consumer 缺口：Core 实施记录

日期：2026-09-08。对应 PowerXPlugin `docs/contracts/framework-consumer-contract-gaps.md`。

## 本轮边界

- Variant 传输：代码、状态字段、正式路径与定向测试已补；未执行业务库迁移、seed、重启或安装态验证。
- Registry/Gateway：补当前服务凭证过滤、目标即时授权与 trace 调用主体隔离；不能以包测试代替八条入口的真实凭证验收。
- 组织写入：**未实现、未发布**。不能把既有 provisioning 或只读 Directory 称为组织同步合同。

## Variant 合同

以下三个 POST 路径均位于 `/api/v1/tenant/media/assets/{asset_uuid}/variants/{variant_uuid}` 下：

| 操作 | 后缀 | 请求 | 成功 data |
|---|---|---|---|
| 上传 ticket | `/presign-upload` | `{ "expires_in_seconds": 900 }`，可省略字段但必须传对象 | `url, method, expires_at, headers` |
| 完成上传 | `/complete-upload` | `{ "checksum": "SHA256_HEX" }` | `asset_uuid, variant_uuid, status, completed_at` |
| 下载 ticket | `/presign-download` | 同上传 ticket | 同上传 ticket |

创建 Variant 新增必填 `checksum`，与 `size_bytes`、`mime_type` 一起固定上传期望；创建和读取响应新增 `status`、`completed_at`。输入 UUID 必须为非零 canonical UUID。拒绝未知字段、重复 JSON 字段、null、tenant 覆盖和任意存储位置输入。

- 读取：`com.corex.media.assets.read`。
- 创建：`com.corex.media.assets.variants.manage`。
- 三项传输：复用 `com.corex.media.assets.transfer`；API Key 精确授权为 `_scope.media.assets.transfer / transfer / api / assets`。
- STS/API Key 都须满足 capability 发布、最新 tenant registration 生效、当前凭证 grant。
- ticket TTL 默认 900 秒，输入范围 60–3600 秒；实际上传 ticket 不超过资产上传窗口（创建后 1 小时）。
- ticket URL 指向 Core `/api/v1/media/transfers/{asset_uuid}/variants/{variant_uuid}/{upload|download}`；客户端消费返回的 URL，不拼接存储路径。上传 PUT 返回 204，下载 GET 返回对象字节。
- 同时修正原文件 ticket 实际路由遗漏 `/api/v1` 的漂移；原文件与 Variant 均使用正式 `/api/v1/media/transfers/*`。不保留根级旧 alias；部署后需重新获取临时 ticket。
- ticket MAC 绑定 asset、variant、操作、过期时间及两级吊销版本；与原文件 ticket 使用不同签名域。
- 状态：`pending_upload → uploaded → ready`。PUT 不等于 ready；complete 重新读取实际对象验证 size、MIME、SHA256。错误对象进入 failed。
- PUT 同时核对实际读取字节数；对象存储写入失败后进入 failed 并吊销 ticket，禁止对可能存在的半成品隐式覆盖重试。下载事务提交失败时关闭对象流，不返回未提交结果。
- ready 后相同 checksum 的重复 complete 返回 200；不同 checksum 返回 422 `MEDIA_UPLOAD_VALIDATION_FAILED`。不允许的状态返回 409。
- 删除父资产或变更吊销版本后，旧 ticket 不再可消费。已开始传输的网络字节不能追回。
- failed 是终态，本版不提供隐式重试或重置；不能把历史对象视为已校验对象。

OpenAPI：`specs/001-media-storage/contracts/http-openapi.yaml`。能力：`backend/config/platform_capabilities/media.yaml`。

## Registry/Gateway 授权执行位置

这些目录/调度入口不是新建的独立授权 capability；不编造目录权限或给开发 Key 自动扩权。

| 操作（相对 /api/v1） | 当前服务态校验 |
|---|---|
| GET /tenant/capabilities | RegistryService 在分页前按 CurrentAccess 过滤发布、registration、实际 grant，total 仅统计可见项 |
| POST /tenant/capabilities:grant-status | 保留 `com.corex.capabilities.grant_status.read` 独立入口授权，查询项不会变成 grant |
| GET /tenant/capabilities/resolve | 复用上述目录过滤；解析不等于执行授权 |
| POST /tenant/invocations | 显式 capability 分支入口校验；InvocationService 对最终目标再次实时校验 |
| GET /tenant/invocations/{traceId} | 凭证 tenant + 持久化 caller_subject + trace 查询，且仍要求目标 grant |
| GET /tenant/integration/routes | tenant 路由可用性 + 当前凭证目标 grant 过滤 |
| GET /tenant/integration/routes/{route_slug} | 同上；无权限与不存在统一不可见 |
| POST /tenant/integration/routes/{route_slug}/invoke | 实际选取 route 的目标 grant + 原有 tool grant；配置 tool grant 但校验器缺失时 503 |

`GrantStatusService.CurrentAccess` 每次读取当前有效凭证、正式 permission 元数据和最新 registration，不缓存授权结果。STS 校验 issuer/audience/exp/client subject；API Key 使用真实 key UUID，而不是 profile numeric ID 作为调用主体。API Key 目标 scope 来自该能力正式 binding 的精确 `api_key` 元数据，不将查询项或目录可见性视为授权。

STS route admission 对六项目录/trace/Gateway 操作标记为明确 runtime contract，精确限定 method；不是全路径、全方法放行。最终资源授权仍在上述业务执行位置。

Gateway HTTP 的缺失凭证上下文、输入错误、目标无权、不可见路由、限流、依赖失败统一返回 `error_code` 和 `reason_code`；对应 401/400/403/404/429/503。可读错误文案由本模块 locale 提供，不返回数据库或执行器原始错误。不会将原始 Authorization 作为业务 actor 传递。返回状态未知或执行结果不是合同规定的 JSON 对象时明确失败，不包装成空成功或 raw 文本结果。

Gateway 顶层 JSON 拒绝重复字段、大小写别名、未知字段、null 和尾随 JSON。任意 tenant 查询字段与 tenant header 不得覆盖凭证上下文。

## 迁移和部署

本轮新增持久化字段，已有集中 migration 已挂载对应模型：

- media_asset_variants：upload_state、expected_checksum、upload_expires_at、completed_at、ticket_version。旧行默认 failed，不能回填为 ready；数据不删除、不猜 checksum。
- capability invocation trace：caller_subject。历史行默认空，不自动认领给任何插件；服务态读取历史无归属 trace 返回不可见。现有后台用户态审计路径不做自动归属转换。
- media_assets.owner_subject_uuid 保持 PostgreSQL nullable UUID 列，Go 模型改为 `*uuid.UUID`。缺省 owner 写 SQL NULL，不再写入空字符串。该类型修正本身不新增列。

部署前备份并先在隔离库验迁移；之后执行 `make migrate`、`make seed`、部署新二进制并重启目标环境。`make seed` 不是给所有 Key 自动授予 transfer 权限。不要将开发实例配置或凭证用于生产。

本轮未执行上述业务库及进程变更。未完成安装态成功/撤权验证之前，状态不是 deployed 或 integration accepted。

## 测试入口

从 backend 执行：

```bash
go test ./internal/service/media ./internal/transport/http/openapi/media ./tests/contract/media -count=1
go test ./internal/service/capability_registry ./internal/transport/http/openapi/capability_registry ./internal/service/integration_gateway/tenant ./internal/transport/http/openapi/integration_gateway -count=1
go test ./internal/http ./pkg/auth/middleware ./cmd/app ./cmd/database -count=1
```

仓库根目录：`make capability-check`、`git diff --check`。

本轮执行结果：上述定向测试与 app/database 编译检查通过；capability-check 通过（declared=972、referenced=43、rest_routes=860、candidates=934）；git diff --check 通过。新增 Gateway HTTP 合同测试覆盖缺失上下文 401、目标无 grant 403、不可见 404、输入错误 400、限流 429、执行失败 503、200/202、凭证不进入业务参数和依赖错误不外泄。service 回归另验证授权成功执行后撤权，原 token 上下文再次调用被拒且不触发执行器。

Media service/HTTP、Capability Registry service、Gateway tenant service/HTTP 五个包的 `go test -race ... -count=1` 已通过。

Variant HTTP 测试支持临时 SQLite 及真实 PostgreSQL 的隔离事务/schema，并使用临时本地对象目录；仍只注入已认证 claims。真实授权 service、repository、传输 handler、对象校验参与测试，但不是实际 STS 签名校验或已部署实例联调。Registry/Gateway 撤权与 trace 隔离测试基于隔离测试库。

已实际执行并通过 PostgreSQL 验证：

```bash
cd backend
POWERX_CONTRACT_TEST_POSTGRES_DSN='dbname=postgres sslmode=disable' \
  go test ./internal/transport/http/openapi/media -run TestVariantHTTPTransferPostgres -count=1
```

该命令仅适用于本机具有本地 PostgreSQL 访问权限的测试环境；其他环境显式传测试 DSN，不使用业务配置自动推导。测试在事务内创建随机 `contract_variant_*` schema，结束回滚，已确认无 schema 残留。验证包含重复 AutoMigrate、UUID 保留、历史 Variant 默认 failed、可选 owner 为 NULL、ticket 读取不改 updated_at、上传/complete/下载、checksum 不匹配及 STS/API-key 撤权拒绝。

## 组织写入前置差异

当前源码仍存在以下旧引用链路，不能直接作为 UUID-only 同步实现复用：

- `iam.Member.UserID` 及 MemberService 的 `user_id` 查询/联表。
- `iam.MemberDepartment` 的 numeric `member_id + department_id` 主键；UUID 列尚不是唯一权威关联。
- Department 的 numeric parent/leader 引用，OrgService 的 numeric path/closure 维护。
- CreateMember 仍按邮箱/手机号寻找既有 User，与来源绑定禁止隐式认领冲突。

组织写入需要先覆盖这些现有 IAM 读写链路及管理端调用，再实现 source namespace、external subject、UUID 绑定、版本冲突和独立审批授权。只添加新 Host handler 或把 numeric ID 隐藏在 DTO 后面都不算完成。迁移不能自动将历史人工成员分配给同步插件。
