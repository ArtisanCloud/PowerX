# Tenant UUID Migration Scripts

本目录存放“终态清理（T8）”阶段所需的数据库脚本，目标是让所有业务表仅保留 `tenant_uuid` 字段，并安全删除历史的 `tenant_id` 列。

## 结构

- `001_add_tenant_uuid_columns.sql`：扫描所有包含 `tenant_id` 列的业务表，自动新增 `tenant_uuid`（若不存在）。
- `002_backfill_tenant_uuid.sql`：基于 `tenants` 主表（`id` ↔ `uuid`）批量回填 `tenant_uuid`，并输出缺失报告。
- `999_drop_tenant_id_columns.sql`：在确认全部表完成回填后，统一删除 `tenant_id` 列及相关索引/约束。

> **注意**：脚本默认 schema 为 `public`，若有自定义 schema，请在执行前通过 `INCLUDE_SCHEMAS`/`EXCLUDE_SCHEMAS` 环境变量覆写（参见脚本头部变量）。

## 依赖

- PostgreSQL 14+，支持并行 DDL 与 `IF NOT EXISTS`/`DROP COLUMN` 语法。
- 表结构要求：所有业务表（agent/iam/workflow/...）均以 `tenant_id` 外键引用 `public.iam_tenant(id)`。
- 执行工具：`psql` 或 Liquibase/Flyway 等迁移工具均可，建议直接 `psql -v ON_ERROR_STOP=1 -f <script>`。

## 推荐执行顺序

1. **快照与校验**
   - 使用 `scripts/ops/tenant-id-cleanup.sh plan` 生成受影响表清单并触发 `pg_dump --schema-only` 备份。
   - 运行 `scripts/ops/checks/tenant_uuid_consistency.sql`（或 `make check-tenant-migrations`）确保当前库状态可迁移。
2. **新增列**：执行 `001_add_tenant_uuid_columns.sql`。
3. **数据回填**：执行 `002_backfill_tenant_uuid.sql`，回填完成后再次运行 `scripts/ops/checks/tenant_uuid_consistency.sql`。
4. **删除旧列**：当应用与脚本均确认不再依赖 `tenant_id` 后，执行 `999_drop_tenant_id_columns.sql`。
5. **记录**：把每次执行日志、耗时与异常记录到 `reports/tenant-uuid-ga-weekly.md`。

### 自动化脚本

若需要一键执行上述 SQL 并带额外控制项（dry-run、指定租户、留存日志），可使用 `scripts/migrations/run-tenant-uuid.sh`：

```bash
# 全量回填 + 一致性检查
scripts/migrations/run-tenant-uuid.sh

# Dry-run + 仅回填两个租户，日志写到自定义路径
scripts/migrations/run-tenant-uuid.sh \
  --dry-run \
  --tenants 97b2a1c2-67d0-4f4d-9c71-e8f316271111,1f6932d6-55c0-4a03-9071-8fb32af39ccc \
  --log /tmp/tenant-run-dev.log
```

参数说明：

| 选项 | 作用 |
| --- | --- |
| `--tenants all|uuid1,uuid2` | 只处理指定租户，默认全量。脚本会把列表传入 `002_backfill_tenant_uuid.sql` 的 `tenant_uuids` 变量。 |
| `--dry-run` | 在事务内执行并自动 `ROLLBACK`，便于 staging 验证。 |
| `--with-drop` | 在回填后顺带执行 `999_drop_tenant_id_columns.sql`。 |
| `--skip-check` | 跳过 `scripts/ops/checks/tenant_uuid_consistency.sql`。 |
| `--log <path>` | 自定义日志路径，默认 `tmp/reports/tenant-run-<timestamp>.log` 并自动 `tee` 输出。 |

脚本会自动从 `backend/etc/config.yaml` 加载数据库连接（若未设置 `DATABASE_URL`/`PG*`），并将最后一次执行记录写入日志。

### 回滚脚本 / 演练

