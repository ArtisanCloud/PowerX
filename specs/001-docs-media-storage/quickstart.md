# Quickstart — Media Asset Admin Capabilities

**Prerequisites**

- 已完成数据库迁移并创建 `media_assets` 表（含唯一索引）。
- `config/storage.go` 配置至少一个启用的存储驱动（如 `s3` 或 `local`），并在 `.env` 设置对应凭证。
- 后台管理员 JWT/Session 可用（以下示例使用 `{{ADMIN_TOKEN}}` 占位）。

## 步骤 1：启动后台服务

```bash
make dev
```

- 预期在 `DEV_PORT=8077` 下监听，日志输出包含 `trace_id`、`tenant_id` 字段。

## 步骤 2：上传测试媒体

```bash
curl -X POST http://localhost:8077/api/v1/admin/media/assets \
  -H "Authorization: Bearer {{ADMIN_TOKEN}}" \
  -F "file=@/tmp/demo.png" \
  -F "driver=s3" \
  -F "folder=demo" \
  -F "owner_type=campaign" \
  -F "owner_id=1001" \
  -F "tags=[\"banner\",\"autumn\"]"
```

- 预期响应 `201`，JSON 包含 `id`, `status=Draft`, `createdBy`。
- 如驱动未启用或凭证错误，应返回 400/500 并记录审计。

## 步骤 3：分页查询验证筛选

```bash
curl -X GET "http://localhost:8077/api/v1/admin/media/assets?keyword=demo&driver=s3&page=1&pageSize=10" \
  -H "Authorization: Bearer {{ADMIN_TOKEN}}"
```

- 预期返回 `total >= 1`，列表含步骤 2 新建条目；确认 `status` 与 `tags` 字段。

## 步骤 4：查看详情并更新状态

```bash
curl -X PATCH http://localhost:8077/api/v1/admin/media/assets/{{ASSET_ID}} \
  -H "Authorization: Bearer {{ADMIN_TOKEN}}" \
  -H "Content-Type: application/json" \
  -d '{"status":2,"description":"季节性主 KV"}'
```

- 预期响应显示 `status=Published`，更新时间更新，审计记录持久化。

## 步骤 5：生成下载预签名链接

```bash
curl -X POST http://localhost:8077/api/v1/admin/media/assets/presign \
  -H "Authorization: Bearer {{ADMIN_TOKEN}}" \
  -H "Content-Type: application/json" \
  -d '{"assetId":"{{ASSET_ID}}","method":"GET"}'
```

- 预期响应包含 `url`, `expireAt`（12 小时内），`method=GET`。
- 使用返回的 URL，GET 请求应成功获取文件。

## 步骤 6：软删除并验证定时任务

```bash
curl -X DELETE http://localhost:8077/api/v1/admin/media/assets/{{ASSET_ID}} \
  -H "Authorization: Bearer {{ADMIN_TOKEN}}"
```

- 预期响应确认软删成功；`deletedAt` 非空。
- 定时任务（独立执行）应在策略期限后物理删除，对象存储日志可见 `delete` 事件。

> 验证完成后，可执行 `make unit-test` 确认所有回归测试通过。
