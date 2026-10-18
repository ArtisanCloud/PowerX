# Quickstart: 自动备份闭环（027-monitor-center）

> 2026-10-15：第 1–10 节保留组件级流程；统一包、独立应用验证和正式切换已在本地实现，接入按[托管恢复手册](../../docs/guides/deploy/027-monitor-center/usecase-us8-managed-recovery.md)。第 11 节本地原生回归已通过，Linux／生产插件及真实定时第二轮待验收。以下 `8080` 是示例 Core 地址，需替换为实际端口。

## 1. 前置条件

- 在用户当前分支验证；`027-monitor-center` 是功能文档标识，不要求切换分支。
- 已完成数据库迁移（包含 `policy/job/artifact/drill/alert` 相关表）
- Root 管理员可登录管理端并拿到 `TOKEN`
- 目标环境具备稳定本地备份目录；现有策略 `target_ref=local_dump` 不代表另一个目标数据库。

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

- `backend/scripts/ops/backup-db.sh`、`cleanup-backups.sh`、`restore-drill.sh` 是服务内部脚本，参数由服务构造；不要按历史单参数模板直接调用。
- 运维优先使用同环境的 `database backup-run -config <实际配置> -policy-id <实际策略ID>` 和 `database backup-restore-verify -config <实际配置> -job-id <实际作业ID>`。
- 独立调度、路径与权限按 [US1](../../docs/guides/deploy/027-monitor-center/usecase-us1-policy-automation.md)；正式数据库切换按 [US3](../../docs/guides/deploy/027-monitor-center/usecase-us3-restore-drill.md)，不是脚本成功后自动发生。

回滚建议：
- 备份／演练失败先保留现有业务状态并定位组件错误，不自动回滚程序。确需正式切换或发布回退时，先核对版本、维护窗口和数据库／媒体／配置回退范围。

## 3. 创建并启用自动备份策略

```bash
# 3.1 创建策略
POLICY_JSON=$(curl -sS -X POST "http://127.0.0.1:8080/api/v1/admin/ops/backup/policies" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "prod-default-policy",
    "interval_hours": 1,
    "retention_count": 168,
    "retention_days": 7,
    "retention_mode": "age_and_count",
    "timezone": "Asia/Shanghai",
    "drill_enabled": false,
    "drill_interval_days": 7,
    "target_ref": "local_dump"
  }')

echo "$POLICY_JSON" | jq
POLICY_ID=$(echo "$POLICY_JSON" | jq -r '.data.policy.id // empty')

# 3.2 首份手动备份与恢复验证通过后再启用（见 US1）
curl -sS -X POST "http://127.0.0.1:8080/api/v1/admin/ops/backup/policies/${POLICY_ID}/enable" \
  -H "Authorization: Bearer $TOKEN" | jq
```

验收点：
- `POLICY_ID` 非空。
- 新建返回 `enabled=false`；先按 US1 验证首份文件与恢复，再执行启用请求。
- 列表可看到 `interval_hours=1`、`retention_days=7`、`retention_count=168`、`timezone=Asia/Shanghai`；这只定义该测试数据库策略，不改变线上策略。

## 4. 验证策略列表分页与过滤

```bash
curl -sS "http://127.0.0.1:8080/api/v1/admin/ops/backup/policies?page=1&page_size=20&status=enabled&keyword=prod&timezone=Asia/Shanghai" \
  -H "Authorization: Bearer $TOKEN" | jq
```

验收点：
- 返回 `data.items` 为数组。
- 返回 `data.pagination.page=1`、`data.pagination.page_size=20`。

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
# 从实际成功作业行取得 ID；仅针对自建测试环境
SOURCE_JOB_ID='<实际成功备份作业ID>'

DRILL_JSON=$(curl -sS -X POST "http://127.0.0.1:8080/api/v1/admin/ops/backup/restore-drills" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"source_job_id\":\"${SOURCE_JOB_ID}\",\"reason\":\"manual-component-drill\"}")

echo "$DRILL_JSON" | jq
DRILL_ID=$(echo "$DRILL_JSON" | jq -r '.data.drill.id // empty')

curl -sS "http://127.0.0.1:8080/api/v1/admin/ops/backup/restore-drills/${DRILL_ID}" \
  -H "Authorization: Bearer $TOKEN" | jq
