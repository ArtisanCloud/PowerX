# Implementation Plan: Integration Gateway & MCP Server（Base Capability Exposure）

**Branch**: `007-integration-gateway-and-mcp` | **Date**: 2025-12-19 | **Spec**: `specs/007-integration-gateway-and-mcp/spec.md`
**Input**: Feature specification from `/specs/007-integration-gateway-and-mcp/spec.md`

## Summary

在现有多插件能力治理的基础上，本阶段要把 CoreX 媒资、事件、定时任务、知识库、Workflow 等底座能力纳入 Capability Registry + Integration Gateway 的统一接口层：
1. 为每个 CoreX 能力输出公开契约（OpenAPI + gRPC），由 Integration Gateway 汇总并生成 SDK；
2. Registry 增加 `source=corex|plugin` 字段，Platform 能力默认带 Tool Grant 并支持租户级限流；
3. `/tenant/capabilities`、`/tenant/invocations` 成为宿主/ Skeleton 插件统一的调用入口，Admin API 仅保留配置职责；
4. 观测、限流、审计沿用 FR-001~FR-016 的规范，确保 fallback、事件广播、Trace 一致，并在 Web Admin「设置 > 开放能力」页面向 `IsRoot` 管理员实时展示平台开放能力（按模块统计能力数量、协议种类与调试入口），方便宿主与 Skeleton 插件直接对照文档调用。

补充：Agent 与多模态模型调用的对外能力也将纳入上述统一入口，新增公开契约并进入 `source=corex` 目录，保证插件能统一调用（非流式 + SSE/WS + gRPC）。

## Admin 开放能力页面设计（T057）

- **入口 & 权限**：Web Admin 侧边栏 “设置” 下新增 “开放能力” 菜单（建议路由 `/settings/open-capabilities`），仅当当前用户 `isRoot=true` 时渲染，同时接口层面也需校验。
- **数据源**：调用 Capability Registry 只读接口（复用 `/tenant/capabilities?source=corex` 或扩展 `/admin/capabilities/platform`）获取字段 `module`, `capability_id`, `title`, `protocols[*].channel`, `contracts`, `capabilities_hash`。
- **分组展示**：前端按 `module` 分组生成卡片，显示能力数量、最新版本、支持协议徽章；展开行列出单个能力，提供状态 Badge（active/disabled）、描述与 tags。
- **调试入口**：为每条能力提供 cURL/Insomnia snippet（自动填 `capability_id`, `preferred_protocol`, `X-PowerX-Tenant`），MCP Tool 名称、OpenAPI/gRPC 文档链接，Media 模块额外提供 `/media/assets` 调试跳转。
- **刷新策略**：进入页面自动加载，提供“立即同步”按钮重新请求；后续可监听 `capability.catalog.sync_*` 事件或轮询以保持实时。
- **安全 & 审计**：所有请求写入 Audit（记录 `admin_id`, `action=open_capability_view`），并遵守现有 RBAC/Feature Flag，确保非 Root 用户无法访问平台能力细节。

## Gateway Proxy 模式（FR-018）

为弥合“统一能力调度”与“获取真实业务结果”之间的落差，需要在 Integration Gateway 中新增 Proxy 模式：

1. **Selector/Invoker 扩展**：在完成能力选择后，由 Gateway 代理执行 REST/gRPC/MCP 调用，并将返回的 payload 直接写回 HTTP 响应体；无论底层协议为何，调用方都能拿到真实业务数据。
2. **统一请求描述**：`CapabilityInvokeRequest` 中新增标准化 payload 约束——REST 需包含 `method`、`endpoint`、`headers?`、`query?`、`body?`；gRPC 需包含 `endpoint`（Service）、`rpc`（方法）、`body`（JSON 映射）；MCP/workflow 同理，Selector 会洞察 Registry 协议信息补齐缺失字段。
3. **统一响应 Envelope**：在代理结果外侧附加 `trace_id`、`protocol_used`、`fallback_used`、`latency_ms` 等元信息，并将底层业务响应原样写入 `payload` 字段，形如：
   ```json
   {
     "code": 200,
     "message": "success",
     "data": {
       "payload": { ...真实业务响应... },
       "trace_id": "xxx",
       "protocol_used": "http",
       "fallback_used": false
     }
   }
   ```
   错误场景沿用相同 Envelope，仅 `payload` 替换为底层错误结构。
4. **调试体验**：文档与契约需明确 Proxy 模式的请求/响应范式，给出 REST（ListMediaAssets）与 gRPC（PublishEvent）示例，在 Quickstart/指南中强调“调用一次即可拿到业务数据 + Trace”。

本阶段将新增 Implementation 任务（T058-T060），覆盖 Handler/Selector 改造、观测补充与文档更新。

## Event Fabric Topic/ACL 自动化（新增）

插件在租户环境启用时，需要自动铺设 Event Fabric Topic 与 ACL，否则能力无法调用。为降低人工操作成本，需新增 manifest + 安装 Hook：

1. **Manifest**：在插件包或 `platform_capabilities` 配置中声明 Event Topic/ACL 需求（命名、namespace、principal 模板、幂等策略），CI/脚本可复用同一份 YAML。
2. **Installer Hook**：插件启用/禁用时，CoreX 自动解析 manifest 并调用 Event Fabric Service 创建/更新 Topic 和 ACL；支持多租户、多 principal、重试与审计。
3. **Resync 脚本**：root 管理员或 CI 可执行脚本（或 API）对所有租户/插件重新 apply manifest，支持 dry-run/diff。
4. **文档 & Quickstart**：在 Event Fabric 调试指南与 quickstart 中添加“自动化配置”章节，说明 manifest 结构、脚本用法。

