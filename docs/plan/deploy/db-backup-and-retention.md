# 生产数据库备份与清理策略（Docker + 非 Docker）

## 1. 目标与原则

- 兼容 Docker 与 systemd 两种部署模式
- 支持定时自动备份、状态可追踪、失败可告警
- 支持数据保留清理与恢复演练

建议目标：

- RPO（可接受数据丢失）：<= 15 分钟（需 WAL 归档）
- RTO（恢复时间）：<= 60 分钟（按数据规模调整）

## 2. PostgreSQL 推荐策略（分层）

### 2.1 日常逻辑备份（保底）

- 每天执行：`pg_dump -Fc`
- 备份内容：主业务库 + 关键元数据
- 保留 14~30 天

### 2.2 周期物理备份（高效恢复）

- 每周执行：`pg_basebackup`
- 配合 WAL 归档，支持时间点恢复（PITR）
- 保留 4~8 周

### 2.3 WAL 归档（强推荐）

- 连续归档到对象存储（MinIO/S3）
- 与全量备份组合实现 PITR

## 3. 存储与保留建议

- 备份统一上传对象存储：`s3://<bucket>/powerx-backups/...`
- 建议目录结构：

```text
powerx-backups/
  logical/YYYY/MM/DD/
  physical/YYYY/MM/DD/
  wal/YYYY/MM/DD/
```

- 生命周期建议：
  - logical：30 天
  - physical：56 天
  - wal：14 天

## 4. 任务调度方案

### 4.1 Docker 模式

- 使用专用 `backup` 容器 + `cron/supercronic`
- 备份脚本挂载只读配置与凭证
- 结果写入：日志 + 状态文件 + 告警通道

### 4.2 非 Docker 模式

- 使用 `systemd service + timer`
- 优先替代 crontab，便于状态查询与失败重试

## 5. 脚本规范（建议）

推荐脚本：

- `scripts/ops/backup-db.sh`：备份并上传
- `scripts/ops/cleanup-backups.sh`：按 retention 清理
- `scripts/ops/restore-drill.sh`：恢复演练（临时库）

每次任务输出结构化字段：

- `job_id`
- `backup_type`（logical/physical/wal）
- `started_at/ended_at`
- `size_bytes`
- `checksum`
- `storage_uri`
- `status`（success/failed）
- `error_message`

## 6. 告警与审计

- 备份失败立即告警（飞书/钉钉/邮件）
- 连续 N 次失败升级告警等级
- 将备份任务日志接入 Loki，Grafana 展示成功率、耗时、失败分布

## 7. 恢复演练（必须）

- 每月至少 1 次恢复演练
- 演练步骤：下载备份 -> 恢复到临时实例 -> 健康校验 -> 记录报告
- 无演练的备份视为“不可信备份”

## 8. UI 管理化（二阶段规划）

### 8.1 数据模型建议

- `backup_policies`：策略配置（周期、保留、存储目标）
- `backup_jobs`：任务实例与结果
- `backup_artifacts`：产物信息（uri/checksum/size）
- `restore_drills`：演练记录

### 8.2 API 建议

- 策略 CRUD
- 手动触发备份
- 历史查询与失败详情
- 触发清理任务
- 触发恢复演练并记录结果

### 8.3 Web Admin 页面建议

- 备份看板：成功率、最近失败、耗时趋势
- 任务列表：可筛选/重试/查看日志
- 策略配置：周期、保留、存储目标
- 演练记录：最近演练时间与结果

## 9. 迁移参考

- 如需把数据库全量导入另一套 PowerX（含表结构），请参考：
  - [PowerX 实例迁移指南（A -> B，含表结构）](./powerx-instance-migration.md)
