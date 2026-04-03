# Dev Hotload 指南

PowerX 的 Dev Hotload 流程允许你在不重新打包/发布插件的情况下，把本地构建产物推送到 Dev API 的沙盒环境，即刻在 Web Admin 中体验最新 UI/接口。本文总结常见问题与正确姿势。

## CLI 对应接口速查

- `px-plugin dev` → `POST /api/v1/internal/dev/plugins/register`（注册热加载会话，返回 sessionId/reloadToken，用于后续 reload/terminate）
- `px-plugin publish` → `POST /api/v1/internal/plugins/releases`（登记发布候选，上传构建产物，进入发布候选列表）

## 核心概念

- **会话 (Session)**：热加载时 CLI 与 Dev API 之间的逻辑上下文，包含 `sessionId` 与 `reloadToken`。Dev API 将会话元数据持久化在 `dev_hotload_sessions`/`dev_hotload_session_events` 表中，CLI 和插件只通过 Dev API 获取会话状态，不直接访问数据库。会话标识当前 artefact 属于哪个插件/租户，并允许多次 reload 复用同一身份。
- **沙盒 (Dev Sandbox)**：PowerX 在 Dev API 内部启动的隔离运行环境，用来临时存放并挂载你上传的 `dist/`、manifest 等资源。沙盒使用项目根目录下的 `tmp/dev-hotload/<session>` 等独立目录和进程，将内容挂载到 `/plugins/<id>/admin/*`，与正式安装目录相互独立，确保热加载不会污染生产版本。
- **插件菜单**：由 plugin manager 维护的“入口定义”，决定 Web Admin 中有哪些插件菜单、对应 URL/权限如何。只有插件安装并启用后才会生成菜单；热加载只替换菜单背后的资源，因此必须依赖已存在的菜单结构。

## 正式发布 vs. 热加载

PowerX 的插件发布链路分为 4 步，热加载属于独立的调试通道：

1. **package**（开发者机器执行）  
   `px-plugin package` 构建插件的前端/后端 artefact + metadata，并将产物写入插件仓库下 `.px-plugin/build/`（含 `package.tar.gz`、manifest、hash 等）。这一步不会触碰 PowerX。

2. **publish**（开发者机器执行）  
   `px-plugin publish` 读取你的 PowerX 配置（`px auth configure` 写入的 dev/publish 基址、API Token，也可以在 `~/.px-plugin/config.json` 或环境变量中重写），然后把包 POST 到该 PowerX 实例的插件发布 API。也就是说，它只是上传到**你当前连接的 PowerX Registry**；不会默认跑去某个云端 Marketplace。Registry 接收后，PowerX Marketplace/插件管理后台才会看到“待审核版本”。
   - 管理员可在 Web Admin 的 `/plugin-release` 页面查看和筛选候选，补件或推动审批。

3. **install**（PowerX 管理后台执行）  
   管理员在 PowerX UI 中选择某个发布版本（或上传本地包），宿主会将 artefact 解压到 `backend/plugins/installed` 等目录、更新数据库、生成菜单。只有安装完成，插件的菜单入口/权限才被注册。

4. **enable / tenant binding**（PowerX 管理后台执行）  
   管理员决定哪些租户可以启用该插件，并可随时启停。插件开始在宿主环境里提供服务。

5. **dev（热加载）**（开发者机器执行）  
   面向已安装/启用的插件。`px-plugin dev` 向 Dev API 注册 session 并推送本地构建 artefact，由 Dev Sandbox 挂载到 `/plugins/<id>/admin/*`。沙盒不会写安装目录，只是临时覆盖；`--watch` 模式下可持续 reload，退出后还原为安装版本。

因此，正式版本仍需走 package → publish → install → enable；热加载 (`px-plugin dev`) 是为了已安装插件的快速调试，不能替代 Marketplace/Registry 流程。

## 使用流程（热加载）

1. **先安装一次插件**  
   通过 `px-plugin package/publish` 或手动登记 `plugins/registry.json`，确保插件处于 `enabled` 状态，Web Admin 才能看到对应菜单/权限。

2. **本地热加载命令**  
   ```bash
   px-plugin dev               # 单次构建 + reload + terminate
   px-plugin dev --watch       # 监听文件并持续 reload，Ctrl+C 结束
   px-plugin dev --list-sessions --list-status all
   px-plugin dev --clear-sessions --clear-sessions-force
   ```
   - CLI 流程：`POST /register` → `POST /reload` →（单次模式）`DELETE /register/{sessionId}`。因此 `--list-sessions` 看到 `status=terminated` 是正常的。
   - `--watch` 模式会保持 session 为 `active`，方便持续迭代。

3. **查看效果**  
   刷新 Web Admin 的 Dev Console 或插件菜单，访问路径是：浏览器 → PowerX Admin → 插件 router → Dev Sandbox → 你刚上传的 `dist/`。只要 reload 成功，即使 session 终止，UI 仍会展示最新 artefact。

## 启动提速（插件自动恢复并发）

当系统已安装并启用了多个插件时，PowerX 启动阶段会执行自动恢复（auto-restore）。默认是串行启用，插件多时会拉长可用时间窗口。  
从当前版本开始支持配置项 + 环境变量覆盖：

```yaml
plugin:
  auto_restore_parallelism: 3
```

环境变量（优先级高于 config.yaml）：

```bash
# 默认 1（串行），建议先从 3 开始
export PX_PLUGIN_AUTORESTORE_PARALLELISM=3
# 或使用统一前缀
export CORE_X_PLUGIN_AUTORESTORE_PARALLELISM=3
```

说明：

- `plugin.auto_restore_parallelism` / 对应环境变量只影响“启动时自动恢复已启用插件”的并发度，不影响手工启停插件 API。
- 默认值 `1`，保持历史行为兼容。
- 最大值会被限制为 `8`，防止误配置导致本机 CPU/IO 抖动。
- 推荐取值：本地开发 `2~4`；CI/轻量环境 `1~2`。

## 常见问题解答

- **`/admin/menus` 为什么没有我的插件？**  
  菜单来自 plugin manager 的 registry，而非热加载 artefact。若插件未安装/启用，系统不知道有这个入口，自然不会出现在菜单 JSON 里。热加载无法“凭空生成菜单”。

- **`--list-sessions` 一直是 terminated？**  
  单次模式在完成 reload 后立刻调用 `DELETE /register/{sessionId}` 释放会话，只表示 CLI 不再占用该 session，不会影响已挂载的 artefact。

- **热加载会写入 `backend/plugins/installed` 吗？**  
  不会。沙盒只暂存 artefact 并在 Dev 环境里替换运行时，退出后恢复到之前启用的版本。真正的安装/发布仍需走 plugin install/publish 流程。

## 工作原理速览

1. `px-plugin dev` 在本地构建，整理出包含 manifest、changed files、artifacts 的 payload。
2. CLI 注册会话，获取 `sessionId` 和 `reloadToken`。
3. CLI 以 `Authorization: Bearer <reloadToken>` 调用 `/internal/dev/plugins/reload`，把 artefact 传给 Dev API。
4. Dev API 记录事件并通知沙盒，解包 artefact 到隔离环境，更新 `/plugins/<id>/admin/` 的反向代理目标。
5. 浏览器访问插件菜单时，通过宿主的动态 router 进入沙盒，即可看到最新 UI/接口。

这套机制让你在不改宿主代码、不重启插件的情况下快速迭代。进入热加载前记得先完成一次正式安装/启用，以便宿主知道菜单和权限。
