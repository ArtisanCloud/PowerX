# CI/CD 流程（Nuxt）

> 描述 PowerX Web Admin 的持续集成与部署流程，涵盖代码检查、测试、构建、发布与回滚。可用于 GitHub Actions / GitLab CI 等平台。

---

## 1. 流程概览

```
Pull Request
 ├─ Lint (ESLint)
 ├─ Unit Tests (Vitest)          # 计划启用
 ├─ E2E Tests (Playwright)       # 计划启用
 ├─ Build (nuxt build)           # 生成产物校验
 └─ Preview (nuxt preview)       # 可选：部署到临时环境

Main 分支
 ├─ Build + Artifact 发布
 ├─ 上传 Source Map (Sentry)
 └─ 部署至 Staging → Production
```

---

## 2. 典型 GitHub Actions 工作流

```yaml
name: CI
on:
  pull_request:
  push:
    branches: [main]

jobs:
  install-deps:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
      - run: npm ci
      - name: Cache node_modules
        uses: actions/cache/save@v3
        with:
          path: node_modules
          key: ${{ runner.os }}-node-${{ hashFiles('package-lock.json') }}

  lint:
    needs: install-deps
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/cache/restore@v3
        with:
          path: node_modules
          key: ${{ runner.os }}-node-${{ hashFiles('package-lock.json') }}
      - run: npm run lint

  unit:
    needs: install-deps
    steps:
      ...
      - run: npm run test:unit

  build:
    needs: [lint, unit]
    steps:
      ...
      - run: npm run build
      - run: npm run generate # 如需静态产物
      - uses: actions/upload-artifact@v4
        with:
          name: nuxt-output
          path: .output
```

> 在 `main` 分支构建完成后，可接续部署逻辑（例如上传到 S3、触发 Kubernetes 滚动更新，或调用 Vercel API）。

---

## 3. 预览环境

- 支持 PR 预览（Vercel / Netlify）：  
  - 构建完成后自动部署临时环境。  
  - 回写 URL 到 PR 评论，便于产品/QA 体验。
- 若使用自建环境，可在 CI 中将 `.output` 上传到对象存储，并启用静态托管。

---

## 4. 环境与变量

- `NODE_VERSION=20` 与 Nuxt 依赖保持一致。  
- `NUXT_APP_VERSION` 可设置为 `${GIT_SHA}` 或 `${semver}`，用于 Sentry/日志追踪。  
- 在 CI 中注入：
  - `POWERX_BACKEND` / `WS_UPSTREAM`（指向 Staging 服务）  
  - `SENTRY_DSN`（可选）  
  - E2E 登录凭证（通过 Secret 注入）

---

## 5. 部署策略

| 环境 | 步骤 | 注意事项 |
| --- | --- | --- |
| Staging | 合并 `develop` / `main` 后自动部署 | 运行冒烟测试，检查监控 |
| Production | 手动触发或合并主干 | 支持蓝绿/金丝雀，保留回滚手段 |

产物部署方式：
- SSR：部署 `.output/server` + Node 运行时，使用 PM2/Nitro Edge。  
- Static：部署 `.output/public` 至 CDN，使用后端代理 API。

---

## 6. 回滚与监控

- 保留最近 N 次构建产物（Artifact），支持快速回滚。  
- 部署完成后触发健康检查，结合 Sentry、Prometheus 监控错误率。  
- 若性能或错误指标异常，自动触发告警并回滚到上一个稳定版本。

---

## 7. Review Checklist

- [ ] CI 阶段包含 lint、测试、构建。  
- [ ] Secrets 安全注入，未在日志中泄露。  
- [ ] 构建产物上传为 Artifact。  
- [ ] 部署脚本可重复执行，支持回滚。  
- [ ] 预览环境可用于产品/QA 验收。  
- [ ] 文档与脚本保持同步更新。

---

## 8. 后续计划

- 将 `scripts/check-refactor.sh` 纳入 CI，对于 Agent 相关改动自动运行。  
- 集成 Lighthouse CI / Bundle size 检查。  
- 构建完成后自动上传 Sentry Source Map。  
- 部署后执行端到端冒烟测试，确保上线质量。
