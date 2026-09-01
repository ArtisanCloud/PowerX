# Implementation Plan: IAM 用户与角色 RBAC 统一能力

**Branch**: `026-iam` | **Date**: 2026-04-06 | **Spec**: [/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/026-iam/spec.md](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/026-iam/spec.md)
**Input**: Feature specification from `/specs/026-iam/spec.md`

## Summary

聚焦 IAM 管理域收敛三类能力：
1. root / tenant admin / member 的权限边界一致化；
2. `/settings/users` 页面交互语义拆分（查看详情、切换租户、跳转行为去耦）；
3. `me/context` 驱动的前后端状态强一致（以服务端上下文为准）；
4. SaaS 自助注册 tenant，并把首个成员初始化为 owner/admin/member；
5. root 默认进入 Platform Console，通过 Support Session 才能进入业务租户上下文；
6. 插件拆分为全局 Plugin Package 与 Tenant Plugin Instance，保证租户启用/停用隔离；
7. 历史 IAM 数据通过只读巡检和可审计迁移补齐，不手动破坏生产组织数据；
8. SaaS 租户注册准入升级为权威策略对象，支持关闭、开放、邀请制、候补、审核、白名单和灰度放量。
9. 角色权限中心消费插件通过 Capability Sync 登记的 `menu/page/action` 细颗粒度权限，按插件与业务模块统一授权；插件设置页不作为正式角色授权入口。

## Technical Context

**Language/Version**: Go 1.26.7（backend），TypeScript（Nuxt 4 / Vue 3，web-admin）
**Primary Dependencies**: Gin HTTP、gRPC（Buf contracts）、GORM、Pinia、Nuxt UI  
**Storage**: PostgreSQL（IAM 用户/成员/角色数据）、Redis（会话与缓存）  
**Testing**: Go test（service/http）、前端 Vitest + 手工回归脚本（setup/status + users 页面）  
**Target Platform**: Linux server + 现代浏览器（Chrome/Safari/Edge）
**Project Type**: CoreX backend + web-admin 双子项目  
**Performance Goals**: 用户管理页进入后 3 秒内完成角色视图分流；上下文切换后 5 秒内收敛到正确视图  
**Constraints**: 多租户隔离不可破坏；root 特权仅限平台运维语义；禁止隐式跨租户写操作  
**Scale/Scope**: 覆盖现有 IAM 管理页、身份上下文链路、SaaS signup、注册准入策略、root support、租户插件实例隔离和历史数据巡检迁移

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- `COREX_DECLARED`: PASS  
  本特性归属 CoreX IAM/RBAC 域（`corex.iam` + `corex.rbac`），不是插件功能。
- `NO_PLUGIN_REGISTRY`: PASS  
  不新增 `plugins/registry.json`，仅在现有 CoreX 模块路径内演进。
- `COREX_LAYOUT_MATCH`: PASS  
  目标路径限定在 `backend/internal/service/iam`、`backend/internal/transport/http/admin/{iam,user/auth}`、`web-admin/app/{pages,components,stores,composables}`。
- `COREX_DUAL_TRANSPORT`: PASS（设计阶段）  
  HTTP 管理契约与 gRPC IAM 管理契约均给出设计草案。
- `COREX_BUF_CONFIG`: PASS（设计阶段）  
  gRPC 设计约束遵循 `backend/api/grpc/contracts/{buf.yaml,buf.gen.yaml}` 与 `api/grpc/gen/go` 输出规范。
- `COREX_SERVER_WIRING`: PASS  
  服务装配维持 `internal/bootstrap/app.go`、`internal/http/router.go`、全局 gRPC server 机制。
- `COREX_MIGRATION_WIRING`: PASS  
  本特性以权限判定与交互语义为主，不引入新的迁移分叉入口；如需新增字段，仍挂载到 `pkg/corex/db/database/migration.go`。
