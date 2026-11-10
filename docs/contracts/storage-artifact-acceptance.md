# 存储真实产物验收

2026-11-09。对应 SG-09 的取回／业务核对及 SG-11 的原生产物验证。本合同只说明源码和自有隔离环境的证据；不表示日常实例已接管、正式站已部署或长期运行已通过。

## 当前有序进度

| 项目 | 状态 | 证据与范围 |
| --- | --- | --- |
| WorkspaceX 存储门禁回归 | 已有实际只读成功回执 | 2026-11-08 14:06，原专用 Key、官方 Framework v0.0.28，两项 grant 均 granted，Lookup 返回 found=false；本轮核对已有记录，没有重复业务调用 |
| 恢复后统一核对 | 源码与隔离验收完成 | 见[统一恢复合同](storage-execution-control.md#2026-11-08-统一重建与逐资源核对)；未据此宣布日常部署、整站物理恢复或旧执行器现场交接完成 |
| 真实产物与取回 | 本轮补齐自有 PostgreSQL / MinIO 验收 | 真实工具及 S3 服务，不以捕获脚本或 HTTP 协议仿真代替；长期在线远端仍未验收 |
| 旧执行器交接、实际策略接管、长期运行与发布 | 未完成 | 继续按正式任务清单及维护授权逐项执行 |

WorkspaceX 已有成功回执为 [Core 导入记录](evidence/workspacex-storage-recovery-recheck-20261108.json)。原记录来自 WorkspaceX 的专用身份联调，不包含客户写入、订单或支付。

## 可重复执行的隔离验收

前置：现有 Python 3、boto3、Go、MinIO 可执行文件，以及完整 PostgreSQL 工具目录。脚本不安装软件，不加载日常配置，不接收既有数据库或远端桶凭据。

从仓库根目录执行，证据目录必须是新目录：

```sh
python3 scripts/ops/validate-storage-artifacts.py \
  --pg-bin /opt/homebrew/opt/postgresql@14/bin \
  --artifacts tmp/storage-artifact-acceptance-本次唯一编号
```

其他安装位置使用实际 PostgreSQL bin 路径；MinIO 不在 PATH 时加 `--minio-bin /实际路径/minio`。不需要重启日常 Core 或 Nuxt。

执行器仅建立自己的 PostgreSQL Unix socket 集群、随机端口回环 MinIO、私有版本桶和随机测试凭据。正式配置控制服务与 TaskBus 实现参与测试，Redis 队列后端使用 miniredis；没有启动真实 Redis、Core 或前端服务。测试在自有 schema / 数据库 / 文件中进行。PostgreSQL 保持 `logging_collector=on`、`log_statement=mod`、`log_truncate_on_rotation=off`。

六项验收必须全部实际运行并通过；显式 opt-in 测试若 skipped，整体失败：

1. 普通文件日志不开归档的控制链：接管默认暂停、发布确认、过期封存文件删除、活动／最新文件和业务文件保留、请求回执幂等。
2. 数据库备份：正式配置保存／发布／显式启用后调用真实 `backup-db.sh` / `pg_dump`，校验下载为 custom archive；源数据继续变化后，通过真实 `restore-drill.sh` / `pg_restore` 恢复到**新数据库**。核对历史中文字段与汇总值，源库的新值继续保留；暂停后旧发布版本不能启动新备份。
3. 受控 S3：目标与策略通过数据库控制域和正式 TaskBus 发布，凭据由独立加密提供器保存，执行节点完成实际版本桶验证；策略保存不自动开启。归档后将对象最新版本改为不同内容，再删除自建源文件及本地 CAS。新恢复代次阻止旧凭据，重新提交凭据并验证目标后，下载仍包含记录中确切旧版本的原文；活动／最新轮转文件保留。这里只模拟新恢复代次，不冒充整站物理恢复。
4. 原生 S3 历史取回、远端清单重建、迁移保留原件与到期确切版本清理。
5. 媒体配套真实数据库 dump / restore，恢复数据核对及错误校验和拒绝。
6. 原有真实备份集成：按需导出、完整下载、校验和损坏拒绝、独立恢复、保护与保留清理。

`processes.json` 记录自有 PID、命令、目录和监听。`tests.json` / JSONL 记录原始测试结果，`result.json` 记录源码校验和、SQL 日志设置、剩余库／schema、S3 计量及清理。脚本退出前停止本轮服务及子进程，确认 PostgreSQL PID、socket 与 MinIO 端口释放，删除测试数据和凭据；启动失败同样执行清理。无法确认清理时保留私有目录并报告失败，不操作用户进程。不要将尚未清理的私有目录提交到版本库。

本轮正式结果见[验收回执](evidence/storage-artifact-acceptance-validation-20261109.json)。先前准备错误和失败试跑保留在 tmp，最终通过不覆盖失败历史。曾发现测试 MinIO 的默认 region 与预期不一致，执行器现显式设置 site region 并从服务读回核对；没有修改用户 MinIO。异步归档须等待远端终态，不能将本地捕获成功当成远端归档完成。

## 下一步：长期在线远端验收

本机 MinIO 不作为长期归档目标。需要操作者提供 UI 中已有目标名称，或非敏感的 endpoint、bucket、region、独立测试前缀及凭据引用。凭据值通过正式 Root UI 的受控凭据入口配置，不进入聊天、普通导出或本验收文档。

在包含统一恢复改动的实际 Core 上，先只读核对配置／消费者／安装身份，再使用独立来源和独立前缀完成：目标验证 → 保存暂停策略 → 明确启用 → 完整归档及确切版本校验 → 回收自建源文件与缓存 → 从目标取回原文。归档关闭时只验收允许直接清理的普通日志；审计／PostgreSQL 来源遵守原有强制保护。不得为验收清理业务数据或用户历史归档。

完成标准包括取回字节与 SHA-256 一致、来源保护项不变、策略和任务可追踪、测试资源收尾及实际版本记录。之后再进入旧执行器交接；真实交接完成前不能同时启用两套自动清理。整站业务恢复、独立密钥保管、双环境 7 天周期和 14 天样本继续单独验收。

## 代码映射

- `scripts/ops/validate-storage-artifacts.py`：自有服务、真实测试编排、证据及失败收尾。
- `backend/internal/service/storage_governance/recovery_artifacts_postgres_test.go`：新版控制链下的真实备份／恢复和 S3 代次取回。
- `backend/internal/service/storage_governance/lifecycle_control_postgres_test.go`：普通日志无归档清理与保护。
- `backend/internal/service/archive_ops/{live_s3_test,postgres_test}.go`：真实版本对象存储、历史取回及媒体数据库恢复。
- `backend/internal/service/backup_ops/integration_test.go`、`backend/scripts/ops/{backup-db,restore-drill}.sh`：真实数据库产物及独立恢复。
