# Cache / TaskCenter Core Host API

合同版本：1.0.0，2026-09-09。对应 Framework 的 `docs/contracts/cache-taskcenter-host-requirements.md`。

本文件与 `specs/contracts/runtime-host.openapi.yaml` 是 Core 交接合同。Framework 的 Go Scope 不直接序列化到请求；delegated 客户端需将受信任 Scope.TenantUUID 与 STS 绑定租户核对，不一致立即失败。本文不表示目标环境已部署或三方安装态联调已通过。

## 路由与授权

调用主体仅为 STS 插件服务：issuer=`powerx-sts`、audience=`powerx:api`、scope=`access`，不代表后台登录用户。本版不支持 API Key、用户 JWT 或 on-behalf-of。复用 Core 的 JWT 验签、DirectGrant 校验和正式目录路由派生。

| Framework 操作 | HTTP | capability_id |
|---|---|---|
| Cache.Get | GET `/api/v1/tenant/runtime/cache/entries?namespace=…&key=…` | `com.corex.runtime.cache.read` |
| Cache.Set | PUT `/api/v1/tenant/runtime/cache/entries` | `com.corex.runtime.cache.manage` |
| Cache.Delete | DELETE `/api/v1/tenant/runtime/cache/entries?namespace=…&key=…` | `com.corex.runtime.cache.manage` |
| TaskCenter.Create | POST `/api/v1/tenant/runtime/tasks` | `com.corex.runtime.taskcenter.manage` |
| TaskCenter.Get | GET `/api/v1/tenant/runtime/tasks/{task_uuid}` | `com.corex.runtime.taskcenter.read` |
| TaskCenter.Update | PATCH `/api/v1/tenant/runtime/tasks/{task_uuid}` | `com.corex.runtime.taskcenter.manage` |

permission_code 分别为 `corex.runtime.cache:read/manage`、`corex.runtime.taskcenter:read/manage`。能力非 agent 可选，均限制当前主体自己的资源。正式登记位于 `backend/config/platform_capabilities/runtime_host.yaml`，不增加静态白名单或 admin/internal 别名。

每次操作重新验证租户启用、capability published、最新 tenant registration published、当前启用的 `auth.credentials` 中的 client_id 与 token subject 一致及 allowed_capabilities 包含目标能力。撤权、禁用租户/凭证、撤销发布或 registration 后，旧 token 的后续请求拒绝；已经开始执行的请求不承诺追回。

主体表用受信任 `(issuer, plugin identifier, token subject)` 的 JSON 元组和 tenant UUID 建立稳定 UUID。任务及缓存均以该 UUID 隔离，不使用 numeric ID 作为关联或接口身份。保留同一 client_id 的 secret 轮换不转移归属；更换 client_id 是新主体，不自动认领旧记录。API 调用不接受 tenant/plugin/owner/subject 覆盖字段或租户头。

## Cache 合同

Set JSON：

```json
{"namespace":"license","key":"entitlement","value_base64":"AAH/","ttl_ms":60000}
```

- 四个字段全部必填；`value_base64` 是 RFC 4648 标准、带必要 padding 的 canonical Base64；空字符串表示零字节值。
- namespace 长度 1–128 UTF-8 字节，key 1–512 字节，拒绝首尾空白、NUL、CR、LF。原样区分大小写，不自动 trim。
- value 解码后最多 1 MiB；ttl_ms 为整数，范围 1–86,400,000（一天）。Framework 必须显式处理 time.Duration：非整毫秒或超界拒绝，不静默截断。
- Redis 用受信任 tenant + subject UUID + namespace + key 的 JSON 元组 SHA-256 作物理键，固定前缀 `powerx:runtime-host:cache:v1:`，用户输入无法构造跨主体键碰撞。
- Lua 使用 Redis TIME 确定固定 expires_at，SET 带 PX 一次原子提交值及 TTL；读取校验 TTL，不接受永久缓存。依赖不可用返回 503，不切换内存。
- Get 命中空值：`{"found":true,"value_base64":"","expires_at":"…"}`。miss：`{"found":false,"value_base64":"","expires_at":null}`。miss 返回 200。
- Set/Delete 成功 data 为 `{}`；Delete 不存在键成功。Get/Delete 不接受 body。
- Cache 是可过期缓存，不承诺 Redis 重启后的持久保留；授权判断每次查实时事实，不能依赖许可证缓存。

