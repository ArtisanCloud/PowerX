# CSP 与安全响应头策略

> 规划 PowerX Web Admin 的安全响应头配置，包括 Content-Security-Policy、HSTS、X-Frame-Options 等，适用于 CDN、Nginx、云服务部署。

---

## 1. 必备响应头

| 头部 | 作用 | 推荐值 |
| --- | --- | --- |
| `Content-Security-Policy` | 限制资源加载来源 | 见下文 |
| `Strict-Transport-Security` | 强制 HTTPS | `max-age=31536000; includeSubDomains; preload` |
| `X-Frame-Options` / `frame-ancestors` | 防点击劫持 | `DENY` 或 `SAMEORIGIN` |
| `X-Content-Type-Options` | 禁止 MIME 嗅探 | `nosniff` |
| `Referrer-Policy` | 控制 Referer  | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | 限制浏览器能力 | `geolocation=(), camera=(), microphone=()` |
| `Cross-Origin-Opener-Policy` / `Cross-Origin-Embedder-Policy` | 防止跨站数据离开上下文，支持 SharedArrayBuffer | `same-origin` / `require-corp`（视需求配置） |

这些响应头可通过：
- Nitro `routeRules`, `hooks` (`hooks: { "request": (event) => setResponseHeaders(...) }`)  
- 部署层（Nginx、CloudFront、Vercel）统一设置。

---

## 2. Content-Security-Policy 示例

### 基础 CSP

```
Content-Security-Policy:
  default-src 'self';
  script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net;
  style-src 'self' 'unsafe-inline';
  img-src 'self' data: https:;
  font-src 'self' https://fonts.gstatic.com;
  connect-src 'self' https://api.powerx.example.com wss://ws.powerx.example.com;
  frame-ancestors 'none';
  base-uri 'self';
```

- 若启用 Sentry、第三方图标等资源，需将域名加入对应 directive。  
- 尽量移除 `'unsafe-inline'` / `'unsafe-eval'`；若难以避免，可结合 `nonce` 或 `sha256-...`。

### Nuxt 实现

1. 在 `nuxt.config.ts` 添加 Nitro route rules：
   ```ts
   export default defineNuxtConfig({
     nitro: {
       routeRules: {
         "/**": {
           headers: {
             "content-security-policy": "...",
             "strict-transport-security":
               "max-age=31536000; includeSubDomains; preload",
           },
         },
       },
     },
   });
   ```
2. 若使用 `@nuxtjs/helmet`（未来可选），可自动管理大部分安全头。

---

## 3. 本地开发注意

- 开发模式使用 `http://localhost` 时，适当放宽 CSP（例如允许 `unsafe-eval`）以兼容 Vue Devtools。  
- 在 `.env.development` 中设置 `DISABLE_CSP=true` 仅限本地；生产环境必须启用。

---

## 4. 第三方资源白名单

- **Sentry**：`https://*.sentry.io`（`script-src`, `connect-src`）。  
- **Analytics**（如 Plausible）：添加脚本和上报域名。  
- **CDN**（图标/字体/图片）：`https://cdn.jsdelivr.net`, `https://fonts.googleapis.com`。  
- **WebSocket**：`connect-src wss://ws.powerx.example.com`，并与后端实际域名保持一致。

---

## 5. 测试与监控

- 使用浏览器 DevTools > Security 检查头部是否生效。  
-.CI 集成 [Mozilla Observatory](https://observatory.mozilla.org/) 或 `npx helmet-csp` 验证配置。  
- 定期扫描 CSP 报错（`report-uri`/`report-to`）并收敛白名单。

---

## 6. Review Checklist

- [ ] 生产环境启用 HTTPS 并配置 HSTS。  
- [ ] CSP 覆盖所有静态/动态资源，无不必要的 `unsafe-*`。  
- [ ] iframe 嵌入需求明确（如插件 WebView），必要时在 `frame-ancestors` 指定白名单。  
- [ ] API/WebSocket 域名与 `connect-src` 一致。  
- [ ] 头部配置在各环境（Dev/Staging/Prod）同步更新。

---

## 7. 后续计划

- 将安全头配置抽离为共享模板 (`scripts/check-refactor.sh` 检查关键文件)。  
- 引入自动化扫描（Zap、Burp）验证 CSP。  
- 针对多租户插件 iframe 场景，设计细粒度的 `Content-Security-Policy`（每个租户动态生成）。
