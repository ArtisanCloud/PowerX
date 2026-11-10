# 存储治理结构就绪与调用准入

日期：2026-11-08。

## 错误合同

存储治理检查失败发生在能力业务调用及调用审计写入前。拒绝仍然是 HTTP 503 / gRPC Unavailable，但按真实原因分类：

| 机器码 | 条件 | 处理 |
| --- | --- | --- |
| `STORAGE_CONTROL_SCHEMA_NOT_READY` | 治理表、关键字段或精确唯一索引缺失／不合法 | Core 管理员完成迁移并核对结构 |
| `STORAGE_GOVERNANCE_UNAVAILABLE` | 数据库读取、主机预算状态、治理配置或正式 TaskBus 装配不可用 | 检查依赖；按 `cause_code` 进入原有确认／恢复流程 |
| `STORAGE_CAPACITY_EXHAUSTED` | 已读取的数据库容量或主机预算确实达到阈值 | 查看存储治理容量状态并处理容量 |

HTTP 继续使用现有 invocation envelope，返回 `error_code`、`reason_code`、`details.trace_id`。gRPC 保留兼容的机器码文本，并增加 `google.rpc.ErrorInfo`，其 `reason` 为上述机器码，`metadata.trace_id` 为追踪标识。只允许必要的 `STORAGE_CONTROL_*` / `DATABASE_RETENTION_*` 恢复机器码进入 `cause_code`；SQL、连接信息、底层错误原文和任意 details 不向插件透传。

`AuditService` 不再把所有失败改为容量耗尽。无法分类的准入依赖错误按暂不可用处理。容量兼容 sentinel 只用于已确认的容量拒绝。未配置治理的原有调用行为不变；不增加 GET、管理员或插件旁路。

## 结构检查与迁移

`database.CheckStorageGovernanceSchema` 只读检查集中迁移登记的六张元数据表，核对两个控制表的 `applied_revision`、`request_digest` 和三个唯一索引。索引必须属于当前 schema 的目标表，具有正确键顺序、唯一性、有效／就绪状态；同名错误索引、部分索引和表达式索引不作为就绪证明。数据库检查失败与结构缺失分别返回类型化错误。

运行时控制仓储复用其控制表检查，保留现有职责范围。Root 读取配置能看到明确的未就绪原因；队列缺失返回 worker 错误，不伪装成缺表。结构检查不自动建表、采纳旧策略或开启清理。

启用治理的 Core 启动时执行限时只读预检，日志给出明确迁移提示。健康、登录、正式升级和 Root 诊断入口保持可用，依赖治理的调用仍受准入保护。HTTP 存活健康成功不代表治理已就绪。

迁移入口仍集中在 `pkg/corex/db/database/migration.go`，`MigrateStorageGovernanceModels` 在 AutoMigrate 后执行结构核验。`make migrate` 的完整迁移和正式升级的 core 步骤均包含这六张表；正式升级 Verify 同样核对结构。升级载荷通过 `deploy/powerx/container/upgrade_registry.py` 生成新的内容校验和，不复用旧载荷的成功记录。

源码部署在仓库根目录使用一行命令：`make migrate CONFIG=/实际路径/config.yaml`。已安装正式镜像按现有升级 plan/apply 流程执行，成功后再验收新 Core 和插件；仅复制程序或重启不会执行迁移。

本地 dev/test 的限定修复命令，在 backend 目录执行：`go run ./cmd/database migrate -config etc/config.yaml -scope storage-governance`。该入口沿用迁移锁、事务和锁等待上限；失败回滚后处理原因，不继续 seed、refresh 或重置。普通请求不能触发迁移。

## 迁移后验收

1. 用 Core 的有效配置确认当前数据库和 schema，复核六张表、两个字段和三个索引。
2. 在存储治理查看旧配置、恢复确认、期望／生效版本及执行开关；建表不代表已接管旧策略。按原有显式确认流程处理，不能删除快照或重置策略绕过。
3. 重新编译并由服务所有者按原方式重启，加载错误分类与启动预检；若治理消费者此前已因缺结构退出，迁移成功本身不会复活退出的消费者。
4. 使用原插件身份复验列表及实时连接，区分结构未就绪、暂不可用与容量问题。业务写入、生成或其他有副作用的请求不能因页面刷新而自动重试。

本轮本机实际迁移和隔离验证分别记录在[验证收据](evidence/storage-admission-readiness-validation-20261108.json)。迁移并未授权服务器发布、策略接管或数据清理；源码验证也不等于新构建已加载到日常进程。
