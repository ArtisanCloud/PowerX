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

DRILL_JSON=$(curl -sS -X POST "http://127.0.0.1:8080/api/v1/admin/ops/backup/drills" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"artifact_id\":\"${ARTIFACT_ID}\",\"reason\":\"weekly-drill\"}")

echo "$DRILL_JSON" | jq
DRILL_ID=$(echo "$DRILL_JSON" | jq -r '.data.id // empty')

curl -sS "http://127.0.0.1:8080/api/v1/admin/ops/backup/drills/${DRILL_ID}" \
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
