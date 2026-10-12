# 已安装 PowerX 实例升级合同 v1

状态：Core 代码与发布门禁已实现；**首次真实旧镜像 → 新镜像的 CI 运行尚待本分支提交后执行**。本文与 Schema 固定 CLI/制品合同，不构成当前远程实例可以升级的声明。

关联：[实施计划](../plan/deploy/powerx-system-upgrade.md)、[manifest Schema](./powerx-upgrade-manifest.schema.json)、[CLI result Schema](./powerx-upgrade-result.schema.json)、[固定旧镜像](../../deploy/powerx/container/upgrade-baseline.json)。

## 1. 发布制品与执行前提

每次发布交付 `powerx-upgrade-manifest.json`、文件 SHA256、成对 backend/web-admin OCI index digest、各平台 manifest digest、完整源码 SHA 和对应升级报告。未通过 CI 的镜像是排查制品；没有正式 manifest 时不能按本合同自动升级。

`images.*.platforms` 是构建平台，`verification.tested_upgrade_platforms` 是真实升级验收平台。XDocker 必须确认选定平台在后者内，并验证报告文件 SHA256、来源/目标 SHA 和成对 digest。机器版本号与源码 SHA 分开记录；同 SHA 重建仍以本次 digest 为准。

首轮固定来源 `3d792fd7ea2426825970bceceb0d6e9db46211ef` 的两架构镜像 digest 及 OCI revision 已从 GHCR 核对。实际升级通过报告交付，不能用这次镜像只读核对替代升级验收。CLI 的 `--source-sha` 是编排端核对镜像后的声明，不是历史镜像证明；CLI 还检查真实 IAM/管理员 schema、历史阻塞及履历，不将未执行的历史迁移补记为成功。

v1 只支持离线 PostgreSQL 迁移。发布门禁使用 PostgreSQL 16 + vector 扩展、本地文件存储、`com.powerx.upgrade.fixture@1.0.0` 真实进程插件；其他对象存储、独立向量库和真实业务插件须获得相应验收证据。保留已有绑定不等于声明第三方 SDK/插件兼容。

## 2. XDocker 固定调用顺序

执行主体是有权读取实例私有配置、连接该数据库、管理该实例进程的本机/容器运维身份；不使用浏览器 JWT、API Key 或插件 STS。Core 不增加远程任意迁移 HTTP 接口。

1. 读取可信发布制品，核对来源镜像对、目标镜像对、平台、存储/插件兼容及回滚条件。
2. 对数据库、私有配置、运行文件和声明的外部存储准备匹配备份；验证恢复可用。
3. 预览 `plan`。进入维护状态并停止所有 Core、Worker、插件和其他会写入的进程。迁移锁不能阻止业务写入。
4. 在停写状态再次 `plan`，把这次准确的 `plan_hash` 用于 `apply`。预览期间的业务运行可能使计划变化，不能跳过锁内复核。
5. 目标镜像执行 `apply`。只有退出 0 且 `status=succeeded` 才启动成对目标镜像。设置 `POWERX_UPGRADE_MODE=1` 保持严格配置校验。
6. 验证 installed、原账号密码登录、关键 API、文件和已声明兼容的插件，再解除维护状态。旧格式 access token 可能需要正常重新登录。

容器内固定命令（`<target_sha>` / `<source_sha>` 为完整 40 位 SHA）：

```sh
/app/container/entrypoint.sh upgrade-check
/app/database migrate --action=status --format=json --config=/etc/powerx/config.yaml --data-dir=/data
/app/database migrate --action=plan --format=json --target-release=sha-<target_sha> --source-sha=<source_sha> --config=/etc/powerx/config.yaml --data-dir=/data
/app/database migrate --action=apply --format=json --target-release=sha-<target_sha> --source-sha=<source_sha> --expected-plan=<plan_hash> --confirm-offline --lock-timeout=30s --config=/etc/powerx/config.yaml --data-dir=/data
```

Compose 包的 `sh run.sh migrate ...` 透传上述参数。目标身份编译进 database 二进制，`/app/database --version` 可回读；未注入完整发布 SHA 的开发二进制不能执行目标发布迁移。执行脚本/SQL 不从调用方传入。

`status` / `plan` 不创建或修改迁移表。stdout 只有一份 JSON；不会回显 SQL、DSN、YAML 内容或密码。参数错误同样输出 JSON。底层错误只保留稳定错误码、步骤 ID 与执行 UUID。

| 退出码 | 含义 | 编排动作 |
|---|---|---|
| 0 | status/plan 成功，或 apply 全部成功 | 检查 status；ready 是可预览，succeeded 才是执行完成 |
| 2 | 配置、来源、校验和、历史数据、恢复要求或参数阻塞 | 保持旧实例/维护状态，解决明确阻塞后重新 plan |
| 3 | 锁被占用或计划漂移 | 有界等待/重新 plan；不得沿用过期摘要 |
| 4 | 数据库操作或事务步骤失败 | 检查 incomplete_runs / failed，按 retryable 新建计划重试 |
| 5 | 非事务步骤失败，需要恢复；或结果通道写入失败 | 按 requires_restore / 执行履历判定；结果不明也不能启动目标实例 |

`--lock-timeout` 支持 0–5m。锁绑定实际数据库 + Core schema，全程使用一条专用连接；断线释放，释放失败的连接不回到池。异常退出留下 running 记录；下一次取得锁后才能将已无执行者的记录标为 interrupted。

## 3. 配置与持久化保证

