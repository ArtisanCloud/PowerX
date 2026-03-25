# Use Case US1：双模式生产部署与回滚

## 1. 场景目标
- 在同一环境分别完成 Docker/systemd 发布，并验证可回滚。
- 输出发布记录、健康状态和 trace_id，满足 QA 独立复现。

## 2. 页面操作步骤（Web Admin）

1. 动作：进入部署发布中心并填写发布参数。
- 入口/命令：`/ops/deploy`，填写 `环境=prod`、`模式=docker|systemd`、版本号与审批票数。
- 预期结果：点击“触发发布”后，发布记录新增一条 `action=release`。
- 失败处理：若返回权限错误，检查账号是否 root/租户管理员。

2. 动作：执行回滚。
- 入口/命令：在发布记录点击“回滚到此版本”。
- 预期结果：出现 `action=rollback` 新记录，健康状态刷新。
- 失败处理：若提示审批不足，确认 `approval_tickets>=2` 或调整审批策略。

## 3. 接口调用步骤（Admin API）

1. 动作：触发发布（Docker 模式示例）。
- 入口/命令：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/deploy/releases?mode=docker&approval_tickets=2" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"environment":"prod","backend_version":"v2.0.1","web_admin_version":"v2.0.1"}'
```

- 预期结果：响应中返回 `release`，状态通常为 `pending/running`，含 `trace_id`。
- 失败处理：`409` 多为已有进行中发布，等待完成后重试。

2. 动作：触发回滚。
- 入口/命令：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/deploy/rollback?mode=docker&approval_tickets=2" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"environment":"prod","target_version":"v2.0.0"}'
```

- 预期结果：新增 `rollback` 记录，最终状态转为 `success`。
- 失败处理：失败后用健康接口和日志按 `trace_id` 定位。

## 4. 本地联调步骤

1. 动作：校验部署健康。
- 入口/命令：`bash backend/scripts/ops/deploy-check.sh`
- 预期结果：输出 `[deploy-check] healthy`。
- 失败处理：检查 backend 服务端口、健康接口路径和网关转发。

2. 动作：运行 US1 集成测试。
- 入口/命令：

```bash
cd backend && go test ./tests/integration/ops -run 'TestDeployReleaseFlow|TestDeployModeParity' -count=1
```

- 预期结果：两条用例通过。
- 失败处理：核对 `deploy/powerx/docker/compose.prod.yaml` 与 `deploy/powerx/systemd/*.service` 资产一致性。

## 5. 验收标准
- 发布记录可查询，且能区分 `docker/systemd` 模式。
- 15 分钟内可完成回滚并恢复健康。
- 关键动作具备 `trace_id` 与审计记录。

## 6. 关键代码映射
- HTTP：`backend/internal/transport/http/admin/deploy/{routes.go,handler.go}`
- Service：`backend/internal/service/deploy_ops/service.go`
- 页面：`web-admin/app/pages/ops/deploy.vue`
- 前端 API：`web-admin/app/composables/api/services/deployOpsService.ts`
- 脚本：`backend/scripts/ops/{deploy-check.sh,rollback-release.sh}`
