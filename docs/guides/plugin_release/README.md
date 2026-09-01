# Plugin Release 指南索引

本目录是插件打包、发布、安装、审核和运行准入的对外执行文档入口。`docs/plan/**` 只描述内部方案和目标架构；插件开发者、发布审核员和运维应优先阅读本目录。

| 文档 | 适用对象 | 用途 |
|---|---|---|
| `application_runbook.md` | 插件开发者、发布经理、运维 | 从本地热更新、提交候选、灰度发布到离线包导入的端到端操作手册。 |
| `permission_declaration.md` | 插件开发者、发布审核员、租户管理员 | 插件包如何声明 `menu/page/action/api` 权限，以及安装后如何在 PowerX 统一授权和排障。 |
| `security_baseline.md` | 安全审核、平台运维 | 插件发布 API、权限声明、审计和发布准入的安全检查清单。 |
| `observability.md` | 运维、SRE | 插件发布链路的指标、告警和 Grafana 配置。 |
| `config_migration.md` | 平台运维 | 插件发布相关配置迁移说明。 |

推荐阅读顺序：

1. 插件开发者先读 `permission_declaration.md`，补齐插件包权限声明。
2. 发布前按 `application_runbook.md` 提交候选和离线包。
3. 审核和上线前按 `security_baseline.md` 做准入检查。
4. 部署后按 `observability.md` 验证指标和告警。
