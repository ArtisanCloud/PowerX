# PowerX Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-11-05

## Active Technologies

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

## Code Style

Go 1.24（backend），Node 20（Web Admin 热更新面板），Go 1.21（px-plugin CLI）: Follow standard conventions

## Recent Changes

- 011-docs-use-cases: Added Go 1.24 (backend services, CLIs); Node 20 + Nuxt 4 (Vue 3 Web Admin) + Gin HTTP stack, google.golang.org/grpc (Buf toolchain), GORM, Redis, PostgreSQL, MinIO/S3 SDK, OpenTelemetry, PowerX CLI, Nuxt 4, Vue 3, Pinia, Nuxt UI, VueUse, Playwright, Vites
- 011-docs-use-cases: Added Go 1.24 (backend services, CLIs) + Gin HTTP stack, google.golang.org/grpc (Buf toolchain), GORM, Redis, PostgreSQL, MinIO/S3 SDK, OpenTelemetry, PowerX CLI
- 010-agent-model-setting: Added Go 1.24 (backend services, CLIs), Node 20 (validation scripts), Go 1.21 (px-plugin CLI) + Gin HTTP stack, google.golang.org/grpc, Buf toolchain, GORM + PostgreSQL, Redis, MinIO/S3 SDK, OpenTelemetry + Prometheus exporters

<!-- MANUAL ADDITIONS START -->
Always respond in Chinese-simplified
<!-- MANUAL ADDITIONS END -->
