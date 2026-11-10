# OpenAI 视觉分析 Core 交接（2026-10-30）

目标绑定为 `openai/gpt-6-luna`，操作为 `vision_understanding`。这是图片输入、文本分析输出；生图配置继续使用 `image / image_generation`。

## 实现

- `backend/config/agents/providers.d/openai.yaml` 登记 `drivers.vlm: openai`、原生型号、输入类型、参数 Schema 和默认值。本轮项目限制为单图，不将它描述为供应商官方图片数量上限。
- OpenAI VLM 驱动对该型号把公共参数 `max_tokens` 映射为 Chat Completions 的 `max_completion_tokens`，传递 `reasoning_effort`；只有 `none` 可以同时指定 `temperature` / `top_p`。旧型号维持既有预算字段。解析支持文本段数组和包含嵌套详情的 usage。
- `image_base64` 校验并转换成图片 data URL；无效 Base64、内容类型不符和超限输入在请求供应商前拒绝。HTTP(S) 图片 URL 作为图片内容传递。
- 两个既有目录能力和 `com.corex.ai.vlm.invoke` 继续使用固定、严格 DTO 的 `core_internal` binding。型号响应包含配置状态、可用状态、原因、完整操作 Schema 和实际默认值；不会通过目录查询执行推理。
- `/settings/ai` 分为“视觉分析”与“图像生成”。视觉参数、草稿、供应商/型号查询、保存、刷新、连接测试、参数检查均按 `vlm` 处理；参数检查不调用模型。视觉保存可以引用同租户同环境已有 Provider 凭据，不重发密钥或覆盖生图画像。
- 原来登记在 `image` 下的明确 `vision_understanding` 操作在内存目录中归入 `vlm`；服务读取兼容旧 `image` 视觉画像及 Provider 别名，不批量修改数据库、不按型号名字猜测能力。

