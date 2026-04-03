# Use Case US3：备份恢复与日志观测

## 1. 目标
- 完成策略管理、手动备份、清理、恢复演练与日志检索闭环。
- 验收口径：备份任务可追踪，恢复演练可产出 RTO，30 天日志保留可见。

## 2. 页面操作（Web Admin）
1. 动作：创建/更新备份策略。
- 入口：`/ops/backup` -> “保存策略”。
- 预期结果：策略列表出现目标策略。
- 失败处理：校验 `retention_days>0`、cron 可解析。

2. 动作：执行手动备份与清理。
- 入口：同页“手动触发备份”“触发清理”。
- 预期结果：任务列表新增 `manual` 任务。
- 失败处理：检查 `error_message` 与脚本返回码。

3. 动作：执行恢复演练并查看结果。
- 入口：同页“触发恢复演练”。
- 预期结果：演练面板显示 `status` 与 `rto_seconds`。
- 失败处理：若失败，核查 `source_job_id` 和恢复脚本输入。

## 3. 接口调用（Admin API）

```bash
curl -X POST "http://127.0.0.1:8080/api/v1/admin/backup/policies" \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"name":"daily-main","backup_type":"logical","schedule":"0 2 * * *","retention_days":30,"enabled":true,"storage_target":"s3://powerx-backup/main"}'

curl -X POST "http://127.0.0.1:8080/api/v1/admin/backup/jobs/run" \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"policy_id":"1"}'
```

## 4. 本地联调

```bash
bash backend/scripts/ops/backup-db.sh 1
bash backend/scripts/ops/cleanup-backups.sh
bash backend/scripts/ops/restore-drill.sh 1
curl -s http://127.0.0.1:2112/metrics | rg 'powerx_ops_(backup|observability)_'
```

## 5. 代码映射
- 路由：`backend/internal/transport/http/admin/backup/routes.go`
- Handler：`backend/internal/transport/http/admin/backup/handler.go`
- Service：`backend/internal/service/backup_ops/{policy_service.go,job_service.go,restore_drill_service.go}`
- 观测配置：`deploy/observability/loki/loki-config.yaml`（`retention_period: 720h`）
- 页面：`web-admin/app/pages/ops/backup.vue`
- E2E：`web-admin/tests/e2e/ops/backup-center.spec.ts`
