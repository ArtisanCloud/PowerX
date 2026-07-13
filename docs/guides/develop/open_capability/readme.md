# PowerX 底座能力查看指南

本指南面向需要调试 PowerX 底座（`source=corex`）开放能力的开发者，帮助你在本地或测试环境中快速定位能力元数据、确认协议定义，并拿到可以直接调试的 API/CLI 命令。

如果你要从 PowerXPlugin 注册 Agent/Skill，并通过 Skill action 调用 capability，请先阅读：[PowerXPlugin Agent / Skill Bridge 开发指南](../plugin_agent_skill_bridge.md)。

> **前提条件**
>
> - 你需要一个 `IsRoot=true` 的管理员账号才能访问底座能力目录。
> - Web Admin 与 Backend 均已运行（前端默认 `http://127.0.0.1:3030`，Backend 默认监听 `http://127.0.0.1:8077`，前缀 `/api/v1`）。
> - 具备 Admin JWT（下文用 `ADMIN_TOKEN` 表示）与目标租户 UUID（`TENANT_UUID`）。

## 1. Web Admin 入口

1. 以 Root 管理员登录 Web Admin。
2. 进入 **设置 > AI > 能力注册表**：
   - 将“来源”筛选器保持在 “插件能力”，即可专注插件/租户透出的能力。
   - 切换为 “平台能力” 可快速确认底座能力是否同步完好。
3. 进入 **设置 > 开放能力**：
   - 仅展示 `source=corex` 的平台能力，按模块（Media、Event、Workflow、Knowledge）聚合。
   - 每个卡片提供协议标签、`capabilities_hash`、调试链接，方便直接跳入对应文档。

> 若你要联调网关鉴权（`API Key / Token`）与 ws-bus，请先阅读：
> [API Key / Token 联调指南](../api_key_token_playbook.md)

## 2. 查询接口（供自动化脚本使用）

能力目录查询接口的完整区别见：[能力目录查询 API 速览](./api.md)。

```bash
export API_ORIGIN="http://127.0.0.1:8077"
export ADMIN_TOKEN="<root-admin-jwt>"
```

### 列出全部平台模块

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$API_ORIGIN/admin/platform-capabilities" | jq .
```

### 查看指定模块（例如 media）

```bash
MODULE_KEY=media
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$API_ORIGIN/admin/platform-capabilities/$MODULE_KEY" | jq .
```

返回结果包含：

- `module`：模块键（如 `media`、`workflow`）
- `capabilities[]`：内含 `capability_id`、协议矩阵、优先策略、Docs 链接等

### 对比插件与平台能力

`/admin/capabilities` 暴露统一的能力列表，可通过 `source` 参数区分。

```bash
# 仅列出插件或租户同步的能力
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$API_ORIGIN/admin/capabilities?source=plugin&page=1&page_size=50" | jq .

