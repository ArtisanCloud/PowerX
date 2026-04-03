# Use Case US2：插件平滑升级与回滚

## 1. 目标
- 完成“安装后不立即生效 -> 验证 -> 切换 -> 回滚”的可追溯流程。
- 验收口径：审计记录完整，关键动作具备 `trace_id`。

## 2. 页面操作（Web Admin）
1. 动作：进入插件生命周期中心。
- 入口：`/ops/plugins`
- 预期结果：看到“插件生命周期中心”。
- 失败处理：无权限时检查 `OpsResourcePlugin`。

2. 动作：输入 `pluginId` 与版本信息，触发 `switch`。
- 入口：页面“触发动作”。
- 预期结果：审计时间线新增 `switch` 记录。
- 失败处理：若失败，读取 `gate_reason` 与 `detail` 字段。

3. 动作：触发 `rollback` 回滚旧版本。
- 入口：动作改为 `rollback` 再次触发。
- 预期结果：时间线新增 `rollback` 记录，结果为 `success`。
- 失败处理：若目标版本缺失，先校验版本存在性后重试。

## 3. 接口调用（Admin API）

```bash
curl -X GET "http://127.0.0.1:8080/api/v1/admin/plugins/plugin.mediax/audit?page=1&page_size=20" \
  -H 'Authorization: Bearer <admin-token>'

curl -X POST "http://127.0.0.1:8080/api/v1/admin/plugins/plugin.mediax/actions" \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"plugin_id":"plugin.mediax","action":"switch","from_version":"1.0.0","to_version":"1.1.0","reason":"canary passed"}'
```

## 4. 本地联调

```bash
cd backend && go test ./tests/integration/ops -run TestPluginLifecycleFlow -count=1
cd ../web-admin && npm run test:e2e -- ops/plugin-lifecycle.spec.ts
```

## 5. 代码映射
- 路由：`backend/internal/transport/http/admin/deploy/plugin_lifecycle_routes.go`
- Handler：`backend/internal/transport/http/admin/deploy/plugin_lifecycle_handler.go`
- Service：`backend/internal/service/deploy_ops/plugin_lifecycle_service.go`
- 页面：`web-admin/app/pages/ops/plugins.vue`
- E2E：`web-admin/tests/e2e/ops/plugin-lifecycle.spec.ts`
