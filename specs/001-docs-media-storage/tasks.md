# Tasks: Media Asset Admin Capabilities

**Input**: `/specs/001-docs-media-storage/plan.md`、`research.md`、`data-model.md`、`contracts/admin-media-assets.yaml`、`quickstart.md`  
**Prerequisites**: `plan.md`（必读）、`research.md`、`data-model.md`、`contracts/`、`quickstart.md`

## Execution Flow (main)

```
1. 读取 plan.md / research.md / data-model.md / quickstart.md / contracts/admin-media-assets.yaml
   → 提取技术栈、实体、测试场景与接口清单
2. 依据设计生成任务：
   → Setup：配置、依赖、脚手架
   → Tests：契约测试、集成测试
   → Core：模型、仓储、驱动、服务、HTTP 端点
   → Integration：依赖注入、权限、定时清理
   → Polish：补充单元测试、文档与性能校验
3. 应用规则：
   → 同一文件避免 [P]，保证串行
   → 测试先行（TDD），验证失败后再实现
   → 模型 → 仓储 → 驱动 → 服务 → HTTP → 集成 → 打磨
4. 建立依赖与并行说明，确保任务可直接执行
5. 输出本文件 `specs/001-docs-media-storage/tasks.md`
```

## Phase 3.1: Setup

- [ ] T001 [P] 在 `config/storage.go` 新建媒体驱动配置（StorageDriverConfig、DriverMap）并更新 `config/config.go`、`config/defaults.go`、`config/loader.go`、`config/validator.go` 以加载默认驱动和 12h 预签名过期
- [ ] T002 在 `go.mod`、`go.sum` 引入 `github.com/minio/minio-go/v7` 与所需依赖（含测试假件），执行 `make deps-tidy` 保持依赖整洁

## Phase 3.2: Tests First (TDD) ⚠️ 在实现前确保失败

- [ ] T003 [P] 在 `specs/001-docs-media-storage/contracts/tests/admin_media_assets_test.go` 编写契约测试，覆盖 POST/GET/LIST/PATCH/DELETE/PRESIGN 六个接口的鉴权与主要响应校验
- [ ] T004 [P] 在 `integration/admin_media_assets_flow_test.go` 新建集成测试，按 quickstart 场景执行上传→列表→详情→发布→预签名→软删全链路（含租户上下文与审计断言）

## Phase 3.3: Core Implementation

- [ ] T005 [P] 在 `pkg/corex/db/persistence/model/media/media_asset_gorm.go` 与 `.../media/tables.go` 定义 `MediaAsset` 模型（状态枚举、标签排序列、GORM 索引/唯一约束标记）
- [ ] T006 更新 `pkg/corex/db/database/migration.go`、`pkg/corex/db/persistence/model/media/migrate_media.go`（新建）执行 AutoMigrate，并通过事务添加 `(tenant_id, driver, object_key)` 唯一约束与 `tags` GIN 索引
- [ ] T007 [P] 在 `internal/service/media/presign_dto.go` 定义 `PresignToken`/`PresignRequest` 结构（默认 12h、GET/PUT 校验）
- [ ] T008 [P] 在 `pkg/corex/db/persistence/repository/media/media_asset_repository.go` 实现仓储：创建/更新/软删、分页/关键字/标签 (JSONB) 查询、Keyset 游标与定时清理查询
- [ ] T009 [P] 在 `internal/infra/media/driver/local/local_driver.go` 实现本地驱动（根目录校验、保存、删除、GET/PUT 预签名）
- [ ] T010 [P] 在 `internal/infra/media/driver/s3/s3_driver.go` 实现 S3 驱动（minio 客户端初始化、分片上传、TLS、重试、预签名）
- [ ] T011 在 `internal/infra/media/manager/manager.go` 聚合驱动（注册表、按租户驱动选择、封装删除/预签名/容量校验）
- [ ] T012 在 `internal/service/media/service.go` 构建 `MediaService` 结构体与构造函数（注入仓储、Manager、审计、RBAC、配置）
- [ ] T013 在 `internal/service/media/service.go` 实现创建资产流程（文件或外链校验、驱动上传、幂等、审计记录）
- [ ] T014 在 `internal/service/media/service.go` 实现分页检索与详情查询（Offset+Keyset、标签/状态过滤、租户隔离）
- [ ] T015 在 `internal/service/media/service.go` 实现业务字段更新（状态机 Draft→Published 等、字段白名单、审计）
- [ ] T016 在 `internal/service/media/service.go` 实现软删流程（删除前权限校验、审计、触发清理队列）
- [ ] T017 在 `internal/service/media/service.go` 实现预签名生成（GET/PUT 限制、过期时间、存在性校验、RBAC）
- [ ] T018 [P] 在 `internal/transport/http/admin/media/dto.go` 定义请求/响应 DTO（分页、打平 `MediaAsset`、错误映射）
- [ ] T019 更新 `internal/transport/http/admin/routes.go` 并在 `internal/transport/http/admin/media/api.go` 注册 `/api/v1/admin/media` 路由组，绑定 handler
- [ ] T020 [P] 在 `internal/transport/http/admin/media/upload_handler.go` 实现 `POST /api/v1/admin/media/assets`（支持 multipart 与 file_url，返回 Draft）
- [ ] T021 [P] 在 `internal/transport/http/admin/media/list_handler.go` 实现 `GET /api/v1/admin/media/assets`（分页、过滤、Meta 转换）
- [ ] T022 [P] 在 `internal/transport/http/admin/media/detail_handler.go` 实现 `GET /api/v1/admin/media/assets/:id`（租户隔离、404 处理）
- [ ] T023 [P] 在 `internal/transport/http/admin/media/update_handler.go` 实现 `PATCH /api/v1/admin/media/assets/:id`（状态 + 白名单更新）
- [ ] T024 [P] 在 `internal/transport/http/admin/media/delete_handler.go` 实现 `DELETE /api/v1/admin/media/assets/:id`（软删、Audit ID 返回）
- [ ] T025 [P] 在 `internal/transport/http/admin/media/presign_handler.go` 实现 `POST /api/v1/admin/media/assets/presign`（GET/PUT 校验、过期时间）

