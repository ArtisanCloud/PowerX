# PowerX 部署快速手册（Docker / systemd）

## 1. 适用范围
- 功能分支：`025-powerx-docker-systemd`
- 目标：提供可直接执行的部署、验收、回滚操作。
- 范围：`backend`、`runner`、`web-admin` 三服务。

## 2. 前置条件

### 2.1 基础依赖
- Docker 模式：默认内置 PostgreSQL（pgvector）与 Redis；也支持改 `DATABASE_DSN` 连接外部 PostgreSQL。
- systemd 模式：需要外部 PostgreSQL、Redis、MinIO/S3 可连通。
- 已准备 PowerX 运行配置（数据库、缓存、对象存储、认证）。
- 服务器时间同步正常（避免证书/Token 时间偏差）。

### 2.2 关键文件
- Docker 资产：`deploy/powerx/docker/compose.prod.yaml`
- systemd 资产：`deploy/powerx/systemd/{powerx-backend.service,powerx-runner.service,powerx-web-admin.service}`
- 部署检查脚本：`backend/scripts/ops/deploy-check.sh`
- 回滚脚本：`backend/scripts/ops/rollback-release.sh`

### 2.3 端口默认值与覆盖规则
- 默认值（dev）：`web-admin=3030`，`backend=8077`
- 默认值（prod）：`web-admin=3000`，`backend=8080`
- 运行时覆盖：`POWERX_WEB_ADMIN_PORT`、`POWERX_BACKEND_PORT`
- setup 端口项：规划为首装向导可编辑项（当前版本待实现，暂以环境变量/配置文件为准）

## 3. 方案 A：Docker 部署（首发主方案）

### 步骤 1：准备环境变量
- 动作：准备 `.env` 文件。
- 入口/命令：

```bash
cd deploy/powerx/docker
cp .env.prod.example .env
```

- 预期结果：`.env` 存在且包含镜像 tag、端口、数据库与缓存配置。
- 失败处理：若不存在示例文件，先从发布包或配置中心补齐。

### 步骤 2：拉取并启动
- 动作：拉取镜像并启动服务。
- 入口/命令：

```bash
docker compose -f compose.prod.yaml pull
docker compose -f compose.prod.yaml up -d
```

- 预期结果：`postgres/redis/backend/runner/web-admin` 服务均处于 running。
- 失败处理：
  - 拉取失败：检查镜像仓库凭证。
  - 启动失败：`docker compose -f compose.prod.yaml logs --tail=200` 查看报错。

### 步骤 3：健康检查
- 动作：检查服务健康。
- 入口/命令：

```bash
curl -f http://127.0.0.1:8080/api/v1/health
bash backend/scripts/ops/deploy-check.sh
```

- 预期结果：返回 200，脚本输出 `[deploy-check] healthy`。
- 失败处理：检查端口映射、依赖连通性、容器健康检查日志。

## 4. 方案 B：systemd 部署（兼容/兜底）

### 步骤 1：安装 service 文件
- 动作：部署 systemd 单元文件。
- 入口/命令：

```bash
sudo cp deploy/powerx/systemd/*.service /etc/systemd/system/
sudo systemctl daemon-reload
```

- 预期结果：`systemctl list-unit-files | rg powerx-` 能看到 3 个 service。
- 失败处理：检查目标目录权限与文件语法。

### 步骤 2：启动并设置自启
- 动作：启用并启动服务。
- 入口/命令：

```bash
sudo systemctl enable powerx-backend powerx-runner powerx-web-admin
sudo systemctl restart powerx-backend powerx-runner powerx-web-admin
sudo systemctl status powerx-backend powerx-runner powerx-web-admin --no-pager
```

- 预期结果：状态为 `active (running)`。
- 失败处理：
  - 检查 `EnvironmentFile=/etc/powerx/powerx.env` 是否存在。
  - 检查 `WorkingDirectory` 与 `ExecStart` 路径是否与实际部署目录一致。

### 步骤 3：健康检查
- 动作：验证 API 可用。
- 入口/命令：

```bash
curl -f http://127.0.0.1:8077/api/v1/health
bash backend/scripts/ops/deploy-check.sh
```

- 预期结果：返回 200，检查脚本通过。
- 失败处理：`journalctl -u powerx-backend -u powerx-runner -u powerx-web-admin -n 200 --no-pager` 定位错误。

## 5. 部署后最小验收

1. 动作：校验 Deploy 管理接口。
- 入口/命令：

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/deploy/health" \
  -H "Authorization: Bearer <ADMIN_TOKEN>"
```

- 预期结果：返回健康聚合状态。
- 失败处理：检查 Token 权限与路由前缀。

2. 动作：校验页面入口。
- 入口/命令：访问 `/ops/deploy`。
- 预期结果：页面可见“部署发布中心”。
- 失败处理：检查 web-admin 进程与前端 API 代理配置。

## 6. 回滚步骤

### 6.1 API 回滚
- 动作：调用回滚接口。
- 入口/命令：

```bash
bash backend/scripts/ops/rollback-release.sh prod v2.0.0 docker
```

- 预期结果：返回回滚记录，后续状态变为 `success`。
- 失败处理：改用手工回滚（镜像 tag/软链回切）后再复盘。

### 6.2 Docker 手工回滚
- 动作：切回旧 tag 后重启。
- 入口/命令：修改 `.env` 的版本变量后执行：

```bash
docker compose -f compose.prod.yaml up -d
```

- 预期结果：服务恢复到上一稳定版本。
- 失败处理：保留日志并升级处理，不要继续滚动变更。

### 6.3 systemd 手工回滚
- 动作：回切二进制目录（或软链）并重启服务。
- 入口/命令：

```bash
sudo systemctl restart powerx-backend powerx-runner powerx-web-admin
```

- 预期结果：服务恢复稳定，健康检查通过。
- 失败处理：立即冻结发布窗口并执行故障通报。

## 7. 常见问题与排障

### Q1：健康检查一直失败
- 现象：`/api/v1/health` 非 200。
- 排查命令：

```bash
docker compose -f deploy/powerx/docker/compose.prod.yaml logs --tail=200
# 或
journalctl -u powerx-backend -n 200 --no-pager
```

- 修复建议：优先检查数据库/Redis 连接和配置文件路径。
  - 外部 PostgreSQL + 向量能力场景，还需确认 `pgvector` 扩展可用。

### Q2：Admin API 401/403
- 现象：部署接口返回未授权或权限不足。
- 排查命令：

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/user/auth/me/context" -H "Authorization: Bearer <ADMIN_TOKEN>"
```

- 修复建议：补齐 `platform_ops` 权限点，或使用 root/租户管理员账号。

### Q3：脚本执行失败
- 现象：`deploy-check.sh` 或 `rollback-release.sh` 报错。
- 排查命令：

```bash
ls -l backend/scripts/ops
printenv | rg 'POWERX_(BASE_URL|HEALTH_PATH|DEPLOY_ROLLBACK_PATH|ADMIN_AUTH_HEADER)'
```

- 修复建议：校正环境变量、脚本权限和 API 地址。

## 8. 代码与资产映射
- Docker：`deploy/powerx/docker/compose.prod.yaml`
- systemd：`deploy/powerx/systemd/*.service`
- 健康检查脚本：`backend/scripts/ops/deploy-check.sh`
- 回滚脚本：`backend/scripts/ops/rollback-release.sh`
- Deploy API：`backend/internal/transport/http/admin/deploy/{routes.go,handler.go}`
- Deploy Service：`backend/internal/service/deploy_ops/service.go`