官方依据：[型号说明](https://developers.openai.com/api/docs/models/gpt-6-luna)、[参数兼容说明](https://developers.openai.com/api/docs/guides/latest-model)、[图片输入说明](https://developers.openai.com/api/docs/guides/images-vision?api-mode=chat)。清单登记和本地配置可用不代表供应商调用已经通过，真实推理单独验收。

## 当前运行环境与权限

- 测试租户：`6b5d0240-9920-46da-b707-88200e0f51ea`（System）。
- 实际服务凭据：MediaXStudio 已保存的 `Media Studio Development Key`，从插件现有环境读取；未生成新 Key，未扩大 Profile 或修改插件 owner。
- 实际 Key 调用 grant-status：`com.corex.ai.catalog.providers.read`、`com.corex.ai.catalog.models.read`、`com.corex.ai.vlm.invoke` 均为 `granted / CAPABILITY_GRANTED`。请求 ID：`10b522c6-8e71-4f20-9e40-5da9a3833e1a`。
- 现有运行进程：8077 端口的 PID `13572`，2026-10-30 22:13:43 启动，工作目录为 Core `backend`，程序为用户 `go run` 生成的 app。它早于本轮修改。
- 使用 MediaXStudio 实际引用的 Framework 本地快照与上述 Key 调用正式能力接口：当前运行进程视觉供应商只有 `qwen-cn` / `qwen-intl`，`provider=openai` 的视觉型号列表为空。它不是本轮修复后的运行验收。
- 本轮开始时底座 `image` 活跃画像为 `agnes/agnes-image-2.5-flash`（profile 187），已保存只读快照。尚未改动该配置。

本轮没有修改三个正式能力合同，因此不需要为 Provider 型号登记扩大能力或重置 seed。`make capability-check` 已验证通过。

## 部署后的正式配置与验收

仓库要求日常 Core 由用户自行重启。必须重新编译并重启 8077 的正确进程，再执行以下步骤，不能拿当前旧进程或单元测试替代。

1. 使用管理员设置入口保存视觉画像。请求 `POST /api/v1/admin/agents/settings/save`：

   ```json
   {"env":"dev","modality":"vlm","vlm":{"provider":"openai","model":"gpt-6-luna","reuseCredential":true,"maxTokens":2048,"reasoningEffort":"none","jsonMode":false}}
   ```

   `reuseCredential` 要求该租户该环境已有有效凭据引用，并拒绝同时提交连接修改。Admin 用于配置，服务 Key 用于后面的调用验收。

2. 用同一 MediaXStudio Key 查询两项目录，均经 `POST /api/v1/tenant/invocations`，`preferred_protocol=core_internal`，payload 只包含 `body`：

   ```json
   {"operation":"list_providers","capability":"vision_understanding"}
   ```

   ```json
   {"operation":"list_models","provider":"openai","capability":"vision_understanding"}
   ```

   期待存在 `openai/gpt-6-luna`，`configured=true`、`available=true`，包含 `vision_understanding.parameter_schema` 与实际 defaults。不存在任意 endpoint、headers 或 method 的透传字段。

3. 经插件现有官方 Framework `Client.VLMInvoke` 执行一次合成单图分析。期待 `contract_version=2`、`type=analysis`、非空 `text`、原生 provider/model 及 `trace_id`。如失败，保留具体 Core 原因码、上游状态和 provider trace；不得把连接测试或 LLM 调用当作视觉推理通过。
4. MediaXStudio 的视觉分析入口与导演模型 05 保存、刷新后检查绑定均为 `openai/gpt-6-luna`；导演 06 绑定和参数与验收前快照一致。该部分必须通过插件真实页面/服务复验。

## 验证范围

受控回归覆盖真实 HTTP 设置 Handler、密钥引用及画像隔离、API Key / STS 的正式能力路由、目录完整元数据、图片 URL 与 Base64、参数拒绝、上游错误与分析响应解析。页面测试使用独立登录态与 API fixture，不写开发数据库。

已通过：后端 7 个相关测试包、最终媒体与 HTTP 设置回归、`go vet`、arm64 编译、前端类型检查、55 项 AI 单测，以及桌面／窄屏 2 项完整页面流程。页面已验证保存后刷新保留参数、切换保留草稿、在线目录更新采用 `vlm`；加载期间参数输入有保护，避免初始化覆盖用户输入。临时 3041 服务及其子进程已关闭，监听端口已释放。

运行证据和剩余验收状态见 [验证记录](evidence/openai-vision-core-validation-20261030.json)。真实供应商分析与 MediaXStudio 两处页面的联合验收在用户重启新版后完成。

可复现的后端检查（在 `backend` 执行）：

```sh
go test ./internal/server/ai/drivers/openai ./internal/server/agent/catalog ./internal/service/ai ./internal/service/agent ./internal/transport/http/admin/agent ./internal/service/integration_gateway/apikeypermissions ./tests/contract/integration_gateway -count=1
go vet ./internal/server/ai/drivers/openai ./internal/server/agent/catalog ./internal/service/ai ./internal/service/agent ./internal/transport/http/admin/agent
```

页面回归（在 `web-admin` 执行，指定独立临时服务地址）：

```sh
PLAYWRIGHT_BASE_URL=http://127.0.0.1:3041 npx playwright test --config playwright.ai-models.config.ts vision-settings.spec.ts --output /tmp/powerx-vision-ui-proof
```

临时验证服务由 Agent 管理，验收结束即关闭。无需重装插件或更换 Framework SDK。


## 2026-11-01 在线目录优先调整

型号候选改为当前连接的完整在线结果，不再要求逐型号登记到本地 YAML。OpenAI 未声明用途的在线 ID 会明确标注，也允许按 `vlm` 用途保存；后端使用已有固定视觉驱动模板及严格参数 Schema，不接受任意 HTTP 透传。`gpt-6-luna` 的专用参数适配保留，其他原生 ID 无需因此加入白名单。

目录、保存和 typed VLM 调用已有受控回归；真实 OpenAI 推理以及 MediaXStudio 两入口的绑定验收仍待新版 Core 重启后执行。本节不将旧进程或 fixture 的结果当作真实供应商通过。

规则与限制见 [在线模型目录](ai-online-model-catalog.md)，本轮证据见 [验证记录](evidence/ai-online-model-online-first-validation-20261101.json)。