## Phase 3.4: Integration

- [ ] T026 在 `internal/app/shared/deps.go`、`internal/app/shared/options.go` 注入 `MediaService` 与 `MediaManager`（读取 storage 配置、构造驱动、复用 Auditor）
- [ ] T027 在 `internal/bootstrap/app.go` 加载媒体存储驱动配置，初始化 Manager，并启动软删清理协程（≥7 天批量删除，panic-safe）
- [ ] T028 在 `cmd/database/seed/seed_permission.go` 与 `cmd/database/seed/swagger_permissions.go` 注册媒体权限（`media.asset.*`）并暴露到 Swagger 权限映射

## Phase 3.5: Polish

- [ ] T029 [P] 在 `internal/service/media/service_test.go` 补充单元测试（创建/预签名/状态流转、Mock 驱动/仓储）
- [ ] T030 [P] 在 `internal/infra/media/driver/s3/s3_driver_test.go` 与 `.../driver/local/local_driver_test.go` 编写驱动层单测（超时/重试/路径拼接）
- [ ] T031 [P] 更新 `specs/001-docs-media-storage/quickstart.md` 与 `docs/api/admin-media.md`（新增示例响应、预签名使用指南、清理策略）

## Dependencies

- Setup：T001 → T002
- Tests：T003、T004 依赖 T001-T002 完成配置/依赖
- Model & Migration：T005 完成后方可执行 T006；T006、T007 完成后才能进入仓储 T008
- 驱动：T008 → T009/T010 → T011
- 服务：T011 → T012 → T013-T017（顺序执行以满足状态机约束）
- HTTP：T018 → T019 → T020-T025（各 handler 在 DTO/路由完成后可并行）
- Integration：T026 依赖 T008-T017、T019；T027 依赖 T026；T028 依赖 T026
- Polish：T029-T031 需在 T003-T028 均完成并通过基础测试后执行

## Parallel Example

```bash
# 并行编写契约/集成测试
codex task run T003 &
codex task run T004 &
wait

# 并行实现驱动与 DTO
codex task run T009 &
codex task run T010 &
codex task run T018 &
wait
```

## Notes

- [P] 表示可以与同阶段其他任务并行执行；涉及同一文件（尤其 service.go）已去除 [P] 以保证串行
- 所有测试任务需先观察失败（TDD），再进入实现阶段；完成核心实现后执行 `make unit-test`
- 清理协程应使用带 context 的 ticker，避免阻塞应用退出；如需外部调度，可改为注册到现有任务框架
