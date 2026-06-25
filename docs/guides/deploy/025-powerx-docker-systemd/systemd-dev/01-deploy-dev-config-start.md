# 01. Deploy Dev Config & Start

## 1. 目标
在同一台机器上部署 dev release，并启动独立 systemd service：
- `powerx-dev-backend.service`
- `powerx-dev-web-admin.service`
- `powerx-dev-runner.service`（可选）

## 2. 设置变量
```bash
export POWERX_DEV_DOMAIN=dev.powerx.example.com
export POWERX_DEV_BACKEND_PORT=8081
export POWERX_DEV_WEB_PORT=3001
```

说明：`dev.powerx.example.com` 是示例域名，部署时替换为实际 dev 域名。

## 3. 创建 dev 运行目录
```bash
sudo apt-get update
sudo apt-get install -y acl

sudo mkdir -p /opt/powerx-dev/{releases,storage,plugins}
sudo mkdir -p /etc/powerx-dev
sudo chown -R root:root /opt/powerx-dev /etc/powerx-dev
sudo setfacl -R -m u:ubuntu:rwx,u:powerx:rx /opt/powerx-dev
sudo setfacl -R -d -m u:ubuntu:rwx,u:powerx:rx /opt/powerx-dev
sudo setfacl -R -m u:ubuntu:rwX,u:powerx:rX /etc/powerx-dev
sudo setfacl -R -d -m u:ubuntu:rwX,u:powerx:rX /etc/powerx-dev
```

说明：
- `ubuntu` 是 VSCode Remote-SSH 登录用户，拥有 dev 目录写权限。
- `powerx` 是 systemd service 用户，拥有 release/config 读执行权限。
- `storage` 与 `plugins` 需要服务运行时写入，后续会单独给 `powerx` 写权限。

## 4. 构建 dev release
```bash
cd /home/ubuntu/workspace/PowerX
git checkout develop
git pull --ff-only origin develop

export POWERX_DEV_VERSION=develop-dev-$(date +%Y%m%d-%H%M)
make dist DIST_VERSION=${POWERX_DEV_VERSION} NPM_INSTALL=0
```

## 5. 部署 release 到 dev 根目录
```bash
rm -rf /opt/powerx-dev/releases/${POWERX_DEV_VERSION}
mv dist/systemd/${POWERX_DEV_VERSION} /opt/powerx-dev/releases/

ln -sfn /opt/powerx-dev/releases/${POWERX_DEV_VERSION}/backend /opt/powerx-dev/backend
ln -sfn /opt/powerx-dev/releases/${POWERX_DEV_VERSION}/web-admin /opt/powerx-dev/web-admin
ln -sfn /opt/powerx-dev/releases/${POWERX_DEV_VERSION}/runner /opt/powerx-dev/runner
```

## 6. 准备 dev 数据库
Dev 必须使用独立 database 或独立 schema。推荐使用独立 database。

先从生产配置确认当前数据库用户：

```bash
grep -n "dsn:" /etc/powerx/config.yaml
sudo -u postgres psql -Atc "SELECT rolname FROM pg_roles ORDER BY rolname;"
```

示例：生产 DSN 用户是 `powerx`，则创建 dev database 并授权：

```bash
sudo -u postgres createdb powerx_dev
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE powerx_dev TO powerx;"
sudo -u postgres psql -d powerx_dev -c "GRANT ALL ON SCHEMA public TO powerx;"
sudo -u postgres psql -d powerx_dev -c "ALTER SCHEMA public OWNER TO powerx;"
```

说明：
- `powerx` 是示例 role，必须替换为生产 DSN 中真实使用的数据库 role。
- `createdb powerx_dev` 已执行过时会报已存在，可跳过创建，继续授权。
- database owner 可以仍是 `postgres`；关键是 dev 运行 role 对 database 和 schema 有权限。

验证：

```bash
sudo -u postgres psql -Atc "SELECT datname, pg_catalog.pg_get_userbyid(datdba) AS owner FROM pg_database WHERE datname IN ('powerx','powerx_dev');"
sudo -u postgres psql -d powerx_dev -Atc "SELECT nspname, pg_catalog.pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname='public';"
```

## 7. 准备 dev 配置文件
推荐让 `switch-develop-systemd.sh` 在首次切换时自动初始化 `/etc/powerx-dev/config.yaml`：
- 底层会从 release 包里的 `backend/etc/config.yaml` 复制一份到 `/etc/powerx-dev/config.yaml`。
- dev wrapper 会在首次初始化时写入 dev 默认端口：backend `8081`，web-admin `3001`。
- 后续 `/setup` 或人工维护会继续修改这份外置 runtime config，release 切换不会覆盖。