- **数据备份**：`scripts/ops/tenant-id-cleanup.sh run` 默认执行 `pg_dump --schema-only`，输出 `tmp/tenant-uuid-schema-<ts>.sql`。
- **快速回滚**：
  - 使用 `scripts/ops/tenant-id-cleanup.sh rollback <backup.sql>`，即可恢复执行前的 schema 状态。
  - 或手动运行 `psql -f <backup.sql>`。
- **建议**：在 staging 先执行一次 `run --drop`，随后用生成的备份文件执行 `rollback`，确认 `tenant_uuid_consistency` 再次通过并记录日志，以满足 T8.22 “回滚脚本” 验收。
- **更高级的 down 迁移**：若需要自动化 drop→add 循环，可在 `999_drop_tenant_id_columns.sql` 旁维护 `999_drop_tenant_id_columns.down.sql`（自定义），但推荐直接使用 `pg_dump` 输出的 schema 备份以降低复杂度。

### CI / 本地自动校验

- **命令**：`make check-tenant-migrations`
- **脚本**：`scripts/ci/check-tenant-migrations.sh`
- **检查内容**：
  1. 若安装了 `sqlfluff`，对 `scripts/migrations/tenant-uuid/*.sql` 执行 lint，保证语法/格式一致。
  2. 检测到 `psql` 且配置了 `DATABASE_URL`/`PGHOST` 时，会对 `scripts/ops/checks/tenant_uuid_consistency.sql` 做一次 `BEGIN ... ROLLBACK` dry-run，确保 SQL 可执行。
  3. 若在 CI 中希望强制所有步骤执行，可设置 `STRICT=true`；缺少依赖时脚本将报错。

此命令将作为 GitHub Actions / CI 的标准步骤，确保每个 PR 都对迁移脚本做最小可用校验。

## 受影响表清单（来自 T4.1）

| 领域 | 示例表 | 说明 |
| --- | --- | --- |
| Agent Core | `agent_agents`, `agent_settings`, `agent_chat_sessions`, `agent_shares` | 行为、会话、分享等记录都需要 `tenant_uuid` 唯一标识 |
| IAM & Auth | `iam_members`, `iam_roles`, `iam_departments`, `iam_refresh_tokens` | 账号体系后续查询仅允许 UUID |
| Workflow | `workflow_definitions`, `workflow_instances`, `workflow_tasks` | HTTP/gRPC 已改为 UUID，存储层必须同步 |
| Integration & Event Fabric | `integration_gateway_*`, `event_fabric_*` | 网关策略、事件队列等全部切到 UUID |
| Knowledge Space | `knowledge_spaces`, `knowledge_deltas`, `qa_bridge_runs` | QA bridge/Provisioning 已完成业务改造 |
| Media / Plugin / Capability / Dev Hotload / Provider Registry / System Settings 等 | 参见 `backend/pkg/corex/db/persistence/model/**` | 所有仍含 `tenant_id` 列的表均纳入脚本扫描 |

若某表确实仍需 `tenant_id`（例如历史归档），请在执行脚本前将其 schema 名加入 `EXCLUDE_TABLES` 变量，或在执行后手动添加。

## 常见问题

- **tenants 表结构不同怎么办？** 脚本默认 `tenants(id bigint, uuid uuid)`, 如不一致，可在执行前设置 `TENANT_TABLE`, `TENANT_ID_COLUMN`, `TENANT_UUID_COLUMN` 变量。
- **如何只处理部分 schema？** 执行命令前通过 `psql "..." -v include_schemas="public,workflow" ...` 传入变量即可。
- **如何回滚？** 所有脚本都可以通过 `BEGIN ... ROLLBACK` 的方式 dry-run；如需彻底回滚，执行 `999_drop_tenant_id_columns.sql` 的 inverse（见 `scripts/ops/tenant-id-cleanup.sh rollback` 说明）。

更多操作细节参见 `docs/projects/tenant-uuid-ga.md`。
