# Skills 使用指南（Agent）

本目录提供 Skills 功能的分场景操作手册，覆盖管理、导入、调用一致性、审计与隔离。

## 文档导航

1. [01-admin-lifecycle.md](./01-admin-lifecycle.md)  
   管理员如何完成 Skills 生命周期管理（登记、发布、回滚、绑定）。

2. [02-import-third-party-skill.md](./02-import-third-party-skill.md)  
   开发者/第三方如何受控导入 Skill（仅 upload 模式）。

3. [03-tenant-invoke-consistency.md](./03-tenant-invoke-consistency.md)  
   如何验证 `tenant/skills/invoke` 与 `tenant/invocations`（`preferred_protocol=skill`）语义一致。

4. [04-audit-and-isolation.md](./04-audit-and-isolation.md)  
   如何查询 Skills 审计、执行 trace，并验证跨租户隔离。

## 统一前置条件

1. 服务已启动，且可访问 API（示例：`http://localhost:8080/api/v1`）。
2. 已有可用 JWT：
   - 管理端接口：需要 `admin root` 用户。
   - 租户端接口：需要带租户上下文的 token。
3. 建议准备环境变量：
   - `POWERX_HTTP_BASE=http://localhost:8080/api/v1`
   - `ROOT_TOKEN=<root_jwt>`
   - `TENANT_TOKEN=<tenant_jwt>`
