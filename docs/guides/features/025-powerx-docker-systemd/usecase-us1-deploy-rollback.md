# Use Case US1：双模式生产部署与回滚

## 1. 目标
- 在 Docker/systemd 任一模式下完成一次发布与一次回滚。
- 验收口径：60 分钟内完成上线，15 分钟内完成回滚。

## 2. 页面操作（Web Admin）
1. 动作：进入部署发布中心。
- 入口：`/ops/deploy`
- 预期结果：看到“部署发布中心”“发布参数”“发布记录”。
- 失败处理：无页面或 403 时检查登录态与 Ops Deploy 权限。

2. 动作：填写发布参数并触发发布。
- 入口：页面“触发发布”按钮。
- 预期结果：发布记录新增，状态进入 `pending/running/success`。
- 失败处理：若卡在 `failed`，检查 `trace_id` 与后端 deploy service 日志。

3. 动作：对指定版本执行回滚。
- 入口：发布记录行内“回滚到此版本”。
- 预期结果：新增 `action=rollback` 记录，健康状态刷新。
- 失败处理：出现审批不足时补齐 `approval_tickets>=2`。

## 3. 接口调用（Admin API）

```bash
curl -X POST "http://127.0.0.1:8080/api/v1/admin/deploy/releases?mode=systemd&approval_tickets=2" \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"environment":"prod","backend_version":"v1.2.3","web_admin_version":"v1.2.3"}'

curl -X POST "http://127.0.0.1:8080/api/v1/admin/deploy/rollback?mode=systemd&approval_tickets=2" \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"environment":"prod","target_version":"v1.2.2"}'
```

## 4. 本地联调

```bash
bash backend/scripts/ops/deploy-check.sh
bash backend/scripts/ops/rollback-release.sh prod v1.2.2 docker
```

## 5. 代码映射
- 路由：`backend/internal/transport/http/admin/deploy/routes.go`
- Handler：`backend/internal/transport/http/admin/deploy/handler.go`
- Service：`backend/internal/service/deploy_ops/service.go`
- 审批策略：`backend/internal/service/deploy_ops/approval_policy_service.go`
- E2E：`web-admin/tests/e2e/ops/deploy-center.spec.ts`
