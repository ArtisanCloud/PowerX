# PowerX 临时文件日志守护

> 协议 2 / 2026-11-01：Core 原生归档必须由 UI 保存目标、实际节点验证并明确开启。此仓库的临时接收器缺少有效授权时默认不传输；已由 Core 接管的来源不再交给旧接收器。`custody_policy.py` 和 `bind_core.py` 提供固定节点／来源及脚本哈希绑定，默认绑定文件 `/etc/powerx-space-guard/core-binding.json`。**仓库更新不代表服务器或 Mac 已安装副本已更新。** 逐来源接管、查询与取回、数据库期限及回退见 [v0.6 手册](../../../docs/guides/deploy/027-monitor-center/usecase-us9-storage-archive-controls.md)。既有 Mac 对象继续保留，本文旧安装记录仅作历史参考。


适用于 47.116.10.200，等待新版存储治理上线期间使用。用户于 2026-10-30 明确选择：**仅清理文件日志，数据库只监控，保留事件数据**。

## 当前策略

- systemd 每 5 分钟执行一次短任务；不是常驻监听服务，不占端口。记录互斥锁避免并发。
- PostgreSQL 旧 stderr 文件暂沿用 copytruncate、100 MiB／每天轮转和立即压缩，但已改成 `rotate -1`、移除 `maxage`，原生 logrotate 不能删除未归档段。`postgresql-audit.conf` 已准备原生采集器配置：独立私有目录、每小时／100 MiB 轮转、不覆盖旧文件；需一次明确授权的 PostgreSQL 重启后才激活。重启前 copytruncate 的短暂窗口仍存在，不能宣称已经消除。
- syslog 单独改为同样的高频检查和立即压缩，并从原 rsyslog 规则排除，避免重复轮转。其余 auth 等系统日志保留原配置。轮转只让 rsyslog 重新打开日志，不重启进程。
- 正式／开发审计目录中超过一天、未打开的明文日志仍分批压缩。`guard.py` 对 PostgreSQL 旧／原生文件及正式／开发 PowerX 审计文件**一律不直接删除**，未归档或超预算会记录阻塞／告警。syslog 512 MiB、Nginx 256 MiB 继续按既有七天／最近一天保护执行普通保留。
- `archive_s3.py` 从接收机通过固定 SSH 操作拉取封存文件，写入独立私有、开启版本保留且无自动过期的 S3 桶。数据对象及清单都须按具体版本完整 GET、核对字节数和 SHA-256，才可签发两分钟有效的凭据；服务器 `audit_custody.py` 再检查签名、来源白名单、目录／inode／属主／链接／打开状态及源内容，原子隔离后回收。回收仍按七天／预算策略，预算压力也保留最近 24 小时；无有效归档凭据时这两条均不能触发删除。PostgreSQL 每来源预算 2 GiB、每环境审计 512 MiB。S3 对象没有自动删除任务。
- 当前目标是 Mac 上的 `127.0.0.1:9000` MinIO、私有桶 `powerx-audit-archive`。服务器没有这个本地服务；Mac 每五分钟经 SSH 拉取。Mac／MinIO 离线时不签发删除凭据，源文件保留；候选受阻或容量到阈值时服务器告警。它是服务器的跨主机临时副本，不是常在线云端存储，也不是 Mac 本地数据的异地灾备。
- 系统 journal 每小时轮转，按 7 天／1 GiB 回收关闭段。容量先到时保留时间可能不足 7 天；活动段不属于 vacuum 对象。此操作作用于整台主机的 journal。
- 每小时只读记录 PostgreSQL 各数据库大小。不会清除数据库行、事件队列、业务附件、上传目录、数据库备份、系统快照、Loki 数据或 WAL。
- 可用空间低于 12 GiB 为 warning；低于 8 GiB 或实际文件系统用量达到 88% 为 critical，服务以非零状态退出，journal 和 latest.json 保留原因。不是已接通的外部消息告警。
- 执行记录仅保留最近 2016 次（约 7 天）。失败与容量不足均不会被记为成功。

这些规则控制文件日志增长，**不能保证数据库、业务文件或快照持续增长时磁盘永不耗尽**。数据库仅监控是此次批准的边界；出现 critical 时需扩容或另行核实这些来源的保留策略。Loki 的 90% 阈值按整个 WAL 文件系统衡量，不等于 `df` 排除预留块后的百分比。

## 查看与手动执行

```sh
systemctl list-timers powerx-space-guard.timer
systemctl status powerx-space-guard.service
cat /var/lib/powerx-space-guard/latest.json
journalctl -u powerx-space-guard.service --since today
python3 /opt/powerx-ops/space-guard/guard.py          # 只读预览
systemctl start powerx-space-guard.service             # 执行一次
```

