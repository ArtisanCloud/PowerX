# 01. 运行时版本矩阵（Docker / systemd 统一基线）

## 1. 目标
统一 PowerX 在 Docker 与 systemd 两种部署模式下的运行时版本，避免“代码版本升级但依赖漂移”。

## 2. 推荐矩阵（生产）

| 组件 | 固定版本 | 说明 |
|---|---|---|
| PowerX | `${POWERX_VERSION}`（明确版本号） | 禁止使用漂移值 |
| Go（构建工具链） | `1.24.12` | 用于构建/本地工具运行 |
| Node.js（web-admin/runner） | `20.20.2` | systemd 与镜像内运行时都应对齐 |
| PostgreSQL | `16` | Docker 对齐 `pgvector/pgvector:pg16` |
| Redis | `7` | Docker 对齐 `redis:7-alpine` |
| Docker Compose | `v2.20+` | Docker 部署最低建议 |

## 3. 固化规则（必须执行）

1. 生产环境禁止使用 `latest`。
2. Docker 模式必须固定：
   - `POWERX_BACKEND_TAG`
   - `POWERX_RUNNER_TAG`
   - `POWERX_WEB_ADMIN_TAG`
3. systemd 模式必须固定 Node/Go/PostgreSQL/Redis 版本，不依赖系统默认滚动升级。
4. 每次发布记录必须包含：
   - `POWERX_VERSION`
   - Go 版本
   - Node 版本
   - PostgreSQL 主版本
   - Redis 主版本

## 4. 模式映射

- Docker 资产：`deploy/powerx/docker/compose.prod.yaml`
  - Postgres：`pgvector/pgvector:pg16`
  - Redis：`redis:7-alpine`
  - 宿主机目录规范：配置层 `/etc/powerx`，数据层 `/var/lib/powerx`
- systemd 资产：`deploy/powerx/systemd/*.service`
  - 通过 `/etc/powerx/config.yaml` + `/etc/powerx/powerx.env` 管理运行参数

## 5. 自检命令

```bash
psql --version
redis-server --version
node -v
go version
docker compose version
```

## 6. 关联文档

- systemd 依赖安装：`systemd/00-runtime-deps-versions.md`
- Docker 依赖安装：`docker/00-runtime-deps-versions.md`
- Docker 启动部署：`docker/02-deploy-pull-config-start.md`
- 必要配置：`00-required-config.md`