对应新增任务 T061-T065。

## Agent & 多模态能力开放（新增）

为补齐“智能体与模型调用能力”的统一开放，需要新增契约、路由与租户隔离要求：

1. **契约输出**：HTTP OpenAPI 放入 `specs/007-integration-gateway-and-mcp/contracts/agent.http-openapi.yaml` 与 `ai-multimodal.http-openapi.yaml`；gRPC 契约放入 `backend/api/grpc/contracts/powerx/agent/v1/agent_api.proto` 与 `backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto`。
2. **Registry 能力登记**：新增 `com.corex.agent.*` 与 `com.corex.ai.*` 能力记录，标记 `source=corex`，并在 Admin “开放能力”页面展示。
3. **租户隔离**：Agent `agent_id/session_id` 与多模态 `model_key/session_id` 必须验证属于当前租户，跨租户请求拒绝并审计。
4. **流式支持**：Agent SSE/WS 与多模态 SSE 流式输出均需记录 Trace/Audit，并支持 Integration Gateway 代理（必要时仅透传响应）。

## Technical Context

**Language/Version**: Go 1.24（backend）、Node 20（脚本+CLI）  
**Primary Dependencies**: Gin、gRPC（Buf toolchain）、GORM、Redis、PostgreSQL、EventBus、OpenTelemetry  
**Storage**: PostgreSQL（CapabilityRecord/SyncJob/InvocationTrace）、Redis（SelectorPolicySnapshot、能力缓存）、MinIO/S3（媒资对象）  
**Testing**: `make contracts-test`、`make unit-test`、`go test ./backend/internal/...`；契约测试覆盖 HTTP+gRPC  
**Target Platform**: Linux container / Kubernetes，单体 CoreX 服务 + px-plugin CLI  
**Project Type**: Backend mono-repo（`backend` + `specs` + `powerx-plugin`）  
**Performance Goals**: Registry 更新 ≤3 分钟同步；Integration Gateway 读调用 p95 < 200ms，写调用全部 gRPC（幂等）  
**Constraints**: 多租户隔离、RBAC/Tool Grant 必须生效；所有协议携带 Trace/Audit；HTTP 与 gRPC 均需保持向后兼容  
**Scale/Scope**: 100+ 插件、每租户 1k+ 能力、每秒 200 次能力调用（读+写），Workflow/Agent 双栈共享治理

## Constitution Check

- **COREX_DECLARED / COREX_DOMAIN** ✅：Spec 已声明 `Domain Ownership: CoreX (corex.agent)`。  
- **HTTP_PRESENT** ✅：现有 Media/Integration Gateway HTTP 契约在 `specs/.../contracts/http-openapi.yaml`，计划中将继续扩充并在 plan/tasks 中跟踪。  
- **GRPC_PRESENT** ✅：`backend/api/grpc/contracts/powerx/integration_gateway/v1/*.proto` 为权威源，计划要求继续复用并扩展 CoreX 能力。  
- **PROTOBUF_DEFINED** ✅：`api/grpc/contracts/buf.yaml` 与 `buf.gen.yaml` 已存在，输出 `api/grpc/gen/go`。  
- **SERVER_DEFINED** ✅：gRPC 服务统一在 `backend/internal/server/grpc/server.go` 注册，Integration Gateway/CapabilityRegistry Handler 已挂载。  
- **MAKE_TARGETS** ✅：仓库已有 `make proto-gen|proto-lint|proto-clean`，计划将继续复用。  
- **COREX_MIGRATION_WIRING** ✅：所有 CoreX 模型在 `pkg/corex/db/database/migration.go` 注册，计划要求扩展 Media/Event 等能力时遵循相同挂载。

## Project Structure

### Documentation (this feature)

```
specs/007-integration-gateway-and-mcp/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
```

### Source Code（repository root）

```
backend/
├── internal/
│   ├── service/
│   │   ├── integration_gateway/
│   │   ├── media/
│   │   └── capability_registry/
│   ├── transport/
│   │   ├── http/admin/integration_gateway/
│   │   ├── http/admin/media/
│   │   ├── http/openapi/*
│   │   └── grpc/*
│   └── bootstrap/
├── api/grpc/
│   ├── contracts/powerx/*
│   └── gen/go/powerx/*
├── pkg/corex/db/persistence/{model,repository}/...
└── cmd/

specs/
├── 001-docs-media-storage/
├── 007-integration-gateway-and-mcp/
└── ...
```

**Structure Decision**: 继续沿用 CoreX 单体结构：合同放在 `specs/<feature>/contracts`（HTTP）与 `api/grpc/contracts/powerx/<domain>/v1`（gRPC），服务实现分别位于 `internal/transport/http/(admin|openapi)/<domain>` 与 `internal/transport/grpc/<domain>`，依赖注入统一通过 `internal/bootstrap/app.go` 与 `internal/server/grpc/server.go`。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _None_ | 当前计划完全遵守 CoreX Gates | 若尝试下放到插件会破坏单一事实来源与治理闭环 |
