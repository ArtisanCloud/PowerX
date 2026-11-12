# 成员表增列导致权限基线提交失败

## 触发与原因

已有租户启用了权限基线，迁移前同一 PostgreSQL 物理连接已读取成员事实，随后迁移给 `iam_member` 增列。权限基线在源事务提交前再次读取相同 SQL，旧实现的 `SELECT *` 与缓存的结果列数不一致，产生 `cached plan must not change result type (SQLSTATE 0A000)`，外层表现为 `IAM_BASELINE_RECONCILE_FAILED`。

这是共享授权事实查询的升级问题，不是插件包错误、Root 权限不足或磁盘已满。仅验证空库或没有预热查询的连接，不能覆盖这个场景。

## 修复边界

`LoadPermissionFacts` 的成员查询固定读取授权判断实际依赖的 `id / uuid / tenant_uuid / user_id / user_uuid / status / deleted_at`。组织关系等新增字段不参与该查询的结果结构；组织能力继续独立读取其需要的正式字段。租户条件、账号和成员有效性、Root 目录、撤权、事务锁及账本提交规则保持原有行为，不关闭准备语句或权限基线，不捕获错误后强行提交。

临时 PostgreSQL 使用单一物理连接，先预热完整授权协调查询，再增列，验证后续成员停用提交与 Denied 历史同时成功，恢复成员后重新 Allowed。修复前该用例实际产生同一 `0A000`；修复后通过，并复验并发授权、源事务保护及 Core 全量迁移／重复迁移。验证记录见[本轮回执](evidence/migration-member-column-cache-20261112.json)。

## 已失败实例的处理

迁移不等于整个数据库的单一事务：失败前的部分 DDL 可能已经提交。例如开发实例的安装任务表及直属上级字段已创建，而后面的通知凭据表仍缺失。不能将命令失败理解为全部回滚，也不能看到某张表存在就视为完整成功。

1. 更新到包含本修复的 Core 源码；保留原目标配置、数据库及授权历史，不执行 refresh、重置或全量 seed。
2. 从服务器源码仓库执行 `make migrate CONFIG=/etc/powerx-dev/config.yaml`。该入口编译当前迁移源码，不依赖仍在运行的旧 Core 二进制。失败时保留错误，不能继续重启或上传插件。
3. 完整成功后，按本批升级说明完成限定 capability 同步，并用同一补丁重新构建／切换 Core 后端。管理前端无本次代码变更，日常服务仍由维护者重启。
4. 只读核对 `plugin_installations`、`actor_uuid`、`failure_details`、`uk_plugin_install_request`、`event_task_histories`，以及本批通知凭据表和直属上级字段。Core 重启后核对安装 worker 就绪，再进行原计划中的插件安装。

本轮远程操作仅为读取结构和运行版本。迁移及服务重启由维护者执行；没有修复 SQL、删除业务记录、补宽权限或接管正式站。
