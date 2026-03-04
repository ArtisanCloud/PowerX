# Quickstart – Agent Model Hub Connectivity & Governance

1. **Checkout feature branch & sync configs**  
   ```bash
   git checkout 010-agent-model-setting
   make deps
   ```
   Copy provider/routing templates into `backend/config/agents/providers.d/` and `routing.d/`.

> 结构说明：  
> - **Agent 框架驱动** 在 `backend/internal/server/agent/drivers/eino`  
> - **AI 多模态驱动** 在 `backend/internal/server/ai/drivers/{provider}`  
> - **模态工厂** 在 `backend/internal/server/ai/factory/{llm,vlm,...}`（仅入口+分发）

> 模型路由约定：  
> - **provider_key 仅鉴权**（provider 级别，覆盖该 provider 下所有 app/model）  
> - **model_key 仅路由**：`provider/model` 或 `provider/app:model`  

2. **Register a sandbox provider**  
   ```bash
   curl -X POST https://api.powerx.local/internal/providers/register \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     -d @examples/provider-openai.json
   ```
   Verify draft entry via CLI: `powerx providers list --status draft`.

3. **Run automated validation & publish**  
   ```bash
   npm run provider:validate -- provider-id <uuid>
   curl -X POST https://api.powerx.local/internal/providers/<uuid>/publish \
     -d '{"tenantWhitelist":[{"tenant_uuid":"demo","environment":"staging"}],"rolloutStrategy":"canary"}'
   ```
   Confirm Vault secret refs exist: `vault kv get powerx/providers/<uuid>`.

4. **Author routing policy & simulate**  
   ```bash
   curl -X POST https://api.powerx.local/internal/model-routing/policies -d @examples/routing-demo.json
   node scripts/ops/routing-simulator.mjs \
     --tenant demo-tenant \
     --scenario critical_tasks \
     --policy rc-$(date +%Y%m%d) \
     --token "$ADMIN_TOKEN"
   ```
   After BU approval, promote via `curl -X POST .../model-routing/rollback` for rollbacks if telemetry drops.

5. **Configure connector instance**  
   ```bash
   curl -X POST https://api.powerx.local/internal/connector-platforms/coze/instances -d @examples/coze-instance.json
   ```
   Trigger a workflow and ensure callbacks are signed (`platform-webhook-guard` flag on).

6. **Feed cost telemetry & confirm enforcement flow**  
   ```bash
   node scripts/qa/provider-drill.mjs \
    --tenant-uuid demo-tenant \
     --provider-id 2b92d17c-9d35-4c22-8a8d-24ddf9a6f1d3 \
     --env staging \
     --spike 1500 \
     --events 5 \
     --api-base https://api.powerx.local/internal \
     --token "$ADMIN_TOKEN" \
     --grafana-url https://grafana.powerx.local/d/cost-guard \
     --pagerduty-url https://events.pagerduty.com/v2/enqueue \
     --pagerduty-routing-key "$PD_KEY"
   ```
   The drill script reports a spike, polls `/provider-quotas`, and (optionally) pings Grafana/PagerDuty webhooks so you can confirm alarms + manual enforcement。  
   - 打开 `http://localhost:3030/settings/ai/cost`，验证「成本守护」子菜单中实时预算快照/异常列表是否刷新。  
   - Grafana 中的「Cost & Quota Guard」看板应出现同名租户的 `agent.provider.alert_total` 与 `agent.provider.cost.anomaly` 计数。

7. **Observability checklist**  
   - Metrics：`agent.provider.onboard_duration`、`agent.provider.health_score`、`agent.routing.hit_rate`、`agent.platform.latency_p95`、`agent.provider.cost_total`、`agent.provider.cost_delta_percent`、`agent.provider.alert_total`。  
   - Logs：`internal/service/provider_registry`、`model_routing`、`connector_guard`、`cost_quota` 会打印 Trace ID + 审计 ID，便于跨组件串联。  
   - Alerts：确认 PagerDuty 路由存在（成本异常、路由 safe-mode、平台 auto-pause）；Grafana Model Hub & Cost Guard 两个 dashboard 都能显示最新指标。

8. **Regenerate契约 & 跑自动化测试**  
   ```bash
   make proto-lint proto-gen
   scripts/ci/agent_model_hub_tests.sh
   ```
   该脚本会串联 Buf lint/代码生成、HTTP 契约测试（密钥去敏）以及 Agent Model Hub 集成测试（Secret Rotation）。完成后再提交 `api/grpc/gen` 与 OpenAPI 变更。

9. **SC-001 Benchmark（可选但推荐）**  
   ```bash
   node scripts/qa/provider-onboard-benchmark.mjs \
     --config specs/010-agent-model-setting/examples/provider-benchmark.json \
     --token "$ADMIN_TOKEN" \
     --api http://127.0.0.1:8077/api \
     --output tmp/provider-onboard-benchmark.json
   ```
   脚本会批量注册/校验/发布样例 provider，计算上线耗时 p95 并扫描 HTTP 响应是否包含 `api_key` / `secret` 等敏感字段，输出 JSON 报告供稽核。

10. **SC-002 Routing Drill（hit ≥90%、fallback ≥95%、safe-mode ≤5min）**  
    ```bash
    node scripts/ops/routing-simulator.mjs \
      --tenant demo-tenant \
      --scenario critical_tasks \
      --policy rc-latest \
      --token "$ADMIN_TOKEN" \
      --hit-threshold 0.9 \
      --fallback-threshold 0.95 \
      --require-safe-mode \
      --safe-mode-window 300 \
      --output tmp/routing-simulator.json
    ```
   Report 中的 `slo` 字段会告诉你是否满足 SC-002；若 `requireSafeMode` 为真，则脚本会检查 safe-mode 在 5 分钟窗口内是否被命中。同时打开 Grafana「Model Hub Overview」查看 `agent.routing.hit_rate`/`fallback_total` 图表是否与报告一致，并确认 safe-mode 告警触发与 5 分钟内自动回滚。

11. **Safe-Mode Chaos Drill（T043a）**  
    ```bash
    node scripts/qa/routing-chaos.mjs \
      --tenant demo-tenant \
      --token "$ADMIN_TOKEN" \
      --api http://127.0.0.1:8077/api \
      --task-type chat/general \
      --output tmp/routing-chaos-report.json
    ```
   该脚本先记录正常路由，再通过 `/model-routing/safe-mode` 注入 “主模型故障”，确认路由落到 fallback 并在恢复后自动清除 safe-mode。运行期间请同时查看 Grafana `agent.routing.safe_mode_active` 曲线以及 Redis safe-mode 键，确保告警链路与缓存切换都按预期执行。

12. **SC-004 Cost Guard Drill（T045）**  
    ```bash
    node scripts/qa/provider-drill.mjs \
     --tenant-uuid demo-tenant \
      --provider-id 2b92d17c-9d35-4c22-8a8d-24ddf9a6f1d3 \
      --env staging \
      --spike 1500 \
      --events 5 \
      --api-base https://api.powerx.local/internal \
      --token "$ADMIN_TOKEN" \
      --alert-timeout 300000 \
      --alert-targets anomaly,enforcement_required \
      --output tmp/provider-drill.json
    ```
    Drill 会回传 `alertResult`（等待耗时、是否命中目标状态）和 `triggeredStatuses`，确保告警 <5 分钟送达。完成后可用 `--action throttle` 测试人工执行链路，并对照 Grafana「Cost & Quota Guard」和 PagerDuty 事件。
