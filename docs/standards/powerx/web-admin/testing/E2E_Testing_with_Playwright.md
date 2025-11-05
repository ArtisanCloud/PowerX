# 使用 Playwright 编写端到端测试

> 说明如何在 PowerX Web Admin 中搭建 Playwright 测试，用于覆盖登录、仪表盘、Agent 会话等关键路径。当前仓库尚未集成 Playwright，请按本文步骤配置。

---

## 1. 安装与初始化

```bash
npx playwright install --with-deps
npx playwright codegen http://localhost:3000
```

- 生成 `playwright.config.ts`，推荐结构：
  ```ts
  import { defineConfig, devices } from "@playwright/test";

  export default defineConfig({
    testDir: "./tests/e2e",
    retries: 1,
    timeout: 60_000,
    expect: { timeout: 10_000 },
    use: {
      baseURL: process.env.PLAYWRIGHT_BASE_URL || "http://127.0.0.1:3000",
      storageState: "./tests/e2e/.auth/admin.json",
      trace: "on-first-retry",
      video: "retain-on-failure",
    },
    projects: [
      { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    ],
  });
  ```

- 在 `package.json` 中添加脚本：
  ```json
  {
    "scripts": {
      "test:e2e": "playwright test",
      "test:e2e:ui": "playwright test --ui"
    }
  }
  ```

---

## 2. 登录钩子（Storage State）

1. 创建 `tests/e2e/auth.setup.ts`：
   ```ts
   import { test as setup } from "@playwright/test";

   const adminState = "./tests/e2e/.auth/admin.json";

   setup("authenticate as admin", async ({ page }) => {
     await page.goto("/users/login");
     await page.fill("#tenant", "demo");
     await page.fill("#identifier", "admin@example.com");
     await page.fill("#password", process.env.E2E_ADMIN_PASSWORD!);
     await page.click("button[type=submit]");
     await page.waitForURL("/home");
     await page.context().storageState({ path: adminState });
   });
   ```
2. 在 `playwright.config.ts` 中将 `globalSetup` 指向该文件。  
3. CI 环境提前注入 `E2E_ADMIN_PASSWORD`，或改用后端颁发的一次性 Token。

---

## 3. 测试组织

- 目录结构建议：
  ```
  tests/e2e/
  ├── auth.setup.ts
  ├── dashboard.spec.ts
  ├── agent-chat.spec.ts
  ├── plugins-market.spec.ts
  └── fixtures/
      └── api.ts         # REST helper、数据清理
  ```
- 通用 fixture 可放在 `tests/e2e/fixtures`，例如：
  ```ts
  export const rest = (page: Page) => ({
    async createAgent(payload) {
      const resp = await page.request.post("/api/v1/admin/agents", { data: payload });
      return resp.json();
    },
  });
  ```

---

## 4. 关键场景示例

### 4.1 登录 + 仪表盘

```ts
import { test, expect } from "@playwright/test";

test("dashboard renders stats", async ({ page }) => {
  await page.goto("/dashboard");
  await expect(page.getByText("访问趋势")).toBeVisible();
  await expect(page.locator("[data-testid=stat-card]").first()).toBeVisible();
});
```

### 4.2 Agent 对话

```ts
test("send message to agent", async ({ page }) => {
  await page.goto("/agent");
  await page.getByRole("textbox", { name: /输入/i }).fill("你好");
  await page.getByRole("button", { name: /发送/i }).click();
  await expect(page.getByText("你好")).toBeVisible();
});
```

> 若后端未提供真实响应，可在测试前启动 Mock Service Worker 或使用测试账户回放固定脚本。

---

## 5. 数据隔离

- 使用独立测试租户或数据库 schema，通过 REST API 在 `beforeEach` 创建数据、`afterEach` 清理。  
- 尽量避免依赖 UI 创建资源，改用 API 直接注入（速度更快）。

---

## 6. CI 集成

- 在 GitHub Actions 中添加步骤：
  ```yaml
  - name: Install Playwright deps
    run: npx playwright install --with-deps
  - name: Run E2E tests
    env:
      PLAYWRIGHT_BASE_URL: http://127.0.0.1:3000
      E2E_ADMIN_PASSWORD: ${{ secrets.E2E_ADMIN_PASSWORD }}
    run: npm run test:e2e
  ```
- 将 `playwright-report/`、`test-results/` 设为构件便于调试。

---

## 7. 常见问题

- **登录频繁超时**：检查 Storage State 是否与后端 Session 过期时间匹配，必要时在测试前刷新 Token。  
- **定位失败**：优先使用 `data-testid` 属性，而不是依赖文本。  
- **网络不稳定**：增加重试或在 CI 中使用稳定的 mock 服务。

---

## 8. Review Checklist

- [ ] 测试步骤独立，可并行运行，不互相污染。  
- [ ] 断言关注业务结果，而非内部实现。  
- [ ] 已添加 `data-testid` 以提高稳定性。  
- [ ] 失败时输出充分日志（trace/video）。  
- [ ] 文档/README 同步更新执行方式。

---

## 9. 后续计划

- 引入视觉回归（Playwright 截图）与 Axe 无障碍检查。  
- 为租户切换、插件安装等复杂流程编写复合场景。  
- 将 E2E 套件纳入 nightly pipeline，结合实时环境数据验证高峰期表现。
