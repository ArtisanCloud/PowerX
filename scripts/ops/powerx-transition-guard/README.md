# 47.116.10.200 新版上线前的临时容量保护

本目录仅用于 `prod-20261019-1452` / `dc8583939bfb524dbd349fe645652e746deffc74` 的过渡维护。用户在 2026-11-09 明确批准以下保留范围；不是新版存储控制面已经接管。

## 已批准范围与执行组件

| 来源 | 策略 | 执行方式 |
| --- | --- | --- |
| Loki 运行日志 | 7 天，保留原异步删除延迟 | Loki 自身 Compactor；独立 SQL/文件审计仍按原归档规则保护 |
| 正式系统自动快照 | 最新 3 份，原 7 天上限不变，保护点不删除 | PowerX Snapshot Service；保留原每日采集时刻 |
| 开发数据库小时备份 | 最新 24 份 | PowerX Backup Policy/Artifact Cleanup Service |
| 正式数据库每日备份 | 最新 3 份 | 已有正式策略和任务，继续生效 |
| 成功的鉴权基线读取追踪 | 保留最近 7 天，分批追赶历史积压 | `retention.py` + 已验证的二进制 COPY 恢复校验工具 |
| 系统/文件日志 | 原有保护不变 | 保留原 `powerx-space-guard.timer`，不整套替换或停用 |

数据库期限只处理 AI Craft 成功、无错误、无 fallback 的 typed `read_member_baselines`，必须有同租户、trace、能力、插件、结果均匹配的唯一关联调用结果通知；通知原状态必须为 pending、attempts=0、publish_time 为空。此项是明确批准的期限清理，不表示通知投递成功。失败、业务操作、软删除、未知引用、近期更新和其他调用方均保留。

每批先将两表完整行导出成私有 gzip 二进制 COPY，完整读回并校验哈希，恢复进事务临时表，逐行比较源数据，写入 fsync 的删除意图后才删除。两表数量不一致即回滚。未知提交或回执丢失停止，不自动重试。只从已确认游标继续；单轮最多 20,000 对、100 秒后不再启动新事务，结束 3 分钟后才执行下一轮。主键窗口 2,000，维护连接使用 custom query plans，避免空窗口的通用计划拖慢后续扫描。

恢复副本位于服务器 `/var/lib/powerx-baseline-retention/`，只对 postgres 可读；总预算 6 GiB，单轮 256 MiB，上限前停止并告警，不自动删除副本。该预算不是归档已经异地完成。达到预算后必须先完成可验证的跨主机归档，再人工核对恢复执行。原 11 月 3 日的恢复副本保持不动。

普通 DELETE 和 autovacuum 主要提供数据库内部可复用页，不将删除行数计为已释放的文件系统空间。两张系统表将 autovacuum 触发比例调整为 1%、阈值 20,000；没有使用 VACUUM FULL 或整表重写。修改前参数和变更回执保存在私有维护目录。

## 容量保护与运行核验

`powerx-transition-capacity.timer` 每轮结束一分钟后检查文件系统、固定三项计划和期限执行器状态。目标可用空间 20 GiB，运行底线 12 GiB，按最近产物大小的 1.5 倍加 1 GiB 估计单项峰值；任务峰值之外必须仍有 12 GiB。余量不足时通过现有服务暂缓该计划，多出 2 GiB 余量后只恢复自己暂缓且未被其他操作改变的计划。

这是旧版计划开关的提前保护，**不等同于新版 Core 在任务启动点的原子容量准入**。并发手动任务、业务附件突增、无法接收告警及长期不升级仍需运维处置。没有提高 Loki 的 90% 阈值；旧 Core 的同步 Loki 投递实现也没有被本次代码替换。

每五分钟实测 Core 公共只读接口和 Loki 实际 push；阈值变化、恢复及期限执行器失败纳入告警。正常清理状态不是历史积压已归零；回执展示 frozen highwater、confirmed cursor 和本轮数量。实际下一次自动备份、快照周期及 24 小时增长率需要继续观察。

容量及运行探针样本保存在私有维护目录 `capacity-observations/`，最多 2,160 份（约 36 小时）；样本最低水位不冒充未被采样捕获的瞬时峰值。

```sh
systemctl list-timers powerx-transition-capacity.timer powerx-baseline-retention.timer
cat /var/lib/powerx-ops/transition-20261109/capacity-latest.json
sudo -u postgres cat /var/lib/powerx-baseline-retention/supervisor.json
journalctl -u powerx-baseline-retention.service -n 20 --no-pager
```

## 飞书个人告警

不接入客户群，也不使用群 Webhook。创建企业自建应用、启用机器人，申请 `im:message:send_as_bot`，将应用可用范围仅设为运维本人，完成企业要求的发布审核。通过该应用的 API 调试台选择自己并复制 `open_id`。`open_id` 与应用绑定，不复用另一个应用的 ID。

