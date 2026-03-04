# 契约测试计划：HTTP 媒体资产接口

> 这些测试将在实现阶段转写为 Go 测试（`internal/transport/http/admin/media`）。当前文档用于明确断言，帮助 /tasks 阶段生成失败测试。

## createMediaAsset
- 断言 201 状态码，响应体字段 `uuid`、`driver`、`businessStatus=draft` 存在。
- 断言缺失 `file` 与 `externalUrl` 时返回 400。
- 断言禁用驱动时返回 400 并提示 `driver_disabled`。

## listMediaAssets
- 断言 200 响应，`total` 字段与 items 数组长度匹配。
- 断言分页参数非法时返回 400。
- 断言带 `tags` 过滤仅返回包含所有标签的资产。

## getMediaAsset
- 断言 200 响应且 `uuid` 匹配。
- 断言软删除资产返回 404。

## updateMediaAsset
- 断言 200 响应，状态流转 `draft → under_review → published` 合法。
- 断言非法状态流转（如 `archived → published`）返回 409。

## deleteMediaAsset
- 断言 204 响应，无 body。
- 断言重复删除返回 404。

## presignMediaAsset
- 断言 200 响应，`expiresInSeconds` 默认 43200。
- 断言 `action=upload` 需要未完成上传的目标资产，否则返回 400。
