# 插件应用市场与多租户使用机制（功能需求）

本文定义 PowerX Admin 的“插件应用市场”与“已安装插件管理”的功能、角色权限、数据与交互流程，覆盖系统级安装/运行与租户级启用/凭证管理的完整闭环。适用于当前本地模拟市场，后续可无缝切换为远端市场源。

---

**核心术语**
- 市场（Marketplace）：展示“可安装的插件目录”。当前为本地模拟，未来支持远端索引（index_url）。
- 系统级（平台维度）：安装/卸载、切换版本、进程启停、反向代理挂载等，全局唯一，与租户无关。
- 租户级（Tenant 维度）：某租户对某插件的启用态与凭证（client_id/client_secret），独立隔离。
- 凭证（Credentials）：落库于 `plugin_instance_configs(tenant_id, plugin_id, key="auth.credentials")`，仅存哈希；明文 secret 仅在首次生成或轮换时返回一次。
- STS（短期令牌）：插件使用 `client_id/secret` 向宿主交换短期 JWT，用于访问宿主 API。

---

**角色与权限（简化）**
- 平台管理员（root/System Admin）
  - 允许：查看市场；系统级安装/卸载；切换版本；全局启用/停用进程；查看运行状态/日志。
  - 可在其“默认租户”下，像租户管理员一样进行租户级启用/停用与凭证管理（注意避免 `tenant_id=0` 场景）。
- 租户管理员（Tenant Admin）
  - 允许：查看市场与“已安装插件”清单；管理“本租户”启用/停用；查看 `client_id`；轮换密钥；删除本租户配置。
  - 不允许：系统级安装/卸载/切换版本/全局启停/查看其他租户配置。

---

**页面信息架构**
- 应用市场（Marketplace）页
  - 数据：GET `/api/marketplace/plugins`（当前：本地模拟清单；未来：从远端 index 拉取，失败回退本地）。
  - 列表项字段：`id/name/description/version/author/category/icon/tags/installed?`。
  - 操作：
    - 平台管理员：对“未安装”项显示“安装（系统级）”；对“已安装”项显示“已安装”并可跳转到“已安装管理”。
    - 租户管理员：仅浏览，无安装按钮。
- 已安装（Installed）页
  - 系统级信息：`version/state`、运行态（`pid/port/state/healthy/restarts/health_ok/health_fails`）。
  - 租户级信息（按当前租户）：`exists/enabled/client_id`。
  - 操作：
    - 平台管理员：系统级“启用/停用/重启/切换版本/卸载/查看日志”。
    - 租户管理员：本租户“启用/停用/查看 client_id/轮换密钥/删除本租户配置”。

---

**后端接口（当前实现）**
- 市场
  - GET `/api/marketplace/plugins` → 市场清单（本地模拟；后续可对接远端）。
- 系统级（平台管理员）
  - GET  `/api/admin/plugins/` → 已安装列表
  - GET  `/api/admin/plugins/:id/status` → 运行状态
  - GET  `/api/admin/plugins/:id/logs` → 运行日志
  - POST `/api/admin/plugins/install/local` → 从本地目录安装
  - POST `/api/admin/plugins/install/url` → 从 URL 安装（支持 sha256）
  - POST `/api/admin/plugins/:id/switch_version` → 切换版本（可选启用）
  - POST `/api/admin/plugins/:id/enable` → 启动进程并挂载反代（全局）
  - POST `/api/admin/plugins/:id/disable` → 停止进程并卸载反代（全局）
  - POST `/api/admin/plugins/:id/restart` → 重启进程（全局）
  - POST `/api/admin/plugins/:id/uninstall` → 卸载（可选 `purge` 删除磁盘产物）
- 租户级（当前租户）
  - GET    `/api/admin/plugins/:id/tenant_config` → 查询本租户配置（exists/enabled/client_id）
  - POST   `/api/admin/plugins/:id/tenant_enable` → 启用/停用本租户；首次启用返回一次性 `client_secret`
  - GET    `/api/admin/plugins/:id/credentials` → 查看凭证（只读：client_id）
  - POST   `/api/admin/plugins/:id/credentials/rotate` → 轮换并返回新 `client_secret`（仅此一次）
  - DELETE `/api/admin/plugins/:id/tenant_config` → 删除本租户配置（硬删）

