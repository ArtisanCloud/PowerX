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
   px-plugin dev --watch --push grpc://localhost:9090
   ```
3. 运行 `powerx publish create --plugin-id <id> --version v1.2.3 --notes release.md` 触发 Release Candidate。

## 5. 审批与灰度
1. 使用 Web Admin 调用 Admin API（或 `curl`）：
   ```bash
   curl -X POST http://localhost:8080/api/admin/plugin-release/plans \
     -H "Authorization: Bearer <token>" \
     -d @specs/001-install-plugin-pxp/examples/release-plan.json
   ```
2. 运行 `powerx publish deploy --candidate <id> --strategy canary`，观察 gRPC 流日志，验证 30 分钟内完成灰度。
3. 当 Prometheus 指标触发告警时，确认自动回滚在 5 分钟内执行，可通过 Grafana Dashboard `Plugin Release / Canary` 观察。

## 6. 离线包与 Marketplace
```bash
powerx publish package --candidate <id> --offline \
  --output s3://plugin-release/offline/<id>.pxp
curl -X POST http://localhost:8080/api/admin/marketplace/listings \
  -d '{"offlinePackageId": "...", "channel":"offline"}'
```
系统会在 48 小时 SLA 内返回审核结果，并在 `MarketplaceListing` 中记录补件/升级状态。

## 7. 清理
```bash
make proto-clean
docker compose -f docker/dev-release.yml down
```
确保手动删除本地对象存储中的测试 `.pxp` 包以及 Redis 令牌，以免污染下次演练。
