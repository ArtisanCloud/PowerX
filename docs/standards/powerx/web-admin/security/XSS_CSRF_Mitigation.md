# XSS / CSRF 防护策略

> 概述 PowerX Web Admin 在跨站脚本（XSS）与跨站请求伪造（CSRF）方面的防御措施及实现要点。

---

## 1. XSS 防护

### 1.1 编码与转义

- 默认使用 Vue 模板（自动 HTML 转义）。在需要 `v-html` 的场景必须进行白名单过滤（例如使用 `DOMPurify`）。  
- 在富文本编辑或 Markdown 渲染中，启用 `sanitize` 选项，禁止内联脚本与 iframe。  
- URL、文件名、动态 class 等字段需使用 `encodeURIComponent`、正则校验，避免注入。

### 1.2 CSP（Content-Security-Policy）

- 配置详见 `docs/security/CSP_and_Security_Headers.md`。  
- 去除 `unsafe-inline`/`unsafe-eval`，改用 `nonce` 或 `hash`。  
- 对第三方脚本（Sentry、Analytics）设定明确白名单。

### 1.3 Token 存储

- 当前 Token 存储在 `localStorage`（参见 `Token_Storage_and_Rotation.md`），暴露于 XSS 攻击。  
- 中长期需迁移到 HttpOnly Cookie，并结合 CSRF Token 防护。  
- 在迁移前确保开启严格 CSP、ESLint XSS 规则、定期安全测试。

### 1.4 依赖审计

- 使用 `npm audit`, `snyk` 定期检查依赖安全问题。  
- 对动态引入的第三方插件进行安全评估，限制 JS 执行权限。

---

## 2. CSRF 防护

### 2.1 使用 Token 或 SameSite Cookie

- 若改用 Cookie 存储认证信息，必须启用：
  - `SameSite=Lax` 或 `SameSite=Strict`  
  - `Secure`, `HttpOnly`  
- 对跨站请求（如 SSO）需要提供 `state` 参数校验。

### 2.2 双重提交或自定义头

- 对敏感操作（安装插件、配置保存）发送自定义头 `X-PowerX-CSRF`，后端校验。  
- 可结合 `CSRF Token` 放在 `meta` 标签或首次 API 响应中，前端请求时附带。

### 2.3 Non-GET 操作要求

- 所有数据修改操作必须使用 `POST/PUT/PATCH/DELETE`，禁止使用 `GET` 触发副作用。  
- 后端在检查 Referer/Origin 时允许受信域名列表。

---

## 3. 输入验证与白名单

- 在表单提交前进行前端校验（长度、格式），并与后端校验一致。  
- 对文件上传限定 MIME 类型、大小，并使用后端重新生成文件名。  
- 对自定义模板、脚本（如 Workflow 节点配置）做深度校验，禁止用户自带 JS。

---

## 4. 安全测试

- 集成自动化安全扫描（OWASP Zap、Burp Suite）至测试流程。  
- 定期进行渗透测试，涵盖 XSS、CSRF、Clickjacking、Token 泄露。  
- 为 QA 提供常见攻击用例脚本，确保回归时覆盖。

---

## 5. Review Checklist

- [ ] 是否避免使用 `v-html`，如必须使用是否通过 `DOMPurify`。  
- [ ] 是否在 `useApiClient` 请求中添加自定义 CSRF 头。  
- [ ] 认证 Cookie 是否设置 `HttpOnly`、`Secure`、`SameSite`（迁移后）。  
- [ ] 是否有 CSP 配置并定期审核。  
- [ ] 表单/上传是否执行了输入校验。  
- [ ] 新功能是否经过安全测试和文档更新。

---

## 6. 后续计划

- 实施 HttpOnly Cookie + 双 Token 模式，并更新前端实现。  
- 在 CI 中集成 ESLint XSS 插件与安全扫描脚本。  
- 建立漏洞响应流程（SLA、责任人、补救步骤）。  
- 对插件/第三方应用提供沙箱机制，隔离潜在 XSS 风险。
