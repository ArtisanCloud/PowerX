# Implementation Plan (PowerX-Aligned): Media Asset Admin Capabilities

**Branch**: `001-docs-media-storage`  
**Date**: 2025-10-08  
**Spec**: `/specs/001-docs-media-storage/spec.md`

> 本计划文件已根据你的反馈**纠正并对齐 PowerX 实际项目**：
> 1) **Project Type** 明确为 *CoreX Modular Monolith (Backend Service)*，不是微服务；  
> 2) **数据库表述**统一为 **GORM 抽象 + 默认 Postgres**（不写死 MySQL）；  
> 3) **去除 Agent/占位可执行** 的残留（不再出现 `cmd/agent` / `update-agent-context.sh`）。

---

## Summary

在 PowerX 后台（CoreX 单体）中实现 **媒体资产管理能力**：上传（直传/外链）、分页检索、详情、业务字段更新、软删 + 定时物理清理、预签名 URL（默认 12h）。遵循既有分层：`transport/http → service → infra → persistence(model/repository)`，新增 `MediaManager` 多驱动组件，复用多租户与 RBAC、审计与可观测性能力。

---

## Technical Context

- **Language/Version**: Go 1.22+（与现有工程一致；如 CI 固定更高版本需统一调整）  
- **Frameworks**: Gin（HTTP） + **GORM**（ORM 抽象）  
- **RDBMS**: **Postgres（默认）**，保持 GORM 兼容（MySQL/SQLite 可切换）  
- **Object Storage**: S3 兼容（MinIO/OSS/S3）+ Local FS（开发环境）  
- **AuthN/AuthZ**: PowerX 后台 JWT / 多租户上下文 / RBAC  
- **Observability**: 统一 JSON 日志（含 `trace_id`/`tenant_id`）、审计落库、指标上报  
- **Project Type**: **CoreX Modular Monolith**（单进程模块化后端，不启动独立微服务）  
- **Performance Goals**: API p95 < 200ms；Presign 生成 < 100ms；分页 < 100ms  
- **Constraints**: 严格多租户隔离、最小权限、软删 + 定时物理清理（7 天可回滚窗口）

---

## Constitution Check（Pre & Post Design）

- **Plugin-First**：模块化落在 `internal/*` & `pkg/corex/*`，不新建独立服务 ✅  
- **Spec-Driven**：以 `/specs/001-docs-media-storage/spec.md` 为准驱动设计 ✅  
- **Multi-Tenant / Secure-by-Design**：所有接口带租户上下文与 RBAC、审计 ✅  
- **DB Independence**：通过 GORM 抽象；默认 Postgres 但不绑定实现 ✅  
- **Observability**：统一日志/trace/metrics/audit ✅  

**结论**：通过，可进入 Phase 0/1。

---

## Project Structure（对齐你现有仓库）

### Documentation (this feature)

```
specs/001-docs-media-storage/
├─ spec.md
├─ plan.md              # 本文件
├─ research.md
├─ data-model.md
├─ quickstart.md
└─ contracts/
```

### Source Code（repository root）

```
cmd/
├─ app/
│  └─ main.go                # 统一入口（复用）
├─ database/
│  ├─ main.go
│  ├─ migrate.go             # 扩展媒体表迁移
│  └─ seed/
│     ├─ seed.go
│     ├─ seed_admin.go
│     ├─ seed_agent.go       # 保持现状（与本特性无强耦合）
│     ├─ seed_department.go
│     ├─ seed_permission.go  # 扩展注册 media 权限
│     ├─ seed_role.go
│     └─ swagger_permissions.go
├─ perm_gen/
│  └─ main.go                # 如需生成权限映射可复用
└─ tools/
   └─ audit_partitions/
      ├─ README.md
      └─ main.go
```

```
config/
└─ storage.go                # 新增：媒体驱动配置体 & 默认值

internal/
├─ infra/
│  └─ media/
│     ├─ driver/
│     │  ├─ local/
│     │  └─ s3/
│     └─ manager/
├─ service/
│  └─ media/
└─ transport/
   └─ http/
      └─ admin/
         └─ media/           # 路由/handler（后台）

pkg/
└─ corex/
   └─ db/
      └─ persistence/
         ├─ model/
         │  └─ media/
         └─ repository/
            └─ media/
```

> 注：**不新增** 任意 `cmd/*` 可执行入口；媒体能力经现有 `cmd/app` 暴露。迁移/权限 Seed 落在 `cmd/database` 体系。

---

## Execution Flow（/plan 命令边界内执行）

