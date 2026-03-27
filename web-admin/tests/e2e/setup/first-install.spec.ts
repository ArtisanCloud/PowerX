import { test, expect } from "@playwright/test";
import type { Page } from "@playwright/test";

test.describe("first install setup", () => {
  const mockSetupApis = async (page: Page) => {
    let completed = false;

    await page.route("**/api/v1/admin/setup/status", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            configured: completed,
            requires_login: false,
            checks: {
              users: 0,
              tenants: 0,
              ai_profiles: 0,
            },
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

      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { ok: true } }),
      });
    });

    await page.route("**/api/v1/admin/setup/complete", async (route) => {
      completed = true;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { ok: true, configured: true } }),
      });
    });
  };

  test("访问根路径自动进入 setup，完成后回到首页", async ({ page }) => {
    await mockSetupApis(page);

    await page.goto("/");
    await expect(page).toHaveURL(/\/setup$/);

    const checks = page.locator('input[type="checkbox"]');
    await checks.nth(0).check();
    await checks.nth(1).check();
    await page.getByRole("button", { name: "下一步" }).click();

    await page.getByPlaceholder("例如：powerx.yourdomain.com").fill("powerx.local");
    await page.getByRole("button", { name: "下一步" }).click();

    await page.getByPlaceholder("8080").fill("18080");
    await page.getByPlaceholder("3000").fill("13000");
    await page.getByRole("button", { name: "下一步" }).click();

    await page.getByPlaceholder("admin@example.com").fill("admin@powerx.local");
    await page.getByPlaceholder("请输入强密码").fill("Password123!");
    await page.getByPlaceholder("再次输入密码").fill("Password123!");
    await page.getByRole("button", { name: "下一步" }).click();
    await page.getByRole("button", { name: "完成设置" }).click();

    await expect(page).toHaveURL(/\/home$/);
  });

  test("setup 端口校验：backend/web-admin 端口不能相同", async ({ page }) => {
    await mockSetupApis(page);

    await page.goto("/");
    await expect(page).toHaveURL(/\/setup$/);

    const checks = page.locator('input[type="checkbox"]');
    await checks.nth(0).check();
    await checks.nth(1).check();
    await page.getByRole("button", { name: "下一步" }).click();

    await page.getByPlaceholder("例如：powerx.yourdomain.com").fill("powerx.local");
    await page.getByRole("button", { name: "下一步" }).click();

    await page.getByPlaceholder("8080").fill("18080");
    await page.getByPlaceholder("3000").fill("18080");
    await expect(page.getByRole("button", { name: "下一步" })).toBeDisabled();
  });
});
