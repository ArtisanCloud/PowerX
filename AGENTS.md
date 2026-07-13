# PowerX Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-11-05

## Active Technologies
- Go 1.24（backend 单体，Buf toolchain） + Gin HTTP 栈、google.golang.org/grpc、Buf、GORM、Redis、PostgreSQL、EventBus、OpenTelemetry、px-plugin CLI (007-integration-gateway-and-mcp)
- PostgreSQL（CapabilityRecord, CapabilitySyncJob, InvocationTrace）、Redis（Capability cache、ToolStore、RateLimit、SelectorPolicySnapshot）、MinIO/S3（插件 workflow/composite 资产引用，仅存 URI） (007-integration-gateway-and-mcp)
- Go 1.24（backend），Node 20 + Nuxt 4（web-admin） + Gin HTTP 栈、gorilla/websocket、Pinia、Nuxt UI (012-websocket-docs-plan)
- PostgreSQL（ai_model_profiles/knowledge_*），Redis（现有队列/缓存） (012-websocket-docs-plan)
- Go 1.24（backend services），Node 20 + Nuxt 4（web-admin） + Gin HTTP、google.golang.org/grpc（Buf）、GORM、Redis、PostgreSQL、OpenTelemetry、Nuxt UI、Pinia (024-ai-engineering-skills)
- PostgreSQL（skills registry + execution trace + audit refs）、Redis（selector/cache/policy snapshot） (024-ai-engineering-skills)
- Go 1.24（backend services）、TypeScript/Nuxt 4（web-admin） + Gin HTTP、gRPC（Buf contracts）、GORM、PostgreSQL、Redis、Loki、Grafana、Promtail (025-powerx-docker-systemd)
- PostgreSQL（主数据）、Redis（缓存/队列）、MinIO/S3（备份与对象产物） (025-powerx-docker-systemd)
- Go 1.24（backend），TypeScript（Nuxt 4 / Vue 3，web-admin） + Gin HTTP、gRPC（Buf contracts）、GORM、Pinia、Nuxt UI (026-iam)
- PostgreSQL（IAM 用户/成员/角色数据）、Redis（会话与缓存） (026-iam)
- Go 1.24（backend）, TypeScript + Nuxt 4（web-admin） + Gin HTTP, GORM, PostgreSQL, Redis, systemd/ops scripts, Nuxt UI (027-monitor-center)
- PostgreSQL（策略/作业/演练/告警元数据）+ 文件或对象存储（备份产物） (027-monitor-center)
- Go 1.24 for backend/CoreX services and CLIs; TypeScript with Nuxt 4/Vue 3 for Web Admin; Buf toolchain for gRPC contracts. + Gin HTTP stack, GORM, PostgreSQL JSONB, Redis only if later used for read caches, google.golang.org/grpc, Buf, Pinia/Nuxt UI, existing PowerX capability registry and IAM/RBAC services. (029-metadata-governance)
- PostgreSQL tables for metadata definitions, tag bindings, references, audit events through existing audit infrastructure; seed YAML under `backend/config/metadata_governance`. (029-metadata-governance)

- Go 1.24（backend），Node 20（Web Admin 热更新面板），Go 1.21（px-plugin CLI） + Gin HTTP 栈、google.golang.org/grpc、Buf toolchain、GORM + PostgreSQL、Redis（队列与 Feature Flag）、MinIO/S3 SDK（离线包存储）、OpenTelemetry + Prometheus Exporter、PowerX CLI (`powerx`, `px-plugin`) (001-install-plugin-pxp)
- Go 1.24 (backend services, CLIs), Node 20 (validation scripts), Go 1.21 (px-plugin CLI) + Gin HTTP stack, google.golang.org/grpc, Buf toolchain, GORM + PostgreSQL, Redis, MinIO/S3 SDK, OpenTelemetry + Prometheus exporters (010-agent-model-setting)
- PostgreSQL (provider profiles, routing policies, quota config), Redis (health scores, safe-mode, feature flags), MinIO/S3 (validator artifacts), Vault-backed secret store (010-agent-model-setting)
- Go 1.24 (backend services, CLIs) + Gin HTTP stack, google.golang.org/grpc (Buf toolchain), GORM, Redis, PostgreSQL, MinIO/S3 SDK, OpenTelemetry, PowerX CLI (011-docs-use-cases)
- PostgreSQL (knowledge space metadata, quota, policy versions), Redis (workflow queues, throttles), MinIO/S3 (artifact staging) (011-docs-use-cases)
- Go 1.24 (backend services, CLIs); Node 20 + Nuxt 4 (Vue 3 Web Admin) + Gin HTTP stack, google.golang.org/grpc (Buf toolchain), GORM, Redis, PostgreSQL, MinIO/S3 SDK, OpenTelemetry, PowerX CLI, Nuxt 4, Vue 3, Pinia, Nuxt UI, VueUse, Playwright, Vites (011-docs-use-cases)

