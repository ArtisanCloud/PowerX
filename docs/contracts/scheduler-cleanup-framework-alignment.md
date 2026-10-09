# Framework Scheduler清理对齐

日期：2026-10-08。Core合同：[任务删除](scheduler-job-cleanup.md)、[OpenAPI](../../specs/028-runtime-scheduler/contracts/cleanup-openapi.yaml)。Core只交付底座，CRM规则/Framework/skeleton由消费端实现。

## 1. 独立扩展

增加SchedulerCleanup接口，避免给Scheduler/HostClient现有接口增加必选方法。建议：

```go
type SchedulerCleanup interface {
    DeleteJob(context.Context, DeleteJobRequest) (*DeleteJobResult, error)
}
```

HostCleanupClient同样独立，通过明确的Provider/能力装配获取；旧适配器未实现时返回不支持。Local/Host在bootstrap选定，Host失败不回退Local。LocalProvider需要自己的revision、删除状态/墓碑、停止后续调度和发布并发控制，不能只从map移除后让已取出的任务继续发事件。

## 2. 字段

- Preview: job_uuid、tenant_uuid、owner_type/owner_id、name、schedule_type/expr、payload、status、revision、updated_at。服务DTO使用job_uuid；旧Admin模型字段uuid与之同义，显式映射，不能按name删除。
- DeleteJobRequest: job_uuid、expected_revision（required positive uint64）；单次确认使用预览版本。
- DeleteJobResult: schema、job_uuid、status=deleted、deleted、already_deleted、revision、deleted_at、history_retained、trace_id。
- Error: HTTP status、reason_code/error_code、request_id/trace_id；409冲突、403撤销或owner错误不能被SDK改成成功、自动换版本或扩大删除范围。
- UTC/RFC3339时间原样回读；revision是单调递增整数，不把updated_at代替版本。

## 3. Transport

| 操作 | tenant REST | typed operation / capability |
| --- | --- | --- |
| Preview list | GET /api/v1/tenant/scheduler/jobs?owner_id={plugin_id} | list_jobs / com.corex.scheduler.jobs.service_read |
| Get / tombstone | GET /api/v1/tenant/scheduler/jobs/{job_uuid}?include_deleted=true | get_job / 同上 |
| Runs | GET /api/v1/tenant/scheduler/jobs/{job_uuid}/runs | list_runs / 同上 |
| Delete | DELETE /api/v1/tenant/scheduler/jobs/{job_uuid}?expected_revision={revision} | delete_job / com.corex.scheduler.jobs.service_delete |

Delegated走 /api/v1/tenant/invocations，preferred_protocol=core_internal，固定INVOKE core://scheduler/jobs。Query/header/raw HTTP代理字段被拒绝；body按正式typed DTO发送。

Local Host simulation可使用上述tenant REST和有效API Key；每条Key grant除scope/action/api/scheduler_jobs外，必须明确plugin_id=owner_id，空/通配owner不能获得删除资格。STS必须可信plugin_id匹配存储owner，同时拥有实时有效服务grant及tenant registration。

Admin接口是用户JWT/RBAC平面。新读取/删除服务调用不得继续请求 /admin/scheduler/jobs。迁移HTTPHostClient的清理/预览方法；旧Scheduler创建/更新等既有入口不作为新清理权限的替代。

## 4. 批量预览和执行

1. 获取当前provider与tenant/owner范围内的完整分页列表。
2. CRM用自己的样例标记/识别规则筛选；Framework仅承接通用清理，不内置CRM名称猜测。
3. 展示预览准确集合（UUID、revision、名称、状态、owner及数量），确认后提交同一集合。
4. 逐项DELETE，报告deleted、already_deleted、conflict、failed；记录每项Trace。
5. 409要求重新预览。禁止重新按条件扫描后把新增任务纳入这次确认，禁止读新revision自动强删。
6. skeleton提供通用按钮、预览、确认和结果；业务识别仍在插件。

## 5. 验收

Go类型/序列化，optional能力不破坏旧mock；Local删除/手动触发/定时扫描竞态；Host真实API Key和STS；只读Key不能删除、跨tenant/owner拒绝、撤销即时403；版本冲突、重复删除、同名重建、已删任务不恢复、不再触发、历史保留；批量部分失败/重新预览与浏览器确认准确集合。

Core的隔离HTTP/多连接证据不替代Framework和CRM浏览器验收。
