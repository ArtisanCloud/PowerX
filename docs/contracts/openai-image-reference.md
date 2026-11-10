# OpenAI GPT-Image-2 参考图输入

本合同对应 Core 的 AI Media v2；MediaXStudio 继续使用现有 Framework，不新增人物、阶段造型、分镜或项目 API。

## 调用和输入

入口仍是 `POST /api/v1/tenant/invocations`，`capability_id=com.corex.ai.image.invoke`，`preferred_protocol=core_internal`。`payload.body` 保持 `operation=invoke`、`contract_version=2`、`model_key=openai/gpt-image-2`、`inputs/params/idempotency_key`。

- 没有参考图：驱动提交 `POST /v1/images/generations`。
- 有参考图：驱动提交 `POST /v1/images/edits`，使用 JSON `images[].image_url`，顺序与 `inputs` 中图片出现的顺序一致。
- 当前接通 `image_url` 中的 `data:image/png;base64,...`、JPEG/WebP data URL，以及 `image_base64` 的内嵌数据。先严格解码、核验 MIME 和实际图片、尺寸，再规范化为 data URL；不改变像素、不合成或调换参考图。
- 普通远程 HTTP(S) 图片 URL 本轮未开放，返回 `AI_INPUT_UNSUPPORTED` / `reference_requires_data_url`，不由 Core 抓取或静默忽略。
- 型号与供应商保持调用方固定的选择；不会失败后切换型号、改走文字生图或重发其他协议。

官方依据：[Create image edit](https://developers.openai.com/api/reference/resources/images/methods/edit)、[GPT Image 2 参数](https://developers.openai.com/api/docs/guides/image-prompting#gpt-image-2-reference)。官方编辑协议支持至多 16 张图；此实现使用 JSON 编辑协议，不声称兼容所有第三方私有编辑协议。

## Core 实际限制

| 限制 | 数值 |
|---|---:|
| 参考图数量 | 最多 16 张，可按连接收紧 |
| 单图解码字节 | 5 MiB |
| 全部图片解码字节合计 | 20 MiB |
| 单图像素数 | 36,000,000 |
| 图片调用 JSON 信封及任务快照 | 32 MiB |
| 供应商成功响应 JSON | 128 MiB；超过时明确报错 |

PNG、JPEG、WebP 共享以上限制。数量、字节和像素限制同时生效；16 张不表示每张都可以达到 5 MiB。能力网关仅对 `com.corex.ai.image.invoke` + `core_internal` 开放 32 MiB，其余能力保留 8 MiB；直接媒体入口中的其他模态仍保留 16 MiB。

目录的 `operations.image_generation` 返回 `input_types/max_images` 和可选的 `max_image_bytes/max_total_image_bytes/max_image_pixels`。`quality` 保持 `auto/low/medium/high`，默认 `auto`；`size` 与 `format` 沿用登记的参数 Schema。目录可用性继续由当前租户、环境、画像、可解封连接凭据和实时生成授权判断，不表示已经完成供应商实际生成验收。

已登记的 `gpt-image-2-2026-04-21` 与共享该编辑合同的 `chatgpt-image-latest` 使用同一适配；其他型号不因 ID 前缀而自动开放参考图。

## 连接限制与自定义通道

已保存的供应商连接可选声明 `image_edit_max_images`，值必须为 0..16 的整数：0 关闭参考图；1 仅开放单图；更小限制同时用于目录返回、提交校验与驱动发送。

这些是连接字段，不是生成 `params`。通过现有供应商连接保存机制持久化，不能由插件单次调用传入路径或扩大上限。正常 OpenAI 通道无需新增配置。已有非标准生成 `api_path` 时不会推测对应编辑路径；必须通过连接显式提供 `image_edit_api_path`，只允许固定相对 API 路径，禁止绝对 URL、查询串、片段、模板、通配或路径穿越。编辑请求使用同一连接的 Base URL、API Key 和组织标识。

仅支持 multipart 或其他私有协议的第三方通道不能直接按此 JSON 合同标记已验收；应按实际限制关闭参考图或完成独立驱动适配。供应商拒绝编辑时保留结构化错误，不自动重试为文字生成。

## 错误与恢复

- 不支持的输入类型、远程 URL、未接通编辑的通道：`AI_INPUT_UNSUPPORTED`。
- 超数量、字节或像素限制：`AI_INPUT_LIMIT_EXCEEDED`。
- 损坏 Base64、MIME 不匹配、图片无法完整解码：`AI_INPUT_INVALID`。
- 非法生成参数：`AI_PARAMETER_INVALID`；错误包含参数位置。
- 图片内容错误在登记任务之前拒绝；驱动发送前使用同一校验再次防御。
- 供应商拒绝：保留 `AI_PROVIDER_REJECTED`、上游状态、业务机器码和供应商请求标识；公共错误不转发原始供应商正文。
- 成功响应损坏或超过响应上限时明确返回错误，不截断后继续解析。

幂等身份仍是租户 + 真实主体 + 能力 + 幂等键；请求摘要包含型号、参数、提示词、完整图片内容与图片顺序。同一键、同一请求返回原任务；换图或调换顺序返回 `AI_IDEMPOTENCY_CONFLICT`。供应商已经接收而响应丢失时保留 `AI_SUBMISSION_OUTCOME_UNKNOWN`，不会自动再次生成；这不等于供应商没有产生费用。

任务读取、SSE 和资产继续使用同一 Core `task_uuid`。恢复先读取权威快照，按 revision 去重，不通过重发生成恢复页面；资产转存可独立恢复。成功后保留 Core 媒体资产 UUID、MIME、大小和 SHA-256，以及供应商请求标识。供应商输入顺序和字节受控通过不等于人物一致性已经真实验收，后者必须检查实际生成结果。

## 验证和部署边界

受控测试使用隔离 SQLite、miniredis、本地模拟供应商及临时媒体目录；不使用日常数据库、真实供应商 Key 或付费模型。

```sh
cd backend && GOWORK=off go test -race ./internal/server/ai/drivers/openai ./internal/server/agent/catalog ./internal/service/ai ./internal/service/agent ./internal/transport/http/openapi/ai ./internal/transport/http/openapi/capability_registry ./internal/service/capability_registry
```

覆盖文字、单图、双图、16 图、顺序、坏图、超限、非法参数、响应大于 8 MiB、服务 Key 目录/授权、幂等冲突、资产读取、响应丢失不重发及现有 SSE 恢复。

本次不修改 formal capability ID、绑定、权限、registration、Key/Profile 或数据库 schema，不需要运行 capability-seed 或业务数据迁移。日常 Core 由所有者按原方式重新编译和重启；重启后先用 MediaXStudio 原服务 Key 读取目录，确认 `max_images` 和输入类型，再由用户安排一次真实双参考图生成，记录 task_uuid、Core trace、provider trace 和可读取资产。连接测试不替代这次验收。

本次验证收据见 [2026-11-07 验证记录](evidence/openai-image-reference-validation-20261107.json)。
