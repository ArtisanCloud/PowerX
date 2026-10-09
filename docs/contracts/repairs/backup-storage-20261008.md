# 数据保全与备份保存路径修复（2026-10-08）

## 已确认的事实

- 正式插件实际 TCP 连接为 `localhost:5432/powerx_pro`，schema 为 `px_com_powerx_plugins_ai_craft`。
- 开发插件实际 TCP 连接为 `localhost:5432/powerx_dev`，schema 名相同。
- 对实际插件连接和 PostgreSQL Unix socket 分别进行只读查询，两者的服务器启动时间、数据库 OID 和业务表计数一致。
- 正式库设计任务、小样轮次、订单、商品表当前均为 0 条。
- 开发库设计任务、小样轮次、订单当前各为 19 条。
- 当前正式库和开发库的备份策略、备份任务、备份产物记录均为 0；未发现本机已有的数据库备份文件。
- 当前正式库设计任务表的文件时间为 `2026-08-26 13:27:40 +08:00`，数据库统计中该表插入、修改、删除计数均为 0。文件时间与统计只是辅助线索，不能单独确定历史数据缺失的原因。

当前没有查明原正式站数据的来源和缺失操作，不将正常卸载、数据库绑定初始化日志或证书操作认定为数据删除的证据。

## 本轮保全产物

独立目录：

```text
/opt/powerx/storage/backups/incident-20261008T094203Z/
```

| 文件 | 字节数 | SHA-256 |
| --- | ---: | --- |
| `powerx_pro.dump` | 4923294 | `fae85230cf61df9c9506706e2b30795abe330776949fe87e4d6069c768dca0f6` |
| `powerx_dev.dump` | 16578313 | `187f6a78a644cbf1110c9f827d6d6cf670d04ca2cb396e4994bf79fe9b529ef0` |

这是本轮保存的当前状态，不能作为数据缺失前的历史备份。目录权限为 0700，dump 文件权限为 0600。

开发库 dump 已完整恢复到临时数据库 `powerx_backup_verify_20261008095020`，恢复后设计任务、订单、小样轮次各为 19 条。该临时数据库已经删除，正式库和开发库未进行恢复写入。

## 保存路径问题及修复

旧实现通过 `APP_ENV` / `POWERX_ENV` 选择保存目录，正式服务未提供这两个变量时走项目根目录推导，导致实际路径为：

```text
/opt/powerx/releases/prod-20261008-1355/backend/backend/tmp/ops-backup/artifacts
```

修复后的路径规则：

1. 显式配置 `POWERX_OPS_BACKUP_ARTIFACT_DIR` 时使用该绝对路径。
2. 否则使用 `POWERX_LINKS_ROOT/storage/backups`。
3. 本地未配置部署根目录时使用用户主目录下的 `.powerx/storage/backups`。
4. 相对路径明确失败，不使用当前工作目录或 release 目录推导保存位置。

远程环境文件已写入：

| 服务 | 环境文件 | 保存目录 |
| --- | --- | --- |
| `powerx-backend` | `/etc/powerx/powerx.env` | `/opt/powerx/storage/backups` |
| `powerx-dev-backend` | `/etc/powerx-dev/powerx.env` | `/opt/powerx-dev/storage/backups` |

环境文件原件已私密保存到 incident 目录。新配置需在服务重启后加载，本轮未重启两套 Core 服务。备份策略仍需在备份中心创建并启用，路径修复本身不创建策略或伪造成功任务记录。

## 验证

通过：

```bash
go test ./internal/service/backup_ops -count=1
go vet ./internal/service/backup_ops
bash -n backend/scripts/ops/backup-db.sh
git diff --check
```

回归覆盖：切换 backend 软链接到另一个 release 后路径保持相同且旧产物可读；开发/正式实例根目录隔离；显式绝对路径覆盖；相对路径拒绝；本地默认路径独立于当前工作目录。

尚未完成：原正式站记录的历史来源确认、历史记录恢复、云盘快照/外部备份核对，以及服务重启后的备份中心真实任务验收。

## 删除时间调查

2026-10-08 追加只读调查：

- 当前 systemd 启动命令只执行 `backend/powerx`，没有 `ExecStartPre` / `ExecStartPost` 清库命令；检查版本切换脚本及仓库部署工作流，未发现启动或部署调用 `database refresh`。
- Core 的显式 `database refresh` 命令存在 `DROP SCHEMA ... CASCADE` 入口。代码中存在清库入口不代表它在此次启动中被执行。
- 当前 PostgreSQL `log_statement=none`、`log_min_duration_statement=-1`、`archive_mode=off`，未安装 `pgaudit`。当前设置不能还原未记录的历史 SQL；不能据此断言历史操作不存在。
- 已检查数据库日志及轮转日志、Shell 历史和 sudo 命令记录，未找到对应业务表的清库执行证据。非交互执行或未写回的 Shell 历史不会自动出现在这些记录中。
- 正式库的业务操作日志和业务迁移任务当前均为 0 条。

