# Quickstart – Agent Model Hub Connectivity & Governance

1. **Checkout feature branch & sync configs**  
   ```bash
   git checkout 010-agent-model-setting
   make deps
   ```
   Copy provider/routing templates into `backend/config/agents/providers.d/` and `routing.d/`.

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
     -d '{"tenantWhitelist":[{"tenantId":"demo","environment":"staging"}],"rolloutStrategy":"canary"}'
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
     --tenant-id demo-tenant \
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
   The drill script reports a spike, polls `/provider-quotas`, and (optionally) pings Grafana/PagerDuty webhooks so you can confirm alarms + manual enforcement. Tenant dashboards will reflect the new enforcement state within one minute.

7. **Observability checklist**  
   - Metrics: `agent.provider.*`, `agent.routing.*`, `agent.platform.*` visible in Grafana dashboards.
   - Logs: `internal/service/provider_registry` + `cost_quota` show audit IDs for every publish/enforce action.
   - Alerts: Ensure PagerDuty routes exist for cost anomaly, routing safe-mode, connector pause notifications.

8. **Regenerate契约 & 跑自动化测试**  
   ```bash
   make proto-lint proto-gen
   scripts/ci/agent_model_hub_tests.sh
   ```
   该脚本会串联 Buf lint/代码生成、HTTP 契约测试（密钥去敏）以及 Agent Model Hub 集成测试（Secret Rotation）。完成后再提交 `api/grpc/gen` 与 OpenAPI 变更。
