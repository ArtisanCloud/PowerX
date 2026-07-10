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
- 027-monitor-center: Added Go 1.24（backend）, TypeScript + Nuxt 4（web-admin） + Gin HTTP, GORM, PostgreSQL, Redis, systemd/ops scripts, Nuxt UI
- 026-iam: Added Go 1.24（backend），TypeScript（Nuxt 4 / Vue 3，web-admin） + Gin HTTP、gRPC（Buf contracts）、GORM、Pinia、Nuxt UI
- 025-powerx-docker-systemd: Added Go 1.24（backend services）、TypeScript/Nuxt 4（web-admin） + Gin HTTP、gRPC（Buf contracts）、GORM、PostgreSQL、Redis、Loki、Grafana、Promtail


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
- 任何提供给插件通过 STS token 调用的 PowerX Core HTTP 接口，必须在 STS route policy 中显式登记 method、path pattern、匹配模式和用途边界；只注册 Gin 路由不代表插件可访问。
- STS route policy 必须按最小权限授权到 HTTP method，不允许只按路径放开管理接口。新增 `GET` 查询能力不得隐式放开 `POST`、`PUT`、`PATCH`、`DELETE` 等写操作。
- 新增或调整插件可调用的 Core API 时，必须同步更新 STS validator 测试，至少覆盖允许的 method/path 和相同 path 下不允许的 method。
- 不允许为方便插件调用而开放 `/admin/*`、`/tenant/*` 或能力前缀的泛化通配策略；需要逐条登记可访问接口。
<!-- MANUAL ADDITIONS END -->
