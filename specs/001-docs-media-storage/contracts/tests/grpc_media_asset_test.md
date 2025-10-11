# 契约测试计划：gRPC MediaAssetAdminService

> 这些场景会在实现阶段生成 protobuf 客户端测试，当前文件先定义断言。

## CreateMediaAsset

- 发送 `UploadChannel=UPLOAD_CHANNEL_DIRECT` 与二进制样本，期望返回 `BusinessStatus=BUSINESS_STATUS_DRAFT`。
- 禁用驱动时返回 gRPC `FAILED_PRECONDITION`。

## ListMediaAssets

- 默认分页返回 20 条以内，`total`>0。
- 传入 `business_status=BUSINESS_STATUS_ARCHIVED` 时只返回归档资产。

## GetMediaAsset

- 存在即返回 `MediaAsset`，不存在或软删除返回 `NOT_FOUND`。

## UpdateMediaAsset

- 允许更新 `tags`、`business_status`，返回更新后的实体。
- 非法状态流转返回 `INVALID_ARGUMENT`。

## DeleteMediaAsset

- 正常返回 `deleted=true`、`scheduled_cleanup=true`。
- `force=true` 且驱动失败时，返回 `FAILED_PRECONDITION` 并在 `details` 携带驱动错误。

## PresignMediaAsset

- 默认返回 `expires_in_seconds=43200`。
- 自定义 `expires_in_seconds` 小于 300 时校验失败，返回 `INVALID_ARGUMENT`。
