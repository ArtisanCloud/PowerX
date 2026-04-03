# systemd 部署索引（025）

## 1. 执行顺序
1. `01-deploy-config-start.md`
2. `02-verify-and-rollback.md`
3. `03-install-plugin.md`

## 2. 前置文档
- `00-runtime-deps-versions.md`（PowerX/PostgreSQL/Redis）
- `00-nginx-install-config.md`（Nginx 反向代理）
- `../00-required-config.md`（必须配置项）
- `../setup.md`（首装向导）

## 3. 资产路径
- unit：`deploy/powerx/systemd/{powerx-backend.service,powerx-web-admin.service,powerx-runner.service}`
- 健康检查：`backend/scripts/ops/deploy-check.sh`
- 版本切换：`backend/scripts/ops/switch-release-systemd.sh`
- 回滚记录：`backend/scripts/ops/rollback-release.sh`
