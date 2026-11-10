# 任务历史幂等结构升级

`TaskHistory` 新增的可空 `idempotency_key` 列会出现在统一历史仓储的 INSERT 中。
旧表缺列时，即使该次写入值为 NULL，也会触发 PostgreSQL 42703；媒体任务与历史在同一事务登记，
因此整体回滚，尚未入队，更不会进入供应商生成。

## 正式迁移入口

- 全量安装／升级继续使用 `MigrateCoreModels → migrateEventFabricModels`，模型集中挂载于
  `backend/pkg/corex/db/database/migration.go`。Core 升级步骤在事务内验证最终任务历史结构。
- Mac 开发库可从仓库根目录执行限定增量：

```sh
cd backend && go run ./cmd/database migrate -config etc/config.yaml -scope task-history-idempotency
```

该 scope 只允许 PostgreSQL、`deployment.env=dev/test`；要求历史表已经存在。
它复用正式数据库迁移锁和事务，锁等待上限 3 秒，整个限定操作上限 1 分钟。
仅通过 GORM Migrator 增加以下结构，不执行其他模型 AutoMigrate、seed、角色／Key 授权或历史回填：

| 对象 | 要求 |
|---|---|
| event_task_histories.idempotency_key | varchar(255)，可空，无历史值回填 |
| uidx_event_task_histories_idempotency | UNIQUE (tenant_key, subscriber_id, idempotency_key) |

旧记录保留 NULL，同一租户／订阅者可保留多个旧记录；新任务非空幂等键按完整三元组唯一。
重复执行不改变已正确的结构。已存在的错误列类型、错误索引、同名索引冲突或重复非空键会失败，
本次增量 DDL 随事务回滚，不删除／覆盖既有索引或任务记录来强行通过。

正式环境通过完整离线升级计划执行，不使用开发限定 scope。
`deploy/powerx/container/upgrade_registry.py` 将模型、迁移和检查源码纳入内容寻址清单；
源代码变化后需要重新生成并运行 `--check`，不会覆盖旧迁移的成功履历。

## 启动与运行

Core 在创建缓存、插件及业务 worker 前调用 `CheckTaskHistorySchema`，只读检查：

- 历史表、列类型、长度和可空性；
- 唯一索引名称、唯一性和精确列顺序；
- PostgreSQL 索引必须 valid/ready、非部分索引、无表达式，保留标准 NULL DISTINCT 语义。

缺失或不兼容时以 `TASK_HISTORY_SCHEMA_NOT_READY` 明确拒绝启动，提示先迁移。
启动不会自动改库。已运行进程的缺列故障在结构迁移提交后即可解除，无需为数据库 DDL 单独重启；
新增启动检查在下次使用新构建启动时生效。

## 受控验收与恢复原尝试

1. 用缺少新增列／索引的原生 PostgreSQL 旧表验证：迁移前拒绝，迁移后结构正确，重复迁移稳定；
   历史内容、时间、状态、额外旧字段及无关表保持不变。
2. 验证错误索引／重复键等失败场景回滚，以及完整租户／订阅者／幂等键唯一性和旧 NULL 记录兼容性。
3. 在隔离媒体运行环境复现提交事务失败，确认媒体任务和历史均为零；迁移后沿同一幂等键提交与重复提交，
   均返回同一任务，只登记一条媒体任务和一条历史。此测试不启动生成消费。
4. 本地实际迁移前备份两张相关表，验证归档可读并恢复至临时隔离库，对照原表内容摘要。
   迁移后重新对照内容摘要及原失败请求标识。
5. 用实际库的正式媒体／历史仓储执行一次只回滚的登记探针，确认两表可在同一事务写入并共同回滚；
   不调用队列 Enqueue，不发供应商请求，不提交探针记录。
6. MediaXStudio 使用现有记录的“继续此尝试”时必须保留原幂等键和请求内容，收到 Core 确认后才能视为提交成功。
   本次修复不自动点击该按钮或重试真实生成。

原生测试需要显式提供隔离 PostgreSQL URL（数据库名必须以 `powerx_ai_media_test_` 开头）：
`POWERX_TASK_HISTORY_TEST_DSN` 用于限定迁移回归，`POWERX_AI_MEDIA_TEST_DSN` 用于媒体事务回归。
无该配置时测试会明确跳过，不能把跳过当成原生升级验收通过。

迁移失败会自动回滚本次增量。已提交迁移不自动删除新列／索引；若后续确需恢复备份，
必须先协调服务和任务消费者停止写入，保留迁移之后产生的数据，再决定恢复范围。

代码与测试：

- `backend/pkg/corex/db/database/migration.go`、`task_history_schema.go`
- `backend/cmd/database/migrate.go`、`upgrade_steps.go`、`task_history_scope_postgres_test.go`
- `backend/internal/bootstrap/app.go`
- `backend/internal/service/ai/media_legacy_task_history_postgres_test.go`
