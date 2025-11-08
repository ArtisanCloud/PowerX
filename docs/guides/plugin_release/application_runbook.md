# Plugin Release 应用层使用指南

本指南基于 `specs/009-install-plugin-pxp` 的交付内容，串联开发者、审核员、运维和租户管理员在现实场景中的操作步骤。按照顺序执行即可完成从本地调试到多渠道分发的整条流水线。

## 0. 角色及工具准备
- **开发者**：安装 `px-plugin` 与 `powerx` CLI。
- **发布经理 / 审核员**：具备 Admin API Token。
- **运维**：掌握 Prometheus/Grafana、对象存储访问权限。
- **企业租户管理员**：拥有离线导入权限。

## 1. 本地热更新（Phase 3，T024-T031）
1. 在插件仓库执行：
   ```bash
   px-plugin build --target local
   px-plugin dev --watch \
     --grpc-addr localhost:9090 \
     --tenant-id 101 --developer-id 2025 \
     --artifact ./dist/plugin-bundle.zip \
     --feature-flag beta_ui
   ```
2. CLI 会触发 `StartLocalInstall` → `PushHotReload` → `StopLocalInstall`，对应 `backend/internal/service/plugin_release/local/...`。
3. 可用 `GET /api/tenant/plugin-release/local/sessions/:id` 查询热更新状态，日志将同步到 `plugin_release.hotload.latency_ms`。

## 2. 提交候选与审批（Phase 4，T032-T040）
1. 使用 `powerx publish create` 提交候选：
   ```bash
   powerx publish create \
     --tenant-id tenant-dev \
     --plugin-id px.demo \
     --version v1.2.3 \
     --artifact-uri s3://bucket/px-demo-v1.2.3.zip \
     --commit <sha> \
     --label channel=beta \
     --label coverage=95
   ```
2. 发布经理可在 Admin API 调用：
   - `POST /api/admin/plugin-release/candidates`（若需 Web Admin 页面）
   - `POST /api/admin/plugin-release/candidates/:id/gates`
   - `POST /api/admin/plugin-release/plans`
3. 质量门禁由 GateRunner 执行：确保 Release Notes ≥ 20 字、Commit Hash ≥ 7 位、`coverage` 标签达标。

## 3. 灰度部署与回滚（Phase 5，T041-T049）
1. 生成计划后，使用 CLI 触发灰度：
   ```bash
   powerx publish deploy \
     --plan-id <plan-id> \
     --batch-name batch-a \
     --final-action promote
   ```
2. HTTP Admin 也可调用：
   - `POST /api/admin/plugin-release/plans/:planId/deploy/canary`
   - `POST /api/admin/plugin-release/plans/:planId/deploy/finalize`
3. Prometheus 指标（`plugin_release.canary.phase_duration_seconds`、`plugin_release.canary.rollback_seconds`）用于监控 5 分钟回滚 SLA。

## 4. 离线包与 Marketplace（Phase 6，T050-T059）

### 4.1 通过 Web Admin UI 操作（推荐）

PowerX Web Admin 提供了完整的图形化操作界面，操作步骤更直观：

#### 4.1.1 提交离线包

1. 登录 PowerX Web Admin（需要 admin 或 system_admin 角色）
2. 导航至「插件发布」→「离线包入库」
3. 填写表单：
   - **发布候选ID**：从流水线获取的候选版本号（如 `candidate-123`）
   - **校验和**：包体的 SHA256 校验值
   - **包体URI**：S3 存储路径（如 `s3://bucket/package.pxp`）
4. 点击「提交审核」按钮
5. 系统返回审计参考ID，可在后续流程中追踪

#### 4.1.2 审核 Marketplace Listing

1. 进入「插件发布」→「Marketplace 审核列表」
2. 查看待审核列表，系统显示：
   - Listing ID、渠道（online/offline）
   - 审核状态（pending/approved/rejected）
   - 审核次数、SLA 截止时间
3. 使用状态筛选功能快速定位待审核项
4. 点击「审核」按钮，在弹窗中：
   - 选择审核结果（通过/拒绝）
   - 填写审核意见
5. 提交后列表自动刷新，显示最新状态

#### 4.1.3 查看审核详情

1. 在列表中点击「详情」按钮进入详情页
2. 详情页展示：
   - **基本信息**：ID、渠道、审核状态、创建时间
   - **SLA 监控**：剩余时间倒计时，预警提示
   - **定价策略**：企业版/专业版等层级定价
   - **支持政策**：SLA 协议、响应时间
3. 可通过「返回」按钮回到列表页

### 4.2 通过 CLI/API 操作

1. 开发者上传离线包：

   ```bash
   powerx publish package \
     --offline \
     --candidate-id <uuid> \
     --artifact ./dist/plugin-release.pxp \
     --grpc-addr localhost:9090
   ```

2. 运营在 Admin API 提交/审批：
   - `POST /api/admin/plugin-release/offline-packages`
   - `POST /api/admin/plugin-release/marketplace/listings`
   - `POST /api/admin/plugin-release/marketplace/listings/:id/reviews`

3. 企业租户自助导入：
   - CLI：`powerx plugin import --offline --tenant-id <tid> --package-uri <uri> --checksum <sha>`
   - OpenAPI：`POST /api/tenant/offline-imports`

### 4.3 部署菜单配置

Web Admin 的菜单项通过配置文件自动生成，位于：

```typescript
web-admin/app/services/menuConfig.ts
```

发布时需确保：

1. 菜单配置已包含插件发布模块
2. 用户角色具备相应权限（admin/system_admin）
3. API Token 权限范围正确（plugin-release.*）

## 5. 观测与告警（Phase 7，T060）
1. 完成 `docs/guides/plugin_release/observability.md` 中的仪表盘与告警配置。
2. 常用指标：
   - `plugin_release.pipeline.duration_seconds`：审批耗时
   - `plugin_release.distribution.sla_seconds`：离线审核 SLA
3. 告警示例：
   - `plugin_release_canary_rollback_sla_breach`
   - `plugin_release_hotload_latency_regression`

## 6. 安全基线（Phase 7，T064）
1. 按 `docs/guides/plugin_release/security_baseline.md` 检查 Feature Flag、RBAC、审计与告警。
2. 使用 `scripts/ci/run_quickstart.sh` 产出最新的 `backend/reports/plugin_release/dry_run.md` 作为验收证据。

## 7. 文档与 SOP 更新
- 快速入门：`specs/009-install-plugin-pxp/quickstart.md`
- Use Case：`docs/use_cases/_from_hub/SCN-PUBLISH-HUB-001/*.md`
- 观测指南：`docs/guides/plugin_release/observability.md`
- 安全基线：`docs/guides/plugin_release/security_baseline.md`

完成以上步骤即可覆盖 spec 中的全部阶段，确保插件发布能力在实践中可被开发、审核、运维、租户多角色复用。
