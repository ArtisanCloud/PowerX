# AI 设置：入口与 API Key 检测

`/settings/ai` 的「测试连接」只验证供应商入口和鉴权，不提交文本、图片、视频、音频或向量生成请求。该操作保留原有的凭据保存行为：验证成功才保存凭据及健康元数据，不修改默认模型路由，不激活画像，也不探测向量维度。

## HTTP 合同

### 插件后端服务入口 v1

服务能力为 `com.corex.ai.connection.test`，固定绑定 `core_internal`、`INVOKE`、`core://ai/connection`。Framework 的 PowerX 来源通过 `POST /api/v1/tenant/invocations` 调用，使用现有 API Key 或 STS，示例：

```json
{
  "capability_id": "com.corex.ai.connection.test",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "contract_version": 1,
      "operation": "test_connection",
      "modality": "llm",
      "provider": "ollama"
    }
  }
}
```

`model` 是可选的原生型号标识；没有选择型号也能检测入口。严格拒绝 `prompt`、`inputs`、生成参数、`env`、租户／身份覆盖、`base_url`、密钥、method、endpoint、headers 及其他未声明字段。当前租户由认证上下文确定，环境来自其 `ai.current_env`；只读取该租户、该环境已保存的提供商连接。未保存时返回 `409 AI_SETTINGS_PROVIDER_NOT_CONFIGURED`，不使用全局配置、其他租户／环境或主机默认实例替代。显式保存了空 Base URL 的连接可以使用提供商目录规定的默认地址。

服务入口只探测：不保存凭据或健康缓存，不修改画像／默认模型，不创建生成任务。同一凭据快照用于解析地址与密钥，避免并发修改时把一份配置的密钥发往另一份配置的地址。沿用 typed Core 的结果包装：HTTP 成功响应的 `data.result.result` 为下述业务结果，`data.result`／`data.payload` 是 Core invoker 的协议结果；Framework 客户端应解开两层 `result`：

```json
{
  "contract_version": 1,
  "ok": true,
  "env": "dev",
  "modality": "llm",
  "verification": {
    "provider": "ollama",
    "probe": "ollama_tags",
    "endpoint_reachable": true,
    "api_key_verified": false,
    "authentication": "not_required",
    "model_access_verified": false,
    "generation_submitted": false
  }
}
```

能力发布、租户 registration 和当前凭据 grant 缺一不可。API Key 的显式元组为 `scope=_scope.ai.connection.test`、`action=test`、`resource_type=api`、`resource_pattern=ai-connection-test`、`effect=allow`，Core grant 不设置插件 owner。STS 在该租户插件运行凭据的 `allowed_capabilities` 中精确包含该能力。允许探测不代表允许任何生成能力。

部署通过现有发布流程运行 `make capability-check`，再执行 `make capability-seed CAPABILITY_SEED_FLAGS='-capability-ids com.corex.ai.connection.test'`。Seeder 只同步该正式能力及其租户注册，不附加 Key grants；服务 registration 仅保留固定 `core_internal` adapter，Admin REST 只保留在能力发布记录中。已知旧混合注册会追加新版本，保留授权、状态、禁用位和历史；未知自定义绑定明确报冲突。

可由租户管理员在正式单 Key `/admin/integration/api-keys/:key_id/explicit-grants` 接口提交该服务 IAM Permission ID，为目标 Key 单独追加授权；不修改共享 Profile，也不轮换 Key。用同一 Key 检查 `grant-status=granted`，再确认真实调用 `protocol_used=core_internal`、`status=completed`、`fallback_used=false` 和 `generation_submitted=false`。未授权、撤销、停用 registration 或下架能力必须在探测前被拒绝。

Framework 应为“测试连接”使用独立 DTO 和此能力；按钮不依赖 prompt、图片、生成参数或生成任务幂等键。“试跑一次”继续使用现有生成能力。Framework 本地来源（例如自有 Ollama）由其本地 adapter 做只读探测；不能把服务凭据转发到下面的 Admin 入口，也不能用生成请求代替不支持的探测。

机器可读定义与共享样例分别见 [OpenAPI](ai-connection-test.v1.openapi.yaml) 和 [fixture](fixtures/ai-connection-test.v1.json)。现有 AI Media v2 契约及其摘要不变。

### Admin 用户入口

沿用 `POST /api/v1/admin/agents/test/connection`、当前登录用户 JWT 和原有 Admin RBAC。请求仍为 `env`、`modality` 及对应配置对象，配置包含 `provider`、`model`、可选 `baseURL` 和 `apiKey`。空密钥由 Core 按当前租户、环境、供应商读取已保存凭据；密钥不返回客户端。服务凭据必须使用正式的类型化能力接口。

