# Use Case US4：A->B 实例迁移与回切

## 1. 目标
- 完成一次迁移演练：触发 runbook、提交验收、流量切换、异常回切。
- 验收口径：记录包含 DB 迁移状态、实例验收状态、切换/回切状态。

## 2. 页面操作（Web Admin）
1. 动作：触发迁移 runbook。
- 入口：`/ops/migration` -> “触发迁移”。
- 预期结果：出现迁移记录（含 `db_migration_status`）。
- 失败处理：检查源/目标环境与依赖连通性。

2. 动作：提交迁移验收。
- 入口：同页勾选“DB 迁移完成”“实例验收通过”后“提交验收”。
- 预期结果：`instance_acceptance_status=success`。
- 失败处理：若 `ErrMigrationNotReady`，先补齐前置步骤。

3. 动作：执行流量切换与回切。
- 入口：同页“流量切换”与“回切”。
- 预期结果：返回 `operation_id`，状态进入 `success`。
- 失败处理：先回切，再检查迁移校验脚本与验收结论。

## 3. 接口调用（Admin API）

```bash
curl -X POST "http://127.0.0.1:8080/api/v1/admin/migration/runbooks/run" \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"source_env":"prod-a","target_env":"prod-b","dry_run":false}'

curl -X POST "http://127.0.0.1:8080/api/v1/admin/migration/traffic/switch" \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"migration_id":"101","rollback":true}'
```

## 4. 本地联调

```bash
bash backend/scripts/ops/export-instance.sh 101 prod-a prod-b
bash backend/scripts/ops/import-instance.sh 101 prod-a prod-b
bash backend/scripts/ops/verify-migration.sh 101 prod-a prod-b
bash backend/scripts/ops/switch-traffic.sh 101 prod-a prod-b
bash backend/scripts/ops/rollback-traffic.sh 101 prod-a prod-b
```

## 5. 代码映射
- 路由：`backend/internal/transport/http/admin/migration/routes.go`
- Handler：`backend/internal/transport/http/admin/migration/handler.go`
- Service：`backend/internal/service/migration_ops/service.go`
- 页面：`web-admin/app/pages/ops/migration.vue`
- E2E：`web-admin/tests/e2e/ops/instance-migration.spec.ts`
