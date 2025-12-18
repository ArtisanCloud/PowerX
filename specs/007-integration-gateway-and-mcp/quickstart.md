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
        -H "X-PowerX-Tenant: tenant-001" \
        "$POWERX_BASE_URL/tenant/capabilities?channel=agent"
   ```
   确认返回的 `capability_id` 与 Admin 结果一致，同时多了 `grants`、`channels` 的裁剪字段。
3. 通过 `POST /tenant/invocations` 发起一次示例调用，并记录响应中的 `trace_id`：
   ```bash
   curl -X POST "$POWERX_BASE_URL/tenant/invocations" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "X-PowerX-Tenant: tenant-001" \
        -H "Content-Type: application/json" \
        -d '{
              "capability_id":"com.demo.template.generate",
              "idempotency_key":"demo-001",
              "preferred_protocol":"mcp",
              "payload":{"prompt":"hello"}
            }'
   ```
4. 使用 `GET /tenant/invocations/{trace_id}` 查看最终状态，确保 `protocol_used` 与 Selector 策略一致。

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
  - `powerx_capability_invoke_total`、`powerx_capability_invoke_latency_ms{protocol="mcp"}`、`integration_gateway_invocation_fallback_total`
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
