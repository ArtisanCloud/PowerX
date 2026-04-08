# Implementation Plan: PowerX 部署与运维治理基线

**Branch**: `025-powerx-docker-systemd` | **Date**: 2026-03-24 | **Spec**: [/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/025-powerx-docker-systemd/spec.md](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/025-powerx-docker-systemd/spec.md)
**Input**: Feature specification from `/specs/025-powerx-docker-systemd/spec.md`

## Summary

围绕 PowerX 生产可用性建立统一运维治理基线：提供 Docker/systemd 双模式部署规范、插件无市场阶段平滑升级规范、Loki+Grafana 日志聚合、数据库备份恢复与清理流程、实例迁移 runbook，并定义 P0 运维管理控制台（部署发布、插件生命周期、备份恢复）的后端契约与前端交互基线。

## Technical Context

**Language/Version**: Go 1.24（backend services）、TypeScript/Nuxt 4（web-admin）  
**Primary Dependencies**: Gin HTTP、gRPC（Buf contracts）、GORM、PostgreSQL、Redis、Loki、Grafana、Promtail  
**Storage**: PostgreSQL（主数据）、Redis（缓存/队列）、MinIO/S3（备份与对象产物）  
**Testing**: Go test（unit/contract/integration）、Playwright（web-admin E2E）、备份恢复演练脚本校验  
**Target Platform**: Linux server（单节点首发，预留 K8s 多节点兼容）
**Project Type**: Web application（backend + web-admin + ops scripts）  
**Performance Goals**: 生产发布回滚 15 分钟内可完成；日志 3 分钟内可定位；备份成功率 >=99%  
**Constraints**: RTO 1 小时、RPO 15 分钟、Loki 默认保留 30 天、审批策略按环境可配置  
**Scale/Scope**: 覆盖 1 套生产环境首发 + P0 运维控制台 3 个域（deploy/plugin/backup）

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- `COREX_DECLARED`: PASS  
  本特性归属 CoreX 运维治理域（`corex.platform_ops`），非插件交付路径。
- `NO_PLUGIN_REGISTRY`: PASS  
  本方案不通过 `plugins/registry.json` 新增“运维功能插件”，仅复用现有插件生命周期接口。
- `COREX_LAYOUT_MATCH`: PASS  
  规划路径落在 `backend/internal/service/*`、`backend/internal/transport/http/admin/*`、`backend/internal/transport/grpc/*`。
- `COREX_DUAL_TRANSPORT`: PASS  
  管理能力同时落地 HTTP Admin API 与 gRPC；gRPC 合同权威源为 `backend/api/grpc/contracts/powerx/platform_ops/v1/ops_admin.proto`，`specs/.../contracts/ops-admin.proto` 仅作为设计草案镜像。
- `COREX_BUF_CONFIG`: PASS  
  在不破坏现有 `backend/api/grpc/contracts/buf.yaml` 与 `buf.gen.yaml` 机制前提下新增 `platform_ops` 契约并接入生成链路。
- `COREX_SERVER_WIRING`: PASS  
  规划遵循现有 `internal/bootstrap/app.go` 与 `internal/http/router.go` 装配方式。
- `COREX_MIGRATION_WIRING`: PASS  
  新数据实体规划遵循 `pkg/corex/db/database/migration.go` 统一迁移挂载约束。
- `HTTP_PRESENT` / `GRPC_PRESENT` / `PROTOBUF_DEFINED` / `SERVER_DEFINED` / `MAKE_TARGETS`: PASS（设计阶段）  
  本计划输出 OpenAPI 与 Proto 契约草案，后续 tasks 阶段将拆分到具体生成、挂载与 CI 校验任务。

**Post-Design Re-check**: PASS（Phase 1 产物已覆盖数据模型、契约、快速验证路径，且无宪章阻断项）

### Ruleset Alignment（@dev-crud-http）

- REST 设计 SoT: `specs/025-powerx-docker-systemd/contracts/http-openapi.yaml`
- HTTP 实现路径: `backend/internal/transport/http/admin/{deploy,backup,observability}`
- 路由统一挂载: `backend/internal/http/router.go` 与各域 `api.go`
- 统一回包约束: `pkg/dto` 标准响应函数

### Ruleset Alignment（@dev-crud-grpc）

- gRPC 合同 SoT: `backend/api/grpc/contracts/powerx/platform_ops/v1/ops_admin.proto`
- Buf 配置: `backend/api/grpc/contracts/{buf.yaml,buf.gen.yaml}`
- 生成产物: `backend/api/grpc/gen/go/powerx/platform_ops/v1/*`
- 服务挂载: `backend/internal/server/grpc/server.go`（统一拦截器链）
- Make 目标: `proto-gen` / `proto-lint` / `proto-clean`

## Project Structure

### Documentation (this feature)

```
specs/025-powerx-docker-systemd/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── ops-admin.proto
└── tasks.md

backend/api/grpc/contracts/powerx/platform_ops/v1/
└── ops_admin.proto
```

### Source Code (repository root)

```
backend/
├── internal/
│   ├── dto/
│   │   └── ops/
│   ├── service/
│   │   ├── deploy_ops/
│   │   ├── backup_ops/
│   │   ├── observability_ops/
│   │   └── migration_ops/
│   └── transport/
│       ├── http/admin/
│       │   ├── deploy/
│       │   ├── backup/
│       │   ├── observability/
│       │   └── migration/
│       └── grpc/ops/
├── pkg/corex/db/persistence/
│   ├── model/ops/
│   └── repository/ops/
└── scripts/ops/
    ├── backup-db.sh
    ├── cleanup-backups.sh
    └── restore-drill.sh

web-admin/
└── app/
    ├── pages/ops/
    │   ├── deploy.vue
    │   ├── plugins.vue
    │   ├── backup.vue
    │   └── migration.vue
    ├── composables/api/services/
    │   ├── deployOpsService.ts
    │   ├── backupOpsService.ts
    │   ├── pluginOpsService.ts
    │   └── migrationOpsService.ts
    └── components/ops/
        ├── deploy/
        ├── plugins/
        ├── backup/
        └── migration/
```

**Structure Decision**: 采用“现有 backend + web-admin 双子项目”结构，新增运维域子模块，不引入独立新工程。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 无 | N/A | N/A |
