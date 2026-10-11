# Framework 内容导览对齐清单

Core 正式依据：[Host 合同](knowledge-content-overview-host.md)、[OpenAPI](../../specs/011-knowledge-space/contracts/content-overview.openapi.yaml)。本次 Core 未修改 Framework/Plugin。

## Go / .NET

1. 新增 typed content overview accessor 与 request/result DTO；可独立扩展，避免强迫既有 Knowledge 接口及所有 mock 新增方法。
2. Delegated/Host provider 使用 `com.corex.knowledge.content_overview.service_read` 的固定 `core_internal` binding，操作 `list_articles`。Local provider 由插件自己的授权目录实现。不得从插件连接 Core DSN、降级 Local 或用检索 top-k 采样伪装目录。
3. 精确保留 `mode`、可选 generation、cursor、page size、预览 codepoint（显式 0）、Core 文档 UUID、原文版本、source_ref、artifact/chunk/bundle UUID、模型/配置/语料版本、total/has_more/null cursor、Trace 与稳定错误。
4. 新 service API-key permission scope/action/selector 按合同配置；通过正式 `capabilities:grant-status` 验证 granted。既有 directory grant 不足以读取内容，不需要 document.manage。
5. `published_source_chunk` 映射为“已发布原文片段预览”；`not_available` 不生成示例摘要/主题/问题。原文件下载和终端用户代授权不属于此 v1。
6. 401/403/404/409/422/501/503 保留原 HTTP/机器码和 Trace。409 时清空旧分页并重新读取；不得隐藏错误成为空数组。

## Plugin

- 服务端受保护的 Lab endpoint 检查本插件用户和空间权限，通过 typed accessor 读取；凭据仅留服务端。拒绝浏览器传 Core endpoint、租户覆盖和任意 capability。
- PowerX view model 从正式 wire DTO 映射；更换空间、模式、generation 后清空旧数据。使用显式分页，目录失败可独立重试。
- “围绕这篇提问”绑定 Core document UUID/version/generation；点击仅填入输入框。
- 验收目录与 source_chunk 检索结果的资格一致，标题/预览与已发布 snapshot 一致，中文/emoji 位置正确，隐藏/跨租户不泄露，401/403/409 与空目录不同。

Core 已用原 `powerx` 库的 `dev` 五篇真实文章完成 API Key/STS 目录验收；这不代表插件浏览器已验收。证据区分真实 dev 和隔离失败 fixture。当前工作区配置的显式 DSN 指向备份验证库，日常 dev 测试前须确认 Core 连接原库；本次未修改用户配置或重启 8077。