## Project Structure

```
src/
tests/
```

## Commands

# Add commands for Go 1.24（backend），Node 20（Web Admin 热更新面板），Go 1.21（px-plugin CLI）

## QA & Observability Checklist

- 运行 `scripts/capability_registry/verify.sh`（需要 `POWERX_BASE_URL/ADMIN_TOKEN/TENANT_TOKEN` 等环境变量），串联 capability-sync → Admin/Tenant API 巡检 → `/tenant/invocations` 调用，输出 trace_id 供后续链路排查。
- `cd backend && go test ./tests/integration/capability_registry/load`：覆盖 5k+ Selector 调用、Redis 缓存击穿保护与 fallback chaos 测试，上线前必须通过。
- Prometheus/Otel：启动 backend（`LOG_LEVEL=info make dev` 或目标环境），在脚本执行期间 `curl http://localhost:2112/metrics | grep powerx_capability_invoke_total`，确认 `powerx_capability_invoke_total`/`powerx_capability_invoke_error_total` 呈现递增；同时设置 `OTEL_EXPORTER_OTLP_ENDPOINT`（或接入现有 collector）确认 Trace 透传。
- 事件补偿与日志：关注 `integration.gateway.invocation.failed`/`capability.catalog.sync_*` 事件，`LOG_LEVEL=debug` 跑一次 `scripts/capability_registry/verify.sh`，确认 stdout/采集日志都包含 `capability_id/plugin_id/protocol` 字段，异常重试应在日志与事件中一致可见。

## Code Style

Go 1.24（backend），Node 20（Web Admin 热更新面板），Go 1.21（px-plugin CLI）: Follow standard conventions

## Recent Changes
- 029-metadata-governance: Added Go 1.24 for backend/CoreX services and CLIs; TypeScript with Nuxt 4/Vue 3 for Web Admin; Buf toolchain for gRPC contracts. + Gin HTTP stack, GORM, PostgreSQL JSONB, Redis only if later used for read caches, google.golang.org/grpc, Buf, Pinia/Nuxt UI, existing PowerX capability registry and IAM/RBAC services.
- 027-monitor-center: Added Go 1.24（backend）, TypeScript + Nuxt 4（web-admin） + Gin HTTP, GORM, PostgreSQL, Redis, systemd/ops scripts, Nuxt UI
- 026-iam: Added Go 1.24（backend），TypeScript（Nuxt 4 / Vue 3，web-admin） + Gin HTTP、gRPC（Buf contracts）、GORM、Pinia、Nuxt UI


