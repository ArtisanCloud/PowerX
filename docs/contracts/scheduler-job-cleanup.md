# Scheduler通用任务删除合同

日期：2026-10-08。Core实现交付；CRM样例识别与Framework/skeleton接入由消费端负责。

正式OpenAPI：[cleanup-openapi.yaml](../../specs/028-runtime-scheduler/contracts/cleanup-openapi.yaml)。

Admin DELETE /api/v1/admin/scheduler/jobs/{job_uuid}?expected_revision=1，用户JWT与corex/scheduler.jobs/delete RBAC；服务态使用DELETE /api/v1/tenant/scheduler/jobs/{job_uuid}?expected_revision=1或固定typed core://scheduler/jobs INVOKE delete_job。

读 capability=com.corex.scheduler.jobs.service_read；删除 capability=com.corex.scheduler.jobs.service_delete。API Key必须有明确plugin_id对应的owner grant；不接受空或通配plugin_id作为owner身份。STS需要published capability、tenant registration、当前有效凭据grant及可信plugin_id与stored owner相等。服务actor只操作plugin owner，core owner仅root。

同次确认传预览的UUID/revision；版本变化409 SCHEDULER_JOB_VERSION_CONFLICT。所有更新/暂停/恢复/手动和自动触发递增revision。删除软删、置deleted、清除next_run_at；普通列表/扫描排除，保留runs/tombstone，授权查询历史。重复删除返回already_deleted=true；同名重建使用新UUID，旧历史仍可查询。

Postgres按任务session advisory fence贯穿DB事务和EventBus.Publish返回，跨Core实例协调。删除等待已有发布调用完成；删除先完成则随后触发/修改/恢复拒绝。已发布事件和消费者业务处理不撤回；run表示调度触发记录，不代表CRM业务完成。事件总线现有Publish接口不提供送达确认；本合同不宣称exactly-once。非Postgres仅提供单进程互斥，不宣称跨实例发布保证。

Framework必须独立扩展SchedulerCleanup/HostCleanupClient，使用强类型参数/结果。服务态读取/list_jobs、get_job、list_runs走tenant接口或typed服务read；旧Admin读取的服务凭据调用明确拒绝。新服务操作不接受HTTP method/endpoint/headers/raw proxy。

```json
{"capability_id":"com.corex.scheduler.jobs.service_delete","preferred_protocol":"core_internal","payload":{"body":{"operation":"delete_job","job_uuid":"11111111-1111-4111-8111-111111111111","expected_revision":1}}}
```

响应data/result为powerx.scheduler.job-deletion/v1：job_uuid、status=deleted、deleted=true、already_deleted、revision、deleted_at、history_retained=true、trace_id。REST统一envelope；typed返回Registry envelope再包含result。

Core不提供批量筛选/CRM样例识别。Framework采集预览的准确UUID/revision/tenant/owner集合，确认后逐项调用，报告deleted/already_deleted/conflict/failed；冲突重新预览，不自动更新revision再删。

## 验收与部署

2026-10-08最终Core验收通过：

- 受影响Go包回归、runtime_scheduler race测试、go vet与app构建通过。
- PostgreSQL独立连接验收通过：发布阻塞期间删除等待；发布返回后删除成功；后续触发拒绝。自动扫描同样使用发布锁，单元竞态覆盖。
- 32条真实HTTP/STS记录：Admin JWT删除、API Key删除、typed STS删除、预览版本变化409、缺版本400、跨tenant404、跨owner403、只读Key删除403、撤销立即403、同名新UUID重建、重复删除及保留历史。
- Admin JWT使用隔离验收服务的私有测试签名配置，验证Core鉴权/RBAC，不代表实际浏览器登录验收。
- 已对实际开发库执行集中migrate，受影响两表备份保存在/tmp/powerx-scheduler-cleanup/scheduler-before.dump；revision默认1，旧全量名称唯一索引替换为活跃部分索引。
- capability-check通过：declared=1024、referenced=62、rest_routes=944、candidates=983、ignore=6。capability-seed同步1024能力及4个tenant registration。
- 开发Host Key精确授予com.powerx.plugins.scrm owner的service_read/service_delete；实际8077 grant-status两项granted，不改变其他Key或扩大owner范围。

证据：[HTTP/Trace](scheduler-cleanup-acceptance-20261008.json)、[实际开发Key grant-status](scheduler-development-grants-20261008.json)。Framework对齐：[清理接口与消费端清单](scheduler-cleanup-framework-alignment.md)。

部署必须重新编译并重启8077及其他Scheduler Core实例，使所有修改/触发/删除遵守同一发布锁和revision协议。旧版本节点不会参与新锁，不能在旧/新版本混跑期间宣称严格删除屏障。后台启动不会自动migrate。

旧服务凭据调用Admin读取被明确拒绝，迁移到tenant读入口或typed service_read；Admin用户需read RBAC。删除结果不撤回已发出的事件，历史保留不等于业务执行已完成。

## 代码映射

- Service合同/授权：backend/internal/service/runtime_scheduler/cleanup.go、cleanup_invoker.go。
- 统一任务变更/发布锁：mutation.go；repository/runtime_scheduler/fence.go；service.go中的手动与自动触发。
- 版本/CAS/软删：model/runtime_scheduler/job.go、repository/runtime_scheduler/job_repository.go。
- 迁移：pkg/corex/db/database/migration.go中的migrateRuntimeSchedulerModels。
- HTTP：internal/transport/http/admin/scheduler/cleanup_handler.go、routes.go。
- 能力：config/platform_capabilities/workflow.yaml；错误透传：openapi/capability_registry/tenant_handler.go。
- 回归：service/runtime_scheduler/cleanup_test.go；真实HTTP/STS/Postgres：tests/contract/scheduler_cleanup/live_test.go。