如果你要在首次启动前就固定数据库、域名、storage 等配置，也可以先创建配置文件：

```bash
sudo mkdir -p /etc/powerx-dev
sudo cp /etc/powerx/config.yaml /etc/powerx-dev/config.yaml
sudo chown root:root /etc/powerx-dev/config.yaml
sudo chmod 0644 /etc/powerx-dev/config.yaml
sudo setfacl -m u:ubuntu:rw,u:powerx:r /etc/powerx-dev/config.yaml
```

必须修改：
```yaml
server:
  port: <dev-backend-port>
  bind_addr: ":<dev-backend-port>"

web_admin_port: <dev-web-port>

gateway:
  base_url: "https://<dev-domain>"

plugin:
  installed_dir: "/opt/powerx-dev/plugins/installed"
  registry_file: "/opt/powerx-dev/plugins/registry.json"

storage:
  local:
    base_path: "/opt/powerx-dev/storage/media"
```

数据库必须隔离。选择一种：
```yaml
database:
  schema: "powerx_dev"
```

或使用独立 database：
```yaml
database:
  dsn: "postgres://<db-role>:<password>@127.0.0.1:5432/powerx_dev?sslmode=disable"
  schema: "public"
```

推荐使用独立 database，并把 DSN 中 database 名改为 `powerx_dev`。

## 8. 准备 dev env
创建 `/etc/powerx-dev/powerx.env`：

```bash
sudo tee /etc/powerx-dev/powerx.env >/dev/null <<EOF_ENV
POWERX_ENV=dev
NODE_ENV=production
LOG_LEVEL=info
NODE_BIN=/usr/bin/node

POWERX_RUNTIME_ROOT=/etc/powerx-dev
POWERX_CONFIG=/etc/powerx-dev/config.yaml
POWERX_LINKS_ROOT=/opt/powerx-dev
POWERX_RELEASES_ROOT=/opt/powerx-dev/releases
POWERX_STORAGE_ROOT=/opt/powerx-dev/storage
POWERX_PLUGIN_RUNTIME_ROOT=/opt/powerx-dev/plugins

POWERX_HTTP_PROXY_BASE=http://127.0.0.1:${POWERX_DEV_BACKEND_PORT}
POWERX_GATEWAY_BASE_URL=https://${POWERX_DEV_DOMAIN}
POWERX_GATEWAY_WS_BASE_URL=wss://${POWERX_DEV_DOMAIN}

POWERX_SUPERVISOR_FORWARD_STDIO=true
POWERX_OPS_SCRIPT_DIR=/opt/powerx-dev/backend/scripts/ops
POWERX_OPS_BACKUP_ARTIFACT_DIR=/var/lib/powerx-dev/ops-backup/artifacts
POWERX_OPS_BACKUP_SCRIPT_TIMEOUT=30m
EOF_ENV

sudo chown root:root /etc/powerx-dev/powerx.env
sudo chmod 0644 /etc/powerx-dev/powerx.env
sudo setfacl -m u:ubuntu:rw,u:powerx:r /etc/powerx-dev/powerx.env
```

如果已按旧路径创建过 `/etc/powerx/powerx-dev.env`，迁移后删除旧文件：

```bash
sudo mv /etc/powerx/powerx-dev.env /etc/powerx-dev/powerx.env
sudo rm -f /etc/powerx/powerx-dev.env
```

## 9. 安装 dev systemd unit
推荐使用仓库脚本生成 `powerx-dev-*` unit，避免手工复制 production unit 后漏改路径或 service name：

```bash
sudo bash backend/scripts/ops/install-develop-systemd-units.sh --with-runner
```

该脚本会写入：
- `/etc/systemd/system/powerx-dev-backend.service`
- `/etc/systemd/system/powerx-dev-web-admin.service`
- `/etc/systemd/system/powerx-dev-runner.service`

这些 unit 会读取 `/etc/powerx-dev/powerx.env`，并通过 `POWERX_CONFIG=/etc/powerx-dev/config.yaml` 使用 dev runtime config。

