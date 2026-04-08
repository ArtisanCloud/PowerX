# 02. 部署机拉镜像、配置、启动

## 1. 目标
在部署机通过 `compose.prod.yaml` 拉取已发布镜像并启动服务。

## 2. 目录与文件
- Compose：`deploy/powerx/docker/compose.prod.yaml`
- 环境变量模板：`deploy/powerx/docker/.env.prod.example`
- 默认内置依赖：`postgres`（pgvector 镜像）、`redis`

## 2.1 版本基线

统一版本矩阵与固化规则请以根文档为准：
- `../01-runtime-version-matrix.md`

## 3. 准备部署目录

```bash
cd /opt/powerx
# 假设你已把仓库或发布包同步到该目录
cd deploy/powerx/docker
cp .env.prod.example .env
```

准备宿主机映射目录（配置层 + 数据层）：

```bash
sudo mkdir -p /etc/powerx
sudo mkdir -p /var/lib/powerx/{postgres,redis,uploads}
sudo chown -R 999:999 /var/lib/powerx/postgres
sudo chown -R 999:999 /var/lib/powerx/redis
```

说明：
- `/etc/powerx`：配置层（`config.yaml`、`powerx.env`、`setup.wizard.config.json`）
- `/var/lib/powerx`：数据层（PostgreSQL/Redis/上传目录）

## 4. 配置 `.env`
必须确认以下键（默认使用 compose 内置 Postgres/Redis）：

```dotenv
POWERX_BACKEND_TAG=v2.0.1
POWERX_RUNNER_TAG=v2.0.1
POWERX_WEB_ADMIN_TAG=v2.0.1
POWERX_BACKEND_PORT=8080
POWERX_WEB_ADMIN_PORT=3000
POWERX_POSTGRES_PORT=5432
POWERX_ENV=prod
POWERX_MODE=docker
POWERX_HOST_CONFIG_DIR=/etc/powerx
POWERX_HOST_DATA_DIR=/var/lib/powerx
DATABASE_DSN=postgres://powerx:powerx@postgres:5432/powerx?sslmode=disable
REDIS_ADDR=redis:6379
POWERX_CONFIG=/etc/powerx/config.yaml
POSTGRES_DB=powerx
POSTGRES_USER=powerx
POSTGRES_PASSWORD=powerx
```

其中：
- `POWERX_*_TAG`：固定业务镜像版本（必须同一发布版本）
- `postgres` 服务镜像固定为 `pgvector/pgvector:pg16`
- `redis` 服务镜像固定为 `redis:7-alpine`
- `POWERX_HOST_CONFIG_DIR`：宿主机配置目录（映射到容器 `/etc/powerx`）
- `POWERX_HOST_DATA_DIR`：宿主机数据目录（映射 Postgres/Redis/本地上传）

端口策略说明：
- `POWERX_ENV=prod` 默认推荐 `POWERX_WEB_ADMIN_PORT=3000`、`POWERX_BACKEND_PORT=8080`
- 开发环境默认口径为 `web-admin=3030`、`backend=8077`
- 若配置了 `POWERX_WEB_ADMIN_PORT` / `POWERX_BACKEND_PORT`，以环境变量为准

动作：
- 默认模式：保留 `DATABASE_DSN=...@postgres...`，直接使用内置 postgres。
- 外部数据库模式：仅改 `DATABASE_DSN` 指向外部实例（例如 RDS），`postgres` 容器可保留不使用。
- 按目标环境替换 `REDIS_ADDR`、端口和 tag。
预期结果：3 个服务 tag 与本次发布版本一致。
失败处理：若拉取到旧版本，优先检查 `.env` 是否被覆盖。

## 5. 登录镜像仓库（部署机）

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USER" --password-stdin
```

## 6. 拉取镜像并启动

```bash
docker compose -f compose.prod.yaml pull
docker compose -f compose.prod.yaml up -d
```

预期结果：`postgres`、`redis`、`backend`、`runner`、`web-admin` 均为 `Up`。
失败处理：
- `pull denied`：检查仓库权限与 tag 是否存在。
- `unhealthy`：先看 `docker compose logs --tail=200 backend`。

## 7. 查看运行状态

```bash
docker compose -f compose.prod.yaml ps
docker compose -f compose.prod.yaml logs --tail=200 backend
docker compose -f compose.prod.yaml logs --tail=200 runner
docker compose -f compose.prod.yaml logs --tail=200 web-admin
docker compose -f compose.prod.yaml logs --tail=200 postgres
```

## 8. 首次启动后的最小初始化建议
- 确认数据库迁移已执行（若发布流程未自动执行，请执行 `cd backend && go run ./cmd/database migrate`）。
- 若使用外部 PostgreSQL 且启用向量能力，确认目标库已安装 `pgvector` 扩展（或允许应用账号执行 `CREATE EXTENSION vector`）。
- 确认 Admin 登录可用。
- 确认 `/ops/deploy` 页面可访问。

## 9. 首次安装引导配置页（推荐）

`docker compose up -d` 后访问 `http://<host>:<web-admin-port>/setup` 完成首次安装引导。  
完整步骤、字段含义、接口说明与排障，请统一参考：`../setup.md`。

## 10. 引导页排障

排障条目已收敛到 `../setup.md`，本节仅保留索引，避免重复维护。