---

**状态与数据模型**
- 系统级安装状态（Registry）：`installed versions`、`current version`、`state=enabled/disabled`。
- 运行态（Supervisor）：`starting/running/unhealthy/stopped/exited`、`pid/port`、健康计数。
- 租户级配置（DB）：表 `plugin_instance_configs`
  - 唯一键：`(tenant_id, plugin_id, key)`，其中 `key="auth.credentials"`
  - `value_json`（示例）：`{"client_id":"<pluginID>.<tenantID>", "client_secret_hash":"...", "secret_version":1, ...}`
  - `enabled`：租户侧启用开关。

---

**启用与凭证生命周期**
- 首次启用（租户级）：
  1) 调 `EnsureCredentials(tenantID, pluginID)` 生成 `client_id` 与一次性明文 `client_secret`；
  2) 仅存 hash 于 DB；明文 `client_secret` 仅本次返回给前端展示；
  3) 置 `enabled=true`。若记录已存在，则不返回明文 secret。
- 再次启用/停用（租户级）：仅切换 `enabled`，不改密钥。
- 轮换：`RotateSecret` 生成新 secret、替换 hash、`secret_version++`，立即使旧 secret 失效，并一次性返回新明文给前端。
- 删除租户配置：删除该租户-插件的凭证记录；后续需再次“启用”以重新生成。

---

**插件端使用 STS（摘要）**
- 插件保存：`client_id` 与 `client_secret`（安全存储：ENV/密钥管理器/私有配置文件）。
- 令牌交换：调用 gRPC STS `Exchange`，传 `client_id/client_secret`，换取短期 JWT（默认 60–300s）。
- 调用宿主：把 `Authorization: Bearer <token>` 加到 gRPC/HTTP 请求头；剩余寿命 <60s 预刷新；401/403 触发强刷。

---

**前端交互规则**
- 应用市场页：
  - 平台管理员：对未安装项显示“安装”，对已安装项显示“已安装/管理”。
  - 租户管理员：仅浏览，无安装按钮。
- 已安装页：
  - 行内并发请求：系统级状态 + 本租户配置。
  - 本租户“启用”成功时，若 `just_issued=true`，弹窗展示一次性 `client_secret`，提示妥善保管；关闭后不再可见。
  - 高危操作需确认：
    - 轮换密钥：提示“旧 secret 立即失效，未更新的插件调用将失败”。
    - 删除本租户配置：提示“删除后本租户将无法访问该插件，需重新启用生成新密钥”。

---

**安全与审计（建议）**
- 不在日志/前端持久存储明文 `client_secret`；仅首次与轮换时短暂展示。
- STS TTL 建议 60–300 秒；插件校验 `audience/issuer`。
- 宿主侧审计关键事件：安装/卸载/启停/切版本/STS 交换/轮换/删除配置，记录 `tenant/plugin/subject/trace_id`。

---

**远端市场对接（后续规划）**
- 配置：`plugin.market.index_url`、`timeout_sec`、`cache_ttl_sec`（可多源合并）。
- 行为：优先拉取远端市场 JSON（失败回退本地），在返回项中合并“已安装标记”（对照 Registry）。
- 安装动作：调用现有 `POST /api/admin/plugins/install/url`（传 `url/sha256`）。

---

