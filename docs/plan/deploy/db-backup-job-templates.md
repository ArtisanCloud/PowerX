# 数据库备份组件：实际调用与调度入口

> 更新：2026-10-15。替代早期概念脚本模板，使用仓库已有的 CLI／API 与调度合同。以下只涉及数据库组件，不是系统快照命令；统一包、应用验证与正式切换已本地实现，生产第二轮待验收，见[系统快照计划](system-snapshot-and-restore.md)。

## 1. 配置与前提

- 使用与目标 Core 一致的配置、EnvironmentFile、用户及工作目录；先只读核对实际源库和保存目录。不要把示例凭据写入正式环境。
- PostgreSQL 客户端、备份目录权限和恢复所需扩展齐全；真实隔离恢复需要创建测试库的权限。
- 当前策略来源是备份中心，不从环境变量中的任意数据库名切换源库。
- 原始脚本由服务构造参数：`backend/scripts/ops/backup-db.sh`、`cleanup-backups.sh`、`restore-drill.sh`。不要复制旧模板中的固定测试库 DROP、文件扫描删除或注释上传代码作为生产执行器。

## 2. 现有 CLI

在已加载该实例真实环境的终端中，使用实际部署的 `database` 程序和配置。以下占位变量需先由运维设置为已核对的路径／ID，不能直接套用其他实例。

```bash
"$POWERX_DATABASE_BIN" backup-run -config "$POWERX_CONFIG_FILE" -policy-id "$BACKUP_POLICY_ID"
```

预期退出 0，作业 status 为 success，并有非空 dump、大小、SHA256 和独立清单。失败检查返回错误和 Trace；没有文件不能只看旧成功记录。

不传 `-policy-id` 时只扫描到期的启用策略：

```bash
"$POWERX_DATABASE_BIN" backup-run -config "$POWERX_CONFIG_FILE"
```

数据库组件恢复验证：

```bash
"$POWERX_DATABASE_BIN" backup-restore-verify -config "$POWERX_CONFIG_FILE" -job-id "$BACKUP_JOB_ID"
```

默认只创建并清理本次独立验证库；需保留用于业务核对时显式加 `-keep-restore-database`。只能清理报告中确认由本次创建的对象，不使用固定库名、不覆盖原库。组件验证不恢复媒体、配置或正式连接。

## 3. 现有 Admin API

示例为自建本地 Core，地址改为实际端口；使用自己的管理员用户 JWT。对已经确认的数据库策略触发备份：

```bash
curl -fsS -X POST http://127.0.0.1:8077/api/v1/admin/ops/backup/jobs/run \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"policy_id\":\"${BACKUP_POLICY_ID}\"}"
```

查询作业确认状态、真实产物和校验信息；不是只检查 HTTP 是否成功。保留与清理由现有服务按登记产物和策略锁执行，不按目录 `find -delete` 绕过保护。

需要保留恢复库时使用当前页面“备份中心 → 备份操作 → 恢复到新库”，见 [US5](../../guides/deploy/027-monitor-center/usecase-us5-retained-copy.md)。不把系统快照完整包提交到仅接受 PostgreSQL `.dump` 的接口。

## 4. 实际 systemd 与 Docker 调度

仓库已有 `deploy/powerx/systemd/powerx-backup.service`／`.timer` 及 dev 对应单元，安装入口为 `backend/scripts/ops/install-backup-timer.sh`。正式／开发必须选择各自 links root、配置和环境文件；实际服务用户以部署为准。

独立 timer 与 Core 调度检查同一批策略，通过数据库锁防重复执行。timer 存在不代表策略已启用；Core 调度启用也不证明独立 timer 已安装。先核对真实服务和下一次执行，再按 [US1](../../guides/deploy/027-monitor-center/usecase-us1-policy-automation.md)操作。

Docker 的脚本、配置和数据卷使用当前 [容器部署说明](../../../deploy/powerx/container/README.md)，不额外安装一份无法与现有锁／策略协同的 cron 模板。

## 5. 统一快照与验收边界

未来系统快照由一个父策略协调数据库、媒体、配置和版本，而不是继续新增互不关联的定时任务。旧策略继续有效，不能仅修改名称或把两个任务调到同一分钟就宣称统一。

系统快照 API／协调器 CLI 见[托管恢复手册](../../guides/deploy/027-monitor-center/usecase-us8-managed-recovery.md)；本文保留数据库组件命令。研发任务见 [T077–T100](../../../specs/027-monitor-center/tasks.md)，两轮验收见 [US8](../../guides/deploy/027-monitor-center/usecase-us8-system-snapshot-acceptance.md)。
