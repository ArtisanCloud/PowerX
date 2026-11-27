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

# Dev unit 默认使用 powerx 作为 systemd service 用户。
# 如果这台机器尚未部署过生产 PowerX，powerx 用户可能不存在；必须先创建，否则后续 setfacl 的 u:powerx 会失败。
getent passwd powerx >/dev/null || sudo useradd --system --home /nonexistent --shell /usr/sbin/nologin powerx
getent passwd ubuntu >/dev/null
getent passwd powerx >/dev/null

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
- 如果登录用户不是 `ubuntu`，需要把上述 ACL 中的 `u:ubuntu` 替换为实际维护用户；`powerx` 用户默认由 dev unit 使用。
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
deployment:
  env: dev

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

`deployment.env: dev` 是 dev Core 的稳定部署身份。它用于区分 PostgreSQL 集群级的插件 Role/User；插件 Schema/Database 名称保持不变，数据隔离仍由 dev 的独立 Core 数据库承担。不得用 `/opt/powerx-dev` 路径或 `plugin.dev_mode` 代替该配置。

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

### 7.1 Redis 实例命名空间

Redis 隔离直接使用环境管理已有的环境 ID：当前安装的 `installation.json.id`，也就是环境管理响应的 `active_environment_id`。首次安装由现有安装生命周期登记一次，后续启动读取同一条记录，不增加 namespace 配置。`queue.namespace` 和 `CORE_X_QUEUE_NAMESPACE` 已移除，遗留设置会明确报错。

所有宿主 Redis key 与频道统一以 `powerx:<环境 ID>:` 开头，覆盖通用缓存、运行态缓存、任务队列及其重试/租约、授权缓存/失效、限流、通知和媒体事件。缓存恢复可在该前缀内切换缓存代次，不改变环境 ID。`deployment.env` 仍是 dev/prod 标签，不替代环境 ID；数据库名称、连接凭据和 Redis DB 号也不参与身份计算。同一实例的 Core、维护命令和 `media_tool` 都读取现有环境记录；环境记录缺失或无效时，离线命令不会自行生成 ID 或降级为公共前缀。

独立创建的环境使用各自的环境 ID；同一环境的进程共享该记录。克隆整套目录时，不能将原环境身份记录当成新环境复用，应通过环境创建流程登记新环境。插件卸载、重装、升级和 Redis DB 调整均不改变宿主环境 ID。

插件 key 在宿主前缀下增加已安装 plugin ID。宿主向 `host-values.yaml` 写入 `host.environment_id`、`host.redis.key_prefix` 和 `event_bridge`，并注入 `POWERX_ENVIRONMENT_ID`、`POWERX_REDIS_KEY_PREFIX`。首次安装、重装、升级和启用都会按当前环境重算绑定，安装包默认值与旧版本 seed 不能覆盖。支持这些字段的现有 AI Craft 内部事件队列无需手改版本目录；插件自行创建的缓存和 Redis 客户端是否消费宿主前缀，需由对应插件维护者核实；本次工作只修改 PowerX，不修改插件源码。宿主注入不等于任意外部程序的 Redis 权限控制。

**已有队列的切换**：先暂停提交并按环境、任务记录核对旧队列的待处理、延迟、处理中与 pending 消息。非空时先按归属完成消费或显式迁移，不能清空共享 DB，也不能同时读取新旧队列作为兼容回退。确认后再部署与重启；切换命名空间不会自动搬移或删除历史数据。回滚须同时保留原程序、配置和新作用域内未完成任务的清单。

验收使用同一个 Redis、同一个 DB，分别向两套实例提交任务，验证只由各自的 worker 消费；对相同 tenant/plugin/key 验证缓存互不可见，对同名事件验证 Pub/Sub 互不接收。重装和升级插件后，再核对 `host-values.yaml` 的流前缀及实际连接。DB 号仍可保留现有 5/6 分配；任务持久化、缓存淘汰策略与独立 Redis 实例的资源隔离需分别管理。

