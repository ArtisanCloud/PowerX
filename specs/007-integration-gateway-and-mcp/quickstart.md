## Quickstart: Capability Registry + Integration Gateway（多插件能力）

### 前置条件
- 已安装 Go 1.24、Buf CLI、px-plugin CLI，并在 `backend/` 下完成依赖下载（`make deps`）。
- Postgres、Redis、EventBus、OpenTelemetry Collector 已根据 `config/config.yaml` 配置，尤其是：
  ```yaml
  capability_registry:
    redis_prefix: "capability_registry:cache"
    event_topic_prefix: "integration.gateway"
    default_rate_limit:
      limit: 60
      burst: 120
      window_seconds: 60
  ```
- Agent Hub MCP Server 与 Workflow Builder 进程已通过 `make dev` 启动。
- 至少存在一个示例插件（可使用 `projects/demo-multi-plugin`）并成功执行 `px-plugin capabilities submit`。

### 鉴权规则（Gateway 全入口）

- 本特性统一采用 **单请求单凭证分流**。
- `Authorization: ApiKey <key>` 仅走 API Key 鉴权链路。
- `Authorization: Bearer <token>` 仅走 JWT 鉴权链路。
- 不采用 API Key 失败回退 JWT 的混合兜底策略。

### 鉴权数据模型检查（迁移后必做）

执行以下 SQL，确认 API Key 新链路表已存在：

```sql
SELECT tablename
FROM pg_tables
WHERE schemaname = 'public'
  AND tablename IN (
    'iam_api_key_profile',
    'iam_api_key_profile_permission',
    'integration_gateway_api_keys',
    'integration_gateway_api_key_permissions',
    'iam_api_key'
  )
ORDER BY tablename;
```

最小链路自检（Profile -> permission_ids -> key 快照）：

```sql
-- 1) 看某租户 profile
SELECT id, tenant_uuid, key, name, status
FROM public.iam_api_key_profile
WHERE tenant_uuid = '<TENANT_UUID>'
ORDER BY id;

-- 2) 看 profile 绑定 permission_ids
SELECT profile_id, permission_id, created_at
FROM public.iam_api_key_profile_permission
WHERE profile_id = <PROFILE_ID>
ORDER BY permission_id;

-- 3) 看 key 是否绑定 profile_id
SELECT uuid, tenant_uuid, profile_id, name, status, created_at
FROM public.integration_gateway_api_keys
WHERE tenant_uuid = '<TENANT_UUID>'
ORDER BY created_at DESC
LIMIT 10;

-- 4) 看 key 权限快照是否生成
SELECT api_key_uuid, scope, action, resource_type, resource_pattern, effect
FROM public.integration_gateway_api_key_permissions
WHERE api_key_uuid = '<KEY_UUID>'
ORDER BY created_at ASC;
```

### 步骤 0：自动播种 Event Fabric Topic/ACL
1. 在插件仓库提供 `event_fabric.yaml`（支持放在 `config/`、`platform_capabilities/` 或包根目录），并声明 Topic/ACL 模板。Manifest 可引用 `{{ tenant_uuid }}`、`{{ plugin_id }}`、`{{ variables.cluster }}` 等变量。
2. 插件启用/升级后，安装 orchestrator 会自动调用 `event_fabric.SeedService` 播种 Topic 与 ACL；若记录已存在，则根据 binding 表跳过重复授权，无需手动访问 Admin Topic/ACL API。
3. 如需在本地或 CI 预览，运行：
   ```bash
   cd backend
   go run ./cmd/event_fabric_seed \
     --tenant "$TENANT_UUID" \
     --plugin "$PLUGIN_ID" \
     --dry-run
   ```
   不带 `--dry-run` 将实际写入（用于补齐权限）。
4. `scripts/capability_registry/verify.sh` 默认会在执行 capability sync 后自动跑一遍上述 dry-run，可通过 `--skip-event-seed` 跳过或用 `--event-seed-manifest` 指定自定义路径，使 CI 也能及时发现 manifest 模板错误。

### 步骤 1：触发并验证 Capability Sync（含缓存刷新）
1. 将 `.pxp` 包上传到 `tmp/plugins/`，执行 `make capability-sync`（包装 `cmd/capability_sync`）。
2. 通过 Admin API 查询最新目录：
   ```bash
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
        "$POWERX_BASE_URL/admin/capabilities?plugin_id=com.demo.multiplugin"
   ```
   响应应包含 `capability_id`、`protocols`、`capabilities_hash` 与 `policy`。
