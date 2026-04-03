# Use Case US4：A->B 环境迁移演练

## 1. 场景目标
- 完成 A->B 迁移 runbook 执行、验收、流量切换与回切。
- 明确区分“DB 迁移完成”与“实例验收通过”的门禁。

## 2. 页面操作步骤（Web Admin）

1. 动作：触发迁移演练。
- 入口/命令：`/ops/migration`，填写 `source_env`、`target_env`、`dry_run`，点击“触发迁移”。
- 预期结果：页面出现迁移记录，状态更新为运行态。
- 失败处理：参数为空或环境不合法时，先修正输入再重试。

2. 动作：提交验收并执行流量切换。
- 入口/命令：勾选“DB 迁移完成/实例验收通过”，点击“提交验收”后再点“流量切换”。
- 预期结果：记录中的 `instance_acceptance_status` 与 `traffic_switch_status` 变为成功。
- 失败处理：若切换失败，点“回切”并记录操作 ID。

## 3. 接口调用步骤（Admin API）

1. 动作：触发迁移。
- 入口/命令：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/migration/runbooks/run" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"source_env":"prod-a","target_env":"prod-b","dry_run":false}'
```

- 预期结果：返回 `record.id`。
- 失败处理：校验源/目标环境配置与脚本执行环境。

2. 动作：提交验收。
- 入口/命令：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/migration/runbooks/<migration_id>/acceptance" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"db_migration_completed":true,"instance_migration_passed":true,"conclusion":"核心能力验收通过"}'
```

- 预期结果：`db_migration_status` 与 `instance_acceptance_status` 进入 success。
- 失败处理：若未通过，修复环境差异后重新提交验收。

3. 动作：执行切换或回切。
- 入口/命令：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/migration/traffic/switch" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"migration_id":"<migration_id>","rollback":false}'
```

- 预期结果：返回 `operation_id` 与更新后的 `record`。
- 失败处理：将 `rollback` 置 `true` 触发回切。

## 4. 本地联调步骤

1. 动作：执行迁移测试。
- 入口/命令：

```bash
cd backend && go test ./tests/contract/ops -run 'TestHTTPMigrationContract|TestGRPCMigrationContract' -count=1
cd backend && go test ./tests/integration/ops -run TestInstanceMigrationFlow -count=1
```

- 预期结果：迁移合同与主流程测试通过。
- 失败处理：核对 `backend/scripts/ops/{export-instance.sh,import-instance.sh,verify-migration.sh,switch-traffic.sh,rollback-traffic.sh}`。

2. 动作：验证 trace 贯通。
- 入口/命令：

```bash
cd backend && go test ./tests/integration/ops -run TestTraceabilityAcrossOpsDomains -count=1
```

- 预期结果：各域审计可按同一 `trace_id` 查询。
- 失败处理：检查 HTTP 中间件 trace 注入与审计写入链路。

## 5. 验收标准
- 导出、导入、验收、切换、回切均可执行。
- 迁移记录字段完整，且状态转换符合门禁规则。
- 异常时可在 15 分钟内回切至原环境。

## 6. 关键代码映射
- HTTP：`backend/internal/transport/http/admin/migration/{routes.go,handler.go}`
- Service：`backend/internal/service/migration_ops/service.go`
- 页面：`web-admin/app/pages/ops/migration.vue`
- 前端 API：`web-admin/app/composables/api/services/migrationOpsService.ts`
- 脚本：`backend/scripts/ops/{export-instance.sh,import-instance.sh,verify-migration.sh,switch-traffic.sh,rollback-traffic.sh}`
