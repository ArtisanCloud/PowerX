# Scheduler Core删除交付

- [x] S01 模型/版本/活跃名称唯一性及集中迁移
- [x] S02 删除与全部任务写入/发布协调、历史/重复删除
- [x] S03 Admin JWT/RBAC与typed/API Key/STS服务授权
- [x] S04 OpenAPI、Core合同和Framework对齐
- [x] S05 单元/并发、真实Postgres多连接与HTTP/STS验收
- [x] S06 迁移、capability-check/seed、开发Host Key grant-status

## 验收证据

2026-10-08：Go回归/race/vet/build通过；32条真实HTTP/STS记录与独立Postgres连接发布竞态通过；实际开发库migrate、capability-check/seed完成；8077开发Key两项owner限定grant-status=granted。

[Core合同](../../docs/contracts/scheduler-job-cleanup.md)、[Framework对齐](../../docs/contracts/scheduler-cleanup-framework-alignment.md)、[HTTP证据](../../docs/contracts/scheduler-cleanup-acceptance-20261008.json)、[开发grant](../../docs/contracts/scheduler-development-grants-20261008.json)。

Core交付完成。8077及其他调度节点需重启加载新代码；Framework/CRM验收属于消费端工作。
