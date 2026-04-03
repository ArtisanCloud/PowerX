# Use Case US2：插件无市场阶段平滑升级

## 1. 场景目标
- 完成“安装后不立即生效 -> 切换版本 -> 异常回滚”闭环。
- 审计时间线保留动作、版本、结果与 trace_id。

## 2. 页面操作步骤（Web Admin）

1. 动作：进入插件生命周期中心并拉取审计。
- 入口/命令：`/ops/plugins`，输入 `Plugin ID`。
- 预期结果：页面加载审计时间线。
- 失败处理：若列表为空，先确认 `pluginId` 是否正确。

2. 动作：执行版本切换/回滚。
- 入口/命令：设置 `action=switch|rollback`，填写 `from_version`、`to_version`、`reason`，点击“触发动作”。
- 预期结果：新增审计记录，`result=success`。
- 失败处理：若返回参数错误，检查切换动作对应版本字段是否完整。

## 3. 接口调用步骤（Admin API）

1. 动作：触发插件切换。
- 入口/命令：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/plugins/plugin.mediax/actions" \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"plugin_id":"plugin.mediax","from_version":"1.0.0","to_version":"1.1.0","action":"switch","reason":"canary passed"}'
```

- 预期结果：响应包含 `audit`，并记录 `trace_id`。
- 失败处理：`400` 多为动作与版本参数不匹配。

2. 动作：查询审计轨迹。
- 入口/命令：

```bash
curl -sS -X GET "http://127.0.0.1:8077/api/v1/admin/plugins/plugin.mediax/audit?page=1&page_size=20" \
  -H "Authorization: Bearer <ADMIN_TOKEN>"
```

- 预期结果：返回最近 N 条动作，含 `action/result/operator/trace_id`。
- 失败处理：检查 RBAC 是否包含 `Ops 插件-查看`。

## 4. 本地联调步骤

1. 动作：执行插件生命周期集成与合同测试。
- 入口/命令：

```bash
cd backend && go test ./tests/contract/ops -run TestHTTPPluginAuditContract -count=1
cd backend && go test ./tests/integration/ops -run TestPluginLifecycleFlow -count=1
```

- 预期结果：测试通过，动作审计字段完整。
- 失败处理：优先核对 handler DTO 与 service 校验逻辑。

## 5. 验收标准
- 至少完成 1 次切换 + 1 次回滚。
- 每次动作均可在审计时间线追溯。
- 失败动作有可读原因并可复现。

## 6. 关键代码映射
- HTTP：`backend/internal/transport/http/admin/deploy/plugin_lifecycle_{routes,handler}.go`
- Service：`backend/internal/service/deploy_ops/plugin_lifecycle_service.go`
- 页面：`web-admin/app/pages/ops/plugins.vue`
- 前端 API：`web-admin/app/composables/api/services/pluginOpsService.ts`
