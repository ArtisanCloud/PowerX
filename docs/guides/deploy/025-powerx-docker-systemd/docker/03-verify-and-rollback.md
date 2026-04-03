# 03. 启动后验收、排障与回滚

## 1. 健康验收

```bash
curl -f http://127.0.0.1:8080/api/v1/health
bash backend/scripts/ops/deploy-check.sh
```

预期结果：HTTP 200，脚本输出 `healthy`。

## 2. 运维域 API 验收

```bash
curl -sS "http://127.0.0.1:8080/api/v1/admin/deploy/health" \
  -H "Authorization: Bearer <ADMIN_TOKEN>"
```

预期结果：返回 deploy 健康聚合信息。

## 3. 典型故障排查

### 3.1 服务未启动

```bash
docker compose -f deploy/powerx/docker/compose.prod.yaml ps
docker compose -f deploy/powerx/docker/compose.prod.yaml logs --tail=200
```

### 3.2 依赖不可达（DB/Redis）
- 现象：backend 日志有连接失败。
- 动作：核对 `.env` 的 `DATABASE_DSN`、`REDIS_ADDR`。
- 额外检查（外部 PostgreSQL + 向量能力）：
  - `SELECT extname FROM pg_extension WHERE extname='vector';`
  - 若无结果，先安装 pgvector 或使用具备 `CREATE EXTENSION` 权限的账号执行初始化。

### 3.3 镜像版本不对

```bash
docker inspect $(docker ps -q --filter name=backend) --format '{{.Config.Image}}'
```

## 4. 回滚方案

### 4.1 通过 API 回滚发布记录

```bash
bash backend/scripts/ops/rollback-release.sh prod <TARGET_VERSION> docker
```

### 4.2 通过 compose 回滚镜像 tag
1. 将 `.env` 中 3 个 tag 回改为上一稳定版本。
2. 执行：

```bash
docker compose -f deploy/powerx/docker/compose.prod.yaml up -d
```

3. 再次执行健康验收。

## 5. 回滚成功判定
- `/api/v1/health` 200
- 管理端关键页面可访问
- 新版本引入的问题消失
