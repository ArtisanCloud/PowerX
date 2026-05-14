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
<!-- MANUAL ADDITIONS END -->
