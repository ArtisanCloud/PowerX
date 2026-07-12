# 能力目录查询 API 速览

本文属于 `docs/guides/develop/open_capability/`，该目录是 PowerX 底座能力文档的唯一开发入口。历史上的 `docs/guides/develop/capability/` 不再作为独立目录使用。

PowerX 管理端提供两组能力查询接口，分别面向“能力注册表巡检”与“底座开放能力”两个场景。下表概览区别：

| 接口 | 适用对象 | 默认包含的数据 | 典型用途 |
| --- | --- | --- | --- |
| `GET /admin/capabilities` | Root 管理员、CI/脚本 | **全部能力**（插件 + CoreX），可通过 `source=plugin|corex` 过滤 | 能力注册表巡检、搜索插件能力、排查 Workflow 模板、核对同步任务 |
| `GET /admin/platform-capabilities`<br>`GET /admin/platform-capabilities/:moduleKey` | Root 管理员 | 仅 CoreX 底座能力，按模块聚合 | 设置 > 开放能力页面、编写宿主/ Skeleton 调试文档 |

`source=corex` 的平台能力包含两部分：

- `backend/config/platform_capabilities/generated.auto.yaml` 生成的底座 raw 能力，默认 `agent_usable=false`，用于完整登记 REST/gRPC/Gin 触点。
- 手写聚合能力 YAML，例如 `media.yaml`、`ai.yaml`、`customer.yaml`，用于表达稳定业务授权单元。

因此平台能力数量达到 700+ 是正常状态。判断是否完整应以 `make capability-check` 为准，而不是只看页面条数。

## `/admin/capabilities`

- Handler：`backend/internal/transport/http/admin/capability_registry/catalog_handler.go`
- 支持参数：
  - `page` / `page_size`
  - `plugin_id`, `intent`, `protocol`, `tool_scope`
  - `search`（模糊匹配 capability_id/title/plugin_id）
  - `status`, `tenant_uuid`, `include_workflows=true`
  - `source=plugin|corex`（若不传则返回所有来源）
- 响应：`CapabilityRecordDTO` 列表，可包含 `workflow_templates`, `capabilities_hash`, `protocols` 等完整字段。
- 其他配套接口：
  - `GET /admin/capabilities/:capabilityId`：单条详情
  - `GET /admin/capability-sync/jobs`：最近同步任务列表

> Web Admin “设置 > AI > 能力注册表” 就是调用该接口，但默认把 `source` 筛选器置为 `plugin`，以便专注插件或租户同步的能力；Root 可以切到 “平台能力” 查看 corex 项。

## `/admin/platform-capabilities`

- Handler：`backend/internal/transport/http/admin/capability_registry/platform_handler.go`
- 行为特点：
  - 内部固定 `Source=corex`
  - 数据按 `module` 聚合（Media、Event、Workflow、Knowledge 等）
  - 返回字段包括 `module`, `capability_count`, `protocol_channels`, `capabilities[]`。单模块接口还会返回 `generated_at` 与具体能力列表。
- 无分页/搜索，定位是“看板”而非调试 API。
- Web Admin “设置 > 开放能力” 页面直接使用这一接口，展示官方模块与调试入口链接。

## 何时选哪一个？

- **想找插件或租户自己注册的能力**：用 `/admin/capabilities?source=plugin`（或在 UI 中选“插件能力”）。
- **想确认 CoreX 底座开放了哪些模块、协议地址、文档链接**：用 `/admin/platform-capabilities`（或 UI “开放能力”页）。
- **需要 Workflow 模板快照、手动升级状态、同步任务**：只有 `/admin/capabilities` 提供。
- **想一次性看到模块统计 + 协议徽章 + 调试入口**：用 `/admin/platform-capabilities`。

## 插件侧查询有效接口

插件运行时不要调用 `/admin/capabilities` 来判断 PowerX 能力是否可用。`/admin/*` 是后台管理/排障视角；插件侧应使用租户侧能力目录：

```http
GET /api/v1/tenant/capabilities?source=corex&page=1&page_size=500
```

该接口返回当前 token 所属租户可见的、已发布的 CoreX 能力。插件从每条能力的 `protocols[]` 中读取 REST `method/endpoint` 或 gRPC `endpoint/rpc`。

示例：

