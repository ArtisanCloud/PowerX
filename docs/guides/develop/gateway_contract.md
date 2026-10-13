# Gateway 路由与认证契约（v1）

本文用于插件/外部系统对接 PowerX Gateway 时的最小契约对齐，覆盖路由拼接、认证优先级、Header 规范与常见管理接口。前缀治理与版本迁移统一见 [API 前缀统一管理与版本升级规范](../../standards/_shared/api-prefix-and-versioning.md)。本文中的 v1 示例不表示把配置改为 v2 即可完成合同升级。

## 1. Base URL 与 Prefix

- `PX_GATEWAY_BASE_URL` 只放主机，不带任何 API 前缀（例如 `http://127.0.0.1:8077`）。
- `PX_GATEWAY_API_PREFIX` 必须显式配置（插件侧默认建议 `/api/v1`），`/` 不等价“无前缀”。
- 最终地址拼接规则固定为：`{PX_GATEWAY_BASE_URL}{PX_GATEWAY_API_PREFIX}/...`
- HTTP OpenAPI 前缀由 `server.api_prefix` 控制。
- 当前代码默认值为 `/api`（`backend/config/defaults.go`），常见部署会显式配置为 `/api/v1`。
- 典型调用路径：
  - LLM Invoke：`{PX_GATEWAY_BASE_URL}{PX_GATEWAY_API_PREFIX}/ai/llm/invoke`
  - Capabilities：`{PX_GATEWAY_BASE_URL}{PX_GATEWAY_API_PREFIX}/admin/capabilities`
  - Platform Capabilities：`{PX_GATEWAY_BASE_URL}{PX_GATEWAY_API_PREFIX}/admin/platform-capabilities`

可通过以下接口读取运行时元信息：

- `GET <CoreAPIPrefix>/admin/gateway/meta`，需要已有管理端身份及相应权限；调用方应先通过部署或 Host 配置获得前缀，不能依赖尚未认证的自动发现。

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

### 1.2 Core 前缀与插件 HTTP base

Core 的 `server.api_prefix`、插件挂载入口 `/_p/<pluginID>/api` 和插件 manifest 的 HTTP base 属于不同路径坐标。插件后端调用 Core 使用 Core 的有效 Gateway prefix；Core 转发给插件使用该插件的有效 HTTP base。Core v2 与插件 v1 可以并存。

公开策略和请求必须按同一规则转换到插件 base 相对路径。不能让 exposure 登记 `/api/v1/integration/...`，而请求检查 `/integration/...`；也不能只复用函数却分别传入不一致的 base。修改公开匹配时，应验证最终上游路径保持原行为。

变更前缀或升级版本时，按共享规范审计路由、权限/能力 binding、认证重定向、SDK、前端代理与固定入口，分别验收配置适配和业务合同兼容。

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

对于需要 PowerX 凭据的 Core 调用及受保护插件路由，PowerX Gateway 按 `Authorization` scheme 分流：

- `Authorization: ApiKey <key>`：仅走 API Key 链路
- `Authorization: Bearer <token>`：仅走 JWT 链路

约束：

- 单次请求只应携带一种凭证语义
- 不支持失败回退（ApiKey 失败不会回退 Bearer）
- 非法 scheme 返回 `400`

### 3.1 已安装插件的公开 exposure

已安装插件的 `type: rest`、`auth: public` exposure 使用明确方法和路径匹配，命中后转发给插件做渠道认证。`{参数}` / `:参数` 只能匹配一个非空段；错误方法、未声明路径和额外层级继续受保护。公开声明必须包含 `security.verifier`，不能通过请求头自行放行。

`X-PowerX-Plugin-Public-Route` 是 Core 路由标记，不是验签成功凭证。Core 应完整保留 `X-AI-Craft-Shopify-App-Proxy-Query` 与 `X-Shopify-Session-Token`，由对应插件入口验证 Shopify 签名、店铺、租户和会话归属。缺失或错误渠道凭据应到达插件后被拒绝。

回归必须走 `exposure → PolicyFromPlugin() → InstallPolicy() → MountAPIProxy() → 实际 /_p 请求 → 模拟上游`。具体用例和真实 Shopify 安装验收见共享规范第 5 节；手工填写 router 策略或仅断言公开标记不能证明整条链路成功。

## 4. Header 规范

- API Key：`Authorization: ApiKey <plain_key>`
- JWT：`Authorization: Bearer <access_token>`

受保护调用不要混传其他 PowerX 凭据语义。公开渠道入口按自己的 verifier 合同携带并验证渠道凭据头，不能用 PowerX 路由标记替代。

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

### 6.5 LLM 统一请求体标准（Selector）

`/tenant/invocations` 调用 AI LLM 时，请固定使用以下结构：

- `payload.query.env`：环境（例如 `dev`）
- `payload.body.model_key`：模型键（例如 `ollama/qwen3:8b`）
- `payload.body.inputs/params`：业务参数

请不要使用以下非标准写法：

- 顶层 `payload.model_key`
- `context.env`
- 旧路径 `/integration/capabilities/invoke`

示例：

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: ApiKey <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.corex.ai.llm.invoke",
    "preferred_protocol": "rest",
    "payload": {
      "method": "POST",
      "endpoint": "/api/v1/ai/llm/invoke",
      "headers": { "Content-Type": "application/json" },
      "query": { "env": "dev" },
      "body": {
        "model_key": "ollama/qwen3:8b",
        "inputs": [{ "type": "text", "text": "解释一下 RAG" }],
        "params": { "temperature": 0.2, "max_tokens": 256 }
      }
    }
  }' | jq .
```
