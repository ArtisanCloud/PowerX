# PowerX 部署方案总览（Docker + systemd）

本文档是 PowerX 在生产环境的部署总规划，覆盖两种运行面：

- Docker（首发主方案，兼容后续 K8s 演进）
- 非 Docker（systemd 管理生命周期）

同时覆盖当前阶段的核心诉求：**插件市场未完成前，采用手动安装插件并保证升级丝滑**。

## 1. 目标与边界

### 1.1 目标

- 同机同域部署（Nginx 统一入口）：
  - `/` -> `web-admin`
  - `/api` -> `backend`
  - `/_p/*` -> 由 backend 插件路由处理
- 插件升级采用“同机双版本切换”：
  - 安装新版本但不启用
  - 验证通过后 `switch_version` 原子切换
  - 失败时快速回切 N-1
- 目录、配置、健康检查、回滚流程在 Docker/systemd 两套方案间保持一致。

### 1.2 当前边界（首发）

- 插件来源：离线包 + 本地导入（`install/local`）
- PostgreSQL / Redis / MinIO 默认作为外部依赖
- 不引入插件市场在线安装链路作为生产默认路径

## 2. 文档索引

- [Docker 生产部署方案](./docker.md)
- [systemd 生产部署方案](./systemd.md)
- [Nginx 同域反代方案](./nginx.md)
- [插件安装与平滑升级 SOP](./plugin-upgrade-sop.md)
- [插件数据库部署环境隔离计划](./plugin-database-isolation.md)
- [上线验收与演练清单](./checklist.md)
- [Docker 镜像存储策略（独立讨论）](./image-registry-strategy.md)
- [生产日志方案：Loki + Grafana](./logging-loki-grafana.md)
- [生产数据库备份与清理策略](./db-backup-and-retention.md)
- [数据库备份任务模板（脚本 + 定时任务）](./db-backup-job-templates.md)
- [PowerX 实例迁移指南（A -> B，含表结构）](./powerx-instance-migration.md)
- [运维管理控制台路线图（页面/API/权限/分期）](./management-console-roadmap.md)
- [运维管理控制台 P0 实施任务清单](./management-console-p0-tasks.md)
- [PowerX 运维治理 Quickstart 验证记录](../../../specs/025-powerx-docker-systemd/quickstart.md)

## 3. 关键 API（插件生命周期）

- `POST /api/v1/admin/plugins/install/local`
- `POST /api/v1/admin/plugins/:id/switch_version`
- `POST /api/v1/admin/plugins/:id/uninstall`
- `GET /api/v1/admin/plugins/:id/status`
- `GET /api/v1/admin/plugins`

## 4. 推荐目录规范（两种部署方式共用）

```bash
/opt/powerx/
  releases/                  # 应用版本目录（用于回滚）
    backend-<version>/
    web-admin-<version>/
  current/                   # 当前生效软链
    backend -> ../releases/backend-<version>
    web-admin -> ../releases/web-admin-<version>
  shared/
    config/
      config.yaml
      powerx-backend.env
    logs/
    runtime/
  plugins/
    registry.json
    installed/
    market_cache/
```

## 5. 配置基线（必须项）

### 5.1 `config.yaml`（关键）

- `deployment.env: prod`（必填；仅允许 `dev/test/staging/prod`，插件数据库对象命名的唯一环境来源）
- `plugin.enabled: true`
- `plugin.registry_file: /opt/powerx/plugins/registry.json`
- `plugin.installed_dir: /opt/powerx/plugins/installed`
- `plugin.market_cache_dir: /opt/powerx/plugins/market_cache`
- `plugin.auto_restore_parallelism: 2`（按机器规格可调）

`deployment.env` 是 PowerX 实例级身份，不属于 `plugin:`。不得从 `version`、`plugin.dev_mode`、目录名或插件安装请求的 `metadata.environment` 推导。详细命名、失败和迁移规则见[插件数据库部署环境隔离计划](./plugin-database-isolation.md)。

### 5.2 环境变量（建议放 EnvironmentFile）

- 服务：`CORE_X_SERVER_PORT`、`CORE_X_SERVER_MODE`
- 鉴权：`CORE_X_AUTH_JWT_SECRET`（生产必须强随机）
- 数据库：`CORE_X_DB_HOST/PORT/USERNAME/PASSWORD/DATABASE/SSL_MODE`
- Redis：`CORE_X_EVENT_BUS_REDIS_ADDR`、`CORE_X_EVENT_FABRIC_REDIS_ADDR`
- 存储：`CORE_X_STORAGE_DEFAULT_DRIVER`、`CORE_X_STORAGE_S3_*`
- 插件发布：`CORE_X_PLUGIN_RELEASE_*`

## 6. 统一发布策略

- 应用发布：新版本部署 -> 健康检查 -> 切流 -> 保留旧版本用于回滚
- 插件发布：安装不启用 -> 健康验证 -> 切换版本 -> 观察窗口 -> 清理旧版本（可选）

## 7. 预发布与门禁

- 发布前阻断：`bash backend/scripts/ops/pre-release-gate.sh`
- 覆盖率门禁（默认 >=80%）：`bash backend/scripts/ci/coverage-gate.sh`
- 性能烟测门禁（默认 p95<200ms）：`bash backend/scripts/ci/perf-smoke.sh`