## TaskCenter 合同

Create JSON：

```json
{"type":"export.orders","idempotency_key":"export-batch-20260909","payload":{"format":"csv"}}
```

Update JSON：

```json
{"expected_revision":1,"state":"running","progress":20,"message_key":"tasks.export.running","result":null}
```

- Create 三字段必填。type 是 1–128 字节的机器标识，匹配 `[a-zA-Z0-9][a-zA-Z0-9._:-]*`；幂等键 1–128 UTF-8 字节，拒绝首尾空白、NUL/CR/LF。
- payload/result 为 JSON 值，包括显式 null；最大各 256 KiB、嵌套不超过 64 层，拒绝重复对象字段、非法 UTF-8 和 JSON 字符串中的 NUL。不从文本解析结构化业务字段。
- 幂等唯一键为 tenant UUID + caller_subject_uuid + idempotency_key。相同 type/payload 返回原 UUID、当前状态和 revision；不同 type/payload 返回 409。对象键顺序与空白不影响比较，数组顺序与数字表示（如 1 和 1.0）参与比较。没有自动过期或重新执行语义。
- 创建默认 queued、progress=0、revision=1。revision 使用 PostgreSQL 有符号 bigint，范围 1–9223372036854775807；Framework uint64 超界须拒绝。达到最大值后的更新返回 conflict。
- Update 必填 expected_revision/state/progress；message_key/result 可省略，省略时分别清空为 `""` 和 JSON null。message_key 为最多 256 字节的机器 i18n key，非空时遵循与 type 相同的字符集，文案由插件 locale 提供。
- progress 为 0–100 整数且不能倒退。状态转换与 Framework ValidateTransition 一致：queued 可以维持 queued 或转 running/failed/cancelled；running 可以维持 running 或转 succeeded/failed/cancelled。queued 不能直接 succeeded，running 不能回 queued。
- succeeded 必须 progress=100；所有终态带 completed_at，终态不允许再更新。queued/running 的 completed_at 为 null。cancelled 只标记任务记录，不表示停止业务执行。
- 更新事务内读取当前行、校验转换，再按 tenant + caller_subject_uuid + task UUID + expected_revision 执行 CAS，递增 1。不会先在 HTTP 层 Get 再无条件覆盖。
- Create/Get/Update 的 data 都是 Task：`task_uuid,tenant_uuid,type,state,progress,revision,message_key,result,created_at,updated_at,completed_at`。不返回数字 ID、幂等键、payload 或其他主体信息。时间采用 RFC3339；数据库精度为微秒。
- 其他租户或主体的任务对 Get/Update 均为 404；不为调用者区分不存在与不归属。TaskCenter 只记录任务，不 enqueue、不启动 worker、不提供轮询回退。

## 信封、校验和故障

成功：`{code:200,message:<locale>,data:…,timestamp:<Unix秒>,request_id?:…}`。

失败使用既有 DTO 信封：`{code,message,error?,error_code,reason_code,timestamp,request_id?}`。客户端按 error_code/reason_code 分支，不能解析 message。中英文文案来自 locale，底层 SQL、Redis 地址及原始错误不进入响应。

| HTTP | Cache | TaskCenter |
|---|---|---|
| 400 | CACHE_INVALID_ARGUMENT | TASKCENTER_INVALID_ARGUMENT |
| 401 | CACHE_UNAUTHORIZED | TASKCENTER_UNAUTHORIZED |
| 403 | CACHE_FORBIDDEN | TASKCENTER_FORBIDDEN |
| 404 | 不用于 cache miss | TASKCENTER_NOT_FOUND |
| 409 | — | TASKCENTER_CONFLICT |
| 503 | CACHE_UPSTREAM_DEPENDENCY | TASKCENTER_UPSTREAM_DEPENDENCY |

请求 body 必须为 application/json 对象，最大 2 MiB；顶层字段严格区分大小写，拒绝重复字段、未知字段、控制字段 null 和尾随 JSON。只有 payload/result 允许 JSON null。查询参数严格白名单且不允许重复；任务接口不接受 query。未知归属字段和 metadata 覆盖全部拒绝。

