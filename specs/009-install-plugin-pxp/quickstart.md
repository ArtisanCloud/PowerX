# Quickstart — Plugin Release & Marketplace Publishing Foundation

> 目标：在本地拉起 plugin_release 模块的 HTTP/gRPC/API + CLI 流程，完成一次从本地构建到测试租户审批的端到端演练。

## 1. 环境准备
1. 安装 Go 1.24、Node 20、buf CLI、Docker（提供本地 Postgres/Redis/MinIO）。
2. 克隆 PowerX 仓库，切换到 `001-install-plugin-pxp` 分支：  
   ```bash
   git checkout 001-install-plugin-pxp
   make deps
   ```
3. 启动依赖服务（Postgres/Redis/MinIO 可直接复用 `make dev-up` 或 `docker compose -f docker/dev-release.yml up -d`）。
4. 配置 `backend/etc/config.yaml` 中的 `database`, `cache`, `media_storage`，并开启 Feature Flag `plugin_release`.

## 1.5 插件初始化与环境自检
1. 使用新的 CLI 校验模板与 Git/权限参数：
   ```bash
   cd backend
   go run cmd/px/main.go plugin init \
     --api http://localhost:8077/api \
     --plugin-id com.powerx.demo \
     --template fullstack-go-nuxt
   ```
   该命令会调用 `POST /api/internal/plugins/bootstrap/validate`，返回推荐的 module path、模板描述以及补救建议。
2. 在任意开发环境运行 `px plugin doctor`，自动收集 Go/Node 版本、git/npm/pnpm 等二进制并调用 `POST /api/internal/plugins/environments/check`：
   ```bash
   go run cmd/px/main.go plugin doctor \
     --api http://localhost:8077/api \
     --template fullstack-go-nuxt
   ```
   若存在缺失/过期的运行时，命令会以非 0 退出并提示整改路径，满足 FR-015/US5 的沙箱前置校验。

## 2. 生成协议与模型
```bash
cd backend
make proto-gen proto-lint           # 生成/校验 plugin_release proto
go run cmd/database/migrate.go up   # 自动迁移 plugin_release 表
```

## 3. 启动 CoreX 服务
```bash
cd backend
go run cmd/server/main.go --enable-plugin-release
```
服务启动后将注册：
- HTTP Admin：`http://localhost:8080/api/admin/plugin-release/*`
- HTTP OpenAPI：`http://localhost:8080/api/openapi/plugin-release/*`
- gRPC：`localhost:9090` 中的 `powerx.plugin_release.v1.PluginReleaseService`

## 4. CLI / 本地构建演练
1. 安装 CLI：`go install github.com/ArtisanCloud/PowerXPlugin/cmd/px-plugin@latest`
2. 在插件仓库执行：
   ```bash
   px-plugin build --target local
   px-plugin dev --watch \
     --grpc-addr localhost:9090 \
     --tenant-id 101 --developer-id 2025 \
     --artifact ./dist/plugin-bundle.zip \
     --feature-flag beta_ui
   ```
   该命令会通过 gRPC `StartLocalInstall`/`PushHotReload`，并在 15 分钟 SLA 内把调试日志写入 `plugin_release.hotload.latency_ms`。
3. 运行 `px publish create --tenant-id 1001 --plugin-id px.demo --version v1.2.3 --artifact-uri s3://bucket/px-demo-v1.2.3.zip --commit <sha>` 触发 Release Candidate，随后 `px publish deploy --plan-id <id> --batch-name batch-a` 验证灰度。

