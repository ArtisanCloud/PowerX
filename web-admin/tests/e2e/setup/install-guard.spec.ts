import { test, expect } from "@playwright/test";

test.describe("install guard", () => {
  test("未安装时仅可进入 /setup，且不展示 AI 欢迎引导", async ({ page }) => {
    await page.route("**/api/v1/admin/setup/status", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            configured: false,
            requires_login: false,
            install_status: "uninstalled",
            guard_mode: "strict",
            checks: { users: 0, tenants: 0, ai_profiles: 0 },
          },
        }),
      });
    });

    await page.route("**/api/v1/admin/setup/config", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              config: {
                domain: { domain: "" },
                https: { mode: "auto" },
                storage: { type: "local", local_path: "/data/uploads" },
                cache: { type: "redis", redis_host: "localhost", redis_port: 6379, redis_db: 0 },
                email: { enabled: false, smtp_port: 587 },
                ports: { backend_port: 8080, web_admin_port: 3000 },
              },
            },
          }),
        });
        return;
      }
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { ok: true } }) });
    });

    await page.goto("/agent");
    await expect(page).toHaveURL(/\/setup$/);

    await page.waitForTimeout(1500);
    await expect(page.getByText("欢迎使用 PowerX 系统！")).toHaveCount(0);
  });
});
