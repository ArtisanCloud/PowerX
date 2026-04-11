# Quickstart: 自动备份闭环（027-monitor-center）

## 1. 前置条件

- 当前分支：`027-monitor-center`
- 已完成数据库迁移（包含 `policy/job/artifact/drill/alert` 相关表）
- Root 管理员可登录管理端并拿到 `TOKEN`
- 目标环境具备备份存储目标（示例：`powerx_bak`）

## 2. 启动服务

```bash
# backend
cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/backend
make dev

# web-admin（新终端）
cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/web-admin
npm run dev
```

## 2.1 关键脚本说明（运维侧）

- `backend/scripts/ops/backup-db.sh <policy_id>`: 执行备份脚本入口（由调度/手动触发调用）。
- `backend/scripts/ops/cleanup-backups.sh`: 执行过期产物清理入口（保留策略在服务层执行，脚本负责外部清理动作）。
- `backend/scripts/ops/restore-drill.sh <source_job_id>`: 执行恢复演练入口。
- `backend/scripts/ops/rollback-release.sh`: 发布回滚入口（与备份中心联动时可作为紧急回退动作）。

回滚建议：
- 当备份/演练持续失败且影响生产变更窗口时，先停用策略，再执行发布回滚脚本，最后通过告警确认恢复状态。

## 3. 创建并启用自动备份策略

```bash
# 3.1 创建策略
POLICY_JSON=$(curl -sS -X POST "http://127.0.0.1:8080/api/v1/admin/ops/backup/policies" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "prod-default-policy",
    "interval_hours": 6,
    "retention_count": 14,
    "timezone": "Asia/Shanghai",
    "drill_enabled": true,
    "drill_interval_days": 7,
    "target_ref": "powerx_bak"
  }')

echo "$POLICY_JSON" | jq
POLICY_ID=$(echo "$POLICY_JSON" | jq -r '.data.id // empty')

# 3.2 启用策略
curl -sS -X POST "http://127.0.0.1:8080/api/v1/admin/ops/backup/policies/${POLICY_ID}/enable" \
  -H "Authorization: Bearer $TOKEN" | jq
```

验收点：
- `POLICY_ID` 非空。
- 列表查询可看到 `interval_hours=6`、`retention_count=14`、`timezone=Asia/Shanghai`。

## 4. 验证策略列表分页与过滤

```bash
curl -sS "http://127.0.0.1:8080/api/v1/admin/ops/backup/policies?page=1&page_size=20&status=enabled&keyword=prod&timezone=Asia/Shanghai" \
  -H "Authorization: Bearer $TOKEN" | jq
```

验收点：
- 返回 `data.items` 为数组。
- 返回 `data.page.page=1`、`data.page.page_size=20`。

## 5. 验证作业与告警链路

```bash
# 5.1 查作业
curl -sS "http://127.0.0.1:8080/api/v1/admin/ops/backup/jobs?policy_id=${POLICY_ID}&page=1&page_size=20&status=failed" \
  -H "Authorization: Bearer $TOKEN" | jq

# 5.2 查告警（高优先级）
curl -sS "http://127.0.0.1:8080/api/v1/admin/ops/backup/alerts?level=high&acked=false&page=1&page_size=20" \
  -H "Authorization: Bearer $TOKEN" | jq
```

验收点：
- 作业查询支持 `policy_id/status/page/page_size` 过滤。
- 告警查询支持 `level/acked/page/page_size` 过滤。

## 6. 发起恢复演练并查询状态

```bash
# 假设已有可用备份产物
ARTIFACT_ID=<artifact_id>

DRILL_JSON=$(curl -sS -X POST "http://127.0.0.1:8080/api/v1/admin/ops/backup/restore-drills" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"artifact_id\":\"${ARTIFACT_ID}\",\"reason\":\"weekly-drill\"}")

echo "$DRILL_JSON" | jq
DRILL_ID=$(echo "$DRILL_JSON" | jq -r '.data.id // empty')

curl -sS "http://127.0.0.1:8080/api/v1/admin/ops/backup/restore-drills/${DRILL_ID}" \
  -H "Authorization: Bearer $TOKEN" | jq
```

验收点：
- `DRILL_ID` 非空。
- 详情状态可见 `queued/running/success/failed` 之一。

## 7. 监控页验收（策略 -> 作业 -> 告警 -> 演练）

- 进入 `监控中心 -> Task / Cron`：可见备份任务状态与下次执行时间。
- 进入 `监控中心 -> Logs / Trace`：可见备份链路摘要。
- 进入 `运维中心 / 备份中心`：可见策略列表、作业历史、告警列表、演练历史。

## 8. 通过标准

- 策略创建/启用成功，默认值符合：`6h / 14 份 / Asia/Shanghai / 每周演练`。
- 作业、告警、演练接口均支持分页与关键过滤参数。
- 监控页面可形成“策略 -> 作业 -> 告警 -> 演练”可观察闭环。

## 9. OTel 与指标验证（Phase 6）

```bash
# 建议在启动 backend 前设置
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4317
export OTEL_SERVICE_NAME=powerx-backend

# 触发一轮策略操作 + 任务 + 演练后，检查指标
curl -sS http://127.0.0.1:2112/metrics | grep -E "powerx_ops_backup_total|powerx_ops_backup_error_total|powerx_ops_backup_latency_ms"
```

验收点：
- 指标包含 `operation` 与 `result` 标签（`success/failed`）。
- Trace 可串联 `策略操作 -> 备份执行 -> 告警/演练` 链路。

## 10. 回归记录模板（Phase 6）

- Backend:
  - `cd backend && GOCACHE=../tmp/gocache GOMODCACHE=../tmp/gomodcache go test ./internal/service/backup_ops ./internal/transport/http/admin/backup ./pkg/corex/db/persistence/repository/ops`
- Frontend:
  - `cd web-admin && npm run build`
- E2E Smoke（可选，CI 或本地浏览器环境）:
  - `cd web-admin && npm run test:e2e -- tests/e2e/ops/backup-center.spec.ts`