### 4.1 宿主模拟器 + 热更新回路
1. 保证 `backend/etc/config.yaml` 中 `plugin_debug.host_simulator.enabled: true`，并在启动 CoreX 前启用 Feature Flag（默认读取 `PX_PLUGIN_HOST_SIMULATOR` 环境变量，未配置时保持开启）。如需定制端口或镜像，可编辑 `config/plugins/debug/host_simulator.yaml`。
2. 使用 `px host start --mock` 为目标插件创建宿主会话（需 admin token）：
   ```bash
   go run cmd/px/main.go host start \
     --api http://localhost:8077/api \
     --token "$(cat ~/.powerx/admin.token)" \
     --plugin-id com.powerx.demo \
     --environment local-mock \
     --ttl 15m \
     --http-port 51701 \
     --grpc-port 52701 \
     --capability debug.hot_reload \
     --capability sandbox.dataset
   ```
   终端会返回 `hostId/httpPort/grpcPort/expiresAt`，后续 `px-plugin dev --watch` 可复用该端口，也可在多端口（macOS/WSL）场景下指定自定义端口避免冲突。
3. 当需要通过 REST 直接汇报热更新（绕过 gRPC 或在 CI 中上传产物）时，使用 `--host-api` 模式：
   ```bash
   px-plugin dev --watch \
     --host-api http://localhost:8077/api \
     --token "$(cat ~/.powerx/admin.token)" \
     --tenant-id 101 \
     --developer-id 2025 \
     --artifact ./dist/plugin-bundle.zip \
     --artifact-uri file://$(pwd)/dist/plugin-bundle.zip \
     --feature-flag beta_ui \
     --reset-cache
   ```
   - CLI 会先调用 `POST /internal/plugins/local/install` 启动 session，再将 chunk 上传到 gRPC；若 `--host-api` 存在，则在每次热更新后额外调用 `POST /internal/plugins/local/reload` 记录 `debug.hot_reload.*` 指标。
   - 失败重试会自动附带最后一次错误信息；当 CLI 检测到 “version mismatch” 字样时，会在后台累加 `debug.host.version_mismatch_total`。
4. （可选）`px debug attach` 预留命令会在后续版本补充断点/日志聚合；在此之前，可通过 `POST /internal/debug/report` 触发调试诊断（见 Phase 10 任务）。

5. 若已启用 Dev Hotload API（`config/dev_hotload.yaml` + Feature Flag `PX_DEV_PLUGIN_HOTLOAD=1`），可直接使用新的 `--dev-api` 模式，让 CLI 调 `/api/v1/internal/dev/plugins/*` 完成注册/热更新/终止，并写入 `~/.px-plugin/sessions/<session>.json`，避免手工拼 host API：
   ```bash
   px-plugin dev --watch \
     --dev-api http://localhost:8077/api/v1 \
     --token "$POWERX_ADMIN_TOKEN" \
     --tenant-id 101 \
     --developer-id 2025 \
     --entry . \
     --artifact ./dist/plugin-bundle.zip
   ```
   - CLI 会调用 `POST /api/v1/internal/dev/plugins/register` 获取 `sessionId/reloadToken`，随后在每次文件变化时 `POST /reload`、终止时 `DELETE /register/{sessionId}`，同时监听 `GET /stream` 推送的会话事件。
   - `--dev-api` 与 `--host-api` 可择一使用：前者面向 Dev API Gateway（多 session/SSE），后者沿用 plugin_release local install 流程。两者都会在成功后更新 `~/.px-plugin/sessions/*.json`，便于 CLI 恢复/回溯。

### 4.2 版本治理 & 兼容性指令
1. 准备治理配置：`config/version/governance_rules.yaml`、`config/version/upgrade_policies.yaml`、`config/version/compat_matrix.yaml` 已提供默认模板，可根据租户标签/优先级定制扫描节奏与灰度策略。部署到测试环境时，保持 `PX_VERSION_GOVERNANCE=enabled` 以便服务加载这些配置。
2. 执行巡检：
   ```bash
   go run cmd/px/main.go version scan \
     --api http://localhost:8077/api \
     --tenant-id 88001 \
     --plugin-id com.powerx.demo \
     --token "$(cat ~/.powerx/admin.token)"
   ```
   该命令会调用 `POST /internal/version/governance/scan`，结合最新 Release Candidate 生成报告并打印风险等级。可通过 `--current-version`/`--target-version` 覆盖自动探测结果。
