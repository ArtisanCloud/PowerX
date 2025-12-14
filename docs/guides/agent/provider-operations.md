# Agent Model Hub 操作手册

面向运维 / 平台同学的逐步指引，涵盖 Provider 配置、验证、上线以及使用情况观测。所有操作均基于 `010-agent-model-setting` 功能集。

> **环境约定**
> - Web 管理端：`http://localhost:3030`
> - 后端 API：`http://127.0.0.1:8077/api`
> - CLI/脚本：仓库根目录运行，Node 20 + Go 1.24。

## 1. 配置 Provider

1. 浏览器访问 `http://localhost:3030/settings/ai`，选择目标环境（默认 `default`）。  
2. 在左侧「模型配置」子菜单中填写 Provider 信息：  
   - **基础信息**：名称、Primary Endpoint、能力标签、Region。  
   - **凭证**：只输入明文 `api_key` / `secret`；保存时会自动密封到 Vault。  
   - **租户白名单**：确保灰度范围明确。  
3. 点击「保存」，等待 toast 成功提示。保存失败可在页面底部看到后端返回的具体错误。
4. 切换到「测试连接 / 快速调用」区域：  
   - 选择模态（LLM、Image、TTS…），填入提示词。  
   - 点击「Test Connection」或「Quick Call」验证连通性；若失败，页面会返回后端错误并记录审计。

> **参考 API**：上述操作对应 `POST /api/internal/providers/register`、`POST /api/internal/providers/:id/validate`。

## 2. 运行自动化验证

### 2.1 Provider Validator

用于批量回放真实 API 调用，生成健康报告：

```bash
node scripts/ops/provider-validator.mjs \
  --config backend/config/provider-openai.json \
  --provider-id <provider-uuid> \
  --suite smoke \
  --output tmp/provider-health.json \
  --api http://127.0.0.1:8077/api \
  --token "$ADMIN_TOKEN"
```

输出的 JSON 会携带 `stats`、每个模态的成功/失败详情，可上传至 MinIO 审计桶。

### 2.2 Onboard Benchmark（SC-001）

验证 95% Provider 能在 24 小时内上线且无明文泄露：

```bash
node scripts/qa/provider-onboard-benchmark.mjs \
  --config specs/010-agent-model-setting/examples/provider-benchmark.json \
  --token "$ADMIN_TOKEN" \
  --api http://127.0.0.1:8077/api \
  --output tmp/provider-onboard-benchmark.json
```

报告字段：
- `p95Hours`：上线耗时 p95，需 ≤ 24 小时。  
- `secretViolations`：若包含 `api_key`、`secret` 等字符串则立即 FAIL。  
- `runs[]`：每个 Provider 注册 → 验证 → 发布的详细记录。

## 3. 发布 & 观测

1. 在 `http://localhost:3030/settings/ai` 中点击「发布」，选择灰度策略：  
   - 「模型配置」子菜单 -> Provider 详情 -> 发布按钮。  
2. 发布前需确保 Validator suite 通过；若失败后端会返回 `validation_status=fail`。  
3. 发布后可在同页看到 Rollout 状态（draft / gray / live）及 Audit ID。

### 3.1 成本守护面板

`http://localhost:3030/settings/ai/cost` 提供租户预算快照，包含：
- 实时 usage/limit、异常状态和最近的 Enforcement 操作。  
- 「打开租户仪表盘」链接跳转至 `http://localhost:3030/dashboards/tenants/<tenantId>` 查看路由命中率、连接器状态、成本趋势。

### 3.2 Drill & Chaos 脚本

| 目的 | 命令 | 说明 |
|------|------|------|
| 成本金丝雀 (SC-004) | `node scripts/qa/provider-drill.mjs --tenant-uuid demo --spike 1500 --events 5 --alert-timeout 300000 --token "$ADMIN_TOKEN"` | 上报 usage，轮询 `/provider-quotas`，并等待告警状态出现，`alertResult` 会记录耗时与状态。 |
| 路由 SLO (SC-002) | `node scripts/ops/routing-simulator.mjs --tenant demo --scenario critical_tasks --token "$ADMIN_TOKEN" --require-safe-mode` | 输出 `slo` 字段（hit ≥90%、fallback ≥95%、safe-mode ≤5 min）。 |
| Safe-mode 混沌 (T043a) | `node scripts/qa/routing-chaos.mjs --tenant demo --task-type chat/general --token "$ADMIN_TOKEN"` | 自动启用/关闭 safe-mode，验证 fallback & recovery。 |

Drill 类脚本的输出位于 `tmp/*.json`。Grafana 推荐查看：
- 「Model Hub Overview」面板：`agent.provider.*`, `agent.routing.*`, `agent.connector.*`。  
- 「Cost & Quota Guard」面板：`agent.provider.cost_total`、`agent.provider.alert_total`、`agent.provider.cost.anomaly`。

## 4. 审计与合规

1. `backend/tests/integration/agent_model_hub/audit_latency_test.go` 验证 `provider.published` / `cost_quota.enforcement` 审计在 <1s 内可查询。  
2. 日常巡检可在数据库 `agent.audit_events` 表（或审计 API）以 `ResourceType=agent.provider_profile` 、`agent.cost_quota_ledger` 查询。  
3. 若需要离线稽核，可以执行：
   ```bash
   cd backend && go test ./tests/integration/agent_model_hub -run TestAuditLatencyPublishAndEnforce
   ```
   该测试会打印审计写入耗时并确保与真实 HTTP handler 一致。

## 5. 故障排查速查

| 症状 | 排查步骤 |
|------|----------|
| 保存 Provider 报错 “provider registry unavailable” | 检查 `backend/internal/app/shared/deps.go` 是否正确注入 `AuditSvc` / `TenantKeySvc`；数据库连接是否健康。 |
| Validator 报 `fetch failed` | 确认 config 中的 `env.API_KEY` 是否正确设置；若调用外部 API 需保证网络出口可达。 |
| 成本告警迟迟未触发 | 使用 `provider-drill.mjs --verbose --alert-timeout 600000`，检查 `alertResult.states` 是否出现 `anomaly`；同时查看 Grafana 是否接入 `agent.provider.cost_total`。 |
| Safe-mode 无法关闭 | 调用 `POST /api/internal/model-routing/safe-mode`，传入 `{"tenantScope":"<scope>","enabled":false}`；若 Redis 缓存仍存在，可手动删除 `agent:modelhub:safe_mode:<env>:<tenant>`。 |

## 6. 常用链接

- 管理端：`http://localhost:3030/settings/ai` / `http://localhost:3030/settings/ai/cost`  
- API：`http://127.0.0.1:8077/api/internal/providers/*`, `/model-routing/*`, `/provider-quotas/*`  
- Grafana（示例）：`https://grafana.powerx.local/d/model-hub`, `https://grafana.powerx.local/d/cost-guard`

> 本文档位于 `docs/guides/agent/provider-operations.md`，更新时请同步 `tasks.md` / quickstart，保持脚本命令一致。
