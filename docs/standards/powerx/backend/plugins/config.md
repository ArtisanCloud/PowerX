# 插件宿主配置说明

PowerX 在安装插件时会为其生成 `config/host-values.yaml`，用于告知插件运行时所需的宿主参数。本节介绍该文件的结构以及各字段的含义，重点说明数据库隔离策略。

> 权威契约：`deployment.env` 只参与插件 Role/User 命名。Schema/Database 沿用现有 `px_<plugin_slug>` 命名，不增加环境段。

## 文件结构概览

```yaml
generated_at: "2025-09-18T03:45:50Z"
deployment:
  env: prod
database:
  driver: postgres
  dsn: postgres://pxu_prod_com_powerx_plugins_base_a1b2c3d4:******@127.0.0.1:5432/powerx?sslmode=disable&search_path=px_com_powerx_plugins_base
  schema: px_com_powerx_plugins_base
  user: pxu_prod_com_powerx_plugins_base_a1b2c3d4
  password: ******
  search_path: px_com_powerx_plugins_base
  managed: true
runtime:
  run_migrate: true
host:
  web_admin_origins:
    - http://localhost:3030
    - http://127.0.0.1:3030
server:
  bind_addr: ":8091"
  log_level: debug
env:
  POWERX_PLUGIN_CONFIG_DIR: /var/.../config
  POWERX_DB_DSN: postgres://pxu_prod_com_powerx_plugins_base_a1b2c3d4:******@...
  POWERX_DEPLOYMENT_ENV: prod
  POWERX_DB_USERNAME: pxu_prod_com_powerx_plugins_base_a1b2c3d4
  POWERX_DB_PASSWORD: ******
  POWERX_PLUGIN_DB_SCHEMA: px_com_powerx_plugins_base
  ...
```

> **提示**：`env` 字段用于回写宿主注入的环境变量，插件代码可继续兼容原有的 `POWERX_*` 读取方式。

## 配置边界

`host-values.yaml` 由 PowerX 底座在插件安装、替换、启用自修复时生成。插件侧可以通过示例配置声明自己支持的私有字段，但 PowerX 不应猜测或写入插件私有字段，例如 `security.cors_origins`、`crawler.*`、`provider.*`。

PowerX 只写入平台标准 Host Contract 字段和宿主确定的运行参数。插件如需将标准字段映射到自身配置，应在插件配置加载器内完成。

## 数据库隔离策略

- Core 的 `config.yaml` 必须显式配置 `deployment.env`，且只允许 `dev/test/staging/prod`。该值是整套实例的部署身份，不属于 `plugin:` 或插件包。
- 安装插件时，宿主沿用 Schema / Database 名称 `px_<plugin_slug>`；依据 `deployment.env` 与插件 ID 生成专用 Role / User：`pxu_<env>_<plugin_slug>_<hash8>`。
- `hash8` 只用于 Role/User，取规范化前稳定插件 ID 的 SHA-256 十六进制摘要前 8 位；裁剪角色名称时必须保留环境段与摘要。
- 宿主在任何数据库 DDL 前校验 `deployment.env`。字段缺失、非法或与安装记录不一致时，安装/replace/restore/purge 必须失败，不得推导、降级或复用旧名称。对同一插件 Schema 的历史对象，安装使用 Core 数据库管理连接将 Schema、表、序列、视图和函数的 owner 收敛为当前目标 Role；管理账号无权转移时必须明确失败。
- 专用账号使用随机强密码，并仅授予对应 Schema 的 `CONNECT/USAGE/CREATE/SELECT/INSERT/UPDATE/DELETE` 权限；其余环境、其余插件和宿主 Schema 不授予访问能力。
- `database.dsn` 会替换为插件专用账号，PostgreSQL 下追加 `search_path=<schema>`，MySQL 下直接指向隔离出来的数据库，确保插件默认只读写自己的 Schema。
- `deployment.env`、实际 Schema/Database 和 Role/User 必须写入 host-values 与数据库绑定。绑定拥有 `binding_uuid`，通过 `plugin_uuid` 关联插件，并保留用于命名的稳定 `plugin_key`；密码不得写入日志或审计。
- 页面/API 的普通卸载与 `purge` 都只能停止插件、移除 Registry 记录和删除目标版本的物理安装目录；不得删除 Schema / Database、业务表、插件数据或 PostgreSQL Role。`plugin.allow_destructive_db_cleanup` 默认必须为 `false`，仅作为受控内部运维清理代码的额外拒绝开关，不向页面/API 暴露删除数据库对象的入口。任何数据库对象删除都必须使用独立、默认 dry-run、人工确认、备份验证和审批的运维工具。

`deployment.env` 不得从以下内容推导：

- `version`、`plugin.dev_mode`、`server.mode`；
- `POWERX_ENV`、安装目录、域名；
- 插件安装请求中的旧 `metadata.environment`；该字段必须返回弃用错误。发布渠道使用 `metadata.release_channel`，也不是实例身份。

## 关键字段说明

| 字段 | 说明 |
| ---- | ---- |
| `deployment.env` | 从宿主 Core 全局配置复制的部署身份；只允许 `dev/test/staging/prod`。 |
| `database.driver` | 使用的数据库驱动，当前支持 `postgres`、`mysql`。 |
| `database.dsn` | 插件使用的实际连接串，内含专用账号与 Schema。 |
| `database.schema` | 插件对应的 Schema（PostgreSQL）或 Database（MySQL）。 |
| `database.user` / `password` | 隔离账号与随机密码，仅限宿主和插件自身使用。 |
| `database.search_path` | PostgreSQL 下的默认 `search_path`，确保 ORM 自动落在专用 Schema。 |
| `database.user_host` | MySQL 用户绑定的 Host（默认 `%`），用于后续回收账号。 |
| `database.managed` | 标记该段配置由宿主管理，卸载时触发自动清理。 |
| `runtime.run_migrate` | 首次启用时默认执行数据库迁移，可按需关闭。 |
| `host.web_admin_origins` | 宿主 Web Admin 来源白名单。插件可用它补齐自身 CORS 配置，但 PowerX 不直接写插件私有 CORS 字段。 |
| `env.*` | 注入给插件进程的环境变量，其中 `POWERX_DEPLOYMENT_ENV` 与 `POWERX_DB_*` 来自同一宿主合同。 |

