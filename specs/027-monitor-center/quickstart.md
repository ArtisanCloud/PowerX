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

## 8.1 Logs / Trace（三驱动验收）

先获取日志能力配置：

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/admin/monitor/logs/config" | jq
```

再执行统一日志查询：

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/admin/monitor/logs/query?trace_id=<trace_id>&page=1&page_size=20" | jq
```

验收点：
- `driver=loki`：`query_meta.grafana_url` 有值，且页面“打开 Grafana”按钮可用。
- `driver=file`：可返回日志列表；页面提示“无标签聚合/无 Grafana 深链”。
- `driver=stdio`：可返回最近窗口日志；页面提示“历史窗口受限”。

常见排障：
- `loki.url 未配置`：检查 `config.yaml -> log.loki.url`。
- `file 无数据`：检查 `log.file.info_file_path/error_file_path` 路径与权限。
- `stdio 无数据`：确认进程已输出日志；该模式仅保留最近窗口，不保证跨重启历史。

详细步骤可参考：`specs/027-monitor-center/checklists/logs-trace-e2e.md`

## 8.3 插件日志（Host 模式）对齐验收

前置条件：
- 插件已通过宿主启用（`POWERX_PROXY=1`）。
- 能通过 `/_p/<plugin_id>/api/v1` 访问插件 Admin API。
- 已启用 Loki/Promtail 或等效统一采集链路。

步骤 A：确认宿主采集开关

```bash
# 本地开发示例（生产请改 /etc/powerx/powerx.env）
grep POWERX_SUPERVISOR_FORWARD_STDIO /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/config/powerx.env.example
```

验收点：
- 宿主环境应开启插件 stdout 透传（建议 `POWERX_SUPERVISOR_FORWARD_STDIO=true`）。

步骤 B：读取插件日志策略

```bash
PLUGIN_ID=<plugin_id>
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:3030/_p/${PLUGIN_ID}/api/v1/admin/runtime/logging/policy" | jq
```

验收点：
- `mode=host`。
- `format=json`。
- 默认 `sinks` 包含 `stdout`。

步骤 C：执行策略探测

```bash
curl -sS -X POST \
  "http://127.0.0.1:3030/_p/${PLUGIN_ID}/api/v1/admin/runtime/logging/probe" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "powerx monitor plugin logger probe",
    "level": "info",
    "component": "monitor.logs.quickstart",
    "trace_id": "probe-quickstart-001",
    "tenant_uuid": "demo-tenant"
  }' | jq
```

验收点：
- 返回 `outcomes`，并包含每个 sink 的状态：`success/failed/retrying/dropped`。
- `trace_id` 回显可用于后续日志检索。

步骤 D：在监控中心检索插件探测日志

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/admin/monitor/logs/query?trace_id=probe-quickstart-001&page=1&page_size=20" | jq
```

验收点：
- 可检索到插件探测日志。
- 日志内容包含 `plugin_id/tenant_uuid/component/level/trace_id`。

常见排障：
- 仅插件 RuntimeLogs 可见、监控日志不可见：优先检查 stdout 透传是否开启。
- Loki 可达但字段过滤不稳定：检查 Promtail 是否已做 JSON 字段提取与低基数标签约束。
- policy/probe 401/403：检查 Root 权限与插件 RBAC（`runtime.ops`）。

## 8.2 Log Retention（统一日志保留）验收

在 `config/powerx.env.local`（本地）或 `/etc/powerx/powerx.env`（生产）启用 `log.retention`（示例）：

```bash
CORE_X_LOG_RETENTION_ENABLED=true
CORE_X_LOG_RETENTION_CRON="10 3 * * *"
CORE_X_LOG_RETENTION_TIMEZONE=Asia/Shanghai
CORE_X_LOG_RETENTION_DEFAULT_DAYS=30
```

或在 `config.yaml` 中配置：

```yaml
log:
  retention:
    enabled: true
    cron: "10 3 * * *"
    default_retention_days: 30
```

验收步骤：
- 触发一次手动清理（或等待定时任务）；
- 查询监控中心 Logs/Trace 中“日志保留任务最近执行”；
- 确认输出包含：`source`、`deleted_count`、`duration_ms`、`status`、`error`（失败时）。

可选 API 验证：

```bash
# 立即执行一次日志保留清理
curl -sS -X POST "http://127.0.0.1:8080/api/v1/admin/monitor/logs/retention/run" \
  -H "Authorization: Bearer $TOKEN" | jq

# 查看最近执行记录
curl -sS "http://127.0.0.1:8080/api/v1/admin/monitor/logs/retention/runs?limit=20" \
  -H "Authorization: Bearer $TOKEN" | jq
```

通过标准：
- 文件日志和数据库日志均按 retention 生效；
- 清理失败时可在页面与日志中看到明确错误原因；
- 连续执行后磁盘使用率与日志表增长趋势可控。

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