3. 查看多租户版本看板：
   ```bash
   go run cmd/px/main.go version board \
     --api http://localhost:8077/api \
     --tenant-id "" \
     --limit 25 \
     --token "$(cat ~/.powerx/admin.token)"
   ```
   CLI 会调用 `GET /internal/version/governance/board`，按风险聚合输出总览，可直接粘贴到运维周报。若需 Web Admin，看板路由 `/internal/version/governance/*` 已接入。
4. 兼容性校验与例外流程：
   ```bash
   # 安装/升级前做矩阵校验
   go run cmd/px/main.go version compat check \
     --api http://localhost:8077/api \
     --host-version 1.24.0 \
     --plugin-version 2.0.0

   # 提交例外申请
   go run cmd/px/main.go version compat exception \
     --api http://localhost:8077/api \
     --tenant-id 88001 \
     --plugin-id com.powerx.demo \
     --current-version 1.0.0 \
     --target-version 2.0.0 \
     --reason "灰度需要旧依赖"

   # 审批或驳回例外
   go run cmd/px/main.go version compat approve \
     --api http://localhost:8077/api \
     --id <exception-uuid> \
     --status approved \
     --reviewer release-admin@example.com
   ```
   所有命令均封装 `POST /internal/version/compat/{check,exception,approve}`，结果写入 `plugin_compat_exceptions` 表并同步审计，可直接满足 SCN-DEV-PLUGIN-VERSION-COMPAT-001 的堵截/例外流程。

## 5. 审批与灰度
1. 使用 Web Admin 调用 Admin API（或 `curl`）：
   ```bash
   curl -X POST http://localhost:8080/api/admin/plugin-release/plans \
     -H "Authorization: Bearer <token>" \
     -d @specs/001-install-plugin-pxp/examples/release-plan.json
   ```
2. 运行 `px publish deploy --candidate <id> --strategy canary`，观察 gRPC 流日志，验证 30 分钟内完成灰度。
3. 当 Prometheus 指标触发告警时，确认自动回滚在 5 分钟内执行，可通过 Grafana Dashboard `Plugin Release / Canary` 观察。

## 6. 离线包与 Marketplace
1. 使用新的 CLI 上传离线包：
   ```bash
   px publish package \
     --offline \
     --candidate-id <candidate-uuid> \
     --artifact ./dist/plugin-release.pxp \
     --grpc-addr localhost:9090
   ```
   命令会对 artifact 计算 SHA256 并通过 `UploadOfflinePackage` 生成包 URI。
2. Web Admin 提交 Marketplace 审核：
   ```bash
   curl -X POST http://localhost:8080/api/admin/plugin-release/marketplace/listings \
     -H "Authorization: Bearer <admin>" \
     -d '{
       "offlinePackageId": 1,
       "channel": "online",
       "pricing": {"tier":"enterprise"},
       "supportPolicy": {"sla":"24x7"}
     }'
   ```
   如需补件或升级，可调用 `POST /api/admin/plugin-release/marketplace/listings/{id}/reviews`。
3. 企业租户完成离线导入：
   ```bash
   px plugin import \
     --offline \
     --tenant-id 88001 \
     --package-uri s3://plugin-release/offline/<id>.pxp \
     --checksum <sha256> \
     --grpc-addr localhost:9090
   ```
   或调用 OpenAPI `POST /api/tenant/offline-imports`。
4. 系统会在 48 小时 SLA 内返回审核结果，所有补件、升级均写入 `MarketplaceListing` 与 `plugin_release_distribution` 指标。

## 7. 清理
```bash
make proto-clean
docker compose -f docker/dev-release.yml down
```
确保手动删除本地对象存储中的测试 `.pxp` 包以及 Redis 令牌，以免污染下次演练。