将当前保留的 9 个 WAL 文件（150994944 字节）保全到 incident 目录的 `wal-evidence`。部分文件已经被 PostgreSQL 回收并重命名，依据 WAL 页头实际地址为副本建立原始段名，使用 PostgreSQL 16 `pg_waldump` 解析，原证据不修改。

可解析的事务提交时间窗口（Asia/Shanghai）：

| 窗口 | 范围 |
| --- | --- |
| 1 | 2026-10-07 22:35:00.451892 — 22:35:34.196138 |
| 2 | 2026-10-07 22:36:00.306665 — 22:38:53.925339 |
| 3 | 2026-10-08 14:52:22.732660 — 17:50:22.891822 |
| 4 | 2026-10-08 17:50:29.059186 — 18:56:03.013040 |

这些窗口均未找到现存正式库设计任务、订单、小样轮次三张表的 WAL 修改记录，亦未找到正式数据库 OID `33770` 的 DROP DATABASE 记录。窗口不连续，既不能推导准确删除时间，也不能排除缺失窗口或其他旧数据位置发生过删除。

原始保全与解析报告：

```text
/opt/powerx/storage/backups/incident-20261008T094203Z/wal-evidence/
/opt/powerx/storage/backups/incident-20261008T094203Z/wal-file-index.json
/opt/powerx/storage/backups/incident-20261008T094203Z/wal-capture-metadata.txt
/opt/powerx/storage/backups/incident-20261008T094203Z/recycled-wal-delete-scan.json
```

表文件的 2026-08-26 时间不是记录删除时间。当前结论仍是：尚未找到原正式站记录的来源或明确删除动作。

## PostgreSQL 写操作日志已启用

按用户明确要求，于 2026-10-08 19:23（Asia/Shanghai）完成配置和验收：

- `log_statement=mod`：记录 DDL 以及 INSERT、UPDATE、DELETE、TRUNCATE、COPY FROM。
- `log_parameter_max_length=0`，`log_parameter_max_length_on_error=0`：不记录协议绑定参数。SQL 正文中的字面量仍可能出现在日志中，日志文件保持 `postgres:adm 0640`。
- `log_connections=on`，`log_disconnections=on`。
- 日志前缀包含时间、PID、数据库用户、数据库名、application name、客户端地址、session ID、事务 ID。
- 保持现有 stderr 文件日志路径：`/var/log/postgresql/postgresql-16-main.log`，配置重载生效，PostgreSQL 进程启动时间保持 2026-06-25，未重启实例。
- logrotate 调整为每日轮转、保留 14 份、`maxsize 100M` 并压缩；debug 验证通过，未强制轮转历史证据。

使用 `postgres` 数据库中的临时表 `powerx_log_probe` 验证了 CREATE、INSERT、DELETE、TRUNCATE、DROP 均写入日志。整个测试事务已回滚；SQL 日志反映语句执行，不单独证明事务提交或实际影响行数。

配置原件及验收报告均私密保存到 incident 目录：

```text
postgresql-logging-before.json
postgresql.auto.conf.before-sql-logging
postgresql-logrotate.before-sql-logging
postgresql-logging-verification.json
```

历史 `refresh` 核对：已检索 Loki 近 30 天、systemd journal、现有 Core 文件日志、Shell/sudo 历史和 PostgreSQL 轮转日志，未找到 refresh/reset 执行证据。database CLI 只使用默认控制台 logger，没有初始化 Core 的 Loki 链路；历史终端输出若未留存，不能凭上述结果排除历史调用。此轮远程操作没有执行 `database refresh`。

## 重新编译安装后的历史启动链路

针对用户所指的 2026-10-07 至 2026-10-08 部署窗口，核对了已发布提交 `4ed054b35184d400a85890b53ddc65ff3f0f0aef` 和 `b0f6d890298790e2b404566cff9cb17952ee81b6` 的 `cmd/app/main.go`、`bootstrap/app.go`、`switch-release-systemd.sh`、`make_files/dist.mk`，以及服务器保留 release 的 systemd 单元与切换脚本。

| 发布阶段 | 启动记录（Asia/Shanghai） |
| --- | --- |
| `prod-20261007-1154` | 2026-10-07 19:15:34 起多次自动重启，22:39:15 再次启动 |
| `prod-20261008-1135` | 2026-10-08 11:38:23 启动 |
| `prod-20261008-1355` | 2026-10-08 14:41:12 启动 |

对应单元只执行 `backend/powerx`，无清库前置/后置命令；上述提交和 release 实物的标准编译、切换、启动链路没有 `database refresh` / `ResetDatabase` 调用。标准启动链路未自动刷新数据库是代码与脚本检查结论，不代表其他未留存的独立手工命令已经全部排除。

正式库当前仍有 8 名 IAM 用户、1 个租户，其创建时间可追溯至 2026-06-25。这是主库仍保存历史记录的事实，不据此排除其他业务数据位置曾发生过删除。