私有配置路径 `/etc/powerx-transition-guard/feishu-personal.json`；目录 0700、文件 root:root 0600：

```json
{
  "mode": "feishu_app",
  "app_id": "cli_REPLACE",
  "app_secret": "REPLACE",
  "receive_id_type": "open_id",
  "receive_id": "ou_REPLACE"
}
```

配置前通知状态为 `not_configured`，不会伪报送达。模块固定使用个人 open_id 接口，拒绝群 ID；不申请读聊天记录、群管理或代用户发送权限。token 只存在内存，日志不含凭据。发送复用固定事件 UUID 去重；API 接受和本人实际收到分别验收。

参考：[机器人与消息能力](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1)、[通过 API 调试台获取 open_id](https://open.feishu.cn/document/server-docs/api-call-guide/generic-error-code)。

## 每日个人磁盘报告

`powerx-disk-daily.timer` 按北京时间每天 00:05 发送磁盘日报，使用同一份私有飞书个人配置。内容包括根分区总容量、已用空间、与 `df` 一致的使用率、可用空间、系统预留和最近十分钟内的容量保护状态；过期或不可读的保护状态明确标为未确认。直接读取文件系统统计，不扫描目录、不访问数据库、不触发清理。

独立部署到 `/opt/powerx-ops/disk-daily/`，回执保存在 root 私有目录 `/var/lib/powerx-ops/disk-daily/`。它不受旧 Core commit 约束，升级后仍继续报告磁盘；新版接管后需单独更新可选的容量保护状态来源。停止过渡清理 timer 不会停止日报。

`Persistent=true` 用于停机错过时开机补发当前日报。同一天的重试复用同一事件 UUID，收到 API 接受回执后跳过重复发送；发送失败每五分钟重试，一小时内最多七次尝试。部署测试与正式日报分开计数，不占用当日正式日报。API 接受回执与用户确认可见仍分别记录。

```sh
python3 /opt/powerx-ops/disk-daily/disk_daily.py  # 只读预览
systemctl list-timers powerx-disk-daily.timer
journalctl -u powerx-disk-daily.service -n 10 --no-pager
systemctl disable --now powerx-disk-daily.timer  # 停止日报
```

## 交接、回退与边界

### 升级后保留临时监控

正式 Core 已升级但新策略尚未接管时，可显式使用 `capacity.py --apply --observe-only`。
该模式不检查旧 commit，不调用 `policies()`／`set_enabled()`，不修改备份或快照开关，也不执行数据库期限删除；原默认入口和 `retention.py` 的版本保护保持。

监控模式继续采集磁盘、Core／Loki 健康和旧期限执行器状态，通过原私有飞书个人配置发送阈值告警及恢复通知，并保留有界采样。首次切换模式发送明确说明；相同严重状态每小时提醒，重复发送复用事件身份。`owned_pauses` 和未知执行结果保留，不能因为改为监控就擅自恢复旧计划。

回执固定标记 `guard_kind=monitor_only_no_schedule_changes_or_deletion`、`automatic_cleanup_active=false`、`handoff_complete=false`。任务退出 0 仅表示监控轮次成功，不能作为自动清理或新版接管的证明。独立的 00:05 磁盘日报及原文件日志守护继续保留。

47.116.10.200 本轮正式部署的配置、执行及验收边界见[正式观测说明](../../../docs/contracts/production-storage-observation.md)。

维护入口源码位于 `backend/tmp/ops-transition-20261109/` 和 `backend/tmp/ops-baseline-retention-20261109/`，仅编译独立工具，不替换 Core 程序，不执行数据库迁移、不修改业务附件、不提交或推送。

工具、systemd 单元及私有回执分别位于 `/opt/powerx-ops/transition-20261109/`、`/etc/systemd/system/`、`/var/lib/powerx-ops/transition-20261109/`。策略调用与删除使用正式服务；快照元数据保持业务进程属主，暂缓恢复保持原 schedule anchor。配置被用户或其他执行器修改时不擅自重新启用。

检测到 Core 发布 commit 改变或快照控制面绑定出现后停止旧版自动接管并要求交接。升级前先盘点在途任务和自己暂缓的计划，确认新版已接管相同职责，再停这两个新增 timer；原文件日志 guard 的其他职责仍需单独交接。停止 timer 不强杀正在提交的批次，不删除水位或未知回执来恢复执行。

```sh
systemctl disable --now powerx-transition-capacity.timer powerx-baseline-retention.timer
systemctl show powerx-baseline-retention.service -p ActiveState -p MainPID
```

Loki 原配置保存在私有维护目录 `loki-config-before.yml`；恢复该配置需要校验后受控重启 Loki。保留数量可通过正式服务恢复，但已删除的旧恢复点无法由调回策略重新生成。表 autovacuum 参数的原值保存在 `table-autovacuum-before.json`。
