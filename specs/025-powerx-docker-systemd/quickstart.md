# Quickstart: PowerX 部署与运维治理基线（P0）

## 1. 前置条件

- 已在 `025-powerx-docker-systemd` 分支。
- 可访问 PostgreSQL、Redis、MinIO/S3。
- 已准备运维配置目录与日志目录。

## 2. 部署基线验证

1. 按 `docs/plan/deploy/docker.md` 或 `systemd.md` 完成一次冷启动。
2. 验证健康状态：

```bash
curl -f http://127.0.0.1:8077/api/v1/health
```

3. 验证插件入口与主站可访问。

## 3. 插件平滑升级演练

1. 执行“安装不启用”。
2. 执行版本切换。
3. 触发回滚并确认恢复。

参考：`docs/plan/deploy/plugin-upgrade-sop.md`

## 4. 备份恢复演练

1. 触发一次逻辑备份。
2. 执行一次清理任务（测试环境）。
3. 执行一次恢复演练并记录 RTO。

参考：`docs/plan/deploy/db-backup-job-templates.md`

## 5. 日志聚合验证

1. 确认 promtail 已采集目标日志源。
2. 在 Grafana Loki 中按 `service` 与 `plugin_id` 检索。
3. 触发一条可识别错误日志，确认告警链路。

参考：`docs/plan/deploy/logging-loki-grafana.md`

## 6. 管理页面 P0 验证

- Deploy 页面：可查看发布历史并执行回滚。
- Plugin 页面：可查看插件版本并执行切换。
- Backup 页面：可查看策略、任务、演练结果。

参考：`docs/plan/deploy/management-console-p0-tasks.md`