- `HTTP_PRESENT` / `GRPC_PRESENT` / `PROTOBUF_DEFINED` / `SERVER_DEFINED` / `MAKE_TARGETS`: PASS（设计阶段）

**Post-Design Re-check**: PASS（Phase 1 产物已覆盖 data model / contracts / quickstart，无宪章阻断项）

## Ruleset Alignment（@dev-crud-http）

- HTTP SoT：`specs/026-iam/contracts/http-openapi.yaml`
- 管理端实现路径：
  - `backend/internal/transport/http/admin/user/auth/*`
  - `backend/internal/transport/http/admin/iam/*`
- 管理端页面与状态路径：
  - `web-admin/app/pages/settings/users/index.vue`
  - `web-admin/app/components/settings/users/*`
  - `web-admin/app/stores/user.ts`
- SaaS 自助注册与租户 bootstrap：
  - `backend/internal/transport/http/public/saas/*`
  - `backend/internal/service/tenant/tenant_service.go`
  - `backend/internal/service/auth/auth_service.go`
- 租户注册准入与灰度：
  - `backend/internal/service/auth/registration_policy_service.go`
  - `backend/internal/service/auth/registration_invite_service.go`
  - `backend/internal/service/auth/registration_request_service.go`
  - `backend/internal/transport/http/public/saas/registration_policy_handler.go`
  - `backend/internal/transport/http/admin/iam/registration_policy_handler.go`
  - `backend/pkg/corex/db/persistence/model/iam/registration_*_gorm.go`
- Root 平台控制台与支持会话：
  - `backend/internal/transport/http/admin/root/*`
  - `backend/internal/service/iam/*support*`
- 插件租户实例隔离：
  - `backend/internal/transport/http/admin/plugin/tenant_handler.go`
  - `backend/internal/transport/http/admin/plugin/menus_agg.go`
  - `backend/internal/infra/plugin/manager/router/router.go`
- 插件细颗粒度权限授权视图：
  - `backend/internal/service/iam/rbac_service.go`
  - `backend/internal/transport/http/admin/iam/*permission*`
  - `web-admin/app/pages/settings/users/index.vue`
  - `web-admin/app/components/settings/users/*`
  - `web-admin/app/stores/permission.ts`
  - 权限声明与同步主流程归属 `specs/007-integration-gateway-and-mcp`
- 回包规范：`pkg/dto` 统一响应函数

## Ruleset Alignment（@dev-crud-grpc）

- gRPC SoT（设计草案）：`specs/026-iam/contracts/iam-rbac-admin.proto`
- 权威合同落地目标：`backend/api/grpc/contracts/powerx/iam/v1/*.proto`
- gRPC 实现路径：`backend/internal/transport/grpc/iam/*`
- 统一拦截器链：`auth`、`tenant`、`logging`、`recovery`
- Make 目标：`proto-gen` / `proto-lint` / `proto-clean`

## Project Structure

### Documentation (this feature)

```
specs/026-iam/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── iam-rbac-admin.proto
└── tasks.md
```

### Source Code (repository root)

```
backend/
├── internal/
│   ├── service/
│   │   ├── auth/
│   │   ├── tenant/
│   │   └── iam/
│   └── transport/
│       ├── http/admin/
│       │   ├── user/auth/
│       │   ├── iam/
│       │   ├── root/
│       │   └── plugin/
│       ├── http/public/
│       │   └── saas/
│       └── grpc/iam/
├── api/grpc/contracts/powerx/iam/v1/
└── pkg/corex/db/persistence/model/iam/

web-admin/
└── app/
    ├── pages/settings/users/index.vue
    ├── components/settings/users/
    ├── stores/user.ts
    └── composables/api/services/
        ├── authService.ts
        ├── userService.ts
        ├── tenantService.ts
        └── registrationPolicyService.ts
```

**Structure Decision**: 保持现有 CoreX 单体后端 + Nuxt 管理端结构，不新增项目；以现有 IAM/Auth 模块增量修复角色语义与上下文一致性。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | N/A |
