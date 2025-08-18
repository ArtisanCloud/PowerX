对的，**从 PluginManager 开始**是最稳的路径。但别一下子“全做齐”，而是分三小步把“扫描→挂载→启停”跑通；先支持**静态 Admin 页面 + 反代到固定 mock API**，再引入**子进程**与**安装器**。下面给你一个**最小闭环顺序**，每一步都有目标、产物、目录落点与验收标准。

---

# 里程碑 0：启动接线（Bootstrap Wiring）

**目标**：在宿主启动时创建并持有一个全局 PluginManager 实例，预留挂载点 `/_p/{pluginId}/**`。

**产物/落点**

* `config/plugin.go`：开关、目录、前缀（`/_p`）、超时等配置字段（仅 struct）
* `internal/infra/plugin/manager/manager.go`：`New()`、`Bootstrap()`、`Shutdown()`
* `internal/http/router.go`：在主路由上预留 `/_p/*` 的子路由组（先空实现）

**验收**

* 应用能启动；日志打印“Plugin subsystem disabled/enabled”；`/_p/` 404（未挂任何插件）

---

# 里程碑 1：Manifest & Loader（扫描与清单）

**目标**：从磁盘读取插件清单 `plugin.yaml`，得到内存中的插件模型；只做“读”，不做启停。

**产物/落点**

* `pkg/plugin/manifest.go`：对齐你前面方案的 MVP 字段（id/name/version/runtime/frontend/rbac/events/endpoints/migrations）
* `internal/infra/plugin/manager/loader.go`

    * `Scan(dir) ([]Manifest, error)`：扫描 `plugins/installed/**/plugin.yaml`
    * `Validate(manifest) error`：基础校验（id、version、路径存在）
* `internal/infra/plugin/manager/registry.go`

    * 内部注册表（内存版 map；后面再换持久化）：`Get(id)`, `List()`, `Put(m *Manifest, state *State)`

**验收**

* 启动时可打印读取到的插件列表（id、version、admin 菜单条目数）
* `GET /api/v1/admin/plugins` 返回内存里的插件元信息（只读）

---

# 里程碑 2：Admin 菜单聚合 & 前端静态挂载（先不跑后端进程）

**目标**：让宿主 Admin 能“看到插件菜单并能打开页面”，即使页面先是静态的。

**产物/落点**

* `api/http/admin/manifest_handler.go`：

    * `GET /api/v1/admin/manifest`：将宿主菜单 + 各插件 `frontend.admin.menus` 聚合（排序、RBAC 可先略）
* `internal/infra/plugin/manager/router.go`：

    * 静态资源挂载：若 `frontend.admin.kind=static` → 把 `frontend/admin` 挂到 `/_p/{id}/admin/**`
    * 统一路径：建议 `/_p/{id}/admin/*`；页面入口 `/plugins/{id}` → 重定向到上面路径
* （可选）宿主 Admin 的菜单读取逻辑（如果你已有，就只要保证清单格式一致）

**验收**

* 宿主 Admin 左侧出现“Hello World”菜单
* 点击进入，能加载 `/_p/hello/admin/index.html` 并显示页面

---

# 里程碑 3：Enable/Disable（内存状态 + 路由装卸）

**目标**：用启停把闭环做起来（不涉及子进程）。**Enable=装载路由**，**Disable=卸载路由**。

**产物/落点**

* `api/http/admin/plugin_handler.go`：

    * `POST /api/v1/admin/plugins/{id}/enable`
    * `POST /api/v1/admin/plugins/{id}/disable`
    * `GET /api/v1/admin/plugins`（带 state：installed/enabled/disabled）
* `internal/infra/plugin/manager/lifecycle.go`：

    * `Enable(id)`：校验→注册表 state=enabled→调用 router 挂载
    * `Disable(id)`：注册表 state=disabled→调用 router 卸载
* `internal/infra/plugin/manager/router.go`：

    * `MountAdminStatic(id, dir)` / `Unmount(id)`

**验收**

* 初始状态：installed/disabled
* `enable` 后菜单出现、页面可访问；`disable` 后菜单隐藏/访问 404

---

# 里程碑 4：反向代理固定后端（先不跑子进程）

**目标**：让插件页面能调用 “插件 API”。第一步可**固定指向宿主内的 mock handler** 或一个本地回环 URL，先把网关与路径协议打通。

**产物/落点**

* `internal/infra/plugin/manager/router.go`：

    * `MountAPIProxy(id, basePath)`：将 `/_p/{id}/api/**` 转发到某个后端（先指向宿主内的 mux group 或固定 http.Handler）
    * 未来再切换到“子进程端口/remote URL”
* `api/http/admin/plugin_handler.go`：

    * 在 `enable` 时，若 manifest 里声明 `endpoints.http_base_path`，一并挂载 `/_p/{id}/api/**`

**验收**

* `GET /_p/hello/api/v1/ping` 返回 `{"ok":true,"id":"hello"}`（无论当前是宿主 mock 还是回环 URL）
* Admin 页面按钮能成功请求

---

# 里程碑 5：Supervisor（子进程）+ Health（最小可用）

**目标**：把“固定代理”换成“真实插件子进程”；按 `plugin.yaml.runtime.entry` 启动，健康检查通过后挂载代理。

**产物/落点**

* `internal/infra/plugin/supervisor/process.go`：

    * `Start(id, entry, env...) (port|url, err)`：分配端口/读取环境变量/启动/日志重定向
    * `Stop(id)`、`Restart(id)`、重启策略（Always/OnFailure）
