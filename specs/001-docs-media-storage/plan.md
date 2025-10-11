```markdown
# Implementation Plan: Media Asset Admin Capabilities (CoreX Module)

**Branch**: `001-docs-media-storage`  
**Date**: 2025-10-10  
**Spec**: `specs/001-docs-media-storage/spec.md`  
**Input**: Feature specification from `specs/001-docs-media-storage/spec.md`

> 本计划为 **CoreX 内核模块** 的实现方案（非插件）。所有路径均为**相对仓库根目录**，不再使用绝对路径。

---

## Execution Flow (/plan command scope)

```

1. Load feature spec from Input path
   → If not found: ERROR "No feature spec at {path}"
2. Fill Technical Context (scan for NEEDS CLARIFICATION)
   → Detect Project Type from repo structure
   → Set Structure Decision based on CoreX module (NOT plugin)
3. Fill the Constitution Check section from constitution.md
4. Evaluate Constitution Check section
   → If violations exist: record in Complexity Tracking
   → If no justification possible: ERROR "Simplify approach first"
   → Update Progress Tracking: Initial Constitution Check
5. Execute Phase 0 → research.md
   → If NEEDS CLARIFICATION remain: ERROR "Resolve unknowns"
6. Execute Phase 1 → contracts, data-model.md, quickstart.md, agent-specific file
7. Re-evaluate Constitution Check
   → If new violations: Refactor design, return to Phase 1
   → Update Progress Tracking: Post-Design Constitution Check
8. STOP - Ready for /tasks command

```

---

## Summary

为后台运营人员提供统一的媒体资产管理能力，支持上传、筛选、详情、业务属性变更、软删除与 **12 小时**预签名链接。  
**实现位置**：作为 **PowerX CoreX 内核模块** 落地，代码直接进入主工程（非 plugins 目录），沿用 CoreX 的多租户、RBAC、审计与迁移框架。

---

## Technical Context

**Language/Version**: Go 1.x（与主工程一致）  
**Primary Dependencies**: Gin、GORM、MinIO/S3 SDK、Buf、OpenTelemetry、JWT 中间件  
**Storage**: PostgreSQL（元数据）、S3 兼容对象存储（媒资）、Redis（可选：缓存预签名/上传票据）  
**Testing**: `testing` + `testify`、HTTP 契约测试（httpexpect）、gRPC e2e（bufconn）  
**Target Platform**: Linux (x86_64) 容器 & 裸机  
**Project Type**: **CoreX 内核模块**（主进程内的 HTTP + gRPC）  
**Performance Goals**: 管理 API p95 < 200ms；预签名生成 < 50ms；后台上传 QPS ≥ 50  
**Constraints**: 多租户隔离、RBAC 鉴权、审计追踪、软删除 + 定时清理、对象存储最小权限

---

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **HTTP_PRESENT ✅**：内部管理端 REST 契约放置于 `specs/001-docs-media-storage/contracts/http-admin.yaml`  
  （**行动**：将现有 `http-openapi.yaml` 重命名为 `http-admin.yaml`，仅保留内部接口，`servers.url` 使用 `/api/v1` 或 `/admin/...`）
- **GRPC_PRESENT ✅**：gRPC 契约位于 `specs/001-docs-media-storage/contracts/grpc-media-asset.proto`（服务名示例：`media.v1.MediaAssetAdminService`）
- **PROTOBUF_DEFINED ✅**：`buf.yaml`/`buf.gen.yaml` 在主工程下维护，`go_package_prefix` 指向 `api/grpc/gen`
- **SERVER_DEFINED ✅**：HTTP/GRPC 入口在 **主工程**（非插件）下：  
  - HTTP：`internal/transport/http/admin/media/`  
  - gRPC：`internal/transport/grpc/media/`
- **MAKE_TARGETS ✅**：Makefile 增加 `proto-gen`、`proto-lint`、`proto-clean`、`contracts-test`，纳入主仓 CI  
- 审计 / 多租户 / 观测性纳入服务设计，符合宪章核心条款

---

## Project Structure

### Documentation (this feature)

```

specs/001-docs-media-storage/
├── plan.md              # 本文件（/plan 输出）
├── research.md          # Phase 0（/plan 输出）
├── data-model.md        # Phase 1（/plan 输出）
├── quickstart.md        # Phase 1（/plan 输出）
└── contracts/           # Phase 1（/plan 输出）
├── http-admin.yaml                # 由 http-openapi.yaml 重命名/修订
├── grpc-media-asset.proto
└── tests/

```

### Source Code (CoreX module in repo root)

```

# 数据模型与仓储

pkg/corex/db/persistence/model/media/
└── asset.go

pkg/corex/db/persistence/repository/media/
└── asset_repo.go

# 领域与服务

pkg/corex/db/persistence/model/media/
gorm实体即使领域对戏那个