- 配置文件必须已存在、可读、非空、合法、非符号链接、私有权限（0600/0400），且 `install.status=installed`。
- 数据挂载及 uploads、plugins/installed、runtime 目录必须已经存在。不会用 mkdir 隐藏挂载缺失。
- PostgreSQL 绑定、部署环境、JWT/加密密钥必须有效。缺失不能借新模板填入测试数据库、弱密钥或新随机值。
- Go 升级加载器使用默认值补可选字段，再按正式环境变量覆盖；不读取开发 `.env`、不搜索其他配置文件。环境不能改变原 deployment.env；摘要冻结原 YAML 与实际生效配置。
- 本次配置 changes 为空，**不写配置文件**，因此也没有配置补丁回滚。未来有持久配置变更须新增明确补丁合同、原子替换与恢复测试；v1 不能提交任意配置 patch。
- 运行 `POWERX_UPGRADE_MODE=1` 的 init/serve 遇到配置异常直接退出；不会进入首次初始化、生成密钥或重建 Setup。

## 4. 迁移集合、系统数据与失败

复用集中 `MigrateCoreModels`、Agent 与 Knowledge 迁移链路。镜像内 `upgrade_payloads.json` 由源码清单与依赖锁计算，构建必须执行 `upgrade_registry.py --check`；载荷改变产生新的内容寻址 migration ID，旧完成记录、校验和和时间不会覆盖。Knowledge 配置绑定参与实际步骤 ID/校验和；它包含独立连接操作，保守声明为非事务步骤。

Core、Agent、catalog 步骤使用事务，变更、后检查与成功记录同提交。失败保留独立 UpgradeRun，不记成功；此前已完成步骤保留，新计划跳过已完成项。Knowledge 步骤失败或不明中断要求恢复对应备份，不声明跨数据库原子性。

平台 catalog 专用步骤只更新平台合同字段，并补缺失项：保留 permission UUID、用户 effect/status/description、现有 Key/Profile grants/owner、Capability UUID/state/用户工作流引用和已有租户注册版本锁。来源冲突明确失败。它不调用 `SeedCoreX`、`SeedRoot`、默认 API Key Profile、示例数据或默认角色授权；新增能力由管理员显式授权。

旧 IAM `database_migration_records` 扩展 checksum/release_id/run_uuid；新增 `database_upgrade_runs`。历史 IAM 无 checksum 的旧记录保留，不伪造其内容校验和。`current_migration_version=legacy-unversioned` / `current_release=unrecorded` 表示没有全局履历，不表示数据库为空。

无 plugin_id/service_actor 的历史安装会话在 plan 阶段输出 `UPGRADE_LEGACY_PLUGIN_ACTOR_UNRESOLVED`，不增加列、不删除或重解释历史记录。管理员应依据真实插件/服务身份解决归属，或先制定有保留证据的归档变更；v1 不自动执行未审核的归档/删除。重复、UUID 关系等其他数据问题由既有迁移验证阻断并回滚对应事务，不能用 refresh 绕过。

响应结构以 Schema 为准；以下仅展示关键字段，其余字段仍由实际命令返回：

```json
{
  "schema_version": "powerx.migration-result/v1",
  "operation": "apply",
  "status": "failed",
  "error_code": "UPGRADE_STEP_FAILED",
  "partial": true,
  "retryable": true,
  "requires_restore": false,
  "applied": ["20261011_core_<payload_sha256>"],
  "failed": ["20261011_catalog_<payload_sha256>"]
}
```

每次执行返回 UUID，status 可以回读失败/中断履历。错误码还包括 `UPGRADE_CONFIG_UNREADABLE`、`UPGRADE_CONFIG_INVALID`、`UPGRADE_INSTANCE_NOT_INSTALLED`、`UPGRADE_SOURCE_UNSUPPORTED`、`UPGRADE_TARGET_RELEASE_MISMATCH`、`UPGRADE_MIGRATION_CHECKSUM_MISMATCH`、`UPGRADE_PLAN_CHANGED`、`UPGRADE_LOCK_BUSY` 和 `UPGRADE_RECOVERY_REQUIRED`。

## 5. CI、证据与回滚

[publish-container workflow](../../.github/workflows/publish-container.yml) 保留空库安装烟测，增加：

- 隔离 PostgreSQL 的事务失败/重试、幂等、并发、只读计划、旧履历兼容、checksum、中断履历和非事务恢复测试。
- 固定旧镜像实际 Setup、自选管理员密码、Scheduler 样例任务、实际文件上传、插件安装/启用/进程路由及插件数据库最小权限。
- 原配置/凭据/grants/文件摘要，原业务对象与插件 UUID/schema/role/数据回读，新二进制身份和成对前端/后端 digest 验证。
- 新镜像真实 apply 的目录失败、前序部分完成、TERM 中断与新计划重试，计划漂移、非法配置、未知来源和历史数据阻塞。
- 第二次 apply 幂等；修改履历 checksum 明确拒绝。
- 恢复原数据库、配置与文件后启动固定旧镜像，验证旧密码、业务和插件。

两架构验收报告全部通过，才能生成 manifest、打包并发布。未执行、未通过或没有平台报告的版本不交付可执行升级声明。CI SQL 故障仅存在于隔离数据库触发器，生产 CLI 不提供任意故障/SQL 注入参数。

v1 回滚固定 `restore_required`：停止新写入，先导出需要保留的升级后新增数据，再恢复匹配的旧数据库、配置、运行文件和必要外部向量库，最后使用 baseline 中的成对旧 digest。只切换旧镜像不能代替恢复；回到备份时间点会失去未另行保存的新写入。

当前本地证据与未完成的真实 CI 边界见 [实施验收记录](./repairs/installed-upgrade-20261011.md)。XDocker 可按本合同先实现解析、预览与编排；正式执行目录只接纳通过发布门禁的制品。
