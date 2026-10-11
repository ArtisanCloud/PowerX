# 内容导览 Core 实施及验收记录

日期：2026-10-10。当前分支实施；提交、推送及合并由用户处理。

## 已交付

- [x] Tenant/Admin 读取路由、正式 typed `list_articles` binding、DTO/OpenAPI。
- [x] 独立 `content_overview.service_read` capability，Admin user/RBAC 与 API Key/STS service 双平面；实时 grant 与 published tenant registration。
- [x] 共用 retrieval 的 published corpus 过滤；真实向量及 source_chunk、成功任务原文快照、文档 UUID、原文版本与 codepoint 来源校验。
- [x] UUID 稳定分页、过滤后 total、签名游标、凭据/模式/参数绑定、15 分钟过期、generation/visibility 变更拒绝。
- [x] 概要/主题/推荐问题明确缺失；合法空目录与认证/授权/跨租户、版本冲突和依赖失败区分。
- [x] Service/HTTP/授权/permission materialization/seed 相关测试；PostgreSQL 隔离 schema 测试。
- [x] 真实 HTTP API Key、正式 gRPC STS Exchange、typed invocation 和撤销/失败证据。
- [x] `make capability-check` 与完整 `make capability-seed` 通过；开发 Host API Key 新 capability 的 `grant-status=granted`。
- [x] Core 合同及 Framework 对齐文档。
- [ ] 用户重启正式 8077，加载新读取路由。
- [x] 原 powerx 库真实 dev 五篇文章：API Key/STS、分页总量、冻结标题/版本、来源、typed binding 验收。
- [ ] Framework/Plugin typed 接入及浏览器验收，由消费端完成。

## 证据与边界

[脱敏证据](../../docs/contracts/evidence/knowledge-content-overview-20261010.json) 来自独立 8099/9199 Core 实例，使用真实开发 API Key 和正式 STS Exchange；未重启用户的 8077。dev 部分来自原 powerx 库，19 次 HTTP/Exchange 调用验证五篇真实文章及权限/版本拒绝，未修改这些文章、发布任务、原文、向量或 generation。

失败与空目录验收另用显式隔离 publication fixture：5 篇可见、1 篇 hidden，成功任务 source snapshot 与实际 pgvector 行；草稿标题/内容与发布版本不同，验证读取正确版本。固定二维向量仅验证读取，不宣称重新验收 Embedding/检索质量。fixture 文档、任务、chunk、binding、profile、向量表和临时凭据均清理。

覆盖 API Key/STS 200、只读授权、缺 grant/撤销/禁止写入 403、跨租户 404、无凭据 401、旧 generation/visibility 变化 409、游标/tenant override 400、来源损坏 503、合法空目录 200，保留 HTTP 和 Trace。

PostgreSQL 测试覆盖分页不重不漏、中文/emoji 范围、草稿不泄露、无向量/旧配置/hidden 排除、跨租户、游标/语料变化、来源完整性与空目录；临时 schema 自动删除。

## 可复验命令

```bash
cd backend
go test ./internal/service/knowledge_space ./internal/transport/http/knowledge_content ./internal/transport/http/openapi/knowledge_space ./internal/transport/http/admin/knowledge_space ./internal/service/integration_gateway/apikeypermissions ./internal/service/capability_registry ./internal/transport/http/openapi/capability_registry ./pkg/auth/middleware ./cmd/capability_gen ./cmd/database/seed
POWERX_CONTENT_POSTGRES_CONFIG=/absolute/private/config.yaml go test ./internal/service/knowledge_space -run '^TestContentOverviewPostgres' -count=1 -v
```

真实 HTTP 测试：设置 `POWERX_KNOWLEDGE_CONFIG`、`POWERX_KNOWLEDGE_HTTP`、`POWERX_KNOWLEDGE_GRPC`、`POWERX_KNOWLEDGE_EVIDENCE` 后运行 `go test ./tests/knowledge_space/hostsnapshot -run '^TestLiveContentOverview' -count=1 -v`。默认要求凭据租户中存在 `dev`；只有显式 `POWERX_CONTENT_ACCEPTANCE_FIXTURE=1` 才创建隔离 fixture，不恢复历史业务数据。测试为开发 Host key 幂等补齐新精确 read grant 及独立 self grant-status 权限，不改其他 grants 或 owner 绑定。

仓库根目录：`make capability-check`、`make capability-seed`。临时实例需显式 `POWERX_BACKEND_PORT=8099 POWERX_GRPC_PORT=9199`，避免 `backend/.env` 覆盖端口。多数据库验收应使用独立 Redis 实例/DB，避免固定开发 key 的缓存上下文混用。

当前配置含显式备份验证库 DSN，会覆盖 host/port/database 字段。最初在该库找不到 dev；随后用私有配置清空 DSN，验证原本地 powerx 库仍保留真实 dev。完整 capability seed 在两库均通过。本次不改用户配置；日常重启前应确认目标数据库，不能把配置字段中的 database: powerx 当作实际连接依据。

## 发布阻塞修复

完整 seed 曾被现有 replay validate 自动 capability ID 超过 `varchar(128)` 阻塞。生成器只对超长 ID 添加确定性 SHA256 后缀，合法 ID 不变；registration adapter ID 同样限长并保留唯一后缀。正式 catalog 由 `make capability-gen` 重生成，完整 seed/registration 发布通过，无需扩宽字段或迁移。

依据：[Host 合同](../../docs/contracts/knowledge-content-overview-host.md)、[Framework 对齐](../../docs/contracts/knowledge-content-overview-framework-alignment.md)、[OpenAPI](contracts/content-overview.openapi.yaml)。
