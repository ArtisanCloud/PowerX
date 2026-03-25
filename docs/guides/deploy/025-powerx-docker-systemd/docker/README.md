# Docker 部署文档索引（025）

本目录提供 PowerX Docker 生产部署的分步手册，按“先打镜像，再部署”组织。

## 文档列表
- `01-build-and-push-images.md`：如何构建并推送 backend/runner/web-admin 镜像。
- `02-deploy-pull-config-start.md`：如何在目标机器拉镜像、配置 `.env`、启动 compose。
- `03-verify-and-rollback.md`：如何验收、排障与快速回滚。

## 对应资产
- Compose 文件：`deploy/powerx/docker/compose.prod.yaml`
- 环境变量模板：`deploy/powerx/docker/.env.prod.example`
- 健康检查脚本：`backend/scripts/ops/deploy-check.sh`
- API 回滚脚本：`backend/scripts/ops/rollback-release.sh`
