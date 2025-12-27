# 能力目录 API 速览

PowerX 管理端提供两组能力查询接口，分别面向“能力注册表巡检”与“底座开放能力”两个场景。下表概览区别：

| 接口 | 适用对象 | 默认包含的数据 | 典型用途 |
| --- | --- | --- | --- |
| `GET /admin/capabilities` | Root 管理员、CI/脚本 | **全部能力**（插件 + CoreX），可通过 `source=plugin|corex` 过滤 | 能力注册表巡检、搜索插件能力、排查 Workflow 模板、核对同步任务 |
| `GET /admin/platform-capabilities`<br>`GET /admin/platform-capabilities/:moduleKey` | Root 管理员 | 仅 CoreX 底座能力，按模块聚合 | 设置 > 开放能力页面、编写宿主/ Skeleton 调试文档 |

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

两套接口共享同一个 Registry 数据源，只是输出视角不同。开发者可以先在平台接口中确认 CoreX 官方能力，再通过能力注册表接口匹配 `capability_id`、核对模板与版本，最后在 `docs/guides/develop/open_capability/` 中查阅各模块的调试示例。

## 路由命名规范

- **Admin/调试接口**：统一挂载在 `/admin/*`，仅 `IsRoot` 或具备 Admin Token 的调用者可以访问，例如 `/admin/capabilities`、`/admin/platform-capabilities`。
- **开放能力接口**：租户可访问的 API 均以 `/api/v1/*` 为前缀（遵循 `server.api_prefix` 配置），例如 `/api/v1/media/assets`、`/api/v1/tenant/invocations`。所有请求需要携带 `Authorization` 与 `X-Tenant-UUID`。
- **gRPC/其他协议**：使用各自的 service 名称（如 `powerx.media.v1.MediaAssetAdminService`、`powerx.event_fabric.v1.EventDeliveryService`），通过 Registry `protocols` 字段暴露。

遵循以上命名约定，可以快速判断一个接口是管理端调试入口还是对插件/宿主开放的能力端点。
