# 契约测试计划：公开版 Media OpenAPI

用于校验 `specs/001-media-storage/contracts/http-openapi.yaml` 的对外能力接口，测试覆盖宿主/插件通过 `/api/v1/media/assets` 调用的关键断言。

## POST /media/assets
- 期望 201 响应，字段 `uuid`、`driver`、`businessStatus` 存在。
- 缺失 `name` 或 `uploadMethod` 时返回 400 并携带 `traceId`。

## GET /media/assets
- 期望 200 响应，`items` 为数组、`total/page/pageSize` 数字字段齐全。
- 非法分页参数（`page=0` 或 `pageSize<=0`）返回 400。

## GET /media/assets/{uuid}
- 期望 200 响应且 `uuid` 匹配路径参数。
- 查不到资源返回 404，并保留统一错误结构。

## DELETE /media/assets/{uuid}
- 期望 200 响应，`deleted=true`。
- 重复删除返回 404。

## POST /media/assets/{uuid}/presign
- 期望 200 响应，字段 `url/method/expiresInSeconds/objectKey` 存在。
- `expiresInSeconds` 未提供时默认 43200。