备份与恢复只捕获、清理和恢复当前实例前缀，快照记录作用域并拒绝跨作用域恢复；当前维护流程仍保留停止业务和排除同 DB 外部连接的检查。旧版无命名空间的 Redis 维护快照不静默转换，升级前需要另行准备兼容的回滚现场。

显式共享的物理模型池仍按部署配置中的 `pool_id` 协调同一个模型服务的真实容量，不因 Core 实例命名空间而重复计算物理槽位。

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
POWERX_PUBLIC_BASE_URL=https://${POWERX_DEV_DOMAIN}
POWERX_PUBLIC_WS_ORIGIN=wss://${POWERX_DEV_DOMAIN}

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
export POWERX_SERVICE_USER=powerx
export POWERX_SERVICE_GROUP=powerx
sudo -E bash backend/scripts/ops/install-develop-systemd-units.sh --with-runner
```

该脚本会写入：
- `/etc/systemd/system/powerx-dev-backend.service`
- `/etc/systemd/system/powerx-dev-web-admin.service`
- `/etc/systemd/system/powerx-dev-runner.service`

这些 unit 会读取 `/etc/powerx-dev/powerx.env`，并通过 `POWERX_CONFIG=/etc/powerx-dev/config.yaml` 使用 dev runtime config。

前端启动逻辑由安装脚本同步至 `/etc/powerx-dev/start-web-admin.sh`，unit 仅使用 `/bin/bash` 执行该文件。端口读取不再嵌套于 systemd 的内联 awk 命令；注释中的数字不会拼入端口。刷新 unit 后，由服务负责人重启 Web 才加载新入口；发布切换默认不覆盖 dev unit，因此仅切换 release 不会修复旧的启动命令。

Runner 为可选组件。没有 `runner/dist/main.js` 时，`ExecCondition` 跳过启动，`Restart=on-failure` 避免持续重试。此时 inactive 属于明确的未启动状态，不表示 Runner 已运行；Core 内部 TaskBus worker 与该可选 Node 服务分别核验。

说明：
- `install-develop-systemd-units.sh` 默认使用 `powerx:powerx`。
- 后续 `switch-develop-systemd.sh` 会调用底层 `switch-release-systemd.sh`；底层脚本在 sudo 场景默认使用 `SUDO_USER`（通常是 `ubuntu`）作为 service 用户。
- 因此 dev 环境建议显式导出 `POWERX_SERVICE_USER=powerx` / `POWERX_SERVICE_GROUP=powerx`，并通过 `sudo -E` 传入脚本，避免 unit 安装与 release 切换时的运行用户不一致。

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

### 11.1 显式接入存储观测，不接管旧清理器

已有开发实例缺少 `storage_governance` 时，可使用 `backend/scripts/ops/configure-develop-storage.py`。需要 Python 3、PyYAML，以及已经完成的 Core 集中迁移。先核对实际主机、文件系统 UUID、PostgreSQL `system_identifier` 和 Core 服务用户，不复制另一安装的标识或运行账本。

```bash
python3 backend/scripts/ops/configure-develop-storage.py --config /etc/powerx-dev/config.yaml --storage-root /opt/powerx-dev/storage --host-id '<verified-host-id>' --filesystem-id '<verified-filesystem-id>' --postgres-cluster-id '<verified-postgres-cluster-id>' --service-user '<actual-core-service-user>' --output-plan /absolute/private/storage-observation-plan.json
sudo python3 backend/scripts/ops/configure-develop-storage.py --apply-plan /absolute/private/storage-observation-plan.json --backup /absolute/private/config.before.yaml
```

第一条只读生成 0600 计划。第二条在核对原配置摘要后原子追加观测配置，并创建由实际 Core 服务用户持有的 0700 元数据目录；配置文件原属主保持。备份目录必须为 0700。现有配置、运行目录、身份或文件内容变化时拒绝，不清空旧账本。其他模块原文和配置保持，正式环境及链接目标不允许执行。

此入口接入容量／数据库采样和配置控制所需运行时，保持 `allow_enforce`、`manage_native_retention`、`admission_enabled`、`advanced_operations_enabled` 为 false。不删除文件／数据库数据，不接管旧脚本，不停旧 timer，也不保存或启用 UI 策略。已有保护入口不会由本命令关闭；该命令只适用于此前尚无本模块配置的实例。

应用后由服务负责人重启 Core。验收当前实例身份、运行时依赖与实际插件只读调用；出现样本后再核对用量历史。UI 应区分运行时就绪和策略尚未接入。清理策略、容量准入及旧执行器交接仍按[存储操作手册](../../027-monitor-center/usecase-us9-storage-execution.md)单独审阅和启用，当前核心发布不开放高级远端归档／现场接管。

启动前需要撤回时，先确认 Core 仍为原 PID、没有使用新配置，保留备份并由配置负责人恢复原文；已启动并形成账本／UI 策略后不得直接还原旧文件或删除元数据目录，应按正式暂停、核对和恢复流程处理。

## 12. 后续切换 dev release
切换 dev release 有两种方式。推荐使用 `switch-develop-systemd.sh`，它会固定 dev root、dev service name 与 dev health url，避免误操作生产 service。

### 12.1 使用切换脚本
```bash
export POWERX_DEV_VERSION=develop-dev-$(date +%Y%m%d-%H%M)
make dist DIST_VERSION=${POWERX_DEV_VERSION} NPM_INSTALL=0
rm -rf /opt/powerx-dev/releases/${POWERX_DEV_VERSION}
mv dist/systemd/${POWERX_DEV_VERSION} /opt/powerx-dev/releases/

