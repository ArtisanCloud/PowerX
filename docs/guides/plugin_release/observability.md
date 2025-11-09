# Plugin Release 观测配置指南

以下检查表适用于任意启用了 `plugin_release` 模块的集群。每一步都依赖上一步，务必按照顺序执行，避免跳步。

## 前置条件
1. CoreX 服务已部署，且配置 `PLUGIN_RELEASE_ENABLE_PIPELINE_DEPLOYMENT=true`。
2. Prometheus 已对后台 Pod 的 `/metrics` 端点进行抓取，并带有 `powerx` job label。
3. Grafana 中具备 “Platform / Plugin Release” 文件夹，或你具备创建仪表盘的管理员权限。

## 第 1 步：启用指标导出
1. 确认后端配置中写入组件前缀：
   ```yaml
   pluginRelease:
     observability:
       alertRulePrefix: plugin_release
   ```
2. 重启后端服务，或在本地执行 `go test ./internal/service/plugin_release/...` 以注册 OpenTelemetry 直方图。
3. 在 Prometheus 中确认以下指标已经产出数据：
   - `plugin_release.hotload.latency_ms`
   - `plugin_release.pipeline.duration_seconds`
   - `plugin_release.canary.rollback_seconds`
   - `plugin_release.distribution.sla_seconds`

## 第 2 步：导入 Grafana 仪表盘
1. 在 Grafana 中新建 **Plugin Release Overview** 仪表盘。
2. 添加以下图表（可直接复制查询语句）：
   - **Local Hotload Latency**：`histogram_quantile(0.95, sum(rate(plugin_release_hotload_latency_ms_bucket[5m])) by (le))`
   - **Pipeline Duration**：`rate(plugin_release_pipeline_duration_seconds_sum[5m]) / rate(plugin_release_pipeline_duration_seconds_count[5m])`
   - **Canary Rollback Count**：`increase(plugin_release_canary_rollback_total[1h])`
   - **Distribution SLA Histogram**：`histogram_quantile(0.9, sum(rate(plugin_release_distribution_sla_seconds_bucket[15m])) by (le))`
3. 将仪表盘导出并保存到仓库保持版本一致：
   ```
   grafana dashboard export > docs/guides/plugin_release/observability-dashboard.json
   ```

## 第 3 步：下发 Prometheus 告警

1. 参考 `backend/internal/service/plugin_release/instrumentation/alerts.go` 中的 `BuildDefaultAlertSuite`，生成告警规则：

   ```yaml
   groups:
     - name: plugin-release
       rules:
         - alert: plugin_release_canary_rollback_sla_breach
           expr: sum(rate(plugin_release_canary_rollback_total[5m])) > 0
           for: 1m
           labels:
             severity: critical
             dashboard: plugin-release
           annotations:
             summary: Canary rollback triggered
             description: "请检查 px publish deploy 的日志与指标。"
   ```

2. 使用 `kubectl apply -f alerts.yaml` 或 Alertmanager CI 流水线将规则部署到集群。

3. 通过 `promtool test rules alerts_test.yaml` 校验表达式是否通过编译。

## 第 3.1 步：Web Admin UI 操作监控

PowerX Web Admin 集成了实时指标展示，便于运维人员快速了解系统状态：

### 页面内置监控面板

1. **「Marketplace 审核列表」页面**
   - 审核状态变更会触发实时指标更新
   - 审核详情页的 SLA 倒计时基于后端 `plugin_release.distribution.sla_seconds` 计算
   - 页面自动显示离截止时间 < 4 小时的预警提示

2. **E2E 监控测试**
   - Web Admin 包含完整的 Playwright E2E 测试，覆盖：
     - 离线包提交流程（T070-1）
     - Marketplace 审核操作（T070-2）
     - 详情页 SLA 监控显示（T070-3）
   - 运行测试：`cd web-admin && npm run test:e2e`
   - 测试报告保存在 `web-admin/test-results/`

### 关键指标映射

- UI 操作直接关联后端指标：
  - 审核提交 → `plugin_release.distribution.review_latency_seconds`
  - 状态变更 → `plugin_release.distribution.status_transition_total`
  - SLA 预警 → `plugin_release.distribution.sla_approaching_total`

### Grafana 仪表盘增强

在现有仪表盘中添加 UI 特定面板：

- **Recent Review Actions**：最近1小时审核操作时间序列
- **SLA Approaching Count**：即将超时的审核数量
- **UI Error Rate**：Web Admin 页面错误率（加载失败、API超时等）

## 第 4 步：演练与 Runbook
1. 执行一次示例灰度：`px publish deploy --plan-id <id> --batch-name batch-a`。
2. 检查仪表盘是否即时刷新：Local Hotload Latency p95 应低于 15 分钟，Canary Rollback Count 在回滚时递增。
3. 如告警触发，按照仪表盘中的链接跳转到 `specs/009-install-plugin-pxp/quickstart.md` 第 5 节查阅回滚指令，同时参考 `backend/internal/service/plugin_release/*/audit_hooks.go` 中的审计记录。
4. 将 Grafana 链接与告警结果写入 `backend/reports/plugin_release/dry_run.md`，便于后续审计追踪。
