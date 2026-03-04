# Tasks: Media Asset Admin Capabilities (CoreX Module)

**Input**: 设计资产 `specs/001-media-storage/`（plan.md、research.md、data-model.md、contracts/、quickstart.md）  
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

- [X] **T001** 将 `specs/001-media-storage/contracts/http-openapi.yaml` 重命名并整理为 `specs/001-media-storage/contracts/http-admin.yaml`，`servers.url` 与 `server.api_prefix` 保持一致（默认 `/api/v1`）并同步标签说明及引用链接。
- [X] **T002** 将 `specs/001-media-storage/contracts/grpc-media-asset.proto` 移入 `api/grpc/contracts/powerx/media/v1/media_asset.proto`，修改 package 为 `powerx.media.v1`，设置 `go_package = github.com/ArtisanCloud/PowerX/internal/transport/grpc/gen/powerx/media/v1;corexmediav1`。
- [X] **T003** 在仓库根目录新增/更新 `buf.yaml`、`buf.gen.yaml`，纳入 `api/grpc/contracts/powerx`，并在 `Makefile` 添加 `proto-gen`、`proto-lint`、`proto-clean`、`contracts-test` 目标（含 CI 钩子）。
- [X] **T004 [P]** 在 `internal/transport/http/admin/media/contract_media_asset_test.go` 编写失败的 HTTP 契约测试（6 个端点），使用 `httpexpect` 校验状态码与响应体。
- [X] **T005 [P]** 在 `internal/transport/grpc/media/contract_media_asset_test.go` 编写失败的 gRPC 契约测试，覆盖 `MediaAssetAdminService` 六个 RPC（`bufconn` + `testify`）。
- [X] **T006 [P]** 在 `internal/tests/integration/media/media_asset_upload_flow_test.go` 落地失败的集成测试，模拟“上传 → 异常回滚 → 错误透传”流程。
- [X] **T007 [P]** 在 `internal/tests/integration/media/media_asset_search_flow_test.go` 落地失败的集成测试，验证分页、标签筛选、软删除访问控制。
- [X] **T008 [P]** 实现 `pkg/corex/db/persistence/model/media/asset.go`：定义 `MediaAsset` 模型（租户字段、枚举、JSONB、审计嵌入、TableName）。
- [X] **T009** 在 pkg/corex/db/migration.go 直接调用 AutoMigrate 纳入媒体模型（唯一/GIN 索引由模型结构体标签生成），不得编写原生 SQL 或生成 .sql 文件。
- [X] **T010** 更新 `cmd/database/migrate.go`，在 `MigrateDatabase` / `ResetDatabase` 中调用 `MigrateMediaModels`，并确保生产保护逻辑覆盖。
- [X] **T011** 在 `pkg/corex/db/persistence/repository/media/asset_repo.go` 实现仓储：CRUD、分页筛选、标签过滤、软删除与清理查询。
- [X] **T012** 新建 `pkg/corex/db/persistence/model/state.go`，定义业务状态常量、合法状态迁移图与验证函数。
- [X] **T013** 编写 `config/storage.go` 并更新 `config/config.go`、`config/defaults.go`，注入驱动配置（local/s3）、默认 12 小时 TTL、MinIO 参数。
- [X] **T014** 在 `internal/infra/media/driver/interface.go`、`internal/infra/media/manager/manager.go` 定义 `StorageDriver` 接口、`MediaManager`，实现驱动注册、默认驱动、健康检查、metrics。
- [X] **T015** 在 `internal/infra/media/driver/local/local.go` 实现本地驱动：目录初始化、Put/Get/Delete、URL 生成、错误分类。
- [X] **T016** 在 `internal/infra/media/driver/s3/s3.go` 实现 S3 驱动：MinIO 客户端、PutObject、Presign、TTL 校验、错误包装。
- [X] **T017** 在 `internal/service/media/service.go` 落地用例服务：状态机校验、租户/RBAC、审计事件、预签名、软删除调度。
- [X] **T018** 更新 `internal/app/shared/deps.go` 与 `internal/bootstrap/app.go`，初始化 `MediaManager`、`MediaService`，挂载至 `shared.Deps`。
- [X] **T019** 在 `internal/transport/http/admin/media/dto.go` 定义请求/响应 DTO、校验标签、错误映射。
- [X] **T020** 新建 `internal/transport/http/admin/media/router.go`，并修改 `internal/transport/http/admin/routes.go` 注册 `{APIPrefix}/admin/media/assets`（默认 `/api/v1/admin/media/assets`）路由及中间件。
- [X] **T021** 在 `internal/transport/http/admin/media/handler.go` 实现 `POST /admin/media/assets`（本地上传/外链/预签名，返回 draft 状态）。
- [X] **T022** 在同文件实现 `GET /admin/media/assets`（分页、筛选、total、软删除过滤）。
- [X] **T023** 在同文件实现 `GET /admin/media/assets/{uuid}`（租户隔离、404 映射）。
- [X] **T024** 在同文件实现 `PATCH /admin/media/assets/{uuid}`（业务状态流转校验、标签更新、审计）。
- [X] **T025** 在同文件实现 `DELETE /admin/media/assets/{uuid}`（软删除、调度清理、返回 204）。
- [X] **T026** 在同文件实现 `POST /admin/media/assets/{uuid}/presign`（上传/下载、TTL 覆盖、可选 Redis 缓存）。
- [X] **T027** 在 `internal/transport/grpc/media/media_handler.go` 实现 gRPC 服务（依赖 `MediaService`，不负责注册），封装租户、审计、错误状态码。
- [X] **T028** 更新 `internal/server/grpc/server.go`，通过已有拦截器注册 `media.v1.MediaAssetAdminServiceServer`，挂载监控。
- [X] **T029** 新建 `cmd/media_tool/main.go`，实现媒资工具集入口，包含软删除清理子命令：扫描过期资产、调用驱动删除、写审计事件。
- [X] **T030 [P]** 在 `internal/infra/media/manager/manager_test.go` 编写单元测试，验证驱动注册、默认回退、错误冒泡。
- [X] **T031 [P]** 在 `internal/service/media/service_test.go` 编写单元测试，覆盖状态流转、RBAC 拒绝、审计记录。
- [X] **T032** 执行 `make proto-gen && make contracts-test && make unit-test`，收集日志并更新 `specs/001-media-storage/quickstart.md` 的命令示例/说明。
- [X] **T033** 扩展 `internal/infra/media/driver/local/local.go` 预签名逻辑，支持上传动作（PUT）、HMAC Token、限流配置。
- [X] **T034** 在 `internal/http` 新增本地读写端点 `GET/PUT /media/*objectKey`，校验 Token/过期时间、限制 `Content-Length`，并确保目录与 `local.public_base_url`/`base_path` 一致。
- [X] **T035** 调整 `POST /admin/media/assets/{uuid}/presign` 请求/响应契约，接入 `content_type`/`expires_in` 字段并返回统一 `storageKey`、`expiresAt` 信息。
- [X] **T036** 强化媒资下载安全：在 `external_link`/本地上传的元数据校验基础上，再新增统一受控的资源访问入口（REST `GET /api/v1/media/assets/{uuid}/resource` 及 Admin 对应端点），禁止直接暴露 `/media/*` 给外部。该 Handler 必须复用 `MediaService` 读取资产记录，检查权限、业务状态、`mimeType`/`sizeBytes` 白名单，动态设置 `Content-Type`、`Content-Disposition` 并根据 `uploadMethod` 选择读取本地驱动或 302 到 `externalUrl`。同步更新 contracts、docs、quickstart，说明 `/media/*` 仅用于开发调试，正式访问都走 `resource` 接口，可选支持 `attachment`/`inline` header。

