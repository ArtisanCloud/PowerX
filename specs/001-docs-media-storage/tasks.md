# Tasks: Media Asset Admin Capabilities (CoreX Module)

**Input**: 设计资产 `specs/001-docs-media-storage/`（plan.md、research.md、data-model.md、contracts/、quickstart.md）  
**目标**: 依据最新计划，将媒体管理功能落地于 CoreX 内核

## 执行节奏

```
1. 同步契约与生成工具配置
2. 先写失败的契约测试 / 集成测试（TDD）
3. 逐层实现：模型 → 迁移 → 仓储 → 基础设施（驱动/管理器）→ 领域/服务
4. 接入 HTTP / gRPC 传输层与依赖注入
5. 构建软删除清理 CLI、单元测试与收尾指令
```

## Phase 3: 任务列表

- [ ] **T001** 将 `specs/001-docs-media-storage/contracts/http-openapi.yaml` 重命名并整理为 `specs/001-docs-media-storage/contracts/http-admin.yaml`，修订 `servers.url=/api/admin/v1`、标签说明及引用链接。
- [ ] **T002** 将 `specs/001-docs-media-storage/contracts/grpc-media-asset.proto` 移入 `api/grpc/corex/media/v1/media_asset.proto`，修改 package 为 `powerx.corex.media.v1`，设置 `go_package = github.com/ArtisanCloud/PowerX/api/grpc/gen/powerx/corex/media/v1;corexmediav1`。
- [ ] **T003** 在仓库根目录新增/更新 `buf.yaml`、`buf.gen.yaml`，纳入 `api/grpc/corex`，并在 `Makefile` 添加 `proto-gen`、`proto-lint`、`proto-clean`、`contracts-test` 目标（含 CI 钩子）。
- [ ] **T004 [P]** 在 `internal/transport/http/admin/media/contract_media_asset_test.go` 编写失败的 HTTP 契约测试（6 个端点），使用 `httpexpect` 校验状态码与响应体。
- [ ] **T005 [P]** 在 `internal/transport/grpc/media/contract_media_asset_test.go` 编写失败的 gRPC 契约测试，覆盖 `MediaAssetAdminService` 六个 RPC（`bufconn` + `testify`）。
- [ ] **T006 [P]** 在 `internal/tests/integration/media/media_asset_upload_flow_test.go` 落地失败的集成测试，模拟“上传 → 异常回滚 → 错误透传”流程。
- [ ] **T007 [P]** 在 `internal/tests/integration/media/media_asset_search_flow_test.go` 落地失败的集成测试，验证分页、标签筛选、软删除访问控制。
- [ ] **T008 [P]** 实现 `pkg/corex/db/persistence/model/media/asset.go`：定义 `MediaAsset` 模型（租户字段、枚举、JSONB、审计嵌入、TableName）。
- [ ] **T009** 在 `internal/server/media/persistence/migrate.go` 编写 `func MigrateMediaModels(db *gorm.DB) error`，使用 `AutoMigrate` + 索引创建（唯一、GIN），禁止生成 `.sql` 文件。
- [ ] **T010** 更新 `cmd/database/migrate.go`，在 `MigrateDatabase` / `ResetDatabase` 中调用 `MigrateMediaModels`，并确保生产保护逻辑覆盖。
- [ ] **T011** 在 `pkg/corex/db/persistence/repository/media/asset_repo.go` 实现仓储：CRUD、分页筛选、标签过滤、软删除与清理查询。
- [ ] **T012** 新建 `internal/domain/media/state.go`，定义业务状态常量、合法状态迁移图与验证函数。
- [ ] **T013** 编写 `config/storage.go` 并更新 `config/config.go`、`config/defaults.go`，注入驱动配置（local/s3）、默认 12 小时 TTL、MinIO 参数。
- [ ] **T014** 在 `internal/infra/media/driver/interface.go`、`internal/infra/media/manager/manager.go` 定义 `StorageDriver` 接口、`MediaManager`，实现驱动注册、默认驱动、健康检查、metrics。
- [ ] **T015** 在 `internal/infra/media/driver/local/local.go` 实现本地驱动：目录初始化、Put/Get/Delete、URL 生成、错误分类。
- [ ] **T016** 在 `internal/infra/media/driver/s3/s3.go` 实现 S3 驱动：MinIO 客户端、PutObject、Presign、TTL 校验、错误包装。
- [ ] **T017** 在 `internal/service/media/service.go` 落地用例服务：状态机校验、租户/RBAC、审计事件、预签名、软删除调度。
- [ ] **T018** 更新 `internal/app/shared/deps.go` 与 `internal/bootstrap/app.go`，初始化 `MediaManager`、`MediaService`，挂载至 `shared.Deps`。
- [ ] **T019** 在 `internal/transport/http/admin/media/dto.go` 定义请求/响应 DTO、校验标签、错误映射。
- [ ] **T020** 新建 `internal/transport/http/admin/media/router.go`，并修改 `internal/transport/http/admin/routes.go` 注册 `/api/admin/v1/media/assets` 路由及中间件。
- [ ] **T021** 在 `internal/transport/http/admin/media/handler.go` 实现 `POST /admin/media/assets`（本地上传/外链/预签名，返回 draft 状态）。
- [ ] **T022** 在同文件实现 `GET /admin/media/assets`（分页、筛选、total、软删除过滤）。
- [ ] **T023** 在同文件实现 `GET /admin/media/assets/{uuid}`（租户隔离、404 映射）。
- [ ] **T024** 在同文件实现 `PATCH /admin/media/assets/{uuid}`（业务状态流转校验、标签更新、审计）。
- [ ] **T025** 在同文件实现 `DELETE /admin/media/assets/{uuid}`（软删除、调度清理、返回 204）。
- [ ] **T026** 在同文件实现 `POST /admin/media/assets/{uuid}/presign`（上传/下载、TTL 覆盖、可选 Redis 缓存）。
- [ ] **T027** 在 `internal/transport/grpc/media/media_handler.go` 实现 gRPC 服务（依赖 `MediaService`，不负责注册），封装租户、审计、错误状态码。
- [ ] **T028** 更新 `internal/server/grpc/server.go`，通过已有拦截器注册 `corex.corex.media.v1.MediaAssetAdminServiceServer`，挂载监控。
- [ ] **T029** 新建 `cmd/media_cleaner/main.go`，实现软删除清理 CLI：扫描过期资产、调用驱动删除、写审计事件。
- [ ] **T030 [P]** 在 `internal/infra/media/manager/manager_test.go` 编写单元测试，验证驱动注册、默认回退、错误冒泡。
- [ ] **T031 [P]** 在 `internal/service/media/service_test.go` 编写单元测试，覆盖状态流转、RBAC 拒绝、审计记录。
- [ ] **T032** 执行 `make proto-gen && make contracts-test && make unit-test`，收集日志并更新 `specs/001-docs-media-storage/quickstart.md` 的命令示例/说明。

## 依赖关系

- T001–T003 必须首先完成，确保契约/生成配置与 Makefile 同步。  
- T004–T007 依赖 T003，需在实现前保持失败状态（TDD）。  
- T008 → T009 → T010 → T011 建立数据持久化链路；T012 补充领域约束后方可继续服务实现。  
- T013–T018 构建配置、驱动、依赖注入，是传输层任务 (T019–T028) 的前置条件。  
- T029 依赖仓储与驱动（T011、T015、T016）。  
- T030、T031 在核心逻辑完成后并行运行；T032 收尾前需确保所有测试全部通过。

## 并行执行示例

```
# 初期：并行启动失败测试，确认 TDD 入口
task run T004
task run T005
task run T006
task run T007

# 实现完成后：并行单元测试
task run T030
task run T031
```

> `[P]` 表示可在依赖满足后并行执行（作用文件互不冲突）。请严格遵循“先测试失败 → 再实现 → 测试通过”的节奏。