# 查看底座能力（与“开放能力”页面相同数据源）
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$API_ORIGIN/admin/capabilities?source=corex" | jq .
```

## 3. 能力治理与补齐

PowerX 底座能力登记由两类 YAML 共同组成，二者都会进入正式 Capability Registry：

| 类型 | 文件 | 用途 | 默认 Agent 可选 |
| --- | --- | --- | --- |
| 生成 raw 能力 | `backend/config/platform_capabilities/generated.auto.yaml` | 从 OpenAPI、gRPC、Gin 路由生成底座接口级能力，覆盖大量管理端、租户端和服务端触点 | 否，`agent_usable: false` |
| 手写聚合能力 | `backend/config/platform_capabilities/*.yaml`（除 `generated.auto.yaml`） | 表达稳定业务授权单元，例如 media read/manage、AI invoke、customer admin manage | 按业务语义显式设置 |

当前完整性基线：

```text
generated.auto.yaml  802
手写聚合能力          22
正式能力合计          824
```

生成 raw 能力不是临时草稿。它必须留在 `backend/config/platform_capabilities/generated.auto.yaml`，并由 `BaseCapabilitySeeder` 写入 `CapabilityRecord`。但生成 raw 能力默认不进入智能体能力选购，避免把大量底层 route 暴露成面向业务用户的能力项。

### 必填元数据

每条正式 capability 必须包含：

- `permission_code`
- 插件 capability 如需普通成员使用，必须声明 `default_role_grants: [role_user]`
- `agent_usable`
- `risk_level`：`low`、`medium`、`high`、`critical`
- `module`
- `categories`
- `intents`
- `tool_scopes`
- `protocols`

每条 REST protocol binding 必须包含：

- `actor_context`：`admin_user`、`service_actor`、`web_user`、`mini_app_user`、`customer_actor`
- `resource_scope`：通常为 `tenant`、`owner`、`self` 等
- `sts_direct`：只允许显式打开，默认不得开放

`sts_direct: true` 只能用于 `actor_context: service_actor`，且不得指向 `/api/v1/admin/*`。后台用户态接口仍由用户 JWT、tenant member、RBAC 和业务权限判断，不受 STS direct 自动开放影响。

插件能力同步到 IAM 时，PowerX Core 会默认把 capability 的 `permission_code` 授给当前租户的 `role_owner` 和 `role_admin`。普通用户角色不会自动获得插件能力；插件 descriptor 必须显式声明 `default_role_grants`，例如：

```yaml
security:
  permission_code: com.powerx.plugins.base.local.template:read
  default_role_grants:
    - role_user
```

该声明可以位于顶层、`security` 或 `rbac` 下。允许角色码为 `role_owner`、`role_admin`、`role_user`、`role_readonly`、`role_vendor`。非法角色码会使 capability sync 失败。

### 服务态调用后台业务能力

插件后端、Agent Skill、系统集成属于 `service_actor`。这类调用优先走：

```http
POST /api/v1/tenant/invocations
```

如果一个能力同时服务后台用户页面和插件服务态调用，应该在同一个 capability 下登记不同 binding：

- `admin_user` + `user_jwt` + `/api/v1/admin/*`：供 PowerX Admin 或插件 Admin 页面用登录用户 JWT 访问。
- `service_actor` + `sts` + `core_internal` 或服务态开放 REST：供插件后端通过 `/tenant/invocations` 调用。

`/tenant/invocations` 的 `payload.endpoint` 可以继续使用该业务能力对应的 REST endpoint 作为选择参数，例如：

```json
{
  "capability_id": "com.corex.customer.accounts.admin_manage",
  "preferred_protocol": "rest",
  "payload": {
    "method": "GET",
    "endpoint": "/api/v1/admin/customers/accounts",
    "query": { "page": "1", "page_size": "20" }
  }
}
```

这不代表 STS token 可以直接访问 `/api/v1/admin/customers/accounts`。对 `com.corex.customer.accounts.admin_manage`，PowerX Core 会先校验 Registry、tenant registration 与 API Key/STS 授权，再在 Core 内部调用 customer service。禁止把 `/api/v1/admin/*` binding 改成 `sts_direct: true`。

### 标准补齐流程

当新增接口、修复生成器或怀疑底座能力缺失时，从仓库根目录执行：

```bash
make capability-gen
make capability-check
```

`make capability-gen` 会重新生成：

```text
backend/config/platform_capabilities/generated.auto.yaml
```

`make capability-check` 会临时生成候选文件并审计正式目录，成功输出应类似：

```text
capability-audit: ok, declared=824 referenced=21 rest_routes=642 candidates=802 ignored_route_rules=0
```

字段含义：

- `declared`：正式目录中会进入 Registry 的能力数量。
- `candidates`：本次从 OpenAPI、gRPC、Gin 识别出的候选数量。
- `rest_routes`：正式目录中 REST binding 覆盖的路由数量。
- `ignored_route_rules`：显式 ignore 规则数量，必须带原因，不能静默忽略。

如果 `declared` 只有几十个，而 `candidates` 有几百个，说明 raw 能力没有完整进入正式目录，不能视为完整。

如果要让当前运行库立即看到最新正式 YAML，继续执行：

```bash
make capability-seed
```

`make capability-seed` 只做 `backend/config/platform_capabilities/*.yaml` 到 Capability Registry 的同步，并为 active tenants 补齐 registrations；它不执行 migrate，也不执行全量 seed。执行后可以用 `/api/v1/tenant/capabilities/resolve` 验证当前租户是否已经能解析到新增 capability。若新增了真实 HTTP route，还必须重启 PowerX Core 到最新代码，否则 Registry 能解析，但转发到旧进程仍会返回 404。

如果部署后希望同时补齐 CoreX 基础种子数据和 Capability Registry，可以执行：

```bash
make seed
```

命令边界：

- `make capability-check`：只做构建/代码侧校验，不写运行库。
- `make db-seed`：只执行 CoreX / 数据库基础种子。
- `make capability-seed`：只同步 Capability Registry。
- `make seed`：等于 `make db-seed` + `make capability-seed`。

远程 dev 环境必须显式指定运行时配置，避免写到错误数据库：

```bash
POWERX_CONFIG=/etc/powerx-dev/config.yaml make seed
```

### 失败时怎么处理

- 缺真实 transport/service：先实现接口、service、repository、权限和测试，再登记 capability。
- `capability_gen` 路径生成错误：先修生成器。例如 Gin `Group("")` 必须保留父路径，不能把 `/api/v1/admin/skills/catalog` 误生成为 `/api/v1/catalog`。
- 正式 YAML 缺元数据：补 `permission_code / agent_usable / risk_level` 或修生成器。
- 路由不应发布：写入带 `category` 和 `reason` 的 ignore 文件，不允许静默跳过。
- STS direct 不通：先检查 capability REST binding 的 `actor_context/resource_scope/sts_direct`，不要只改鉴权白名单。

也可以直接让 Codex 执行：

```text
用 $capability-governance 做一次完整能力治理补齐
```

## 4. 如何调试

1. 根据模块（Media/Event/Workflow/Knowledge）打开对应的文档：
   - [Media 能力](./media.md)
   - [Event Fabric 能力](./event_fabric.md)
   - [Workflow & Scheduler 能力](./workflow.md)
   - [Knowledge Space 能力](./knowledge_space.md)
   - [AI 能力总览](./ai/README.md)
2. 文档中包含：
   - 可直接复制的 REST `curl` 与 `grpcurl` 命令
   - 所需 Header（`Authorization`）与典型请求体（租户由 JWT claims 提供）
   - 对应的 `capability_id`、意图、协议优先级
   - 统一的资源访问入口（例如 Media 的 `GET /api/v1/media/assets/{uuid}/resource`），方便你在调试阶段直接下载或跳转外链
3. 若需要走统一 Selector，可以使用 `/tenant/capabilities` / `/tenant/invocations`：

```bash
curl -sS -H "Authorization: Bearer $TENANT_TOKEN" \
  "$API_ORIGIN/api/v1/tenant/capabilities?source=corex"
```

更多细节请参考各模块文档，确保在调用前已为目标租户授予对应 Tool Grant/Feature Flag。这样即可在插件或宿主场景中直接复用 PowerX 底座提供的开放能力。

### 插件侧查询有效接口

插件侧查询 PowerX 当前租户可用的底座能力，使用租户侧能力目录：

```http
GET /api/v1/tenant/capabilities?source=corex&page=1&page_size=500
```

示例：

```bash
curl -sS "$PX_GATEWAY_BASE_URL/api/v1/tenant/capabilities?source=corex&page=1&page_size=500" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY"
```

该接口返回当前 token 所属租户可见的、已发布的 CoreX 能力。插件应从返回项的 `protocols[]` 读取 REST `method/endpoint` 或 gRPC `endpoint/rpc`，不要直接读取 `backend/config/platform_capabilities/*.yaml`。

如果插件已经知道 REST method + endpoint，可以反查 capability：

```bash
curl -sS "$PX_GATEWAY_BASE_URL/api/v1/tenant/capabilities/resolve?source=corex&method=GET&endpoint=/api/v1/admin/customers/accounts" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY"
```

插件侧接口选择：

| 场景 | 接口 |
| --- | --- |
| 查询当前租户可用 CoreX 能力 | `GET /api/v1/tenant/capabilities?source=corex` |
| 按 method + endpoint 反查 capability | `GET /api/v1/tenant/capabilities/resolve?...` |
| 调用能力 | `POST /api/v1/tenant/invocations` |
| Root/Admin 排障查看全量注册表 | `GET /api/v1/admin/capabilities?source=corex` |

### 关于 `/api/v1/tenant/invocations`

这是 PowerX 的“能力调度”入口，插件可以通过它调用 REST/gRPC/MCP 等不同协议的开放能力，而无需自己维护多套协议栈。使用方式如下：

```bash
curl -sS -X POST "$API_ORIGIN/api/v1/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "capability_id": "com.corex.media.assets.read",
        "preferred_protocol": "rest",
        "payload": {
          "method": "GET",
          "endpoint": "/api/v1/media/assets",
          "query": { "page": 1, "page_size": 20 }
        }
      }'
```

- `capability_id`：要调用的能力 ID，来自 Registry（如 `com.corex.media.assets.read`）。
- `preferred_protocol`：可选 `rest`、`grpc`、`mcp` 等，Selector 会按 Registry 的协议优先级路由。
- `payload`：描述具体调用。  
  - REST：提供 `method`、`endpoint`、可选 `headers`、`query`、`body`。  
  - gRPC：提供 `endpoint`（Service 名）+ `rpc`（方法），以及 `body`（JSON 序列化后的请求）。
- 返回结果：Gateway 会在统一的 Envelope 中附带真实业务响应与元信息。例如：

  ```json
  {
    "code": 200,
    "message": "success",
    "data": {
      "payload": {
        "items": [/* 这里就是 GET /media/assets 的 JSON */],
        "pagination": { "page": 1, "page_size": 20, "total": 1 }
      },
      "trace_id": "b28a79b9-a653-4ed3-b7aa-ea063432df6d",
      "protocol_used": "http",
      "fallback_used": false
    }
  }
  ```

  - `data.payload`：直接复用底层 REST/gRPC/MCP 响应体，插件无需关心协议差异。
  - `data.trace_id` / `protocol_used` / `fallback_used`：用于调试与观测；当发生 gRPC fallback 或 Workflow 补偿时，这些字段会反映最终采用的协议。
  - 错误场景下同样会在 `data.payload` 中返回底层错误内容，同时 HTTP 状态码与 `code` 字段会指示失败原因。

因此，当你希望“统一入口 + 自动协议适配”时就用 `/tenant/invocations`；若只是简单的 REST 调试，也可以直接调用文档中的业务接口。两者可以并行使用。

### Direct REST 调用与 STS 自动开放

插件使用 STS token 直接调用底座 REST 接口时，不再为普通开放能力逐条维护手工白名单。PowerX 会从正式能力目录自动派生可访问路由：

```text
STS direct route policy =
  static plugin runtime contracts
  + REST endpoints in backend/config/platform_capabilities/*.yaml
  - STS blocklist
```

开发新开放能力时，顺序必须是：

1. 实现真实 HTTP/gRPC/OpenAPI transport、service、permission 和测试。
2. 将 REST endpoint 登记到正式 `platform_capabilities` 的 `protocols[]`，并补齐 `actor_context`、`resource_scope`、`sts_direct`。
3. 运行 `make capability-check` 确认生成候选、正式声明和路由覆盖一致。
4. 运行 STS validator 相关测试，确认 direct REST 自动开放结果。

`/api/v1/admin/*` 是后台用户态 API 命名空间。插件 Admin 页面、PowerX Admin 页面、以及任何携带用户 JWT 的后台请求，可以继续调用 `/api/v1/admin/*`，并由用户鉴权、租户成员、RBAC 和业务权限判定。STS direct 自动开放规则只约束插件后端使用服务态 STS token 直接调用 PowerX Core。

以下路径默认不会通过服务态 STS direct 自动开放：`/admin/*`、`/internal/*`、`/public/*`、`/auth/*`、`/setup/*`、debug、migration、root、drain、bootstrap、mock、health、根级动态路径。若某个 `/admin/*` 入口确实是插件服务运行时合同，不应伪装成普通能力；需要进入静态合同 allow，并补用途说明和测试。

所以插件侧不需要猜“网关白名单是否加了”。只要底座能力完成正式 capability 登记且没有命中 blocklist，STS direct REST 就应自动可访问；否则应修能力声明或接口边界，而不是单独改鉴权放行。

## 5. 追溯能力定义位置

若需要从代码或文档层面追踪底座能力的来源，可按以下路径查找（所有文件都在仓库内）：

| 背景 | 文件/目录 | 作用 |
| --- | --- | --- |
| 能力注册定义 | `backend/config/platform_capabilities/generated.auto.yaml`<br>`backend/config/platform_capabilities/*.yaml`（手写聚合能力）<br>`backend/internal/service/integration_gateway/base_capabilities.go` | `generated.auto.yaml` 登记底座 raw 能力；手写 YAML 登记业务聚合能力；`base_capabilities.go` 负责加载并写入 Registry。新增/禁用能力时优先修改生成器或 YAML，再通过进程重启生效，可通过环境变量 `PLATFORM_CAPABILITIES_DIR` 指定自定义目录。 |
| OpenAPI 契约 | `specs/001-media-storage/contracts/http-openapi.yaml` 等 `specs/<module>/contracts/*.yaml` | 描述对外 REST 契约，Web Admin/CLI/Scripts 都引用这些文件生成示例。 |
| gRPC 契约 | `backend/api/grpc/contracts/...` | 包含 Media/Event/Workflow/Knowledge 等 gRPC 服务定义，与 Registry 中 `protocols` 字段一一对应。 |
| Web Admin 指南 | `docs/guides/develop/open_capability/<module>.md` | 本目录下的各模块文档（media/event_fabric/workflow/knowledge）提供调试示例。 |
| 路由与实现 | `backend/internal/transport/http/openapi/<module>/`、`backend/internal/transport/grpc/<module>/` | 可直接阅读 Handler 代码了解参数校验和实际调用流程。 |

通过上述线索，可以从“文档 → 契约 → 代码”逐层定位能力详情，确保开放接口与底层实现一致。

### 能力与接口的映射规则

- **一个 `capability_id` = 一个业务授权单元，不是一个 URL。** 例如 `com.corex.media.assets.read` 表示“媒体资产读取”能力，它在 `media.yaml` 中聚合了多个只读接口（`GET /media/assets`、`GET /media/assets/{uuid}`、`POST /media/assets/{uuid}/presign` with `operation=download` 等），租户只要获批该能力即可调用这些接口。
- **Admin/OpenAPI/gRPC 可以是同一个能力的不同 binding。** 如果 `/api/v1/admin/<resource>`、`/api/v1/<resource>` 与某个 gRPC service 表达同一业务语义、同一授权边界，应登记在同一个 `capability_id` 下，而不是因为路径不同拆成多个能力。
- **入口身份差异不等于能力差异。** `/api/v1/admin/*` 通常是用户态后台入口，`/api/v1/*` 可以是服务态开放入口；两者可以共享同一个 capability，但必须在 binding metadata、service 层 actor/owner 校验和 tool_scope 中明确调用边界。
- **Web / mini-app / customer 是外部业务入口。** 它们不属于后台治理能力，也不属于普通插件服务态 STS。设计这类接口时必须先声明 actor：`web_user`、`mini_app_user`、`customer_actor` 或 `service_actor`。
- **外部自助能力默认 owner/self scoped。** customer 或 mini-app 只能访问当前 customer/user/owner 可见资源，不得复用 admin 全量管理能力。
- **写入/管理能力单独建一条。** `com.corex.media.assets.manage` 则聚合了写入链路（`POST`、`PATCH`、`DELETE`、`POST .../presign` with `operation=upload`），方便在授权策略中单独开关可变更的接口。
- **协议字段列出全部触点。** 每条能力下的 `protocols[]` 会把 REST endpoint 或 gRPC method 一并列出，形成“能力 → 接口清单”的映射，便于 Selector/Registry 精确校验。
- **新增接口先选定归属能力。** 若要扩展新的 REST/gRPC 触点，先判断它属于现有的 read/manage 等能力还是需要新建能力；在 YAML 中更新 `protocols[]` 后，配套修改 OpenAPI/gRPC 契约与文档即可。
- **只有授权边界不同才拆能力。** 如果 admin 全量治理接口和插件 owner-scoped 自助接口的可操作资源范围、actor 约束、风险等级或授权开关不同，应拆成不同 capability；否则保持一个 capability 多个 binding。

以 Runtime Scheduler 为例：`/api/v1/admin/scheduler/jobs`、未来的 `/api/v1/scheduler/jobs`、`powerx.scheduler.v1.SchedulerService` 如果都表达“管理 Runtime Scheduler jobs”，应优先归到 `com.corex.scheduler.jobs`。用户态 admin 调用、服务态 direct REST 调用、`/tenant/invocations` 统一调度调用的差异，应通过 binding metadata 与 service 层 owner 校验表达，而不是创建多个语义重复的能力。

以 Customer Account 为例，如果后台管理员管理全租户客户、插件服务管理租户客户、客户门户只读自己的账号，它们通常是不同授权单元：

```text
com.corex.customer.accounts.admin_manage    # admin_user，全租户治理
com.corex.customer.accounts.service_manage  # service_actor，租户内服务态操作
com.corex.customer.account.self_read        # customer_actor，只读当前客户账号
com.corex.customer.account.self_update      # customer_actor，只更新当前客户允许字段
```

这些能力可以共享底层 service/repository，但不能共享同一个 admin capability。

借助上述约定，读写授权边界清晰，平台与插件对接时只需基于能力 ID 进行授权，而无需逐条维护接口白名单。

> 推荐的配置目录示例
>
> ```
> backend/config/
>   platform_capabilities/
>     generated.auto.yaml  # capability-gen 生成的 raw 能力，正式登记但 agent_usable=false
>     media.yaml           # 媒体模块能力
>     event_fabric.yaml
>     workflow.yaml
>     knowledge.yaml
> ```
> 每个 `*.yaml` 文件使用统一字段描述 capability 列表，便于在不同环境下通过配置管理平台能力。程序加载逻辑可参考 `base_capabilities.go`。

## 6. 相关操作文档

- Skills 导入与第三方安装（Admin）：[../../agent/skills/05-import-and-install.md](../../agent/skills/05-import-and-install.md)
