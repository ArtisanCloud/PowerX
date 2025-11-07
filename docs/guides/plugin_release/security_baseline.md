# Plugin Release 安全基线检查清单

当新的环境准备开放 plugin_release API 时，请严格按照以下步骤执行，每一步都需提供可验证的结果。

## 第 0 步：输入与责任人
- **平台负责人**：负责配置管理的运维/平台工程师。
- **安全审核**：确认配置与企业安全基线一致的 Reviewer。
- **所需文件**：`config.yaml`、Helm `values.yaml`、Feature Flag 控制台访问权限。

## 第 1 步：Feature Flag
1. 在配置管理系统中写入：
   ```yaml
   pluginRelease:
     featureFlags:
       enableLocalInstall: true          # 仅 dev/staging
       enablePipelineDeployment: true    # 全环境
       enableOfflineDistribution: true   # Marketplace 离线渠道上线后再启用
   ```
2. 在 CMDB/工单中记录引用本指南的备注。
3. 重启后端或执行 `make reload`，确保 Flag 生效。

## 第 2 步：运行时阈值
1. 配置回滚与包体限制：
   ```yaml
   pluginRelease:
     localInstall:
       maxArtifactSizeMB: 512
     runtime:
       rollbackTimeoutSeconds: 300
     distribution:
       offlineBucket: plugin-release-artifacts
       offlinePrefix: packages
       escalationThreshold: 2
   ```
2. 执行 `go test ./internal/service/plugin_release/local ./internal/service/plugin_release/runtime`，验证守卫逻辑覆盖 artifact 大小与自动回滚 SLA。

## 第 3 步：访问控制
1. Admin HTTP 路由必须包装 `AdminOnlyMiddleware`（参考 `backend/internal/transport/http/admin/plugin_release/routes.go`）。
2. OpenAPI 租户路由需强制携带 Authorization 头（`offline_import_handler.go` 为实现示例）。
3. gRPC 服务需部署在 API Gateway 后并通过 mTLS/JWT 鉴权，仅放行 `powerx publish` CLI 与受信自动化账号。

## 第 4 步：审计与日志
1. 确认以下审计钩子生效：
   - `local/audit_hooks.go`
   - `pipeline/audit_hooks.go`
   - `distribution/audit_hooks.go`
2. 运行 `scripts/ci/run_quickstart.sh`，检查日志中是否出现 `plugin_release.local_install.*`、`plugin_release.plan.generated`、`plugin_release.distribution.*` 等审计事件。
3. 将 `backend/reports/plugin_release/dry_run.md` 归档到合规存储，作为变更证据。

## 第 5 步：观测与告警
1. 按 `docs/guides/plugin_release/observability.md` 完成指标与仪表盘配置。
2. 确认 Prometheus 中存在以下告警：
   - `plugin_release_canary_rollback_sla_breach`
   - `plugin_release_hotload_latency_regression`
   - `plugin_release_offline_import_stuck`
3. 使用 `promtool test rules` 校验告警规则是否可用。

## 第 6 步：签署与发布
1. 收集证据（配置 diff、测试日志、Grafana 截图）。
2. 安全审核在发布工单中引用本指南给出签署意见。
3. 完成签署后，解除维护模式并通知 Marketplace/CLI 团队环境已准备就绪。
