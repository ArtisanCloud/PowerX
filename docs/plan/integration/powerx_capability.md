# PowerX Capability Exposure Plan

## 背景

为支持宿主模式与 Skeleton 模式的插件统一调用 PowerX 核心能力，需要将底座模块的开放接口纳入 "Integration Gateway & MCP" 专题管理。当前 Media、事件总线、定时任务、AI 知识库、Workflow 等能力大多仅通过 Admin API 暴露，导致插件需依赖内部路由。此计划旨在提供统一的 HTTP/OpenAPI 与 gRPC 契约，使任何插件或第三方在拿到授权后即可调用底座能力。

## 建议方案

1. **Registry 扩展**：在 Capability Registry 中新增 `source=corex|plugin` 字段，并预置 Media、Event Fabric、Scheduler、Knowledge、Workflow 等 CoreX 能力记录，统一走 Tool Grant 与限流策略。
2. **对外契约**：每个底座模块维护 `specs/<module>/contracts/http-openapi.yaml` 与 `backend/api/grpc/contracts/<module>/v1/*.proto`，Integration Gateway 以这些契约为源生成 SDK 和文档。
3. **统一调用入口**：第三方通过 `/tenant/capabilities` 与 `/tenant/invocations`（或 gRPC `IntegrationGatewayTenantService`）调用底座能力；宿主模式可继续使用 Admin API 进行配置，但实际能力调用全部归口 Integration Gateway。
4. **观测与治理**：沿用 FR-001~FR-015 的追踪/限流/审计要求，对平台能力与插件能力实施一致的 metrics/audit/event 采集。
5. **媒资公开 API**：PowerX 底座的 **Media Assets Management** 模块已在 `specs/001-docs-media-storage/contracts/http-openapi.yaml` 提供 `{APIPrefix}/media/assets` 路径，包含上传、列表、详情、软删、预签名能力；插件（宿主或 Skeleton）只需携带 Bearer Token 与 `X-Tenant-UUID` 即可直接调用，对应调用流程记录在本计划与 Quickstart 中。

## 已内置的平台能力（2025.01）

| Capability ID | 模块 | 访问场景 | 协议/入口 |
| --- | --- | --- | --- |
| `com.corex.media.assets.read` | Media | 媒资列表、详情查询 | REST：`GET {APIPrefix}/media/assets`、`GET {APIPrefix}/media/assets/{uuid}`；gRPC：`powerx.media.v1.MediaAssetAdminService/List|Get` |
| `com.corex.media.assets.manage` | Media | 上传、删除、预签名 | REST：`POST/DELETE {APIPrefix}/media/assets`、`POST {APIPrefix}/media/assets/{uuid}/presign`；gRPC：`Create/Delete/PresignMediaAsset` |
| `com.corex.eventfabric.publish` | Event Fabric | 事件发布 & 订阅 | gRPC：`corex.event_fabric.v1.EventDeliveryService/PublishEvent`、`EventSubscriberService/Subscribe` |
| `com.corex.scheduler.jobs` | Workflow Scheduler | Workflow/Scheduler 实例触发、暂停、继续 | gRPC：`powerx.workflow.v1.WorkflowService/StartInstance`、`ControlInstance`、`ListInstances` |
| `com.corex.workflow.builder` | Workflow Builder | 模板创建、发布、查询 | gRPC：`powerx.workflow.v1.WorkflowService/CreateDefinition`、`PublishDefinition`、`ListDefinitions` |
| `com.corex.knowledge.space` | Knowledge Space | 空间/策略/增量管理 | gRPC：`powerx.knowledge.v1.KnowledgeSpaceAdminService/Create|Update|TriggerIngestion` |

## 宿主 vs Skeleton 调用流程