<!-- MANUAL ADDITIONS START -->
Always respond in Chinese-simplified
Do not preserve compatibility with legacy, incorrect, or deprecated specifications/code paths; prefer explicit failure and strict enforcement over fallback behavior.
Without explicit user request, do not implement fallback, graceful degradation, or backward-compatibility branches; implement exact behavior with fail-fast errors when required inputs/context are missing.
When introducing or correcting a policy, protocol, API contract, field semantics, routing mode, migration behavior, or runtime convention, treat the new rule as authoritative immediately. Do not keep old request formats, deprecated field names, legacy route aliases, implicit tenant/source inference, or compatibility shims unless the user explicitly asks for backward compatibility.
If existing data or callers use a deprecated or wrong format, surface the mismatch with a clear error, log field, migration note, or manual cleanup instruction instead of silently accepting or translating it.
Do not add generic fallback behavior unless the user explicitly requests it. This applies across frontend runtime behavior, backend services, API contracts, channel/transport selection, authentication, authorization, data parsing, build/runtime configuration, and developer tooling.
If a frontend workflow is designed as WebSocket/SSE real-time delivery, do not secretly add polling as a fallback. Show a clear disconnected/error state, log the cause, and provide an explicit recovery action such as reconnect/retry.
Do not parse structured fields from free-form text as a fallback. Required structured values must come from the declared contract, schema, typed field, or explicit user input; otherwise fail fast with a clear validation error.
Do not silently downgrade channels, transports, auth modes, data contracts, schema versions, or runtime/build modes. A mismatch must produce an explicit error, observable log, and actionable remediation path.
Prefer explicit failure, visible error states, structured logs, and user/operator recovery actions over hidden compatibility branches or "helpful" bypasses.
All human-readable text must go through i18n/locale resources instead of being hard-coded in application code, contracts, templates, or tests. This includes frontend buttons, labels, placeholders, toasts, validation messages, error descriptions, backend user-facing responses, email templates, Agent-visible/user-visible prompts and replies, business role display names, role aliases, blocklists/deny-word lists, and tests that assert user-visible copy.
Machine-semantic identifiers may remain in code when they are not intended as user-facing text. Examples: protocol constants, enum values, JSON field names, route paths, log keys, metric names, status codes, capability IDs/names, database table/column names, i18n keys, permission codes, and other stable wire/storage identifiers.
When adding or changing user-visible text, update the relevant locale files and reference keys from code or contracts. Do not introduce new hard-coded copy as a temporary shortcut.
UI must not show object UUIDs or raw technical identifiers as the primary human-facing label unless the user explicitly requests it or the view is clearly a debug/trace surface. Default labels in tables, cards, detail headers, selectors, dropdown options, breadcrumbs, confirmations, and chat/business replies must use the object's display name, name, title, alias, or other localized human-readable label.
For tenants, users, agents, skills, templates, providers, roles, departments, integrations, and similar business objects, show the object name as the visible label and keep UUIDs only as hidden values, route params, payload fields, tooltip/debug metadata, or secondary copy in explicit technical diagnostics. For example, selector options should show the tenant name rather than `tenant_uuid`.
When a UUID is required for disambiguation, prefer showing it as secondary muted metadata, shortened and labeled via i18n, and only in admin/debug contexts. Do not use UUIDs as the default visible option text or user-facing object identity.
All business object tables MUST include a stable UUID column for external identity and cross-domain references. Numeric auto-increment IDs may exist only as internal storage implementation details and MUST NOT be used as API identifiers, route parameters, event payload references, audit subject identifiers, or cross-table business references.
Business object relationships MUST use the referenced object's UUID as the mapping key. Foreign-key fields in domain tables, join tables, policy tables, audit records, trace records, event payloads, and API DTOs MUST be named with the object semantic plus `_uuid` (for example `tenant_uuid`, `agent_uuid`, `user_uuid`, `role_uuid`, `capability_uuid`) and MUST reference the target business object's UUID, not its numeric ID.
Join tables may omit their own UUID when they represent only a pure association, but their association columns MUST still use the related objects' UUIDs. If a join record becomes addressable, auditable as an independent object, stateful, or referenced by other records, it MUST also have its own UUID.
New migrations, GORM models, protobuf contracts, OpenAPI schemas, frontend API types, and tests MUST use UUID-based identity consistently for tenants, users, agents, skills, templates, providers, roles, departments, integrations, capabilities, plugins, workflows, media assets, knowledge spaces, and similar business objects. Do not introduce new `*_id` fields for business-object references unless the field is explicitly a non-public internal surrogate key and is not exposed outside the table implementation.
When correcting an existing table or contract that uses numeric IDs for business-object references, make the UUID rule authoritative immediately: add explicit migration/backfill notes and fail fast on missing UUIDs instead of silently accepting legacy numeric identifiers or translating them as a compatibility path.