3. 检查 Redis 中的缓存键：`redis-cli keys 'capability_registry:cache:*' | xargs -r redis-cli ttl`，确认 TTL < 180s，必要时执行 `POST /admin/capabilities/cache:flush` 后重新回源。
4. 失败时查看 `backend/logs/capability_sync.log`，并关注 `capability.catalog.sync_failed` 事件。

### 步骤 2：验证管理端/租户 API 一致性
1. 管理员查看单个能力：
   ```bash
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
        "$POWERX_BASE_URL/admin/capabilities/com.demo.template.generate"
   ```
2. 租户查询授权能力：
   ```bash
   curl -H "Authorization: Bearer $TENANT_TOKEN" \
        "$POWERX_BASE_URL/tenant/capabilities?channel=agent"
   ```
   确认返回的 `capability_id` 与 Admin 结果一致，同时多了 `grants`、`channels` 的裁剪字段。
3. 通过 `POST /tenant/invocations` 发起一次示例调用，并记录响应中的 `trace_id`：
   ```bash
   curl -X POST "$POWERX_BASE_URL/tenant/invocations" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
              "capability_id":"com.demo.template.generate",
              "idempotency_key":"demo-001",
              "preferred_protocol":"mcp",
              "payload":{"prompt":"hello"}
            }'
   ```
   响应示例：
   ```json
   {
     "code": 200,
     "message": "success",
     "data": {
       "payload": {
         "choices": [{"content": "hello world"}],
         "usage": {"prompt_tokens": 12, "completion_tokens": 24}
       },
       "trace_id": "f76d5e1a-72d4-46cf-b3b7-0190c54d3ac3",
       "protocol_used": "http",
       "fallback_used": false
     },
     "timestamp": 1766559302
   }
   ```
   `data.payload` 固定承载真实业务响应体，而 `protocol_used/fallback_used` 有助于排查 Selector 是否进行了降级。
4. 使用 `GET /tenant/invocations/{trace_id}` 查看最终状态，确保 `protocol_used` 与 Selector 策略一致。

### 步骤 2.3：租户侧 SSE 变体调用（`/tenant/invocations/stream`）

`/tenant/invocations` 保持 JSON 包装响应；如果上游是 SSE 且需要“边收边转发”，请使用新增接口 `POST /tenant/invocations/stream`。

```bash
curl -N -X POST "$POWERX_BASE_URL/tenant/invocations/stream?env=dev" \
     -H "Authorization: Bearer $TENANT_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
           "capability_id":"com.corex.ai.llm.stream",
           "payload":{
             "method":"POST",
             "endpoint":"/api/v1/ai/llm/stream",
             "headers":{"Content-Type":"application/json"},
             "body":{
               "model_key":"openai/gpt-4o-mini",
               "inputs":[{"type":"text","text":"写一段自我介绍"}],
               "params":{"temperature":0.7,"max_tokens":256},
               "stream_options":{"include_usage":true}
             }
           }
         }'
```

预期返回 `Content-Type: text/event-stream`，并持续输出 `event: start/delta/done`（可选 `usage`）。

### 步骤 2.4：Gateway 代理 gRPC（Event Fabric 示例）
当能力声明 `preferred_protocol=gRPC` 时，`/tenant/invocations` 也会直接代理 gRPC 请求与响应。以平台能力 `com.corex.eventfabric.publish` 为例：

```bash
PAYLOAD=$(printf '{"orderId":"ord_123","amount":99.9}' | base64)

curl -sS -X POST "$POWERX_BASE_URL/tenant/invocations" \
     -H "Authorization: Bearer $TENANT_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
           "capability_id": "com.corex.eventfabric.publish",
           "preferred_protocol": "grpc",
           "payload": {
             "endpoint": "powerx.event_fabric.v1.EventDeliveryService",
             "rpc": "PublishEvent",
             "body": {
               "tenant_uuid": "'$TENANT_UUID'",
               "topic": "tenant-demo.orders.created",
               "event_id": "evt-demo-001",
               "trace_id": "trace-demo",
               "version": "v1",
               "payload_format": "json",
               "payload": "'$PAYLOAD'",
               "attributes": {
                 "source": "plugin.demo"
               }
             }
           }
         }'
```

