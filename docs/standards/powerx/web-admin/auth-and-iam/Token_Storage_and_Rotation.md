# Token 存储与轮换策略

> 本文描述 PowerX Web Admin 前端在管理 Access / Refresh Token 时的存储方式、安全注意事项及轮换流程，确保多标签、跨租户操作的会话安全。

---

## 1. 当前实现快照

| 内容 | 位置 | 说明 |
| --- | --- | --- |
| Token 写入 | `useAuth().setAuth()`（`app/composables/useAuth.ts: "13`） | 登录成功后将 `access_token`、`refresh_token`、`token_type`、`expires_in`、`expires_at` 写入 `localStorage`。 |"
| Token 读取 | `useApiClient` 请求拦截器（`app/composables/api/index.ts: "34`） | 每次请求前从 `localStorage` 读取 Token，加到 `Authorization` 头。 |"
| 过期检测 | `useAuth().isTokenExpired()` 与 `auth` 中间件（`app/composables/useAuth.ts: "35`，`app/middleware/auth.ts:31`） | 超时后清理 Token 并跳转登录。 |"
| 清理逻辑 | `useAuth().clearAuth()`、`logout()`（`app/composables/useAuth.ts: "21`，`53`） | 移除所有 Token 相关存储与上下文。 |"

> 当前 Token 存储在 `localStorage`，需通过严格的 Content Security Policy 和 XSS 审查减少泄露风险；后续如切换到 Cookie + HttpOnly，请同步更新此文档。

---

## 2. 存储策略对比

| 选项 | 优点 | 风险 | 适用场景 |
| --- | --- | --- | --- |
| `localStorage` | 实现简单、便于跨标签共享；可配合 `storage` 事件同步登出 | 易受 XSS 攻击窃取；需手动清理 | 当前默认方案 |
| `sessionStorage` | 隔离标签页；关闭页面即失效 | 无法跨标签共享；用户体验较差 | 调试模式或高敏感页面 |
| HttpOnly Cookie | 防止 XSS 读取；浏览器自动带上 | 需后端配合设置 `SameSite`，CSRF 风险需额外防护 | 推荐在生产环境落地 |
| IndexedDB / Secure Storage | 大容量、可加密 | 实现复杂，兼容性问题 | 移动端 App 或高安全需求 |

当前项目建议在生产部署时改用 **HttpOnly + SameSite=Lax Cookie** 存储 Access Token，前端仅管理 Refresh Token 或 Session 标识；迁移步骤需与后端协作。

---

## 3. 轮换与刷新节奏

1. **登录后存储**：`setAuth()` 记录 `expires_at`，用于后续刷新。  
2. **主流程**（待完善）：
   - 在请求拦截器或定时任务内，判断 `expires_at - now <= refreshThreshold`（例如 120 秒）。  
   - 调用 `authService.refreshToken({ refreshToken })` 更新访问令牌。  
   - 用 `setAuth()` 写入新 Token，并触发 `storage` 事件同步多标签页。  
3. **刷新失败兜底**：  
   - `RefreshToken` 失效 → 清理存储 → 跳转登录页（带上 `redirect` 参数）。  
   - 可在登录页显示“会话已失效，请重新登录”。  
4. **长时间未操作**：结合浏览器 `visibilitychange`、`focus` 事件在用户回到标签页时执行一次刷新，避免操作中途过期。

---

## 4. 多标签同步

- 使用 `window.addEventListener("storage", handler)` 监听其他标签页对 `access_token`/`refresh_token` 的更新：  
  ```ts
  if (process.client) {
    window.addEventListener("storage", (event) => {
      if (event.key === "access_token") {
        useAuth().initAuth();
      }
      if (event.key === "px:logout") {
        useAuth().clearAuth();
        navigateTo("/users/login");
      }
    });
  }
  ```
- 登出时可先写入 `localStorage.setItem("px:logout", Date.now().toString())`，触发所有标签页清理。  
- 若迁移至 Cookie 存储，则通过后端在登出接口返回 `Set-Cookie: token=; Max-Age=0` 并刷新页面。

---

## 5. 安全建议

1. **CSP 与 XSS 防护**：结合 `Content-Security-Policy` 禁止内联脚本，防止 Token 被窃取（参考 `docs/security/CSP_and_Security_Headers.md`）。  
2. **HTTPS 强制**：所有生产环境必须启用 TLS，避免 Token 在传输过程中泄露。  
3. **Refresh Token 使用次数限制**：若后端支持，一次使用后立即旋转新的 Refresh Token，降低泄露影响。  
4. **设备指纹 / 会话绑定**：后端可校验用户代理、IP、设备 ID，防止 Token 被盗用。  
5. **错误追踪**：在 `normalizeApiError` 中记录 401/403 响应并打点，监控异常刷新或被动登出。

---

## 6. 落地步骤清单

- [ ] 在 `useAuth()` 中实现 `refreshAuth()` 并在请求拦截器调用。  
- [ ] 封装 `useTokenStorage()`，统一读写，并暴露事件监听。  
- [ ] 引入 `storage` 事件同步多标签登录/登出状态。  
- [ ] 与后端商定 Cookie 化策略，确保 `SameSite`、`Secure`、`HttpOnly` 均已配置。  
- [ ] 在登录、登出、刷新时更新最近活动时间，便于后台审计。  
- [ ] 将新的 Token 相关环境变量（如 Cookie 名称、刷新阈值）记录到 `Env_Variables_Schema.md`。  
- [ ] 安排渗透测试验证 XSS/CSRF 防护是否有效。

> 对于移动端或嵌入式场景，建议使用系统提供的安全存储（如 iOS Keychain、Android Keystore），并实现双向 TLS 以降低 Token 泄露风险。
