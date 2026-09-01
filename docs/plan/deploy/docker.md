# Docker 生产部署方案（首发主方案）

## 1. 设计原则

- 使用容器标准化运行环境，便于后续迁移 K8s
- 将插件目录和注册表放在持久卷，避免容器重建导致插件状态丢失
- 外置 PostgreSQL / Redis / MinIO，应用容器专注业务

## 2. 参考 compose（生产骨架）

```yaml
version: "3.9"

services:
  powerx-backend:
    image: registry.example.com/powerx/backend:${POWERX_VERSION}
    container_name: powerx-backend
    restart: always
    env_file:
      - /opt/powerx/shared/config/powerx-backend.env
    volumes:
      - /opt/powerx/shared/config/config.yaml:/app/etc/config.yaml:ro
      - /opt/powerx/plugins:/opt/powerx/plugins
      - /opt/powerx/shared/logs/backend:/app/logs
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8077/api/v1/health"]
      interval: 30s
      timeout: 3s
      retries: 5
    networks:
      - powerx

  powerx-web-admin:
    image: registry.example.com/powerx/web-admin:${POWERX_VERSION}
    container_name: powerx-web-admin
    restart: always
    environment:
      NODE_ENV: production
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:3000/healthz"]
      interval: 30s
      timeout: 3s
      retries: 5
    networks:
      - powerx

  powerx-nginx:
    image: nginx:1.27-alpine
    container_name: powerx-nginx
    restart: always
    depends_on:
      - powerx-backend
      - powerx-web-admin
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /opt/powerx/shared/config/nginx.conf:/etc/nginx/conf.d/default.conf:ro
      - /opt/powerx/shared/logs/nginx:/var/log/nginx
      - /etc/letsencrypt:/etc/letsencrypt:ro
    networks:
      - powerx

networks:
  powerx:
    driver: bridge
```

## 3. 发布流程（应用）

1. 准备镜像：`backend` 与 `web-admin` 同版本 tag。
2. 更新版本变量：`POWERX_VERSION=<new_version>`。
3. 执行灰度重建：
   ```bash
   docker compose -f compose.prod.yaml pull
   docker compose -f compose.prod.yaml up -d
   ```
4. 健康检查：
   - `curl -f http://127.0.0.1/api/v1/health`
   - `curl -f http://127.0.0.1/healthz`（由 Nginx 转发）
5. 保留上一版本镜像与 compose 变量，用于快速回滚。

## 4. 回滚流程（应用）

1. 将 `POWERX_VERSION` 切回旧版本。
2. 执行：
   ```bash
   docker compose -f compose.prod.yaml up -d
   ```
3. 验证健康状态与关键业务链路。

## 5. 插件目录持久化要求（必须）

- 容器挂载的 `config.yaml` 必须显式包含 `deployment.env: prod`；该值是插件 Role/User 命名的唯一环境来源，Schema/Database 名称保持不变。
- 容器内插件配置需指向宿主持久目录：
  - `plugin.registry_file=/opt/powerx/plugins/registry.json`
  - `plugin.installed_dir=/opt/powerx/plugins/installed`
  - `plugin.market_cache_dir=/opt/powerx/plugins/market_cache`
- 挂载后，容器替换不会影响已安装插件版本与 current 指针。
- 不得用 compose 项目名、容器名、`NODE_ENV`、插件目录或安装请求元数据推导数据库隔离环境。若 `deployment.env` 缺失或与已有插件安装记录不一致，插件安装/恢复必须失败并要求显式迁移。

## 6. K8s 演进建议

- 将 backend/web-admin/nginx 拆为 Deployment
- 将 `/opt/powerx/plugins` 映射为 PVC（ReadWriteOnce）
- 将 `config.yaml`/env 映射为 ConfigMap + Secret
- 健康探针与本方案保持一致（`/api/v1/health`、`/healthz`）