## 部署身份变更与旧安装

`deployment.env` 在首次插件安装后视为不可变。人工修改配置不会自动重命名、复制或重新授权数据库对象。

旧安装记录缺少 `deployment_env`，或仍使用不带环境段的 `pxu_*` Role 时，重新安装会保留原 Schema 名称并创建/确认当前 `pxu_<env>_<slug>_<hash8>` Role；随后由 Core 数据库管理连接将该 Schema 内对象的 owner 转给目标 Role，并撤销历史插件 Role 对该 Schema 的显式权限。旧 Role 不会被自动删除。若管理账号不具备 owner-transfer 权限，安装明确失败并提示修复数据库管理权限；不得以旧 Role 作为兼容回退。

所有权收敛只作用于目标插件 Schema，且旧 owner 必须是 Core 数据库管理账号或该插件精确的历史 Role `pxu_<plugin_slug>`；任意其他 owner 都会被拒绝。Schema、表、独立 Sequence、视图、物化视图和函数的转移与旧插件 ACL 撤销在同一事务中执行，失败整体回滚；serial/identity 从属 Sequence 由其所属表的 owner 转移连带处理。

已安装实例变更 `deployment.env` 仍不是普通重装场景，必须使用独立 repair/migration 工具，执行前要求 dry-run、人工确认、备份、权限验证、Registry 更新与旧对象清理审批。

## Web Admin Origin 规则

`host.web_admin_origins` 表示浏览器访问 PowerX Web Admin 的公开 Origin，用于插件宿主模式下的 CORS/Origin 校验。它不是插件后端监听端口，也不是插件前端静态资源目录。

PowerX 生成该字段时会读取：

1. `http_security.web_admin_origins` 中配置的公开 Web Admin Origin。
2. `http_security.frame_ancestors` 中配置的明确 Origin（仅作为兼容同源安全配置的补充来源）。
3. setup/install 保存的 `web_admin_port`，生成本机开发来源，例如 `http://localhost:3030`、`http://127.0.0.1:3030`。
4. 环境变量 `POWERX_WEB_ADMIN_ORIGINS` 中的补充 Origin，多个值用英文逗号分隔。
5. 环境变量 `POWERX_PLUGIN_CORS_ORIGINS` 中的补充 Origin，多个值用英文逗号分隔。

生产环境应显式配置公网管理后台 Origin，例如 `https://admin.example.com`。只配置端口只能覆盖本机 Origin，不能推导反向代理后的域名、协议或外部端口。

推荐生产配置：

```yaml
web_admin_port: 3000

http_security:
  web_admin_origins:
    - https://admin.example.com
```

## 迁移执行流程

当插件在 `plugin.yaml` 中声明 `migrations` 块时，宿主会在安装阶段自动执行指定的迁移入口：

```yaml
migrations:
  driver: go
  entry: ./backend/bin/migrate
  args: ["--with-sample-data"]
  workdir: ./backend
  once: true
  timeout: 60s
```

- `entry`：必须指向插件包内可执行文件或脚本（相对路径会自动拼接插件根目录），宿主以子进程方式调用。
- `args`：附加的命令行参数，原样传递给迁移程序。
- `workdir`：可选工作目录，未配置时默认使用插件根目录。
- `once`：设为 `true` 时仅在首次安装执行（后续升级可按需扩展）。
- `timeout`：可选的超时时间，语法与 `time.ParseDuration` 一致，超时会终止迁移并视为失败。

宿主为迁移子进程注入与运行态一致的环境变量，包括数据库 DSN（`POWERX_DB_*`）、Redis 连接、`POWERX_PLUGIN_CONFIG_DIR` 以及 `POWERX_PLUGIN_ROOT`、`POWERX_PLUGIN_VERSION` 等基础信息。迁移成功会在插件注册表中记录 `migrations` 元数据（入口、执行时间、哈希值、结果），便于后续判断是否需要重跑；失败则中止安装并返回 `migration_failed` 错误码。

如需跳过自动迁移，可在生成的 `config/host-values.yaml` 中将 `runtime.run_migrate` 调整为 `false`，宿主在安装阶段会检测该配置并跳过执行，保留迁移记录为 `skipped` 状态。

## 最佳实践

1. 插件必须按声明的 Host Contract 读取结构化 `database.*` 或同版本明确声明的 `POWERX_DB_*` 映射；缺少必填字段时直接失败，不得自行拼接 DSN、Schema 或环境标识。
2. 迁移脚本应假定仅能访问自己的 Schema，避免直接引用宿主表；如需跨 Schema 数据，请通过宿主提供的 API 访问。
3. 插件卸载或版本切换后，如需保留数据，请在卸载前自行导出；宿主默认会回收 Schema 与账号。
4. 若插件需要额外的外部资源（Redis、消息总线等），可在 Manifest `runtime.env` 中声明，宿主会在 `env` 段中补齐。
5. 插件有私有配置结构时，应显式把 `host.*` 标准字段映射到私有字段。例如插件内部使用 `security.cors_origins` 时，应读取 `host.web_admin_origins` 并合并到自身 CORS 白名单。

通过以上机制，即使插件被攻破，也只能访问自己受限的 Schema，从数据库层面最大限度降低越权风险。
