# 02. 部署机本地构建、配置、启动

## 1. 目标
在部署机基于本地仓库源码，通过 `compose.prod.yaml` 构建并启动服务。

## 2. 目录与文件
- Compose：`deploy/powerx/docker/compose.prod.yaml`
- 环境变量模板：`deploy/powerx/docker/.env.prod.example`
- 默认内置依赖：`postgres`（pgvector 镜像）、`redis`、`loki`、`promtail`、`grafana`
- Docker 运行时安装（Ubuntu）：`00-runtime-deps-versions.md`

## 2.1 版本基线

统一版本矩阵与固化规则请以根文档为准：
- `../01-runtime-version-matrix.md`

## 3. 获取代码并进入目录

首次机器（无仓库）：

```bash
mkdir -p ~/workspace
cd ~/workspace
git clone https://github.com/ArtisanCloud/PowerX.git
cd ~/workspace/PowerX
git fetch --tags --prune
```

已有仓库（更新代码）：

```bash
cd ~/workspace/PowerX
git fetch --tags --prune
git pull --ff-only
```

进入 Docker 部署目录：

```bash
cd ~/workspace/PowerX/deploy/powerx/docker

# 若你的代码部署在系统目录，再用：
# cd /opt/powerx/deploy/powerx/docker
```

## 4. 推荐：两步启动（主路径）

```bash
cd ~/workspace/PowerX/deploy/powerx/docker
./scripts/bootstrap-host.sh
./scripts/up.sh
```

说明：
- `bootstrap-host.sh`：创建 `/etc/powerx` 与 `/var/lib/powerx/{postgres,redis,uploads}`，并在缺失时生成 `.env`；会自动写入 `POWERX_IMAGE_TAG`（优先现有配置，其次 `POWERX_IMAGE_TAG/POWERX_VERSION`，最后最近 git tag，仍无则 `local`）。
- `up.sh`：执行 `docker compose pull postgres/redis/loki/promtail/grafana + build backend/web-admin + up -d + ps`。

## 4.1 一键重置并启动（清空后重装）

```bash
cd ~/workspace/PowerX/deploy/powerx/docker
export DOCKER_PROXY_URL=http://127.0.0.1:8890   # 如无需代理可不设
./scripts/install-and-up.sh
```

说明：
- `install-and-up.sh` 会执行 `clean.sh --yes -> bootstrap-host.sh -> up.sh`。
- 若设置 `DOCKER_PROXY_URL`，脚本会自动写入 docker daemon 代理并重启 docker。
- `clean.sh --yes` 会删除：
  - compose 相关容器/网络/卷
  - `${POWERX_HOST_CONFIG_DIR:-/etc/powerx}`
  - `${POWERX_HOST_DATA_DIR:-/var/lib/powerx}`
  - 当前目录 `.env`

模式说明（`POWERX_DOCKER_MODE`）：
- `infra`：只启动 `postgres + redis`（推荐用于源码运行 / PowerXPlugin init，本机后端例如 `127.0.0.1:8077`）。
- `full`：启动全栈（`postgres/redis/backend/web-admin`，backend/web-admin 由本地代码构建镜像）。
- `auto`（默认）：等同 `full`。

## 5. 手工等价步骤（可选）

如果你不使用脚本，可执行与脚本等价的手工步骤。

### 5.1 准备宿主机映射目录（配置层 + 数据层）

```bash
sudo mkdir -p /etc/powerx
sudo mkdir -p /var/lib/powerx/{postgres,redis,uploads}
sudo chown -R 999:999 /var/lib/powerx/postgres
sudo chown -R 999:999 /var/lib/powerx/redis
```

说明：
- `/etc/powerx`：配置层（`config.yaml`、`powerx.env`、`setup.wizard.config.json`）
- `/var/lib/powerx`：数据层（PostgreSQL/Redis/上传目录）

### 5.2 配置 `.env`

```bash
cp .env.prod.example .env
```

必须确认以下键（默认使用 compose 内置 Postgres/Redis）：

```dotenv
POWERX_IMAGE_TAG=v2.0.1
POWERX_BACKEND_PORT=8080
POWERX_WEB_ADMIN_PORT=3000
POWERX_POSTGRES_PORT=5432
POWERX_LOKI_PORT=3100
POWERX_GRAFANA_PORT=3001
POWERX_GRAFANA_ADMIN_USER=admin
POWERX_GRAFANA_ADMIN_PASSWORD=admin
POWERX_GRAFANA_ANONYMOUS_ENABLED=false
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
- `POWERX_IMAGE_TAG`：本地构建镜像标签（backend/web-admin 会统一使用该 tag）。
- `postgres` 服务镜像固定为 `pgvector/pgvector:pg16`
- `redis` 服务镜像固定为 `redis:7-alpine`
- `loki/promtail/grafana` 服务镜像固定为 grafana 官方镜像
- `POWERX_HOST_CONFIG_DIR`：宿主机配置目录（映射到容器 `/etc/powerx`）
- `POWERX_HOST_DATA_DIR`：宿主机数据目录（映射 Postgres/Redis/本地上传）

端口策略说明：
- `POWERX_ENV=prod` 默认推荐 `POWERX_WEB_ADMIN_PORT=3000`、`POWERX_BACKEND_PORT=8080`
- 开发环境默认口径为 `web-admin=3030`、`backend=8077`
- 若配置了 `POWERX_WEB_ADMIN_PORT` / `POWERX_BACKEND_PORT`，以环境变量为准

动作：
- 默认模式：保留 `DATABASE_DSN=...@postgres...`，直接使用内置 postgres。
- 外部数据库模式：仅改 `DATABASE_DSN` 指向外部实例（例如 RDS），`postgres` 容器可保留不使用。
- 按目标环境替换 `REDIS_ADDR`、端口和 `POWERX_IMAGE_TAG`。
预期结果：本地构建出 `powerx-backend:<tag>`、`powerx-web-admin:<tag>`。
失败处理：若构建到旧版本，优先检查 `git branch` 和 `.env` 的 `POWERX_IMAGE_TAG`。

## 6. 构建并启动

```bash
docker compose -f compose.prod.yaml pull postgres redis
docker compose -f compose.prod.yaml pull loki promtail grafana
docker compose -f compose.prod.yaml build backend web-admin
docker compose -f compose.prod.yaml up -d
```

预期结果：`postgres`、`redis`、`loki`、`promtail`、`grafana`、`backend`、`web-admin` 均为 `Up`。
失败处理：
- 构建失败：先看 `docker compose build backend web-admin` 的报错（网络/依赖/代理）。
- `unhealthy`：先看 `docker compose logs --tail=200 backend`。

## 7. 查看运行状态

```bash
docker compose -f compose.prod.yaml ps
docker compose -f compose.prod.yaml logs --tail=200 backend
docker compose -f compose.prod.yaml logs --tail=200 web-admin
docker compose -f compose.prod.yaml logs --tail=200 postgres
docker compose -f compose.prod.yaml logs --tail=200 loki
docker compose -f compose.prod.yaml logs --tail=200 promtail
docker compose -f compose.prod.yaml logs --tail=200 grafana
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
