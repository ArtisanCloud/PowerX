# OIDC / SSO 与会话管理流程

> 面向前端与后端协作者，说明 PowerX Web Admin 目前的登录流程、Token 生命周期、Silent Renew 计划，以及后续对接企业 IdP 的扩展点。

---

## 1. 架构概览

- **登录端点**：`useAuthService().login()` 调用 `/admin/user/auth/login`（`app/composables/api/services/authService.ts: "210`）。"
- **状态持久化**：`useAuth()` 将 `access_token`、`refresh_token`、`expires_at` 等写入 `localStorage`，并通过 `useState` 保持运行时状态（`app/composables/useAuth.ts: "6`）。"
- **路由守卫**：`app/middleware/auth.ts: "1` 在客户端校验 Token，并在过期时重定向至 `/users/login?redirect=...`。"
- **用户上下文**：登录成功后调用 `useUserStore().fetchUserContext()` 以获取租户、成员、权限信息（`app/stores/user.ts: "55`）。"
- **全局请求**：`useApiClient` 拦截器在每个请求中加入 `Authorization` 头（`app/composables/api/index.ts: "37`）。"

> 当前实现为“Password Grant + Refresh Token” 模式，尚未与外部 IdP 联动；以下流程为后续对接 OIDC 做铺垫。

---

## 2. 登录/登出流程

### 2.1 登录

1. 登录页收集 `tenant`、`identifier`、`password`（`LoginParams`）。  
2. 调用 `authService.login()`，`skipAuth: true` 避免拦截器附加旧 Token。  
3. 成功后执行 `useAuth().setAuth(response.data)`：  
   - 写入 `localStorage`：`access_token`、`refresh_token`、`token_type`、`expires_in`、`expires_at`。  
   - 更新 `useState` 中的 `isAuthenticated`/`token`。  
4. 可选：拉取用户上下文与权限菜单。  
5. 跳转至重定向目标（如登录页带 `redirect` 参数）或默认 `/home`。

### 2.2 登出

1. 点击登出时调用 `useAuth().logout()`（`app/composables/useAuth.ts:53`）。  
2. 先尝试调用 `authService.logout()`（非强制，失败后仍继续）。  
3. 清理本地状态：`localStorage`、`sessionStorage`、常见认证 Cookie、Pinia store。  
4. 重定向至 `/users/login`。  
5. 若后端支持单点登出，需同时重定向至 IdP 的 `end_session_endpoint`（待接入）。

---

## 3. Token 生命周期

| Token | 存储 | 过期策略 | 备注 |
| --- | --- | --- | --- |
| `access_token` | `localStorage` + `useState` | `expires_in`（秒）换算为 `expires_at`，路由守卫与 `useAuth().getToken()` 会校验 | 目前默认 Bearer Token |
| `refresh_token` | `localStorage` | 与后端配置一致，建议 ≥ 7 天 | 暂未自动刷新 |
| `token_type` | `localStorage` | 仅在请求头中拼接 | 默认 `Bearer` |

### 3.1 过期检查

- `useAuth().isTokenExpired()` 比较当前时间与 `expires_at`，过期后自动执行 `clearAuth()`。  
- `auth` 中间件同样在进入受保护页面时校验并重定向（`app/middleware/auth.ts:31`）。

### 3.2 Silent Renew（待落地）

> 参考实现暂缺，下列为推荐步骤：

1. 在 `useAuth()` 增加 `refreshAuth()`：  
   - 若当前时间距 `expires_at` 少于阈值（例如 2 分钟），调用 `authService.refreshToken({ refreshToken })`。  
   - 成功后复用 `setAuth()` 更新 Token，失败则清理状态并跳转登录。  
2. 在 `useApiClient` 的请求拦截器中挂钩：  
   - 若请求前检测到即将过期，先执行 `refreshAuth()`。  
3. 在 `window.focus`、`visibilitychange` 事件或轮询任务中触发刷新，确保长时间打开的标签页保持会话。  
4. 多标签页同步：监听 `storage` 事件，当其他标签页更新 `access_token` 时，同步本地状态。

---

## 4. OIDC / SSO 集成计划

目前接口基于后端自建 OAuth/OIDC 服务，未来对接企业 IdP 时需：

1. **Discovery 配置**：在 `runtimeConfig.public` 暴露 `OIDC_ISSUER`、`CLIENT_ID`、`REDIRECT_URI` 等信息，并在登录页选择“SSO 登录”。  
2. **授权流程**：  
   - 点击 SSO 按钮 → 重定向至 `authorize`（`response_type=code`）。  
   - IdP 回跳 → 通过后端 `/callback` 交换 Token，再下发给前端。  
   - 前端依旧调用 `setAuth()` 保存 Token。  
3. **Silent Renew**：利用隐藏 iframe + `prompt=none` 或者后台 Refresh Token 刷新访问令牌。  
4. **多租户处理**：在重定向参数中携带 `tenantKey`，或在回调后调用 `switchTenant` API 完成上下文设置。  
5. **单点登出**：在前端登出时重定向至 `end_session_endpoint` 并携带 `id_token_hint`、`post_logout_redirect_uri`。

---

## 5. 错误与降级

- 登录失败：`normalizeApiError()` 可解析后端错误码，提供友好提示（如密码错误、租户不存在）。  
- Token 失效（401/403）：在 `useApiClient` 响应拦截器中捕获，清理状态并跳转登录；必要时显示“会话已过期” Toast。  
- Refresh 失败：在 Silent Renew 中如果遇到网络异常，可重试一次；若后端返回 400/401，视为 Refresh 失效。  
- IdP 不可用：保留账号密码登录入口或提供离线模式说明。

---

## 6. 审查清单

- [ ] 登录成功后是否立即拉取用户上下文并更新菜单/权限。  
- [ ] 是否在所有需要认证的路由上启用了 `auth` 中间件。  
- [ ] Token 过期与刷新逻辑是否一致，避免重复清理。  
- [ ] 是否处理多标签页下 Token 更新与登出事件。  
- [ ] 若加入 SSO，是否区分企业登陆与本地账号流程，并提供回退机制。  
- [ ] 新增的环境变量（IdP 信息、回调路径）是否写入 `docs/environment/Env_Variables_Schema.md`。

> 后续如需支持 PKCE、设备码或内嵌浏览器登录，请在本档补充协议细节与前端实现差异。