```bash
curl -sS "$PX_GATEWAY_BASE_URL/api/v1/tenant/capabilities?source=corex&page=1&page_size=500" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY"
```

如果插件已知道 REST method + endpoint，想反查对应 capability，使用：

```http
GET /api/v1/tenant/capabilities/resolve?source=corex&method=GET&endpoint=/api/v1/admin/customers/accounts
```

示例：

```bash
curl -sS "$PX_GATEWAY_BASE_URL/api/v1/tenant/capabilities/resolve?source=corex&method=GET&endpoint=/api/v1/admin/customers/accounts" \
  -H "Authorization: ApiKey $PX_GATEWAY_API_KEY"
```

调用能力时优先使用统一入口：

```http
POST /api/v1/tenant/invocations
```

示例：

```json
{
  "capability_id": "com.corex.customer.accounts.admin_manage",
  "preferred_protocol": "rest",
  "payload": {
    "method": "GET",
    "endpoint": "/api/v1/admin/customers/accounts",
    "query": {
      "page": "1",
      "page_size": "20"
    }
  }
}
```

插件侧接口选择规则：

| 场景 | 接口 |
| --- | --- |
| 查询当前租户可用 CoreX 能力 | `GET /api/v1/tenant/capabilities?source=corex` |
| 按 method + endpoint 反查 capability | `GET /api/v1/tenant/capabilities/resolve?...` |
| 调用能力 | `POST /api/v1/tenant/invocations` |
| Root/Admin 排障查看全量注册表 | `GET /api/v1/admin/capabilities?source=corex` |

## 治理验收

开发或变更底座能力后，从仓库根目录运行：

```bash
make capability-gen
make capability-check
```

`make capability-check` 只验证代码和正式声明是否对齐，不写运行库。需要让当前 PowerX DB 的 Capability Registry 立即加载最新正式 YAML 时，执行：

```bash
make capability-seed
```

该命令只同步 `backend/config/platform_capabilities/*.yaml` 到 `capability_registry_records`，并补齐 active tenants 的 registrations；它不执行 migrate，也不执行全量 seed。新增了真实 HTTP route 时，还需要重启 PowerX Core 到最新代码，否则 `resolve` 可以成功，但 `/tenant/invocations` 转发到旧进程会返回 404。

通过时应看到类似：

```text
capability-audit: ok, declared=824 referenced=21 rest_routes=642 candidates=802 ignored_route_rules=0
```

其中：

- `declared` 是正式目录会写入 Registry 的能力数量。
- `candidates` 是本次从 OpenAPI/gRPC/Gin 识别出的候选数量。
- `generated.auto.yaml` 必须保留在 `backend/config/platform_capabilities/`，并包含 `permission_code / agent_usable / risk_level`。
- REST binding 必须声明 `actor_context / resource_scope / sts_direct`。

两套接口共享同一个 Registry 数据源，只是输出视角不同。开发者可以先在平台接口中确认 CoreX 官方能力，再通过能力注册表接口匹配 `capability_id`、核对模板与版本，最后在本目录中查阅各模块的调试示例。

## 路由命名规范

- **Admin/调试接口**：统一挂载在 `{api_prefix}/admin/*`，仅 `IsRoot` 或具备 Admin Token 的调用者可以访问，例如 `{api_prefix}/admin/capabilities`、`{api_prefix}/admin/platform-capabilities`。
- **开放能力接口**：租户可访问的 API 均以 `{api_prefix}/*` 为前缀（由 `server.api_prefix` 配置，默认 `/api`，常见部署为 `/api/v1`），例如 `{api_prefix}/media/assets`、`{api_prefix}/tenant/invocations`。所有请求需要携带 `Authorization`，租户信息由 JWT claims、STS/API Key/OAuth 等声明凭证提供。
- **Capability 调用入口**：插件后端、agent、skill 等服务态调用优先使用 `{api_prefix}/tenant/invocations`；direct REST 只作为明确开放的 protocol binding 使用。
- **gRPC/其他协议**：使用各自的 service 名称（如 `powerx.media.v1.MediaAssetAdminService`、`powerx.event_fabric.v1.EventDeliveryService`），通过 Registry `protocols` 字段暴露。

遵循以上命名约定，可以快速判断一个接口是管理端调试入口还是对插件/宿主开放的能力端点。
