# Use Case US3：备份恢复与日志观测闭环

## 1. 场景目标
- 建立“策略配置 -> 手动备份 -> 清理 -> 恢复演练 -> 日志告警”可执行链路。
- 满足 30 天日志可检索、备份结果可追踪。

## 2. 页面操作步骤（Web Admin）

1. 动作：创建或更新备份策略。
- 入口/命令：`/ops/backup`，填写策略并点击“保存策略”。
- 预期结果：策略列表可见新策略。
- 失败处理：检查 `backup_type/schedule/storage_target` 格式。

2. 动作：触发备份与恢复演练。
- 入口/命令：选择策略后点击“手动触发备份”，随后点击“触发恢复演练”。
- 预期结果：任务列表新增记录，演练面板显示 `rto_seconds`。
- 失败处理：若演练按钮不可用，先确认至少存在一条备份任务。

3. 动作：查看日志观测状态。
- 入口/命令：同页“日志观测”卡片。
- 预期结果：显示 `30 天保留`、`job/app/env` 标签、`backup failed spike` 告警。
- 失败处理：若不一致，检查 `deploy/observability` 配置是否被正确部署。

## 3. 接口调用步骤（Admin API）

1. 动作：手动触发备份任务。
- 入口/命令：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/backup/jobs/run" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"policy_id":"1"}'
```

- 预期结果：返回 `job`，状态进入 `running` 后转 `success`。
- 失败处理：查看 `error_message` 并核对脚本权限与存储连通性。

2. 动作：触发恢复演练。
- 入口/命令：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/backup/restore-drills/run" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"source_job_id":"1"}'
```

- 预期结果：返回 `drill.status` 与 `rto_seconds`。
- 失败处理：若返回 job 不存在，先执行并确认备份任务成功。

## 4. 本地联调步骤

1. 动作：执行 US3 测试。
- 入口/命令：

```bash
cd backend && go test ./tests/contract/ops -run 'TestHTTPBackupContract|TestGRPCBackupContract' -count=1
cd backend && go test ./tests/integration/ops -run 'TestBackupRestoreFlow|TestLoggingLokiRetention' -count=1
```

- 预期结果：合同与集成通过。
- 失败处理：检查 `backend/scripts/ops/{backup-db.sh,cleanup-backups.sh,restore-drill.sh}` 与观测配置。

2. 动作：检查指标。
- 入口/命令：

```bash
curl -sS http://127.0.0.1:2112/metrics | rg 'powerx_ops_backup_(total|error_total|latency_ms)'
```

- 预期结果：存在对应指标并随动作增长。
- 失败处理：确认服务已开启 OTEL/Prometheus 导出。

## 5. 验收标准
- 至少 1 条备份成功 + 1 条恢复演练记录。
- 日志保留策略为 30 天，且告警规则可加载。
- 失败任务可通过状态与错误信息追踪。

## 6. 关键代码映射
- HTTP：`backend/internal/transport/http/admin/backup/{routes.go,handler.go}`
- Service：`backend/internal/service/backup_ops/{policy_service.go,job_service.go,restore_drill_service.go}`
- 页面：`web-admin/app/pages/ops/backup.vue`
- 组件：`web-admin/app/components/ops/backup/LogObservabilityPanel.vue`
- 脚本：`backend/scripts/ops/{backup-db.sh,cleanup-backups.sh,restore-drill.sh}`
- 观测：`deploy/observability/{loki,promtail,grafana}/...`
