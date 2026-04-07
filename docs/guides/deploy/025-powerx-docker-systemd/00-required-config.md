# 00. 必须配置项清单（Deploy Required Config）

本清单用于部署前核对“必须配置对象”，避免服务可启动但不可用。

适用范围：
- systemd 部署
- docker 部署（字段语义一致）

---

## 0) 端口默认策略（开发/生产分离）

默认值按 `POWERX_ENV` 生效：

| 环境 | web-admin | backend | grpc |
|---|---:|---:|---:|
| dev | 3030 | 8077 | 9001 |
| prod | 3000 | 8080 | 9010 |

覆盖优先级（高 -> 低）：
1. 进程环境变量（`POWERX_WEB_ADMIN_PORT`、`POWERX_BACKEND_PORT`、`POWERX_GRPC_PORT`）
2. setup 保存的端口配置（`/api/v1/admin/setup/status` 可见 `desired_ports`）
3. 配置文件默认值（按 `POWERX_ENV` 选择 dev/prod）

生效语义：
- setup 修改端口后，若 `restart_required=true` 表示当前进程尚未生效，需重启 backend/web-admin。

---

## 1) backend/etc/config.yaml（必须）

至少要改下面这些对象（不要使用示例默认值）：

### 1.1 `server`
- `server.mode`：生产环境建议 `release`
- `server.secret_key`：必须替换为你自己的 32 字节随机 Base64 密钥

示例（生成密钥）：

```bash
umask 077 && head -c 32 /dev/urandom | base64
```

### 1.2 `database`
- `database.dsn`：必须指向真实 PostgreSQL
- 推荐同时核对：`database.host/port/username/password/database`
- 若启用知识库向量能力（pgvector）：目标库必须可用 `vector` 扩展（已安装，且账号具备创建扩展权限或已由 DBA 预装）

### 1.3 `cache`
- `cache.host`
- `cache.port`
- `cache.password`（有密码时必须配置）

### 1.4 `queue.redis`
- `queue.redis.addr`
- `queue.redis.password`（有密码时必须配置）

### 1.5 `auth`
- `auth.jwt_secret`：必须替换，禁止使用示例值

### 1.6 `media.s3`（启用对象存储时必须）
- `media.s3.endpoint`
- `media.s3.bucket`
- `media.s3.access_key`
- `media.s3.secret_key`

---

## 2) /etc/powerx/powerx.env（必须）

来源模板：`backend/.env.example`（打包后在 `dist/.../config/powerx.env`，兼容保留 `powerx.env.example`）

至少确认：
- `POWERX_ENV`
- `POWERX_MODE`
- `DATABASE_DSN`
- `REDIS_ADDR`
- （Docker 内置库模式）`POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD`

---

## 3) web-admin 环境变量（按需）

来源模板：`web-admin/.env.example`（打包后在 `dist/.../config/web-admin.env`，兼容保留 `web-admin.env.example`）

最常用必改项：
- `POWERX_BACKEND`（指向 backend 地址）
- `WS_UPSTREAM`（指向 websocket 地址）

如果 web-admin 仍与 backend 共用 `/etc/powerx/powerx.env`，请确保 service 与实际变量来源一致。

---

## 4) 部署前最小核对（建议逐条打勾）

- [ ] `backend/etc/config.yaml` 中 `server.secret_key` 已替换
- [ ] `backend/etc/config.yaml` 中 `auth.jwt_secret` 已替换
- [ ] PostgreSQL 可连通（`database.dsn` 正确）
- [ ] Redis 可连通（`cache` 与 `queue.redis` 正确）
- [ ] `/etc/powerx/powerx.env` 已从模板复制并改成真实值
- [ ] web-admin 的 `POWERX_BACKEND/WS_UPSTREAM` 指向正确地址