**Marketplace 接口（与前端 Plugin 类型对齐）**
- 建议提供 v2 形态：GET `/api/marketplace/plugins` 返回数组，字段尽量贴合前端 `Plugin` 类型；暂无远端源时可以“fork 本地数据 + 默认值”。
- 字段映射（当前实现能力 → 前端字段）：
  - `id` ← manifest.id；`name` ← manifest.name；`version` ← manifest.version
  - `slug` ← 由 id 衍生（示例：`strings.ReplaceAll(id, ".", "-")`）
  - `description` ← manifest.description
  - `author` ← metadata.author；`authorUrl` ← metadata.homepage（或未来扩展 metadata.author_url）
  - `homepage` ← metadata.homepage；`repository` ←（未来扩展 metadata.repository，暂空）
  - `license` ← metadata.license
  - `icon` ← 若已安装：复用 admin 静态资源解析（`/_p/<id>/admin/icon.svg|png`）；否则使用远端清单的绝对 URL；都缺省则空
  - `screenshots` ← 远端清单提供；本地无，默认为空数组
  - `category/tags` ← metadata.category/tags
  - 系统级状态：
    - `systemStatus` ∈ {`not_installed`,`installed`,`enabled`,`disabled`}：
      - 未安装：`not_installed`
      - 已安装且 state=enabled：`enabled`
      - 已安装且 state=installed：`installed`
      - 已安装且 state=disabled：`disabled`
    - `isSystemInstalled` ← 是否出现在 Registry；`isSystemEnabled` ← state==enabled
    - `installPath` ← paths.root（仅已安装时）
  - `configSchema/config/dependencies/requirements` ← 暂不提供，置空或 `{}`
  - `downloadUrl` ← 远端清单提供；本地无
  - `downloadCount/rating/reviewCount` ← 远端清单提供；本地默认 `0`
  - `lastUpdated/createdAt/updatedAt` ← 远端清单提供；本地默认 `""`
- 返回示例（v2）：
  ```json
  {
    "id": "com.powerx.demo.hello_world",
    "name": "Hello World",
    "slug": "com-powerx-demo-hello_world",
    "version": "0.1.1",
    "description": "Demo plugin",
    "author": "PowerX",
    "authorUrl": "https://powerx.dev",
    "homepage": "https://powerx.dev/plugins/hello",
    "repository": "",
    "license": "MIT",
    "icon": "https://cdn.example.com/icons/hello.svg",
    "screenshots": [],
    "category": "demo",
    "tags": ["demo","hello"],
    "systemStatus": "installed",
    "isSystemInstalled": true,
    "isSystemEnabled": false,
    "installPath": "/var/lib/powerx/plugins/installed/com.powerx.demo.hello_world/0.1.1",
    "configSchema": {},
    "config": {},
    "dependencies": [],
    "requirements": {},
    "downloadUrl": "https://market.example.com/com.powerx.demo.hello_world-0.1.1.zip",
    "downloadCount": 0,
    "rating": 0,
    "reviewCount": 0,
    "lastUpdated": "",
    "createdAt": "",
    "updatedAt": ""
  }
  ```
- 访问权限：读取接口向平台管理员与租户管理员开放；安装/卸载等系统级操作仅平台管理员可见与可操作。

---

**常见边界情况**
- 无租户上下文调用租户级接口：返回 400，避免产生 `tenant_id=0` 记录。
- 系统未安装：租户级按钮隐藏或禁用（进程未跑）。
- 升级/切换版本：不影响租户侧 `enabled` 与凭证；但进程重启期间需注意调用可用性。
- 卸载（系统级）：会导致所有租户无法访问该插件；与删除租户配置不是一回事。

---

**API 示例（便于联调）**
- 安装本地：`curl -X POST http://localhost:8077/api/admin/plugins/install/local -H 'Content-Type: application/json' -d '{"src_dir":"/ABS/PATH/to/dist/0.1.1","enable":false}'`
- 安装远端：`curl -X POST http://localhost:8077/api/admin/plugins/install/url -H 'Content-Type: application/json' -d '{"url":"https://.../plugin.zip","sha256":"...","enable":true}'`
- 启用进程（系统级）：`curl -X POST http://localhost:8077/api/admin/plugins/<id>/enable`
- 启用本租户：`curl -X POST http://localhost:8077/api/admin/plugins/<id>/tenant_enable -H 'Content-Type: application/json' -d '{"enabled":true}'`
- 查看凭证（只读）：`curl -X GET http://localhost:8077/api/admin/plugins/<id>/credentials`
- 轮换密钥：`curl -X POST http://localhost:8077/api/admin/plugins/<id>/credentials/rotate`
- 删除本租户配置：`curl -X DELETE http://localhost:8077/api/admin/plugins/<id>/tenant_config`
