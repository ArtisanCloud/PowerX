# 00. systemd 依赖版本与安装（PowerX / PostgreSQL / Redis）

## 1. 目标
给 systemd 部署提供统一依赖口径：
- PowerX 版本（`POWERX_VERSION`）
- PostgreSQL 版本（含 `pgvector`）
- Redis 版本

## 2. 版本兼容口径（推荐）

与官方 Docker 基线对齐（仓库 `deploy/powerx/docker/compose.prod.yaml`）：
- PostgreSQL：`16`（镜像基线：`pgvector/pgvector:pg16`）
- Redis：`7`（镜像基线：`redis:7-alpine`）

推荐矩阵：
- PowerX `${POWERX_VERSION}`（例如 `v1.0.0` / `v2.0.2`）
- PostgreSQL `16` + `pgvector` 扩展可用
- Redis `7`

说明：
- 生产建议严格使用上述推荐组合，避免跨大版本兼容差异。
- 若你使用外部 PostgreSQL，必须确认目标库可用 `vector` 扩展（`CREATE EXTENSION vector;`）。

## 3. Ubuntu 安装示例（可直接执行）

### 3.1 PostgreSQL 16（先加官方 PGDG 源）
你如果直接 `apt install postgresql-16` 报 `Unable to locate package`，说明系统默认源里没有 PG16，需要先加官方 PGDG 源。

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg lsb-release

sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
  | sudo gpg --dearmor -o /etc/apt/keyrings/postgresql.gpg

# 注意：必须单行写入，避免 malformed entry
printf 'deb [signed-by=/etc/apt/keyrings/postgresql.gpg] http://apt.postgresql.org/pub/repos/apt %s-pgdg main\n' "$(lsb_release -cs)" \
  | sudo tee /etc/apt/sources.list.d/pgdg.list >/dev/null

sudo apt-get update
sudo apt-get install -y postgresql-16 postgresql-client-16

sudo systemctl enable --now postgresql
```

### 3.2 pgvector 安装与初始化

优先尝试 apt 包：
```bash
sudo apt-get install -y postgresql-16-pgvector
```

创建业务账号、数据库与扩展（以 `powerx` 库为例）：
```bash
# 1) 创建账号（密码请替换为强密码）
sudo -u postgres psql -c "CREATE USER powerx WITH PASSWORD 'CHANGE_ME_STRONG_PASSWORD';"

# 2) 创建数据库并指定 owner
sudo -u postgres psql -c "CREATE DATABASE powerx OWNER powerx;"

# 3) 最小权限（同 owner 场景可省略）
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE powerx TO powerx;"

# 4) 在业务库启用 pgvector 扩展
sudo -u postgres psql -d powerx -c "CREATE EXTENSION IF NOT EXISTS vector;"
```

如果你已经有数据库，只需执行扩展初始化：
```bash
sudo -u postgres psql -d powerx -c "CREATE EXTENSION IF NOT EXISTS vector;"
```

如果 `postgresql-16-pgvector` 仍找不到，使用源码安装：
```bash
sudo apt-get install -y build-essential git postgresql-server-dev-16
git clone --branch v0.8.0 https://github.com/pgvector/pgvector.git /tmp/pgvector
cd /tmp/pgvector
make
sudo make install
sudo -u postgres psql -d powerx -c "CREATE EXTENSION IF NOT EXISTS vector;"
```

PowerX `config.yaml` 常用 DSN 示例：
```text
postgres://powerx:CHANGE_ME_STRONG_PASSWORD@127.0.0.1:5432/powerx?sslmode=disable
```

若使用云数据库（要求 TLS）：
```text
postgres://powerx:CHANGE_ME_STRONG_PASSWORD@<rds-host>:5432/powerx?sslmode=require
```

### 3.3 Redis 7
```bash
sudo apt-get update
sudo apt-get install -y redis-server
sudo systemctl enable --now redis-server
```

若发行版仓库不是 7.x，请改用官方 Redis APT 源安装 7.x，或使用容器运行 Redis 7。


## 4. 版本与连通性自检

### 4.1 版本检查
```bash
psql --version
redis-server --version
id postgres
```

### 4.2 PostgreSQL 扩展检查
```bash
sudo -u postgres psql -d powerx -c "SELECT extname FROM pg_extension WHERE extname='vector';"
```

### 4.3 Redis 连通检查
```bash
redis-cli ping
```

## 5. 与 PowerX 配置的对应关系

你至少要在 `config.yaml` 里确认：
- `database.dsn`（PostgreSQL）
- `cache` 与 `queue.redis`（Redis）

参考：
- `../00-required-config.md`
- `01-deploy-config-start.md`

## 6. 常见报错对照

1. `E: Unable to locate package postgresql-16`
- 原因：默认 apt 源没有 PG16。
- 处理：按 3.1 先加 PGDG 源再安装。

2. `E: Malformed entry ... /etc/apt/sources.list.d/pgdg.list (Component)`
- 原因：`pgdg.list` 被写成多行或格式错误。
- 处理：按 3.1 用 `printf` 单行重写 `pgdg.list`。

3. `sudo: unknown user postgres`
- 原因：PostgreSQL 尚未安装或服务未初始化成功。
- 处理：
  - 先执行 3.1 安装 PostgreSQL；
  - 再执行 `sudo systemctl enable --now postgresql`；
  - 用 `id postgres` 确认系统用户存在。

4. `Unable to locate package postgresql-16-pgvector`
- 原因：当前发行版/镜像未提供该包。
- 处理：按 3.2 使用源码安装 `pgvector`。