export POWERX_SERVICE_USER=powerx
export POWERX_SERVICE_GROUP=powerx
sudo -E bash backend/scripts/ops/switch-develop-systemd.sh ${POWERX_DEV_VERSION} --with-runner --without-setup-trace
```

说明：
- `switch-develop-systemd.sh` 内部调用 `switch-release-systemd.sh`，但默认设置了 `POWERX_SYNC_SYSTEMD_UNITS=0`，不会从 release 里复制默认生产 unit 文件。
- backend health check 端口默认从 `/etc/powerx-dev/config.yaml` 的 `server.port` 读取，读不到才 fallback 到 `8081`。
- 如果 `/etc/powerx-dev/config.yaml` 不存在，首次切换会自动初始化，并写入 backend `8081`、web-admin `3001`。
- 默认 dev root 是 `/opt/powerx-dev`，默认 dev runtime config 是 `/etc/powerx-dev`。
- 默认 dev service 是 `powerx-dev-backend`、`powerx-dev-web-admin`、`powerx-dev-runner`。
- 如需临时覆盖 dev root 或 service name，仍可通过 `POWERX_RELEASES_ROOT`、`POWERX_LINKS_ROOT`、`POWERX_RUNTIME_ROOT`、`POWERX_BACKEND_SERVICE` 等环境变量覆盖。

### 12.1.1 切换后的 migrate 和 seed

发布切换不会自动执行 migrate 或 seed。需要按变更类型显式执行：

```bash
cd /opt/powerx-dev/backend

# 有数据库结构变更时执行
sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml ./database migrate

# 需要补齐基础种子数据和 Capability Registry 时执行
sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml ./database seed
sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml ./platform_capability_seed
```

seed 命令语义按目录区分：

- `/opt/powerx-dev/backend` 是发布产物目录，不使用 `make`。
- `./database seed`：只执行 CoreX / 数据库基础种子。
- `./platform_capability_seed`：只把 `config/platform_capabilities/*.yaml` 同步到 Capability Registry，并为 active tenants 补齐 registrations。
- 源码目录才使用 `make seed`，例如 `cd /home/ubuntu/workspace/PowerX && sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml make seed`。

如果只是改 Go 业务逻辑或前端页面，通常不需要 seed。如果新增或修改了 `backend/config/platform_capabilities/*.yaml`，切换新 release 后需要执行 `./platform_capability_seed`；如果同时需要基础数据补齐，再执行 `./database seed`。

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

生产环境可以继续使用默认参数。dev 环境必须使用 `switch-develop-systemd.sh` 或同时覆盖 root、service name 和 health url；不能只覆盖 root，否则仍可能重启默认生产 service。
