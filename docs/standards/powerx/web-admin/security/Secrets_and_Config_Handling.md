# 机密信息与配置管理

> 定义在 PowerX Web Admin 中处理 API Key、Token、环境变量等敏感配置的流程，保障开发、测试、生产环境的一致性与安全性。

---

## 1. 分类与存储

| 类型 | 示例 | 建议存储方式 |
| --- | --- | --- |
| 构建时变量 | `NUXT_PUBLIC_API_BASE` | `.env` + CI 注入，不包含机密 |
| 运行时机密 | API Key、Sentry DSN、OAuth Client Secret | 后端下发或运行平台 Secret 管理（Vault、KMS、Ci/CD Secret） |
| 用户凭证 | `access_token`、`refresh_token` | 客户端临时存储（参考 `Token_Storage_and_Rotation.md`），优先 HttpOnly Cookie |
| 私有文件 | 证书、插件包 | 仅在后端或部署管道存储，前端不直接接触 |

---

## 2. 环境变量管理

- `.env.example` 仅保留非机密变量示例。  
- 本地开发使用 `.env.local`，确保 `.gitignore` 排除。  
- CI/CD 通过平台（GitHub Actions Secrets、GitLab Variables）注入机密，构建时写入环境。  
- 前端 `runtimeConfig` 中 `public` 属性只放置非敏感信息（API 基础路径、主题配置等）。

---

## 3. 构建流程

1. CI 在构建前注入环境变量（`SENTRY_DSN`、`POWERX_BACKEND` 等）。  
2. 使用 `nuxt prepare` / `nuxt build` 时仅读取所需配置，禁止在源码中硬编码机密。  
3. 生产部署时与后端共享版本号 `NUXT_APP_VERSION`，便于日志追踪。

---

## 4. 秘密轮换

- 为 API Key/Token 设置有效期，记录最后更新时间。  
- 一旦轮换需更新：后端配置、前端 `.env`（若仅供本地调试）、文档与监控。  
- 对 Sentry、第三方服务等机密，建议使用自动轮换脚本（CI 触发）。

---

## 5. 访问控制

- 机密只对最小权限人员开放。使用组织密码库（1Password、Vault）集中管理，不通过即时通讯发送。  
- 记录机密使用者/用途，建立审计日志。  
- 开发环境可使用 Mock Key，不要复用生产真实凭证。

---

## 6. 本地调试提示

- 若必须使用真实机密（如连接生产 API），请在 `.env.local` 中配置，并在使用后立即清理。  
- 避免将机密写入浏览器 Local Storage；若暂时需要，确保完成后删除。  
- 对 CLI/脚本的输出进行遮罩处理，防止日志泄露。

---

## 7. Review Checklist

- [ ] 新增环境变量是否记录在 `docs/environment/Env_Variables_Schema.md`。  
- [ ] PR 中是否避免暴露机密（检查 diff、日志、截图）。  
- [ ] 部署凭证是否通过安全渠道分发并记录。  
- [ ] Token/Key 轮换后相关服务是否同步更新。  
- [ ] 是否设置 Sentry/监控告警，检测机密滥用或异常访问。

---

## 8. 后续计划

- 引入 Vault/SSM Parameter Store 与 Nuxt 运行时配置集成。  
- 构建阶段生成机密使用报告，提醒开发者清理无效变量。  
- 建立“机密生命周期表”，记录生成/轮换/废弃时间节点。  
- 针对插件市场提供独立的机密管理 UI，避免将凭证暴露给前端。
