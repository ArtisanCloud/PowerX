# 02. 部署机拉镜像、配置、启动

## 1. 目标
在部署机通过 `compose.prod.yaml` 拉取已发布镜像并启动服务。

## 2. 目录与文件
- Compose：`deploy/powerx/docker/compose.prod.yaml`
- 环境变量模板：`deploy/powerx/docker/.env.prod.example`

## 3. 准备部署目录

```bash
cd /opt/powerx
# 假设你已把仓库或发布包同步到该目录
cd deploy/powerx/docker
cp .env.prod.example .env
```

## 4. 配置 `.env`
必须确认以下键：

```dotenv
POWERX_BACKEND_TAG=v2.0.1
POWERX_RUNNER_TAG=v2.0.1
POWERX_WEB_ADMIN_TAG=v2.0.1
POWERX_BACKEND_PORT=8080
POWERX_WEB_ADMIN_PORT=3000
POWERX_ENV=prod
POWERX_MODE=docker
DATABASE_DSN=postgres://powerx:powerx@postgres:5432/powerx?sslmode=disable
REDIS_ADDR=redis:6379
```

动作：按目标环境替换 `DATABASE_DSN`、`REDIS_ADDR`、端口和 tag。
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

预期结果：`backend`、`runner`、`web-admin` 均为 `Up`。
失败处理：
- `pull denied`：检查仓库权限与 tag 是否存在。
- `unhealthy`：先看 `docker compose logs --tail=200 backend`。

## 7. 查看运行状态

```bash
docker compose -f compose.prod.yaml ps
docker compose -f compose.prod.yaml logs --tail=200 backend
docker compose -f compose.prod.yaml logs --tail=200 runner
docker compose -f compose.prod.yaml logs --tail=200 web-admin
```

## 8. 首次启动后的最小初始化建议
- 确认数据库迁移已执行（若发布流程未自动执行）。
- 确认 Admin 登录可用。
- 确认 `/ops/deploy` 页面可访问。