```

验收点：
- `DRILL_ID` 非空。
- 详情状态可见 `queued/running/success/failed` 之一。

## 7. 监控页验收（策略 -> 作业 -> 告警 -> 演练）

- 进入 `监控中心 -> Task / Cron`：可见备份任务状态与下次执行时间。
- 进入 `监控中心 -> Logs / Trace`：可见备份链路摘要。
- 进入 `监控中心 → 备份中心`（`/ops/backup`）：可见策略列表、作业历史、告警列表、演练历史。

## 8. 通过标准

- 测试数据库策略符合显式设置的 `1h / 7 天 / 168 份 / Asia/Shanghai`；自动验证默认关闭，手动验证通过后再启用策略。
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
## 先通过 PowerX 监控编排接口列出可操作插件
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/admin/monitor/logs/plugins" | jq

# 方式一：通过 PowerX 监控编排接口读取策略（推荐）
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/admin/monitor/logs/plugins/${PLUGIN_ID}/policy" | jq

# 方式二：直连插件接口读取策略（联调用）
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
# 方式一：通过 PowerX 监控编排接口执行 probe（推荐）
curl -sS -X POST \
  "http://127.0.0.1:8080/api/v1/admin/monitor/logs/plugins/${PLUGIN_ID}/probe" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "powerx monitor plugin logger probe",
    "level": "info",
    "component": "monitor.logs.quickstart",
    "trace_id": "probe-quickstart-001",
    "tenant_uuid": "demo-tenant"
  }' | jq

# 方式二：直连插件接口执行 probe（联调用）
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

## 8.4 request_id 全链路串联验收（integration + gateway + audit + wsbus）

目标：验证同一次 `/api/v1/integration/<slug>/...` 请求可被单个 `request_id` 串联，且 `plugin_id` 非空。

步骤 A：触发一次 integration 请求（示例）

```bash
curl -i -sS -X POST \
  "http://127.0.0.1:8080/api/v1/integration/ai-craft/webhooks/shopify" \
  -H "Content-Type: application/json" \
  -d '{"probe":"quickstart-chain"}'
```

记录响应头 `X-Request-ID=<RID>`。

步骤 B：用统一日志查询接口按 `request_id` 检索

```bash
curl -sS -H "$AUTH" \
  "$BASE/admin/monitor/logs/query?page=1&page_size=200&request_id=<RID>" | jq
```

步骤 C：在 Grafana Explore 验证（Loki 驱动）

```logql
{system="powerx",service="powerx-backend",env="prod"} |= "request_id=<RID>"
```

通过标准：
- 同一 `request_id` 至少命中：`http_request`、`audit_event`。
- 若经过 `_p` 代理链路，应命中 `API-IN/GATE-*/PROXY-*`。
- 若触发 WS 总线事件，应命中 `wsbus.*`。
- 命中日志中的 `plugin_id` 非空（不得出现稳定复现的 `plugin_id=""`）。

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

## 11. 系统快照两轮验收（2026-10-15，本地通过，部署验收待执行）

设计与任务：[系统快照计划](../../docs/plan/deploy/system-snapshot-and-restore.md)、[Phase 10 / T077–T100](tasks.md)。实际验收步骤以 [US8 手册](../../docs/guides/deploy/027-monitor-center/usecase-us8-system-snapshot-acceptance.md)为准。

1. 本地创建可辨识状态 A，包含业务记录、原图／附件／变体和测试配置，生成并完整导出快照 S。
2. 在源测试实例新增、修改、删除，形成 B；在空的独立目标导入 S，完成配置／身份／绑定映射和启动前外部写入隔离。
3. 源实例不可访问时恢复 A：业务数据、全部有效媒体及内容摘要、配置与版本匹配；源 B 不变化。另在自建测试实例验证正式切换与失败整套回退。
4. 第一轮通过后由运维升级服务器，至少观察一个真实计划周期，可按一天验收。取得标记 scheduled 的完整系统快照包，不能以手动任务冒充。
5. 将包带到本地／独立开发实例重复独立恢复与业务验收，记录组件状态、业务断言、隔离证明、耗时和结论。

缺内容、缺密钥／配置、版本／平台不兼容、空间不足、并发变更、生产目标误配、进程中断及切换失败均需明确阻断／失败。单纯构建、组件测试、HTTP 202 或数据库恢复成功不作为本节通过依据。本地已执行真实 PostgreSQL／Redis／Core／Web 的 A/B、切换及协调器强制终止回退；证据见[报告](../../docs/contracts/evidence/system-snapshot-managed-validation-20261015.json)。生产升级和真实定时包第二轮未执行。