* `internal/infra/plugin/manager/health.go`：

    * 主动探测子进程 `/healthz`（manifest.health 配置）
* `internal/infra/plugin/manager/lifecycle.go`：

    * `Enable(id)`：若 `runtime.kind=process` → 先 `Start()`，探活通过后再挂载 API 代理
    * `Disable(id)`：先卸载代理，再 `Stop()`

**验收**

* 插件后端以子进程方式启动；`/_p/hello/api/v1/ping` 透传子进程响应
* 子进程异常退出 → Supervisor 自动拉起（OnFailure 策略）

---

# 里程碑 6：Installer（本地包/远程 URL）

**目标**：从 zip/tgz 安装一个插件版本到 `plugins/installed/<id>/<version>`，更新注册表为 `installed/disabled`。

**产物/落点**

* `internal/infra/plugin/installer/local_installer.go`：解压、校验、落盘
* `internal/infra/plugin/installer/remote_installer.go`：下载到 `plugins/market_cache` → 解压
* `api/http/admin/plugin_handler.go`：`POST /api/v1/admin/plugins/install`（输入：本地上传/远程 URL）
* `internal/infra/plugin/manager/migrations.go`：发现 `migrations/` 并执行（先支持 goose/sql 文件）

**验收**

* 上传/URL 安装后，`/api/v1/admin/plugins` 可见 `installed` 状态的插件，`enable` 即可使用

---

# 里程碑 7：RBAC & Admin Manifest 过滤、Event Bus 桥接（增强）

**目标**：把权限/事件纳入闭环，但这一步可以在跑通 Demo 后再做。

**产物/落点**

* `internal/infra/plugin/manager/rbac.go`：将 `manifest.rbac.resources` 注入宿主 RBAC 模型
* `api/http/admin/manifest_handler.go`：基于当前用户权限过滤菜单
* `internal/infra/plugin/manager/events.go`：`subscribe/publish` 与宿主 `pkg/event_bus` 对接（HTTP 回调/SDK）

**验收**

* 无权限的用户看不到插件菜单；事件能被转发/发布

---

## 你今天可以开动的最小 TODO（推荐顺序）

1. **Bootstrap + Manifest + Loader + Registry（只读）**
   文件：`config/plugin.go`、`pkg/plugin/manifest.go`、`internal/infra/plugin/manager/{manager,loader,registry}.go`

2. **Admin 菜单聚合 + 静态挂载**
   文件：`api/http/admin/manifest_handler.go`、`internal/infra/plugin/manager/router.go`（静态）
   → 能在 Admin 里看见并打开插件页面

3. **Enable/Disable（状态 & 路由装卸）**
   文件：`api/http/admin/plugin_handler.go`、`internal/infra/plugin/manager/lifecycle.go`

> 到此，你就有了“**可安装（手放目录）、可启停、可展示页面**”的 MVP。不涉及子进程，也能演示一条闭环。

4. **API 反代（先 Mock）**
   文件：`internal/infra/plugin/manager/router.go`（加 `/_p/{id}/api/**` 反代到宿主 mock）
   → Admin 页面能点击按钮打到 `/_p/{id}/api/v1/ping`

5. **子进程 Supervisor + Health**
   文件：`internal/infra/plugin/supervisor/process.go`、`internal/infra/plugin/manager/{health,lifecycle}.go`
   → 把反代目标切成“真实子进程”

---

## 最小 Demo 插件（你需要准备的内容）

```
plugins/installed/com.powerx.demo.hello_world/0.1.0/
├── plugin.yaml                 # runtime.kind=static 起步，然后再切到 process
├── frontend/admin/             # 一个 index.html，展示“Hello, PowerX”
└── contracts/openapi.yaml      # 可选，先空
```

*第二阶段* 把 `runtime.kind` 改为 `process`，提供：

```
backend/bin/hello_world         # 可执行，暴露 /healthz 与 /v1/ping
```

---

## 验收清单（MVP）

* [ ] 服务启动：日志显示插件数、各自状态
* [ ] `GET /api/v1/admin/manifest`：包含插件菜单
* [ ] `POST /api/v1/admin/plugins/{id}/enable`：菜单出现且能打开
* [ ] `/_p/{id}/admin/index.html`：可访问
* [ ] `/_p/{id}/api/v1/ping`：能返回 200（先 mock，后子进程）
* [ ] `POST /api/v1/admin/plugins/{id}/disable`：菜单消失，相关路由 404
* [ ] （子进程后）异常退出能自动拉起、健康探测成功后才挂载代理

---

## 常见坑（提前规避）

* **路径规范**：统一以 `/_p/{id}/admin/**`、`/_p/{id}/api/**`，避免与宿主业务路由冲突
* **热更新**：`enable/disable` 必须是“幂等级路由装卸”，不要重启宿主
* **静态资源缓存**：加上 ETag/Cache-Control；但调试期先禁用强缓存
* **权限延迟**：菜单缓存与 RBAC 检查要有失效策略（enable/disable 后刷新）
* **多版本并存**：注册表以 `<id>@<version>` 作为 key，`current` 指针指向活动版本（为后续升级/回滚铺路）

---

> 总结一句：**先做“读清单 + 挂静态 + 启停路由”**，跑通后再“反代 API → 子进程 → 安装器 → RBAC/事件”。
> 如果你同意这条路线，我可以把 **manifest 的字段表（带校验规则）** 和 **路由/启停的状态机序列图** 整理成 README 片段，直接放进 `pkg/plugin/`。
