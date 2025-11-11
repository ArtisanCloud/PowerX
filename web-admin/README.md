# PowerX Web Admin – AI 设置模块

本目录包含 Nuxt 4 管理端，重点模块为 “AI 设置” (AI Settings)。该模块现已拆分为两个子页面，方便按能力维度快速演示：

| 子菜单 | 路径 | 功能摘要 |
|--------|------|----------|
| 模型配置 | `/settings/ai` | Provider 凭证、模型参数、模态化 Ping/Quick Call、审计视图 |
| 成本守护 | `/settings/ai/cost` | 按租户展示预算快照、异常/告警、执行限流或降级动作，并链接到租户仪表盘 |

## 开发与构建

```bash
cd web-admin
npm install          # 初次安装依赖
npm run dev          # 本地开发，默认监听 http://localhost:3030
npm run build        # 生产构建（需先解决 lint 报错）
```

> 管理端菜单数据来自 `http://127.0.0.1:8077/api/v1/admin/menus`，后端新增 “AI Setting → 成本守护” 子节点后，前端会自动展开同层级下两个子菜单。

## 代码入口

- `app/pages/settings/ai/index.vue`：模型配置页面，复用 `useAISettingsStore`，负责环境切换与模态化测试。
- `app/pages/settings/ai/cost.vue`：成本守护页面容器，渲染 `CostQuotaPanel` 并对接 Pinia `useCostQuotaStore`。
- `app/components/settings/ai/cost/CostQuotaPanel.vue`：展示实时预算快照、异常列表、操作按钮（刷新、打开租户仪表盘）。
- `app/stores/costQuota.ts`：Pinia store，封装租户选择、Quota 拉取、错误态。
- `app/composables/api/services/costQuotaService.ts`：客户端 API 包装器，对应后端 `provider-quotas` 与 `provider-quotas/enforce`。

## i18n 与文案

- 所有新文案需添加到 `i18n/locales/zh-CN.json` 与 `i18n/locales/en-US.json`，再通过 `<i18n-t>` 或 `useI18n()` 引用。
- “成本守护”“模型配置”等菜单名称统一使用 i18n key：`menu.aiSettings.modelConfig`、`menu.aiSettings.costGuard`。
- 页面内提醒（例如 “预算快照刷新失败”）应位于 `i18n/modules/ai-settings.json`，避免硬编码。

## 故障排查

1. **菜单不展开**：确认后端菜单 API 返回 `children` 包含 `route: "/settings/ai/cost"`；前端 Sidebar 会根据 `activeRoute.startsWith(parent.route)` 自动展开。
2. **API 500**：检查后端 `GET /api/v1/internal/provider-quotas` 与 `POST /api/v1/internal/provider-quotas/enforce` 是否带上 `env`/`tenantId`，并查看 `backend/internal/transport/http/admin/agent_model_hub/cost_handler.go` 日志。
3. **Grafana 数据缺失**：确保后端 `agent.provider.*` 与 `agent.platform.*` 指标已抓取到 Prometheus；前端只负责展示 API 结果。

如需更多使用示例，可参考 `scripts/qa/provider-drill.mjs` 输出的 JSON 报告与 web-admin 画面做联动验证。*** End Patch***} to=functions.apply_patch