internal/service/media/
└── service.go

# 传输层（主工程内）

internal/transport/http/admin/media/
├── handler.go
├── dto.go
└── router.go

internal/server/grpc/
└── server.go # 全局唯一 gRPC Server（new + 拦截器 + register）

internal/transport/grpc/media/
└── media_handler.go # 模块实现（仅实现接口，禁止 new/register）

internal/transport/grpc/auth/middleware/
└── auth_interceptor.go # 既有拦截器目录（示例）

# 迁移 & CLI

- pkg/corex/db/migration.go  # func MigrateCoreModels(db *gorm.DB) error（模块自迁移，AutoMigrate/Migrator，无 .sql）
- cmd/database/migrate.go                       # func MigrateDatabase/ResetDatabase 编排调用（全局入口，含生产环境保护）（不交付任何 .sql 迁移文件）
- cmd/media_tool/main.go                        # 媒资工具集入口（含清理子命令）

# 协议生成

api/grpc/contracts/powerx/media/v1/media_asset.proto
Makefile                              # proto-gen / contracts-test / etc.

```

**Structure Decision**：媒体资产作为 **CoreX 内核能力** 常驻主进程；统一复用主工程的鉴权、租户、审计、迁移与观测组件；不创建插件工程，不在 `plugins/` 目录下放置任何实现代码。

---

## Phase 0: Outline & Research

- 汇总未知项（对象存储驱动抽象、预签名 TTL、租户头部与审计策略、状态机约束）  
- 在 `research.md` 归档 **Decision / Rationale / Alternatives**  
- 结论：采用 PostgreSQL + 软删除 + JSONB 元数据；MediaManager 统一驱动；预签名默认 12h，可配置

**Output**: `specs/001-docs-media-storage/research.md`

---

## Phase 1: Design & Contracts

1) **数据模型**（`data-model.md`）  
- 实体：`MediaAsset`（含 `TenantID`、`Driver`、`StorageKey`、`OwnerType/OwnerID`、`BusinessStatus`、`Tags/Meta`、审计与软删）  
- 索引：`(tenant_id, driver, storage_key)` 唯一；`(tenant_id, business_status)`；`(tenant_id, owner_type, owner_id)`  

2) **HTTP 契约**（`contracts/http-admin.yaml`）  
- 路由前缀：`/admin/media/assets`（内部管理）  
- 鉴权：`bearerAuth: JWT`（内部管理端）  
- 端点：创建、列表、详情、更新、软删、预签名
- 移除任何对外字段/鉴权描述
  - 统一上传流程：预签名返回 `method/url/headers`，本地驱动通过 `GET/PUT /media/*objectKey` 提供下载与直传能力（开发环境启用、HMAC Token 防伪，`public_base_url` 与该路由保持一致）

3) **gRPC 契约**（`contracts/grpc-media-asset.proto`）  
- Service：`media.v1.MediaAssetAdminService`  
- Method：Create/List/Get/Update/Delete/Presign  
- 与 HTTP 字段语义对齐

4) **契约测试草稿**（`contracts/tests/`）  
- HTTP：请求/响应 schema 断言（httpexpect）  
- gRPC：proto 兼容性与 e2e（bufconn）

5) **Quickstart**（`quickstart.md`）  
- 本地启动、迁移、样例上传/预签名/筛选验证步骤

**Output**:  
- `specs/001-docs-media-storage/data-model.md`  
- `specs/001-docs-media-storage/contracts/`  
- `specs/001-docs-media-storage/quickstart.md`

---

## Phase 2: Task Planning Approach  *(DO NOT execute here)*

**Task Generation Strategy**  
- 基于 Phase 1 文档生成 `tasks.md`：  
  - 每个端点 → 契约测试任务 [P]  
  - 每个实体 → 模型与迁移任务 [P]  
  - 每个用户故事 → 集成测试任务  
  - 使测试逐步通过的实现任务

**Ordering Strategy**  
- TDD 顺序：先测试后实现  
- 依赖顺序：模型 → 仓储 → 服务 → 传输层  
- 标记 [P] 以并行化（互不依赖）

**Expected**: 25~30 个有序任务（`/tasks` 生成）

---

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *None* | - | - |

---

## Progress Tracking

**Phase Status**  
- [x] Phase 0: Research complete (/plan)  
- [x] Phase 1: Design complete (/plan)  
- [ ] Phase 2: Task planning complete (/plan - describe approach only)  
- [ ] Phase 3: Tasks generated (/tasks)  
- [ ] Phase 4: Implementation complete  
- [ ] Phase 5: Validation passed

**Gate Status**  
- [x] Initial Constitution Check: PASS  
- [x] Post-Design Constitution Check: PASS  
- [x] All NEEDS CLARIFICATION resolved  
- [x] Complexity deviations documented
```
