# systemd 部署文档索引（025）

本目录提供 PowerX systemd 生产部署分步手册，按“先准备制品，再部署启动”组织。

## 文档列表
- `01-prepare-artifacts.md`：如何准备 backend/runner/web-admin 发布制品。
- `02-deploy-config-start.md`：如何安装 service、配置环境、启动服务。
- `03-verify-and-rollback.md`：如何验收、排障与快速回滚。

## 一键打包
- 提供 `make dist`（等价 `make dist-systemd`），输出目录：`dist/systemd/<version>/`
- 常用命令：
  - `make dist DIST_VERSION=v2.0.1`
  - `make dist DIST_VERSION=v2.0.1 NPM_INSTALL=0`（跳过 `npm ci`，仅构建）

## 对应资产
- systemd 单元文件：`deploy/powerx/systemd/{powerx-backend.service,powerx-runner.service,powerx-web-admin.service}`
- 健康检查脚本：`backend/scripts/ops/deploy-check.sh`
- API 回滚脚本：`backend/scripts/ops/rollback-release.sh`
