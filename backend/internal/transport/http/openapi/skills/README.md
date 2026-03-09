# Tenant Skills HTTP Transport

本目录实现租户侧 Skills 调用入口：
- `POST /tenant/skills/invoke`
- 统一入口 `POST /tenant/invocations` 的 skill 适配

约束：
- 必须从请求上下文解析 `tenant_uuid`。
- 权限不足返回统一错误码，不泄露跨租户信息。