```
1) 校验 spec.md 存在与版本号 → 否则 ERROR
2) 填充技术上下文（本文件）并进行初次 Constitution Check
3) Phase 0 → 产出 research.md：唯一性/索引、分页策略、JSONB+GIN 标签、go-minio 实践、软删清理
4) Phase 1 → 产出 data-model.md、contracts/*、quickstart.md；落地占位测试（httptest）
5) 再次 Constitution Check：若有违背则回退 Phase 1 调整
6) 描述 Phase 2 任务生成策略（不创建 tasks.md） → STOP
```

**已删除步骤**：任何与 Agent/上下文脚本相关的同步动作。

---

## Phase 0 — Research（摘要）

- **唯一性/索引**：`UUID` 主键 + `(tenant_id, driver, object_key)` 唯一约束；`(tenant_id, driver, status, updated_at DESC, id DESC)` 组合索引。  
- **分页策略**：默认 Offset/Limit；提供 Keyset/Seek 作为深翻页优化（`ORDER BY updated_at DESC, id DESC`）。  
- **标签检索**：`tags JSONB` + **GIN**；AND 用 `@>`，OR 用 `OR` 组合或多次查询。  
- **S3 客户端**：`minio-go/v7`，TLS/超时/指数重试（3 次）、STS/短期 AKSK、分片上传、SSE 可选；Presign 默认 12h。  
- **软删清理**：`deleted_at` 软删 + 定时任务（≥7 天）批量物理删除；幂等 & 审计。

> 详见 `/specs/001-docs-media-storage/research.md`。

---

## Phase 1 — Design & Contracts

1) **Data Model**（data-model.md）  
- `MediaAsset`（持久化）：字段/校验/状态机（Draft/UnderReview/Published/Archived）  
- `PresignToken`（瞬时）：方法、过期策略（默认 12h）、允许方法 GET/PUT  
- 关系：与租户上下文、Owner 业务域的弱关联（外部查询）

2) **API Contracts**（contracts/admin-media-assets.yaml）  
- `POST /api/v1/admin/media/assets`（文件上传或 `file_url`）  
- `GET /api/v1/admin/media/assets`（分页 + 过滤：关键字/driver/status/tags/owner）  
- `GET /api/v1/admin/media/assets/:id`  
- `PATCH /api/v1/admin/media/assets/:id`（仅业务字段）  
- `DELETE /api/v1/admin/media/assets/:id`（软删）  
- `POST /api/v1/admin/media/assets/presign`（上传/下载预签名）

> 合同输出 OpenAPI 片段 + JSON 示例；遵循 PowerX 的分页与错误码约定。

3) **Tests（占位）**  
- `contracts/tests/admin_media_assets_test.go`：路由绑定、Schema 校验、鉴权失败/成功覆盖。  
- 集成测试：上传→检索→详情→预签名→软删的 happy path。

---

## Phase 2 — Task Planning Approach（描述不落地）

- 从 contracts & data-model & quickstart 自动生成 `tasks.md`：  
  - 每个接口→合同测试任务 [P]  
  - 每个实体→模型/迁移任务 [P]  
  - 用户故事→集成测试任务  
- 依赖顺序：模型→仓储→服务→HTTP handler → 文档/样例  
- TDD：先使失败测试出现，再实现直至通过

---

## Migrations & Seeds

- 迁移：创建 `media_assets` 表，唯一约束 `UNIQUE(tenant_id, driver, object_key)`；  
  索引 `idx_media_tenant_driver_status_updated`；`tags JSONB` + `GIN`。  
- 种子：在 `cmd/database/seed/` 扩展 **`media.asset.*`** 权限注册；按需补充 swagger 权限映射。

---

## Risk & Mitigation

- **对象重复/并发写**：以唯一约束 + 幂等校验（可选 sha256）避免重复。  
- **大对象上传失败**：分片 + 指数退避重试；仅对幂等动作重试。  
- **越权生成 Presign**：严格 RBAC + 仅允许受管对象路径 + method 限制 + 过期控制。  
- **深翻页性能**：提供 Keyset 改写为可选；必要时引入只读副本/缓存层。

---

## Progress Tracking

- [x] Initial Constitution Check  
- [x] Phase 0: Research complete  
- [x] Phase 1: Design outline complete  
- [ ] Phase 2: Task plan generated  
- [ ] Phase 3: Implementation complete  
- [ ] Phase 4: Validation passed

---

## PowerX Compliance

- Constitution: v2.1.1  
- Plugin-First: ✅  
- Spec-Driven: ✅  
- Secure-by-Design: ✅  
- Multi-Tenant: ✅  
- Observability (trace/audit): ✅
