# Admin Skills HTTP Transport

本目录实现管理端 Skills 路由与 Handler，包括：
- catalog/list/register/import
- publish/rollback
- bind capability
- audit 查询
- install tasks（第三方 skill 安装任务）

约束：
- 管理接口仅允许 admin root 角色。
- Handler 只做参数绑定与响应封装，业务逻辑下沉到 service。
- `POST /admin/skills/import` 支持 `import_type=upload|marketplace`；`marketplace` 会尝试从 `source_url` 在线解析 `SKILL.md`。
- `POST /admin/skills/install-tasks` 支持 `provider + repo/repo_url + path` 触发异步安装；GitHub 优先复用 skill-installer 脚本，其他 provider 走通用 git 安装流程。安装完成后可选自动导入到 registry，使用 `GET /admin/skills/install-tasks` 与 `GET /admin/skills/install-tasks/:taskId` 查询进度。