- `payload.endpoint` / `payload.rpc`：覆盖 Registry 中声明的默认 Service/方法，便于临时调试或自定义。
- `body` 字段完全遵循 gRPC Proto（`PublishEventRequest`），其中 `payload` 是 **Base64** 字节串。
- 响应的 `data.payload` 会回放 gRPC `PublishEventResponse` JSON，例：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "payload": {
      "@type": "google.protobuf.Empty"
    },
    "trace_id": "fd7fb6ce-d0e9-4367-9b3f-0c73c0b71626",
    "protocol_used": "grpc",
    "fallback_used": false
  }
}
```

若请求体或 Service 名填写错误，`code` 会是 `integration.invoke_failed`，`data.payload` 保留底层 gRPC Status，便于直接排查。

### 步骤 2.5：宿主 vs Skeleton —— 调用底座能力
无论插件运行在 **宿主模式（嵌入 Web Admin）** 或 **Skeleton 模式（独立进程）**，都需要先通过 `/tenant/capabilities` 发现 `source=corex` 的平台能力，再由 `/tenant/invocations` 或公开 OpenAPI/gRPC 入口完成调用。

1. **能力发现（Host/Skeleton 通用）**
   ```bash
   curl -H "Authorization: Bearer $TENANT_TOKEN" \
        "$POWERX_BASE_URL/tenant/capabilities?source=corex&channel=media"
   ```
   响应中可见 `com.corex.media.assets.read/manage` 等平台能力；若租户尚未授权，会返回 403 并提示所缺少的 Tool Grant。

2. **Insomnia 配置示例**
   - **Request**: `POST {{POWERX_BASE_URL}}/tenant/invocations`
   - **Headers**:
     - `Authorization: Bearer {{TENANT_TOKEN}}`
     - `Content-Type: application/json`
   - **Body**:
     ```json
     {
       "capability_id": "com.corex.media.assets.manage",
       "idempotency_key": "demo-upload-001",
       "preferred_protocol": "rest",
       "payload": {
         "action": "upload",
         "filename": "hero.png",
         "mime_type": "image/png"
       }
     }
     ```
   - **Workspace Variables**: `POWERX_BASE_URL`, `TENANT_TOKEN`, `TENANT_UUID`，可通过 STS/ServiceAccount 获取，宿主与 Skeleton 场景仅 token 来源不同。

3. **直接访问公开 Media OpenAPI（`{APIPrefix}/media/assets`）**
   - `APIPrefix` 默认 `/api/v1`，实际以 `cfg.Server.APIPrefix` 为准，可重写为 `/api/admin/v1`、`/api/v2` 等。
   - cURL 上传示例：
     ```bash
     curl -X POST "$POWERX_BASE_URL/media/assets" \
          -H "Authorization: Bearer $TENANT_TOKEN" \
          -F "file=@samples/logo.png" \
          -F 'metadata={"title":"demo"}'
     ```
   - 预签名示例：
     ```bash
     curl -X POST "$POWERX_BASE_URL/media/assets/{asset_uuid}/presign" \
          -H "Authorization: Bearer $TENANT_TOKEN" \
     ```
   该路径由 `backend/internal/transport/http/openapi/media` 提供，仅校验租户身份，不再依赖 Admin Router，因此插件（宿主或 Skeleton）均可复用。

### 步骤 2.6：管理端开放能力页面（IsRoot 专用）
- 使用 `IsRoot` 管理员登录 Web Admin，打开 **设置 > 开放能力**（非 Root 账号不会看到该入口）。
- 页面按模块（Media、Event、Scheduler、Knowledge、Workflow 等）展示：
  - 能力数量（来自 Capability Registry，`source=corex`）
  - 支持的协议标签（REST/gRPC/MCP/Workflow）
  - 最新 `capabilities_hash` 与状态 Badge（active/disabled）
  - 调试入口（`/tenant/invocations` 样例、OpenAPI 链接、MCP Tool 名称）
- 点击模块卡片可展开“复制 cURL/Insomnia snippet”“跳转到 OpenAPI / `/media/assets`”等操作，使宿主与 Skeleton 插件在上线前即可验证平台能力。
- 如需刷新数据，可点击“立即同步”按钮重新从 Capability Registry 读取。

### 步骤 2.7：Agent 与多模态对外调用（平台能力）

> 这些接口走统一的 tenant 鉴权与租户隔离：`agent_id/session_id/model_key` 必须属于当前租户；`model_key` 允许使用该租户已配置 Profile 或已测试通过且凭据已保存的 provider。租户仅从 JWT claims 解析，不支持租户 header fallback。

1. **Agent 非流式调用（需要 session）**
   ```bash
   curl -sS -X POST "$POWERX_BASE_URL/agents/invoke?env=dev" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
          "agent_id": "agent-uuid",
          "session_id": "session-uuid",
          "message": "帮我总结一下这份文档"
        }'
   ```
2. **Agent SSE 流式输出（需要 session）**
   ```bash
   curl -N "$POWERX_BASE_URL/agents/stream/sse?env=dev&q=你好&agent_uuid=agent-uuid&session_uuid=session-uuid" \
        -H "Authorization: Bearer $TENANT_TOKEN"
   ```
3. **Agent 会话（Session）**
   - **创建会话**
     ```bash
     curl -sS -X POST "$POWERX_BASE_URL/agents/sessions?env=dev" \
          -H "Authorization: Bearer $TENANT_TOKEN" \
          -H "Content-Type: application/json" \
          -d '{"agentUuid":"agent-uuid"}'
     ```
   - **会话列表**
     ```bash
     curl -sS "$POWERX_BASE_URL/agents/sessions?env=dev&agent_uuid=agent-uuid&limit=20" \
          -H "Authorization: Bearer $TENANT_TOKEN"
     ```
   - **会话详情**
     ```bash
     curl -sS "$POWERX_BASE_URL/agents/sessions/{session_id}?env=dev" \
          -H "Authorization: Bearer $TENANT_TOKEN"
     ```
   - **会话消息**
     ```bash
     curl -sS "$POWERX_BASE_URL/agents/sessions/{session_id}/messages?env=dev&limit=50" \
          -H "Authorization: Bearer $TENANT_TOKEN"
     ```
   - **会话内对话（非流式）**
     ```bash
     curl -sS -X POST "$POWERX_BASE_URL/agents/sessions/{session_id}/invoke?env=dev" \
          -H "Authorization: Bearer $TENANT_TOKEN" \
          -H "Content-Type: application/json" \
          -d '{"message":"你好，帮我总结一下今天的待办"}'
     ```
   - **会话内对话（SSE 流式）**
     ```bash
     curl -N "$POWERX_BASE_URL/agents/sessions/{session_id}/stream/sse?env=dev&q=你好" \
          -H "Authorization: Bearer $TENANT_TOKEN"
     ```
   - **归档/删除**
     ```bash
     curl -sS -X POST "$POWERX_BASE_URL/agents/sessions/{session_id}/archive?env=dev" \
          -H "Authorization: Bearer $TENANT_TOKEN"
     curl -sS -X DELETE "$POWERX_BASE_URL/agents/sessions/{session_id}?env=dev" \
          -H "Authorization: Bearer $TENANT_TOKEN"
     ```
4. **LLM 无状态调用**
   ```bash
   curl -sS -X POST "$POWERX_BASE_URL/ai/llm/invoke" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
          "model_key": "ollama/llama3",
          "inputs": [{"type":"text","text":"解释这段话"}],
          "params": {"temperature":0.2,"max_tokens":256}
        }'
   ```
5. **LLM 无状态流式调用（SSE）**
   ```bash
   curl -N -X POST "$POWERX_BASE_URL/ai/llm/stream?env=dev" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
          "model_key":"openai/gpt-4o-mini",
          "inputs":[{"type":"text","text":"写一段自我介绍"}],
          "params":{"temperature":0.7,"max_tokens":256},
          "stream_options":{"include_usage":true}
        }'
   ```
6. **LLM 会话流式调用（SSE）**
   ```bash
   curl -N "$POWERX_BASE_URL/ai/llm/sessions/{session_id}/stream?env=dev" \
        -H "Authorization: Bearer $TENANT_TOKEN"
   ```
5. **LLM 会话调用（创建 → 追加消息 → SSE 流式）**
   ```bash
   # 创建会话
   curl -sS -X POST "$POWERX_BASE_URL/ai/llm/sessions" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"model_key":"ollama/llama3"}'

   # 追加消息
   curl -sS -X POST "$POWERX_BASE_URL/ai/llm/sessions/{session_id}/messages" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
          "role":"user",
          "content":[{"type":"text","text":"这张图是什么"},{"type":"image_url","url":"https://.../a.png"}]
        }'

   # SSE 输出
   curl -N "$POWERX_BASE_URL/ai/llm/sessions/{session_id}/stream" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
   ```
5. **图像/视频/TTS/Embedding（无状态）**
   - 图像/视频/TTS 若未启用对应驱动，接口会返回 `202 Accepted`（占位），不影响整体联调。
   ```bash
   # Image
   curl -sS -X POST "$POWERX_BASE_URL/ai/image/invoke" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"model_key":"provider/image-model","inputs":[{"type":"text","text":"生成一张海报"}]}'

   # Video
   curl -sS -X POST "$POWERX_BASE_URL/ai/video/invoke" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"model_key":"provider/video-model","inputs":[{"type":"text","text":"生成 5 秒视频"}]}'

   # TTS
   curl -sS -X POST "$POWERX_BASE_URL/ai/tts/invoke" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"model_key":"provider/tts-model","inputs":[{"type":"text","text":"你好，PowerX"}]}'

   # Embedding
   curl -sS -X POST "$POWERX_BASE_URL/ai/embedding/invoke" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"model_key":"ollama/mxbai-embed-large","inputs":["a","b","c"]}'
   ```

### 步骤 3：MCP 工具端到端
1. 在 MCP Client 中运行 `tools/list`，可见 `com.demo.template.generate` 的 schema 与 `tool_scope`。
2. 执行：
   ```json
   {
     "tool": "com.demo.template.generate",
     "arguments": {
       "tenant_uuid": "tenant-001",
       "payload": {"prompt": "hello"}
     }
   }
   ```
3. 断开 MCP 连接或模拟故障（停止插件进程），在下一次调用中观察 Selector fallback：HTTP 响应 `protocol_used=gRPC`、`fallback_used=true`，并在日志中看到 `integration.gateway.invocation.fallback`。

### 步骤 4：Workflow 模板导入与手动升级
1. 登录 Workflow Builder，触发 Catalog 刷新（或调用 `POST /admin/workflow/catalog:refresh`）。
2. 在 UI 中拖拽 `com.demo.workflow.quality_review` 模板，完成一个简单编排并发布。
3. 更新插件 Workflow 模板（例如修改某节点参数），再次执行 Capability Sync。此时 Builder 会提示“检测到新的 `capabilities_hash`，需确认升级”。
4. 管理员调用：
   ```bash
   curl -X POST "$POWERX_BASE_URL/admin/workflow-templates/{templateId}/upgrade" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"capabilities_hash":"<latest_hash>","reason":"qa verified"}'
   ```
   成功后 Workflow UI/CLI 重新加载 Catalog，此时 `needs_upgrade=false`，Workflow Engine/Agent Hub 才会切换到新版本。

### 步骤 5：观测与排障
- 在 Prometheus 中关注：
  - `powerx_capability_invoke_total{protocol="http"}`、`powerx_capability_invoke_total{protocol="grpc"}`、`powerx_capability_invoke_latency_ms{protocol="mcp"}`、`integration_gateway_invocation_fallback_total`
  - `powerx_workflow_template_snapshot_total{needs_upgrade="true"}` 追踪模板升级待办
  - `powerx_workflow_invocation_total{template_id="tpl.demo.workflow"}` 验证 Workflow Telemetry
- 通过 `trace_id` 在 Loki / Elasticsearch 检索日志，确认 HTTP、gRPC、MCP、Workflow 节点均串联。
- 订阅 `integration.gateway.invocation.failed`、`integration.gateway.catalog.sync_failed` Topic，核对 InvocationTrace/Audit 表内容。
- 若缓存与数据库版本不一致，可执行 `POST /admin/capabilities/cache:flush`（后续 task 实现）强制回源，再次校验 Redis 键与事件刷新。

### 步骤 6：一键验证脚本
1. 设置环境变量：
   ```bash
   export POWERX_BASE_URL="https://powerx.dev/api/v1"
   export ADMIN_TOKEN="..."
   export TENANT_TOKEN="..."
   export TENANT_UUID="tenant-001"
   export PLUGIN_ID="com.demo.multiplugin"
   export CAPABILITY_ID="com.demo.template.generate"
   ```
2. 运行脚本：
   ```bash
   scripts/capability_registry/verify.sh \
     --artifact tmp/plugins/demo.pxp \
     --plugin-id "$PLUGIN_ID" \
     --capability-id "$CAPABILITY_ID" \
     --tenant-uuid "$TENANT_UUID"
   ```
3. 脚本会：
   - 执行 `make capability-sync`
   - 调用 Admin/Tenant API 对比 `capabilities_hash`
   - 发起一次租户调用（输出 `trace_id`）
   - 刷新并列出 Workflow 模板，提示需升级的模板
   执行结束后检查 `powerx_workflow_template_snapshot_total`/`powerx_workflow_invocation_total` 是否出现最新样本。