成功响应的 `data` 示例：

```json
{
  "ok": true,
  "verification": {
    "provider": "agnes",
    "probe": "models",
    "endpoint_reachable": true,
    "api_key_verified": true,
    "authentication": "api_key",
    "model_access_verified": false,
    "generation_submitted": false
  }
}
```

成功仅证明入口和 Key 通过验证，不证明账户余额、具体型号的调用权限、生成能力、生成结果或媒体任务闭环。Ollama 的 `authentication=not_required`、`api_key_verified=false`，界面明确说明只验证了入口。

## 只读检测策略

| 供应商／协议 | 检测入口 |
|---|---|
| Agnes、OpenAI、Qwen／Qwen CN、目录声明为 Bearer + OpenAI 文本驱动的兼容供应商 | `GET {base_url}/models` |
| OpenRouter | `GET {base_url}/auth/key` |
| Ollama | `GET {base_url}/api/tags`，兼容配置地址的 `/v1` 或 `/api` 后缀 |
| 其他供应商／TC3 | 返回不支持，不调用生成接口代替检测 |

整个检测最长 10 秒，无自动重试，不跟随重定向，响应体上限 2 MiB。API Key 模式先检查当前 Key 的成功响应及 JSON 格式，再对同一只读入口使用固定无效 Key 做反向校验；无效 Key 必须被 401／403 拒绝。最多两个 GET。公开目录或接受任意 Key 的入口不能证明鉴权有效，返回明确错误。

## 结构化错误

错误沿用 Core 统一响应中的 `error_code`、`details` 和追踪字段，不向浏览器透传供应商原始错误正文、密钥或内部地址。`details.generation_submitted` 始终为 `false`；有供应商响应时保留 `details.upstream_status`。

| Core HTTP 状态 | `error_code` | 含义 |
|---|---|---|
| 422 | `AI_CONNECTION_CONFIG_INVALID` | 配置、型号或入口格式无效／缺少凭据 |
| 422 | `AI_CONNECTION_AUTH_FAILED` | 供应商拒绝当前 Key 或检测入口权限；上游 401／403 不映射为 Core 登录失效 |
| 501 | `AI_CONNECTION_PROBE_UNSUPPORTED` | 没有支持的只读鉴权检测协议 |
| 501 | `AI_CONNECTION_AUTH_UNVERIFIABLE` | 无效 Key 未被拒绝，无法证明当前 Key 有效 |
| 429 | `AI_CONNECTION_RATE_LIMITED` | 供应商限流 |
| 502 | `AI_CONNECTION_REDIRECT_REJECTED` | 入口发生跳转，未转发 Key |
| 502 | `AI_CONNECTION_UPSTREAM_FAILED` | 供应商接口异常 |
| 502 | `AI_CONNECTION_RESPONSE_INVALID` | 响应格式／大小不符合检测合同 |
| 502 | `AI_CONNECTION_UNREACHABLE` | 网络或连接错误 |
| 504 | `AI_CONNECTION_TIMEOUT` | 连接或读取响应超时 |
| 500 | `AI_CONNECTION_CREDENTIAL_SAVE_FAILED` | 验证通过但凭据保存失败 |

验证失败不保存本次凭据，不产生生成任务。无只读检测能力或鉴权无法确认时健康状态为 `unknown`，不能显示已验证成功。

## 页面与验收

- 访问 `http://127.0.0.1:3030/settings/ai`，选择图片或视频配置，点击「测试连接」。当前按钮显示进度，两个测试按钮暂时禁用，配置表单保留；检测及随后凭据刷新不触发全屏 Loading。
- 媒体旁边的按钮准确标为「检查参数」：只检查本地参数，不代表鉴权或生成验收。LLM 独立按钮标为「试跑文本（调用模型）」，实际执行文本调用。
- 检查有效 Key、无效 Key、错误地址、供应商限流和超时后的提示与重试；检查桌面和窄屏按钮布局。
- Network 中供应商侧只能出现只读检测 GET；不得调用图片／视频创建、文本补全或 embedding 入口。具体生成验收仍通过正式媒体调用及任务接口单独进行。

回归入口：

```sh
cd backend && go test -race ./internal/server/ai/connectionprobe ./internal/service/agent ./internal/transport/http/admin/agent
```

```sh
cd web-admin && npx vitest run tests/unit/ai tests/unit/components/secret-input.spec.ts
```

原 Admin 入口证据见 [ai-settings-connection-validation-20261023.json](evidence/ai-settings-connection-validation-20261023.json)。插件服务入口新增独立 v1 调用契约，不新增迁移或数据库结构；Framework 需按该契约单独接入按钮和客户端。
