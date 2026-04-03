# PowerX 运维管理控制台路线图（Deploy 维度）

## 1. 目标

建立一个统一的运维管理页面集合，覆盖：

- 部署发布
- 插件生命周期
- 备份恢复
- 日志告警
- 配置密钥
- 容量成本

并与当前部署方案（Docker + systemd）保持一致。

## 2. 角色与权限模型

建议按现有角色扩展最小权限边界：

- `system_admin`：全功能（跨租户/跨环境）
- `admin`：本租户运维与观测（不含全局密钥轮换）
- `ops`（可选）：偏运维能力（发布、备份、告警处理）
- `auditor`（可选）：只读审计与报表

权限建议命名：

- `deploy.release.*`
- `plugin.lifecycle.*`
- `backup.*`
- `observability.logs.*`
- `config.change.*`
- `secrets.rotate.*`
- `capacity.report.*`

## 3. 页面清单与分期

### 3.1 P0（首批必须上线）

1. 部署发布中心  
2. 插件生命周期中心  
3. 备份与恢复中心  

### 3.2 P1（稳定后增强）

4. 日志与告警中心  
5. 配置与密钥中心  

### 3.3 P2（运营优化）

6. 容量与成本中心  

## 4. 页面详细规格

### 4.1 部署发布中心（P0）

核心能力：

- 查看当前生效版本（backend/web-admin）
- 查看发布记录（版本、操作者、时间、结果）
- 一键回滚到上一稳定版本
- 显示健康状态（API、Web、关键依赖）

关键字段：

- `environment`
- `backend_version`
- `web_admin_version`
- `release_id`
- `release_status`
- `rollback_target_version`
- `health_summary`

建议 API：

- `GET /api/v1/admin/deploy/releases`
- `POST /api/v1/admin/deploy/releases`
- `POST /api/v1/admin/deploy/rollback`
- `GET /api/v1/admin/deploy/health`

权限：

- 查看：`deploy.release.read`
- 发布/回滚：`deploy.release.write`

### 4.2 插件生命周期中心（P0）

核心能力：

- 安装插件（本地/离线包）
- 新版本安装不启用
- `switch_version` 原子切换
- 一键回滚到 N-1
- 升级审计与门禁结果展示

关键字段：

- `plugin_id`
- `current_version`
- `installed_versions`
- `runtime_state`
- `last_switch_at`
- `last_switch_by`
- `gate_result`

建议 API（复用现有 + 扩展）：

- `GET /api/v1/admin/plugins`
- `GET /api/v1/admin/plugins/:id/status`
- `POST /api/v1/admin/plugins/install/local`
- `POST /api/v1/admin/plugins/:id/switch_version`
- `POST /api/v1/admin/plugins/:id/uninstall`
- `GET /api/v1/admin/plugins/:id/audit`（新增建议）

权限：

- 查看：`plugin.lifecycle.read`
- 操作：`plugin.lifecycle.write`

### 4.3 备份与恢复中心（P0）

核心能力：

- 配置备份策略（周期、保留、目标存储）
- 手动触发备份任务
- 查看历史任务和失败原因
- 执行恢复演练并记录结果
- 执行备份清理策略

关键字段：

- `policy_id`
- `backup_type`（logical/physical/wal）
- `schedule`
- `retention_days`
- `last_run_status`
- `artifact_uri`
- `checksum`
- `restore_drill_status`

建议 API：

- `GET /api/v1/admin/backup/policies`
- `POST /api/v1/admin/backup/policies`
- `POST /api/v1/admin/backup/jobs/run`
- `GET /api/v1/admin/backup/jobs`
- `POST /api/v1/admin/backup/cleanup`
- `POST /api/v1/admin/backup/restore-drills/run`

权限：

- 查看：`backup.read`
- 策略与执行：`backup.write`

### 4.4 日志与告警中心（P1）

核心能力：

- Loki 查询模板（按 service/plugin/tenant）
- 快速时间窗查询（5m/15m/1h/24h）
- 告警规则列表与订阅目标
- 告警确认与处理闭环

关键字段：

- `query_template`
- `service`
- `plugin_id`
- `tenant`
- `alert_rule`
- `alert_status`
- `ack_by`

建议 API：

- `GET /api/v1/admin/observability/logs/query-templates`
- `POST /api/v1/admin/observability/logs/search`
- `GET /api/v1/admin/observability/alerts`
- `POST /api/v1/admin/observability/alerts/:id/ack`

权限：

- 查看：`observability.logs.read`
- 告警管理：`observability.alerts.write`

### 4.5 配置与密钥中心（P1）

核心能力：

- 配置快照与 diff 对比
- 变更审批（可选）
- 密钥轮换记录与状态（不展示明文）
- 密钥即将过期提醒

关键字段：

- `config_snapshot_id`
- `config_diff_summary`
- `secret_name`
- `last_rotated_at`
- `next_rotate_due`
- `rotated_by`

建议 API：

- `GET /api/v1/admin/config/snapshots`
- `GET /api/v1/admin/config/diff`
- `POST /api/v1/admin/config/apply`
- `GET /api/v1/admin/secrets`
- `POST /api/v1/admin/secrets/:name/rotate`

权限：

- 配置：`config.change.*`
- 密钥：`secrets.rotate.*`

### 4.6 容量与成本中心（P2）

核心能力：

- DB 容量趋势与增长预测
- Loki 日志摄入量与存储占用
- 对象存储增长趋势
- 成本预算阈值与预警

关键字段：

- `db_size_bytes`
- `db_growth_daily`
- `loki_ingest_rate`
- `object_storage_size`
- `forecast_30d`
- `budget_threshold`

建议 API：

- `GET /api/v1/admin/capacity/overview`
- `GET /api/v1/admin/capacity/forecast`
- `GET /api/v1/admin/cost/overview`

权限：

- 查看：`capacity.report.read`

## 5. 技术实现建议

### 5.1 后端

- 新增 `deploy/backup/observability/config/capacity` admin 模块路由
- 关键操作统一写审计事件（谁、何时、操作、结果）
- 任务执行统一接入队列或调度器，避免页面阻塞等待

### 5.2 前端（web-admin）

- 在现有 Admin 导航下新增一级菜单：`运维中心`
- 每个页面分：概览卡片 + 列表 + 详情抽屉/弹窗
- 操作按钮均支持二次确认与幂等反馈

### 5.3 观测

- 页面操作与后端任务共用 trace_id
- 失败任务自动关联日志查询链接（Loki）

## 6. 验收标准（按分期）

P0 验收：

- 可在页面完成一次发布回滚
- 可在页面完成一次插件版本切换与回滚
- 可在页面触发备份并查看任务结果

P1 验收：

- 可在页面按 plugin_id/tenant 检索日志
- 配置变更与密钥轮换有审计记录

P2 验收：

- 容量预测可提前 7 天给出阈值预警
- 成本趋势可按环境维度查看

## 7. 与现有文档关系

- 部署基线：`README.md`、`docker.md`、`systemd.md`
- 插件流程：`plugin-upgrade-sop.md`
- 日志方案：`logging-loki-grafana.md`
- 备份方案：`db-backup-and-retention.md`、`db-backup-job-templates.md`
- 迁移方案：`powerx-instance-migration.md`