1. **统一发现入口**：无论插件运行模式如何，均需调用 `/tenant/capabilities?source=corex`（或 gRPC `ListTenantCapabilities`）列出平台能力。Host 模式可直接复用宿主 Web Admin 已注入的 TENANT Token；Skeleton 模式通常通过 STS/Service Account 获取租户级 JWT，两者都必须带 `X-PowerX-Tenant`。
2. **Invocation 请求**：插件向 `/tenant/invocations`（或 gRPC `InvokeCapability`）发送 `CapabilityInvokeRequest`。当 `capability_id` 属于 `source=corex` 时，Selector 会将调用路由到 Media/Event/Workflow 等底座模块，同时写入 `InvocationTrace` 与 `integration.gateway.invocation.*` 事件。
3. **OpenAPI 直连**：部分平台能力（例如 Media）还暴露 `{APIPrefix}/media/assets` 等公开路径，允许插件在确认 Tool Grant 后直接调用。`APIPrefix` 默认 `/api/v1`，可在 `cfg.Server.APIPrefix` 中改为 `/api/admin/v1` 或 `/api/v2`，调用时只需要租户 Token。
4. **Insomnia/cURL 模板**：
   - Insomnia 请求：`POST {{POWERX_BASE_URL}}/tenant/invocations`，Headers 包含 `Authorization: Bearer {{TENANT_TOKEN}}`、`X-PowerX-Tenant: {{TENANT_UUID}}`，Body 为 `capability_id` + `payload` JSON。
   - cURL 上传 Media：
     ```bash
     curl -X POST "$POWERX_BASE_URL/media/assets" \
          -H "Authorization: Bearer $TENANT_TOKEN" \
          -H "X-PowerX-Tenant: tenant-001" \
          -F "file=@samples/logo.png"
     ```
   以上流程在宿主模式（插件嵌入式）与 Skeleton 模式（独立服务）保持一致，只是 Token 获取来源不同。

## 管理端“开放能力”页面（T057）

- **入口**：Web Admin “设置 > 开放能力”，仅 `IsRoot` 管理员可见。页面进入后自动拉取 Registry 中 `source=corex` 能力。
- **模块分组**：基于能力的 `module` 字段生成卡片（Media/Event/Scheduler/Knowledge/Workflow 等），显示能力数量、最新 `capabilities_hash`、支持协议徽章。
- **能力明细**：展开后可查看 `capability_id`、描述、协议列表与最新状态（active/disabled），同时提供复制 cURL/Insomnia/MCP Tool snippet 的按钮，以及跳转到 OpenAPI/gRPC 文档或 `/media/assets` 等公开接口的链接。
- **刷新机制**：提供“立即同步”按钮触发再拉取，如需实时也可订阅 `capability.catalog.sync_*` 事件刷新；所有读取动作写入审计日志。
- **用途**：该页面成为宿主/Skeleton 插件开发者的官方入口，可直接复制调试示例并了解各模块提供的底座能力，减少依赖内部路由的情况。

## 里程碑

- **M1 (Media)**：将 Media Assets Management 能力纳入 Registry，发布对外 OpenAPI/gRPC 契约，支持 `/tenant/invocations` 把文件上传/预签名请求路由到 Media Service。
- **M2 (Event Fabric & Scheduler)**：为事件广播与定时任务输出能力记录，公开订阅/触发接口以及安全策略。
- **M3 (Knowledge & Workflow)**：Knowledge Space、Workflow Builder/Engine 对外提供模板查询与执行能力，确保 Skeleton 插件能直接调用。
- **M4 (统一 SDK/Docs)**：Integration Gateway 汇总上述契约生成 SDK 与统一说明书，覆盖宿主与 Skeleton 场景。

## 风险与缓解

- **契约漂移**：通过 Buf/OpenAPI 验证与 CI contracts-test 防止接口不一致。
- **鉴权复杂度**：复用现有 Tool Grant / STS / Tenant Auth 体系，避免重复造轮子。
- **兼容性**：在 Admin API 保留原有路径，逐步引导插件迁移到 Integration Gateway 调用链。

## Next Steps

1. 更新 `specs/007-integration-gateway-and-mcp/spec.md`，纳入 Base Capability Exposure Roadmap（已完成）。
2. 为 Media/Event/Workflow 等模块补充对外 OpenAPI/gRPC 契约，并在 Registry 中登记 `source=corex` 能力。
3. 扩展 Integration Gateway Handler/Service，支持 1P 能力的调用路由与治理策略。
