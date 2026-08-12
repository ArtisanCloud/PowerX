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
3. gRPC 服务需部署在 API Gateway 后并通过 mTLS/JWT 鉴权，仅放行 `px publish` CLI 与受信自动化账号。

## 第 3.5 步：插件权限声明准入

1. 插件包必须提供 `permissions[]`，并按 `docs/guides/plugin_release/permission_declaration.md` 声明 `menu/page/action/api`。
2. 每个插件后台业务页面必须有 `type=page` 和 GET `protocol_bindings`；静态资产、`/_nuxt/**`、图片、CSS、JS 不声明 page 权限。
3. 每个敏感接口必须有 `type=api` binding，并通过 `business_permission_code` 指向业务 action，除非该接口显式 `independent: true`。
4. delegated/host 模式插件后端必须消费 `permission_codes`、`policy_version`、`perms_hash`，并按接口 effective permission 做二次校验，不得回退旧粗权限或 raw API 权限。
5. 插件通过 runtime ws-bus/taskbus 发布事件时，`event_fabric` manifest 必须给插件服务态 principal 授权 `publish`：`principal_type=plugin`、`principal_id="{{plugin_id}}"`。只给 `member:system` 或 `role:role_admin` 授权不得通过发布审核。
6. 插件通过 STS/Bearer 调用 PowerX Core 运行时合同接口时，底座必须有明确 STS direct route policy。Host Scheduler 的 `/api/v1/admin/scheduler/jobs` 系列属于显式例外；Event Fabric topic bootstrap 推荐使用正式能力 `POST /api/v1/event-fabric/topics`，`POST /api/v1/admin/event-fabric/topics` 只作为显式运行时合同例外；其他 `/api/v1/admin/*` 不得因为普通插件声明或 topic ACL 自动放开。
7. `permission_code`、i18n、风险等级、`actor_context`、`resource_scope` 缺失时必须拒绝发布或同步，不得降级到旧粗权限。

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

## 第 5 步：签署与发布

1. 收集证据（配置 diff、测试日志、Grafana 截图）。
2. 安全审核在发布工单中引用本指南给出签署意见。
3. 完成签署后，解除维护模式并通知 Marketplace/CLI 团队环境已准备就绪。

## 附录 A：Web Admin 前端安全配置

### A.1 身份认证与授权

1. **Token 管理**
   - Web Admin 使用 Bearer Token 进行身份认证
   - Token 存储在 localStorage，页面刷新后自动恢复会话
   - Token 过期后自动跳转登录页

2. **路由守卫**
   - 所有 `/admin/*` 路由需要 `admin` 或 `system_admin` 角色
   - 前端路由守卫依赖用户角色信息（`localStorage.user`）
   - 实际权限验证在后端 API 层执行

3. **API 安全**
   - 所有 Admin API 请求强制携带 Authorization 头
   - 后端 `AdminOnlyMiddleware` 验证 Token 权限范围
   - 无效 Token 或权限不足返回 401/403 错误

### A.2 前端安全措施

1. **Playwright E2E 测试安全检查**
   - 测试套件包含权限验证测试：
     - 验证未登录用户无法访问管理页面
     - 验证普通用户无法执行审核操作
     - 验证 Token 失效时页面正确处理
   - 运行安全测试：`cd web-admin && npm run test:e2e -- --grep "安全"`

2. **依赖安全审计**
   - 定期执行 `npm audit` 检查依赖漏洞
   - package.json 使用固定版本号（避免供应链攻击）
   - .gitignore 忽略敏感文件（.env、测试产物等）

3. **前端日志脱敏**
   - E2E 测试中禁止输出真实 Token
   - 测试数据使用模拟值（如 `test-token`、`admin@test.com`）
   - 生产环境错误日志不包含用户敏感信息

### A.3 部署安全检查清单

- [ ] Web Admin 构建时移除 console.log 调试信息
- [ ] 生产环境开启 CSP（内容安全策略）
- [ ] HTTPS 强制启用，HTTP 重定向到 HTTPS
- [ ] 敏感 API 启用请求频率限制（防止暴力攻击）
- [ ] XSS 防护：所有用户输入进行转义
- [ ] CSRF 防护：Admin 页面操作携带 CSRF Token
