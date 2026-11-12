# 通知 SDK/CRM 隔离合同服务

仅用于跨仓库自动验收，非业务服务、部署组件或消息模拟成功接口。它挂载真实Core鉴权、通知路由、service及PostgreSQL receipt，使用固定合成租户/成员/服务身份；支持 SDK和CRM的实际丢响应重试用例。源码不会被普通启动执行。

必须使用**新建且独立**的临时PostgreSQL数据库`crm_notification_lab`。工具先读取实际数据库名，非此名称拒绝后续操作。调用者负责临时集群、备份边界与清理；不得将日常数据库改名以通过检查。`prepare`创建合成fixture，不在日常seed/migrate/CI启动链中调用，也不用于历史数据修复。

从`backend/`执行：

```bash
CRM_TEMP_CORE_DSN='<fresh isolated crm_notification_lab DSN>' go run ./internal/testutil/notification_contract_fixture prepare
```

随后从Core根目录运行`make capability-seed`，必须以`CAPABILITY_SEED_CONFIG`明确指向上述临时数据库配置，并将`CAPABILITY_SEED_FLAGS`限制为`-capability-ids com.corex.notifications.create,com.corex.iam.organization.service_read,com.corex.iam.member_accounts.admin_write`。使用隔离配置的测试secret，不能读取日常配置或复制真实凭据。

回到`backend/`，以同一DSN执行`go run ./internal/testutil/notification_contract_fixture verify`，验证实际隔离API-key的通知/组织读取grant可用且Admin写能力没有授予。`CRM_TEMP_CORE_DSN`始终须显式提供。

```bash
CRM_TEMP_CORE_DSN='<same isolated DSN>' CRM_NOTIFICATION_FIXTURE_FILE='<temporary absolute fixture path>' go run ./internal/testutil/notification_contract_fixture serve
```

服务仅监听`127.0.0.1`随机端口，输出所属PID/端口，并以0600写入短期合成fixture。Framework用`CoreIdempotentContractTests`过滤，CRM用`Actual_core_postgres_and_sdk_lost_response_replay_matches_crm_durable_result`过滤；两者均显式传入`CRM_NOTIFICATION_FIXTURE_FILE`，拒绝非loopback/非合成契约。客户端无代理、无重定向；测试结束向所属fixture进程发送SIGTERM，确认端口释放，再停止独立数据库并删除临时目录/fixture文件。

普通`dotnet test`没有fixture时会明确跳过联合用例，不能当作实际Core联调通过。新验证应分别记录原生数据库、SDK实际HTTP、CRM实际重试和浏览器接收结果；本服务没有证明真实浏览器收到或完成生产启用。
