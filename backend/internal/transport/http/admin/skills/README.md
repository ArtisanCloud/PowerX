# Admin Skills HTTP Transport

本目录实现管理端 Skills 路由与 Handler，包括：
- catalog/list/register/import
- publish/rollback
- bind capability
- audit 查询

约束：
- 管理接口仅允许 admin root 角色。
- Handler 只做参数绑定与响应封装，业务逻辑下沉到 service。
- `POST /admin/skills/import` 支持 `import_type=upload|marketplace`；`marketplace` 会尝试从 `source_url` 在线解析 `SKILL.md`。