主机安装路径 `/opt/powerx-ops/space-guard/`；systemd 单元安装于 `/etc/systemd/system/`；原 PostgreSQL／rsyslog 轮转配置备份保存在安装目录 `original-postgresql-common`、`original-rsyslog`。本次离线归档及校验记录保存在操作者本机 `/Users/michaelhu/PowerX-ops-backups/47.116.10.200/2026-10-30-space-guard/`。

接收机私有配置和状态位于 `/Users/michaelhu/PowerX-ops-backups/47.116.10.200/audit-archive/`，权限 0700／0600，日志不输出 SQL 正文、密钥或签名。S3 凭据只从接收机现有私有配置读取，不复制到服务器。服务器只保存接收方验证密钥与固定目标绑定。Mac 定时任务为 `com.powerx.audit-archive`，状态见 `state/last-run.json`；`source_deleted` 仅为已确认结果，丢失提交响应时单独记录 `source_commit_unconfirmed`，不得按零删除解释。

```sh
launchctl print gui/$(id -u)/com.powerx.audit-archive
cat /Users/michaelhu/PowerX-ops-backups/47.116.10.200/audit-archive/state/last-run.json
python3 scripts/ops/powerx-space-guard/run_receiver.py --config /path/to/private/receiver.json
```

接收机依赖 `boto3`、`PyYAML`；服务器 helper 仅依赖 Python 标准库。桶必须无公开策略／公开 ACL、开启版本保留且无对象过期规则。创建／检查桶使用 `archive_s3.py --config /path/to/private/receiver.json --initialize`。定时／手动接收使用 `run_receiver.py`：独立于 SDK 的 285 秒监督会终止本轮工作进程及其 SSH 子进程，超时结果为未确认，不伪造零删除。归档操作不应在其他机器上用服务器自己的 `127.0.0.1` 冒充接收机。

## 新版上线时交接

先在目标主机执行只读盘点，再由 Core Root 进入交接锁定，逐资源核对来源、任务调度、失败记录、磁盘阈值与保留边界。报告不是停止任务的许可：

```sh
python3 /path/to/powerx-space-guard/handoff_inventory.py --output /root/powerx-handoff-before.json
```

报告区分 failed、空闲及仍会自动启动的 timer，拒绝把权限／探测失败当作零任务。工具没有 `--apply`，不读取或导出凭据；详细解释见[交接盘点合同](../../../docs/contracts/storage-executor-handoff-inventory.md)和[手册场景 D](../../../docs/guides/deploy/027-monitor-center/usecase-us9-storage-execution.md#场景-d旧执行器只读盘点与预演2026-11-09)。

旧 guard 同时承担日志轮转、journal、系统日志和容量观察。**只接管部分来源不能直接停止整个 guard**；必须先核实剩余职责的归属。只有经过批准、确认整套职责已移交且在途任务已核对的维护步骤，才使用以下整套退役命令。停止守护不会还原已过期的日志。

```sh
systemctl disable --now powerx-space-guard.timer
systemctl stop powerx-space-guard.service
touch /etc/powerx-space-guard.disabled
```

如果新版还没有接管 PostgreSQL 高频轮转及归档回收，需保留临时任务。**禁止直接恢复原 PostgreSQL 按份／按天删除配置绕过归档保护。**仅停止 Mac 接收任务时，服务器继续保留审计原件：

```sh
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.powerx.audit-archive.plist
```

回退时保持 Core 交接锁定和策略暂停，核对程序理解当前绑定、来源执行权及在途任务，再在维护步骤中恢复 timer；不得只删除 disabled、绑定或水位文件来恢复旧执行器。暂停任务不要求重启业务服务；激活 PostgreSQL 原生采集器则必须单独取得重启许可。配置备份位于 `revisions/`，回滚也不能关闭 SQL 写操作记录或删除已归档版本。

## 本地验证

```sh
python3 -m unittest discover -s scripts/ops/powerx-space-guard -p 'test_*.py'
```

测试覆盖白名单、链接／打开文件、未归档保留、签名与时效、具体对象版本、数据及清单损坏、源替换、轮转竞争、空间压力、传输增大和提交结果未知。实际 SSH／MinIO 验收用 Agent 自有过期 canary 验证：损坏或无效签名时原件保留，完整归档后可回收，回收后归档仍可读，匿名 GET 为 403。没有用业务表做破坏测试。

完整操作、边界和激活步骤见[审计归档手册](../../../docs/guides/deploy/027-monitor-center/usecase-us9-storage-audit-custody.md)。

参考：[Loki WAL 磁盘保护](https://grafana.com/docs/loki/latest/operations/storage/wal/)、[journalctl vacuum](https://www.freedesktop.org/software/systemd/man/latest/journalctl.html)。
