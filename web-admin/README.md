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

开发中间文件使用 `.nuxt`，生产构建使用 `.nuxt-build`，最终部署产物仍为 `.output`。开发服务运行时执行普通生产构建不会覆盖开发服务的资源清单。特殊验证可用 `POWERX_NUXT_BUILD_DIR`、`POWERX_NITRO_OUTPUT_DIR` 分别指定中间目录和部署输出目录；并行进程应使用不同目录。

`npm run dev` 启动前会自动回收旧验证构建产物。规则见 `dev-artifacts.config.json`：按相同后缀将 `.nuxt-*`／`.output-*` 配成一组，保留最近 3 组，并至少保留 7 天；任一文件刚更新也会保护整组。仅管理名称含 `validation` 或日期且具有 Nuxt／Nitro 生成标记的目录。默认开发、正式构建、恢复目录、Git 跟踪文件和已配置的输出路径均受保护；进程仍在使用目录、另有本项目开发／构建任务运行，或无法完整检查占用时，本轮不删除。不会启动常驻清理进程。

查看具体清单用 `npm run artifacts:preview`；手动按同一规则执行用 `npm run artifacts:clean`。执行前会再次检查占用、目录身份和最近修改，预览后新生成或被替换的目录不会按旧计划删除。直接调用 `nuxt dev` 会绕过 npm 的启动前清理，请沿用 `npm run dev`。

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

若页面白屏且多个 `/_nuxt/*.js` 返回 404，先核对当前进程是 `nuxt dev` 还是生产预览，以及页面引用的资源是否属于该进程。旧版配置中，生产构建与开发服务共用 `.nuxt`，可能使开发页面误引用生产脚本。更新目录隔离配置后，旧开发进程若未自动重新加载，应由服务所有者在原终端停止，再运行 `npm run dev -- --host 127.0.0.1 --port 3030`；无需改动插件安装记录。

1. **菜单不展开**：确认后端菜单 API 返回 `children` 包含 `route: "/settings/ai/cost"`；前端 Sidebar 会根据 `activeRoute.startsWith(parent.route)` 自动展开。
2. **API 500**：检查后端 `GET /api/v1/internal/provider-quotas` 与 `POST /api/v1/internal/provider-quotas/enforce` 是否带上 `env`/`tenantUuid`，并查看 `backend/internal/transport/http/admin/agent_model_hub/cost_handler.go` 日志。
3. **Grafana 数据缺失**：确保后端 `agent.provider.*` 与 `agent.platform.*` 指标已抓取到 Prometheus；前端只负责展示 API 结果。

如需更多使用示例，可参考 `scripts/qa/provider-drill.mjs` 输出的 JSON 报告与 web-admin 画面做联动验证。