任务变更和操作审计同事务提交，审计失败回滚任务。Cache 写操作先持久化 attempt，再执行 Redis，最后记录 succeeded/failed；PostgreSQL 与 Redis 不构成分布式事务。Redis 超时或完成审计失败可能已写入缓存但返回 503，不能宣称写入回滚；操作者可用授权 Get 核查后显式重试。审计只保留主体 UUID、任务 UUID、键摘要、操作及 request_id，不保存 value/payload/result/凭证。

## 迁移、部署和交接

集中迁移挂载在 `backend/pkg/corex/db/database/migration.go`，新增 `runtime_host_subjects`、`runtime_host_tasks`、`runtime_host_operations` 三表。启动装配不执行 AutoMigrate。既有 Queue、安装任务或插件 local 任务不自动导入，没有 tenant/主体 UUID 的历史记录不得自动公开。

Host 服务经 shared.Deps 装配，复用 IntegrationGateway 已配置的 Redis 连接。Redis 未配置、数据库缺表或依赖异常均明确 503；配置 Host 服务时必须确保数据库迁移和 Redis 可用。不会为该模块自动启动 Redis 或切换存储实现。

部署步骤：备份并在隔离库验证迁移 → 使用目标环境配置执行 `make migrate` → `make capability-seed` 同步正式目录和 registration → 通过既有插件授权流程授予所需四项能力 → 部署并重启 Core → 重新签发/使用实际 STS 执行六项操作。capability-seed 不等于给插件授予 grant，不应执行全量 seed 扩大范围。

本轮代码验收与部署分开：Framework 可据此实现 typed delegated provider；电商 local adapter 同步 TTL、大小、revision 上限、JSON 和状态语义。安装态最终验收仍需目标环境的真实插件凭证，覆盖成功、无 grant、跨租户、跨插件、旧 token 撤权以及客户端错误映射。

验证命令（从 backend 执行）：

```bash
go test -race ./internal/service/runtime_host ./internal/infra/cache/runtime_host ./internal/transport/http/openapi/runtime_host -count=1
go test ./internal/http ./pkg/auth/middleware ./cmd/app ./cmd/database -count=1
POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN='dbname=postgres sslmode=disable' go test ./internal/service/runtime_host ./internal/http -run 'Test(Task|Cache|InvalidInput|RuntimeHost)' -count=1
POWERX_RUNTIME_HOST_TEST_REDIS=1 go test ./internal/infra/cache/runtime_host -count=1
```

PostgreSQL DSN 必须显式指向有建 schema 权限的测试库；测试只建立随机 `contract_runtime_host_*` schema，结束删除该 schema。Redis 测试显式启动临时 redis-server，不使用业务实例。仓库根目录另运行 `make capability-check` 与 `git diff --check`。

## 本轮已执行结果

- 新模块的默认存储测试与 `-race` 通过；OpenAPI 通过 schema 校验并与六项实际注册路由逐条核对。
- 真实 PostgreSQL 隔离 schema 下通过持久化、重复迁移、8 路并发幂等创建、8 路 CAS 单胜者、状态转换、租户/插件隔离、撤权、租户/凭证禁用及任务审计失败回滚测试。
- 独立真实 Redis 进程下通过空值命中、二进制值、固定过期时间、TTL 到期、拒绝无 TTL 记录及幂等删除测试。
- HTTP 合同使用测试密钥签发的 STS token，经实际 JWT 中间件、正式 YAML 路由准入、DirectGrant 校验和 Host handler；PostgreSQL 模式与 `-race` 均通过。覆盖伪造/过期 token、无认证、无 grant、旧 token 撤权、跨租户/跨插件、未知字段、metadata 覆盖、版本冲突及 Redis 故障。
- `internal/http`、`pkg/auth/middleware` 全包测试及 `cmd/app`、`cmd/database` 编译/测试通过。
- `make capability-check` 通过：declared=976、referenced=47、rest_routes=866、candidates=940。`git diff --check` 通过。
- 已检查无 `contract_runtime_host_*` 测试 schema 残留。未运行目标业务库 migrate/capability-seed，未重启业务进程；没有将上述合同测试称为插件安装态联调。