## 10. 授权运行目录
```bash
sudo mkdir -p /opt/powerx-dev/storage/media /opt/powerx-dev/plugins/installed
sudo touch /opt/powerx-dev/plugins/registry.json
sudo chmod 0644 /opt/powerx-dev/plugins/registry.json

# Release/config：ubuntu 可维护，powerx 可读执行
sudo setfacl -R -m u:ubuntu:rwx,u:powerx:rx /opt/powerx-dev/releases
sudo setfacl -R -d -m u:ubuntu:rwx,u:powerx:rx /opt/powerx-dev/releases
sudo setfacl -R -m u:ubuntu:rwX,u:powerx:rX /etc/powerx-dev
sudo setfacl -R -d -m u:ubuntu:rwX,u:powerx:rX /etc/powerx-dev

# Runtime storage/plugins：ubuntu 可维护，powerx 可运行时写入
sudo setfacl -R -m u:ubuntu:rwx,u:powerx:rwx /opt/powerx-dev/storage /opt/powerx-dev/plugins
sudo setfacl -R -d -m u:ubuntu:rwx,u:powerx:rwx /opt/powerx-dev/storage /opt/powerx-dev/plugins
```

## 11. 启动 dev service
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now powerx-dev-backend.service
sudo systemctl enable --now powerx-dev-web-admin.service

# runner 可选
sudo systemctl enable --now powerx-dev-runner.service
```

## 12. 后续切换 dev release
切换 dev release 有两种方式。推荐使用 `switch-develop-systemd.sh`，它会固定 dev root、dev service name 与 dev health url，避免误操作生产 service。

### 12.1 使用切换脚本
```bash
export POWERX_DEV_VERSION=develop-dev-$(date +%Y%m%d-%H%M)
make dist DIST_VERSION=${POWERX_DEV_VERSION} NPM_INSTALL=0
rm -rf /opt/powerx-dev/releases/${POWERX_DEV_VERSION}
mv dist/systemd/${POWERX_DEV_VERSION} /opt/powerx-dev/releases/

sudo bash backend/scripts/ops/switch-develop-systemd.sh ${POWERX_DEV_VERSION} --with-runner --without-setup-trace
```

说明：
- `switch-develop-systemd.sh` 内部调用 `switch-release-systemd.sh`，但默认设置了 `POWERX_SYNC_SYSTEMD_UNITS=0`，不会从 release 里复制默认生产 unit 文件。
- backend health check 端口默认从 `/etc/powerx-dev/config.yaml` 的 `server.port` 读取，读不到才 fallback 到 `8081`。
- 如果 `/etc/powerx-dev/config.yaml` 不存在，首次切换会自动初始化，并写入 backend `8081`、web-admin `3001`。
- 默认 dev root 是 `/opt/powerx-dev`，默认 dev runtime config 是 `/etc/powerx-dev`。
- 默认 dev service 是 `powerx-dev-backend`、`powerx-dev-web-admin`、`powerx-dev-runner`。
- 如需临时覆盖 dev root 或 service name，仍可通过 `POWERX_RELEASES_ROOT`、`POWERX_LINKS_ROOT`、`POWERX_RUNTIME_ROOT`、`POWERX_BACKEND_SERVICE` 等环境变量覆盖。

### 12.2 手动切换 symlink
也可以只更新 `/opt/powerx-dev` 下的 symlink，并重启 dev service：

```bash
export POWERX_DEV_VERSION=develop-dev-$(date +%Y%m%d-%H%M)
make dist DIST_VERSION=${POWERX_DEV_VERSION} NPM_INSTALL=0
mv dist/systemd/${POWERX_DEV_VERSION} /opt/powerx-dev/releases/

ln -sfn /opt/powerx-dev/releases/${POWERX_DEV_VERSION}/backend /opt/powerx-dev/backend
ln -sfn /opt/powerx-dev/releases/${POWERX_DEV_VERSION}/web-admin /opt/powerx-dev/web-admin
ln -sfn /opt/powerx-dev/releases/${POWERX_DEV_VERSION}/runner /opt/powerx-dev/runner

sudo setfacl -R -m u:ubuntu:rwx,u:powerx:rx /opt/powerx-dev/releases/${POWERX_DEV_VERSION}
sudo setfacl -R -d -m u:ubuntu:rwx,u:powerx:rx /opt/powerx-dev/releases/${POWERX_DEV_VERSION}
sudo systemctl restart powerx-dev-backend powerx-dev-web-admin powerx-dev-runner
```

## 13. 切换脚本边界
`backend/scripts/ops/switch-release-systemd.sh` 默认用于生产环境，默认操作：
- `/opt/powerx/releases`
- `/opt/powerx`
- `/etc/powerx`
- `powerx-backend`
- `powerx-web-admin`
- `powerx-runner`

生产环境可以继续使用默认参数。dev 环境必须按 14.1 同时覆盖 root、service name 和 health url；不能只覆盖 root，否则仍可能重启默认生产 service。
