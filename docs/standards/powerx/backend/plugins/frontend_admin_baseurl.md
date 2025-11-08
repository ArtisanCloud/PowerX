# Nuxt 插件 Admin 前端 baseURL 排查指南

> 适用场景：基于 Nuxt（或其他会把资源路径写死进 HTML 的 SPA 框架）构建插件 Admin 前端时，PowerX Admin 中打开页面出现白屏、404、`module script 的 MIME 是 text/html` 等错误。

---

## 结论（一句话）

**白屏/404/MIME 错误的根因是：Nuxt 前端产物的 `app.baseURL` 与 PowerX 的反向代理前缀不一致**，导致浏览器请求 `/_p/<pluginId>/admin/` 下的页面时，静态资源落到了宿主兜底返回的 HTML（MIME=`text/html`）或直接 404。

---

## 发生了什么（最小心智模型）

1. 浏览器访问宿主拼好的入口，例如：`http://localhost:3030/_p/<pluginId>/admin/...`
2. 但是页面里的脚本/样式链接却是 **`/assets/...`（根路径）** 或 **`/<locale>/_p/...`** —— 与插件的前缀不一致；
3. 资源请求没有走到插件前端 Admin 进程 → 被宿主兜底返回 HTML → 浏览器提示 **“module script 的 MIME 是 text/html”** 或直接 404 → 页面白屏。

---

## 解决要点（只做这三件事）

### 1. 构建期固定 Nuxt 的 `baseURL`

```ts
// nuxt.config.ts
const pluginId = "com.powerx.plugins.base";
const pluginAdminBase = process.env.POWERX_ADMIN_BASE || `/_p/${pluginId}/admin/`;

export default defineNuxtConfig({
  app: { baseURL: pluginAdminBase, buildAssetsDir: "/assets/" },
  ssr: false,
  // ... 其他保持原样
});
```

* **不要**依赖运行期再去修改 `app.baseURL`（HTML 已经写死了）。
* `POWERX_ADMIN_BASE` 在宿主启动子进程时会自动注入（也可以在本地构建时手动传入）。

**构建命令（关键是带上 `POWERX_ADMIN_BASE`）：**

```bash
cd web-admin
POWERX_ADMIN_BASE="/_p/com.powerx.plugins.base/admin/" \
NODE_ENV=production \
npx nuxi build
```

构建完后，直接连插件前端端口检查：

```bash
curl http://127.0.0.1:<port>/_p/com.powerx.plugins.base/admin/ | \
  grep 'app:{baseURL:"/_p/com.powerx.plugins.base/admin/"}'
```

### 2. 宿主反向代理要把页面与静态资源都送到插件进程

* 管理端路由：`/_p/:id/admin/*filepath`（以及可选 `/:prefix/_p/:id/admin/*` 兼容）。
* 处理规则：
  * **静态资源**：请求路径以 `/_p/<id>/admin/assets/` 开头 → 直接反代到插件 Admin 进程（不要加 locale）。
  * **页面路由**：其它路径（如 `/dashboard`）同样直接反代到插件 Admin 进程。
  * 宿主若提供 `/<locale>/_p/...` 入口：在进入插件前，把 `/<locale>` 剥掉，并把 locale 插到 `/_p/<id>/admin/` 后（仅页面路由）。静态资源始终保持 `/_p/<id>/admin/assets/...`。

> 重点：**assets 永远走 `/_p/<id>/admin/assets/...`**，别被语言前缀污染。

### 3. 启动插件 Admin 子进程要按 `plugin.yaml`

* `frontend.admin.kind: proxy`：严格按 manifest 的 `entry + args` 启动（相对路径要转成插件安装目录的绝对路径）。
* 注入环境变量：`POWERX_ADMIN_BASE="/_p/<id>/admin/"`（宿主已默认注入）。

---

## 验证 checklist

1. **直连插件 Admin 端口**访问 `/_p/<pluginId>/admin/`：页面源码内应该能看到 `app:{baseURL:"/_p/<pluginId>/admin/"}`。
2. **经由 PowerX** 访问同一路径：浏览器 Network 面板里，所有静态资源都应该是 `/_p/<pluginId>/admin/assets/*.js` 且状态 `200`。

---

## 避免的误操作

* 不要在运行期用 `POWERX_PROXY` 去切 `app.baseURL`。
* 不要把 i18n `pages` 映射拆开手调；只要 `baseURL + strategy` 正确，Nuxt 会自动在 `baseURL` 后拼接 `/en` 等。
* 不要在宿主里全局兜 `/assets/*` 到某个插件（多插件会冲突）。**构建期 baseURL 设置正确** 时，资源路径天然带有插件前缀。

---

## FAQ

- **Q：为什么构建时的 `baseURL` 这么重要？**  
  A：Nuxt 在构建阶段就把 `baseURL` 写进 HTML、runtime 配置和每个 chunk 的预加载链接。运行期再改只是改了内存变量，已经注入的 `<script src="/assets/...">` 不会变。
- **Q：本地 dev server 需要设置吗？**  
  A：开发阶段可以保持默认根路径。但在生成产物（`nuxi build`）时，务必传入与宿主约定一致的 `POWERX_ADMIN_BASE`。
- **Q：`buildAssetsDir` 要怎么设？**  
  A：保持 `"/assets/"`。只要 `baseURL` 正确，最终资源路径会是 `/_p/<id>/admin/assets/...`。

做到以上三点，**PowerX ↔ 插件前端** 就会稳定连通，白屏/404/MIME 问题自然消失。