UUID 规范：
- 业务对象表必须有稳定 uuid。
- 跨表、跨服务、API、事件、审计引用统一使用对象 uuid。
- 中间表可以没有自己的 uuid，但关联字段必须用两端对象的 uuid。
- 如果中间表变成可审计/可引用/有状态对象，也必须有自己的 uuid。
- 新增迁移、GORM、Proto、OpenAPI、前端类型、测试都按 UUID 规则执行。
- 修正旧 numeric id 引用时不做兼容兜底，缺 UUID 明确失败并给迁移说明。

STS 插件访问规范：
- Capability 是业务授权单元，不是 URL。REST/OpenAPI/Admin/gRPC endpoint 只是同一个 capability 的 protocol binding；同一业务语义、同一授权边界的多个入口 MUST 复用同一个 `capability_id`。
- 如果 `/api/v1/admin/<resource>` 与 `/api/v1/<resource>` 表达同一业务能力，只是用户态后台入口和服务态开放入口的差异，应登记在同一个 capability 的不同 REST bindings 下；不得因为路径不同生成两个可授权能力。
- 只有当可操作资源范围、actor 约束、风险等级或授权开关必须独立时，才拆成不同 capability，例如 admin 全量治理能力和插件 owner-scoped 自助能力。
- API 路径必须先声明调用主体和资源边界：`/api/v1/admin/*` 面向后台用户态（PowerX Admin、插件 Admin 页面，用户 JWT + member + RBAC）；`/api/v1/*` 面向外部业务或服务态开放接口（web、mini-app、customer、第三方、插件服务，按 user/customer/API Key/STS/OAuth 等凭证约束）；`/api/v1/tenant/invocations` 面向服务态 capability 调度（插件后端、agent、skill）。
- web、mini-app、customer 这类外部业务入口不得复用 admin 全量治理语义。即使底层资源相同，也必须按 actor、资源范围和风险拆分或标注 capability，例如 `*.admin_manage`、`*.service_manage`、`*.self_read`、`*.self_update`。
- customer/mini-app 自助接口默认只能 owner-scoped/self-scoped，不得通过路径或 capability 继承后台管理员的全租户管理能力。
- 插件通过 STS token 直接调用 PowerX Core HTTP 接口时，允许集合由正式 `backend/config/platform_capabilities/*.yaml` 中的 REST protocol 自动派生，再扣除 STS blocklist；不得再为普通开放能力手工维护分散白名单。
- STS direct route policy 的来源是：少量静态插件运行时合同入口（例如 `/tenant/invocations`、`/tenant/invocations/stream`、runtime ws-bus/task-queue、租户查询等）+ 正式 platform capability REST endpoints - blocklist。
- STS direct 自动开放必须按 HTTP method 精确匹配。新增 `GET` 查询能力不得隐式放开 `POST`、`PUT`、`PATCH`、`DELETE` 等写操作。
- `/api/v1/admin/*` 是后台用户态 API 命名空间，不等于“禁止插件后台页面使用”。浏览器中的 PowerX Admin、插件 Admin 页面、以及任何携带用户 JWT 的后台请求，仍然按用户鉴权、租户成员、RBAC 和业务权限判断；STS route policy 不得影响用户态 JWT。
- STS token 是插件服务态身份，不携带 `uid/mid`，不能代表登录用户通过 `/api/v1/admin/*` 绕过用户 RBAC。插件后端如果要代表当前用户调用底座后台 API，必须引入明确的 delegated/on-behalf-of 机制，不能复用普通 `powerx:api` STS token。
- 默认 blocklist 只约束插件服务态 STS direct call，必须拦截 `/admin/*`、`/internal/*`、`/public/*`、`/auth/*`、`/setup/*`、debug、migration、root、drain、bootstrap、mock、health、根级动态路径等非服务态开放入口。`/admin/*` 若确认为插件服务运行时合同，必须显式加入静态 allow、补充用途说明和 validator 测试。
- 新增插件可调用的 Core API 时，先实现真实 transport/service/permission/test，再登记到正式 platform capability REST protocol；通过 `make capability-check` 与 STS validator 测试验证自动开放结果。不得只改鉴权 validator 绕过 capability 登记。
<!-- MANUAL ADDITIONS END -->
