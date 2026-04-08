# PowerX 实例迁移指南（A -> B，含表结构）

## 1. 结论先行

- 使用 `pg_dump` 可以导出 **表结构 + 数据**，并可全量恢复到另一套 PowerX 的 PostgreSQL。
- 但完整迁移 PowerX 实例，不仅是数据库，还必须同时迁移：
  - 配置（`config.yaml`、环境变量、密钥）
  - 插件目录（`plugins/installed`、`plugins/registry.json`）
  - 对象存储数据（MinIO/S3）

## 2. 适用前提

- 源库和目标库 PostgreSQL 大版本兼容（建议同大版本）。
- 目标库已安装源库使用的扩展（如 `pgvector`）。
- 迁移窗口内可接受短暂停写（建议切换前冻结写流量）。

## 3. 迁移对象清单（必须）

1. 数据库（结构 + 数据）
2. PowerX 配置
3. 插件安装产物
4. 对象存储桶内业务文件

## 4. 数据库迁移步骤（推荐 `-Fc`）

### 4.1 源端导出（含结构 + 数据）

```bash
export PGPASSWORD='<SRC_DB_PASSWORD>'
pg_dump \
  -h <SRC_DB_HOST> \
  -p <SRC_DB_PORT> \
  -U <SRC_DB_USER> \
  -d <SRC_DB_NAME> \
  -Fc \
  -f /tmp/powerx_full_$(date +%Y%m%d_%H%M%S).dump
```

说明：

- `-Fc` 为自定义格式，便于并行恢复与选择性恢复。
- 默认包含 schema（表结构）、数据、索引、约束等对象。

### 4.2 目标端准备

```bash
export PGPASSWORD='<DST_DB_PASSWORD>'
psql -h <DST_DB_HOST> -p <DST_DB_PORT> -U <DST_DB_USER> -d postgres -c "DROP DATABASE IF EXISTS <DST_DB_NAME>;"
psql -h <DST_DB_HOST> -p <DST_DB_PORT> -U <DST_DB_USER> -d postgres -c "CREATE DATABASE <DST_DB_NAME>;"
```

### 4.3 目标端恢复

```bash
export PGPASSWORD='<DST_DB_PASSWORD>'
pg_restore \
  -h <DST_DB_HOST> \
  -p <DST_DB_PORT> \
  -U <DST_DB_USER> \
  -d <DST_DB_NAME> \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  /tmp/powerx_full_xxx.dump
```

### 4.4 快速校验

```bash
psql -h <DST_DB_HOST> -p <DST_DB_PORT> -U <DST_DB_USER> -d <DST_DB_NAME> -c "SELECT now();"
psql -h <DST_DB_HOST> -p <DST_DB_PORT> -U <DST_DB_USER> -d <DST_DB_NAME> -c "\dt"
```

## 5. 非数据库部分迁移（不可省略）

### 5.1 配置与密钥

- 同步 `config.yaml` 的核心段：`plugin`、`storage`、`auth`、`database`、`cache`
- 同步环境变量（尤其 `CORE_X_AUTH_JWT_SECRET`、`CORE_X_STORAGE_*`）

### 5.2 插件目录

- 同步：
  - `plugins/registry.json`
  - `plugins/installed/`
  - `plugins/market_cache/`（如有）

### 5.3 对象存储

- 同步 MinIO/S3 中 PowerX 使用桶及路径。
- 建议按 bucket 级别做一次性镜像复制，避免仅迁移元数据导致文件缺失。

## 6. 切换流程（生产推荐）

1. 在 B 环境完成全量导入与冒烟验证（不对外）。
2. 冻结 A 写流量。
3. 对 A 执行最终增量（或短窗口全量重导）。
4. 切换入口流量到 B（Nginx/LB/DNS）。
5. 观察 30~60 分钟后再解除 A 的保留态。

## 7. 回滚策略

- 保留 A 环境可回切入口（DNS/LB）。
- 切换初期不要破坏 A 数据与配置。
- 回滚触发条件建议预定义：
  - 核心 API 持续失败
  - 登录态异常
  - 插件页面不可用

## 8. 常见风险与规避

- 扩展缺失：目标库未安装 `pgvector` 等扩展导致恢复失败。
- 权限差异：使用 `--no-owner --no-privileges` 规避角色不一致问题。
- 仅迁库未迁对象存储：运行时报“文件不存在”。
- 仅迁库未迁插件目录：插件状态与 UI 不一致。

## 9. 验收清单

- [ ] 目标库表结构、数据量、关键业务表记录数校验通过
- [ ] PowerX 登录、菜单、插件页面正常
- [ ] 关键业务接口冒烟通过
- [ ] 日志无持续错误峰值
- [ ] 已完成回滚演练

## 10. 相关文档

- 管理控制台任务：`./management-console-p0-tasks.md`
- Quickstart 验证记录：`../../../specs/025-powerx-docker-systemd/quickstart.md`
- 预发布门禁脚本：`backend/scripts/ops/pre-release-gate.sh`