## Phase 4: Web Admin UI（内容管理 / 媒体库）

> UI 方案与页面流：`docs/plan/content/media.md`

- [X] **T037** 新增 `web-admin/app/pages/content/media/index.vue`：媒体库列表（网格/表格切换、筛选条、分页、回收站开关）。
- [X] **T038** 新增 `web-admin/app/pages/content/media/[uuid].vue`：媒体资产详情页（预览、编辑、状态流转、删除/下载/复制链接）。
- [X] **T039** 新增 `web-admin/app/composables/api/services/mediaAssetService.ts`：封装 Admin 媒体资产 API（list/create/get/update/delete/presign/resource）。
- [X] **T040 [P]** 新增 UI 组件：`MediaFilterBar`、`MediaUploadDrawer`、`MediaGrid`/`MediaTable`、`MediaPreview`、`MediaAssetDetailPanel`（按 `docs/plan/content/media.md` 拆分）。
- [X] **T041** 实现“预签名上传”闭环：create(asset) → presign(upload) → upload(file) → refresh(list/detail)，包含进度/失败重试与错误提示。
- [X] **T042** 实现“外链入库”：通过 external URL 创建资产并在详情页可预览/跳转。
- [X] **T043** 实现“安全预览/下载默认策略”：默认走鉴权资源入口（Admin `/resource`）进行预览/下载；提供复制链接策略（鉴权链接/公开链接可配置）。
- [X] **T044 [P]** 增加测试：
  - 最低要求：`mediaAssetService` 的单测（参数拼装、错误透传）。
  - 可选：Playwright e2e（登录态 fixture → 打开媒体库 → 触发列表请求并渲染空态/列表）。
- [X] **T045** 更新文档：在 `specs/001-media-storage/quickstart.md` 增加“Web Admin 手工验收步骤”（路由、上传、筛选、详情编辑、删除）。
- [X] **T046** 后台系统菜单追加“媒体”入口：`/content/media`，并确保在 Admin `GET /admin/menus` 的置顶分组中稳定出现在“流程”和“仪表盘”之间（按 `order` 排序）。

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
