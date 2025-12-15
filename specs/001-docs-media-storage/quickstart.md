# Phase 1 快速验证指南

## 前置条件

1. 启动核心服务：`make dev`（默认端口 8077）。
2. 确保 PostgreSQL、Redis、MinIO（或其他 S3 兼容服务）就绪，并在 `.env` 中填写驱动配置。
3. 通过管理端登陆获取 `Bearer` Token，或使用种子脚本创建管理员账号。
4. 所有请求应在 Header 或 Query 中携带 `tenant_uuid`（默认租户可为 system/demo）。

## 启用 CoreX 模块

1. 不需要在 `plugins/registry.json` 注册；
2. 如果宿主已启用 CoreX 组件加载（Kernel Bootstrapping），系统会自动注册 MediaX 模块的 HTTP 与 gRPC 接口；
3. 如需初始化数据库，请执行：`make migrate`。

## 核心验证流程

1. **上传媒体资源**
   - 调用 `POST /api/admin/v1/media/assets`，表单或 JSON 指定上传方式。
   - 期望响应包含 `uuid`、`driver`、`businessStatus=draft`。
2. **查询与过滤**
   - 调用 `GET /api/admin/v1/media/assets?keyword=logo&driver=s3&page=1&pageSize=20`。
   - 响应需返回 `items`、`total`、分页信息；软删除记录不可出现。
3. **更新业务属性**
   - 调用 `PATCH /api/admin/v1/media/assets/{uuid}` 更新 `name`、`tags`、`businessStatus`。
   - 校验状态流转约束与审计日志。
4. **生成预签名链接**
   - 调用 `POST /api/admin/v1/media/assets/{uuid}/presign` 指定 `action=download`。
   - 响应内包含 `url`、`method`，且 `expiresInSeconds=43200`。
5. **软删除与清理**
   - 调用 `DELETE /api/admin/v1/media/assets/{uuid}`。
   - 验证记录标记 `softDeletedAt`，后台任务或守护进程应记录清理计划。
6. **gRPC 验证（可选）**
   - 调用 `MediaAssetService.Upload(MediaUploadRequest)` 并验证响应。
   - 调用 `MediaAssetService.List(Pagination)` 验证分页逻辑。
   - 调用 `MediaAssetService.GeneratePresignURL()` 验证 URL 有效期。

## 观测与调试

- 查看 `logs/app.log`，确认请求链路携带 `trace_id`、`tenant_uuid`。
- 通过 `make unit-test` 执行新增单元与契约测试。
- 使用 `mc`（MinIO Client）或 AWS CLI 验证对象存储中的文件写入与删除。

## 自动化验证命令

开发完成后建议按照以下顺序执行自检，确保契约、生成物与单测全部通过：

```bash
make proto-gen
make contracts-test
make unit-test
```

若命令执行成功，终端会打印 `Done`、`ok` 等绿色提示；如遇失败请根据日志修正后重试。

## 媒资工具集 CLI

Media 模块新增 `cmd/media_tool` 工具集，可通过子命令管理媒资维护任务。清理软删除且超时的媒资对象示例：

```bash
go run ./cmd/media_tool cleanup --dry-run --before=24h --limit=50
```

- `cleanup` 子命令支持软删除清理，并写入审计事件；
- `--dry-run`：仅打印将被清理的记录，不执行真实删除；
- `--before`：仅处理软删除时间早于该时长的记录，默认 24 小时；
- `--limit`：单次扫描的最大数量，可多次执行直至结果为空；
- `--tenant` / `--drivers`：按租户或驱动过滤待清理对象。

生产环境建议去掉 `--dry-run` 参数，并结合 Cron/任务编排按需执行。运行 `go run ./cmd/media_tool help` 查看更多子命令与参数说明。

## 回滚策略

1. 通过 `DELETE /api/admin/v1/media/assets/{uuid}` 恢复至软删除状态再重试。
2. 若需要彻底回滚服务端改动，执行数据库迁移回滚：`make migrate-down STEP=media_assets`（待实现）。
3. 禁用模块或恢复到默认配置，确保缓存清理完成后再重新启用。
