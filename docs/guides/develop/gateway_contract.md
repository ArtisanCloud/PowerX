# Gateway 路由与认证契约（v1）

本文用于插件/外部系统对接 PowerX Gateway 时的最小契约对齐，覆盖路由拼接、认证优先级、Header 规范与常见管理接口。

## 1. Base URL 与 Prefix

- `PX_GATEWAY_BASE_URL` 建议只配置主机地址（例如 `http://127.0.0.1:8077`）。
- HTTP OpenAPI 默认前缀为 `/api/v1`（由 `server.api_prefix` 控制）。
- 典型调用路径：
  - LLM Invoke：`{PX_GATEWAY_BASE_URL}/api/v1/ai/llm/invoke`
  - Capabilities：`{PX_GATEWAY_BASE_URL}/api/v1/admin/capabilities`
  - Platform Capabilities：`{PX_GATEWAY_BASE_URL}/api/v1/admin/platform-capabilities`

可通过以下接口读取运行时元信息：

- `GET /api/v1/admin/gateway/meta`

## 2. Source 枚举

能力来源使用 `source` 查询参数：

- `corex`：PowerX 底座能力
- `plugin`：插件/租户注册能力
- 空值/`all`/`any`：全部来源

可通过以下接口获取来源清单：

- `GET /api/v1/admin/capabilities/sources`

### 2.1 Source 的后端定义来源

`source` 不是前端枚举，来自后端能力记录：

1. 优先读取 `CapabilityRecord.annotations.source`
2. 若为空，按 `plugin_id` 推断：
   - `plugin_id` 以 `corex.` 开头 => `corex`
   - 其余 => `plugin`

别名归一化：

- `platform` -> `corex`
- `all`/`any`/空值 -> 不过滤（查询全部）

因此当前稳定来源语义只有两类：`corex`、`plugin`。

## 3. 认证规则（单请求单凭证）

PowerX Gateway 按 `Authorization` scheme 分流：

- `Authorization: ApiKey <key>`：仅走 API Key 链路
- `Authorization: Bearer <token>`：仅走 JWT 链路

约束：

- 单次请求只应携带一种凭证语义
- 不支持失败回退（ApiKey 失败不会回退 Bearer）
- 非法 scheme 返回 `400`

## 4. Header 规范

- API Key：`Authorization: ApiKey <plain_key>`
- JWT：`Authorization: Bearer <access_token>`

建议不要额外混传自定义认证头，避免宿主/插件在不同模式下出现歧义。

## 5. 错误码约定（最小）

- `401`：凭证缺失或无效（例如 API Key 不存在、JWT 无效）
- `403`：主体已认证但无权限（如模型未开通、能力未授权）
- `404`：资源不存在（路由/能力 ID/会话 ID 等）
- `422`：参数校验失败（请求体字段不满足约束）

错误体采用统一结构（示意）：

```json
{
  "code": 400,
  "message": "invalid request",
  "error": "detail"
}
```

> 说明：不同模块历史实现中 `error`/`detail` 字段可能并存，建议调用方优先读取 `message`，并保留原始响应用于诊断。

## 6. 最小联调样例

### 6.1 查询 source 清单

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/capabilities/sources" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" | jq .
```

### 6.2 核心能力列表（推荐主入口）

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/capabilities?source=corex&page=1&page_size=20" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" | jq .
```

### 6.3 Gateway 元信息

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/gateway/meta" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" | jq .
```
