# Gateway 路由与认证契约（v1）

本文用于插件/外部系统对接 PowerX Gateway 时的最小契约对齐，覆盖路由拼接、认证优先级、Header 规范与常见管理接口。

## 1. Base URL 与 Prefix

- `PX_GATEWAY_BASE_URL` 只放主机，不带任何 API 前缀（例如 `http://127.0.0.1:8077`）。
- `PX_GATEWAY_API_PREFIX` 必须显式配置（插件侧默认建议 `/api/v1`），`/` 不等价“无前缀”。
- 最终地址拼接规则固定为：`{PX_GATEWAY_BASE_URL}{PX_GATEWAY_API_PREFIX}/...`
- HTTP OpenAPI 前缀由 `server.api_prefix` 控制。
- 当前代码默认值为 `/api`（`backend/config/defaults.go`），常见部署会显式配置为 `/api/v1`。
- 典型调用路径：
  - LLM Invoke：`{PX_GATEWAY_BASE_URL}/api/v1/ai/llm/invoke`
  - Capabilities：`{PX_GATEWAY_BASE_URL}/api/v1/admin/capabilities`
  - Platform Capabilities：`{PX_GATEWAY_BASE_URL}/api/v1/admin/platform-capabilities`

可通过以下接口读取运行时元信息：

- `GET /api/v1/admin/gateway/meta`

建议先读取 `gateway/meta`，再拼接调用路径：

- `HTTP_BASE = base_url + api_prefix`

### 1.1 错误示例 vs 正确示例

- 错误（漏了 API 前缀，最终打到 `/tenant/invocations`）：
  - `PX_GATEWAY_BASE_URL=http://127.0.0.1:8077/api/v1`
  - `PX_GATEWAY_API_PREFIX=/`
  - 结果：`http://127.0.0.1:8077/tenant/invocations`
- 正确：
  - `PX_GATEWAY_BASE_URL=http://127.0.0.1:8077`
  - `PX_GATEWAY_API_PREFIX=/api/v1`
  - 结果：`http://127.0.0.1:8077/api/v1/tenant/invocations`

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
- `400`：参数校验失败（当前多数 handler 使用 `400` 返回校验错误）

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

### 6.4 能力调用入口（务必区分）

- 统一能力选择器（按 capability_id / intent 调用）：
  - `POST {HTTP_BASE}/tenant/invocations`
- Integration Gateway 路由调用（按 route_slug 调用）：
  - `POST {HTTP_BASE}/tenant/integration/routes/{route_slug}/invoke`

示例（Selector）：

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: Bearer <TENANT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.corex.media.assets.read",
    "idempotency_key": "demo-001",
    "payload": {}
  }' | jq .
```

示例（Route Invoke）：

```bash
curl -sS -X POST "$HTTP_BASE/tenant/integration/routes/media-assets-read/invoke" \
  -H "Authorization: Bearer <TENANT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "idempotency_key": "demo-002",
    "payload": {}
  }' | jq .
```
