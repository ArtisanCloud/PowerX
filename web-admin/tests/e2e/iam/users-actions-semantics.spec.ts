import { test, expect } from "../fixtures/authenticatedTest";

test.describe("iam users action semantics", () => {
  test("租户行点击不切换租户，仅点击“切换并管理”才调用 switch-tenant", async ({
    page,
  }) => {
    let switchTenantCalls = 0;

    await page.route("**/api/v1/admin/user/auth/me/context**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            is_root: true,
            current_tenant_uuid: "6b5d0240-9920-46da-b707-88200e0f51ea",
            current_member_id: 1,
            user: {
              id: 1,
              email: "admin@powerx.io",
              display_name: "Playwright Admin",
              avatar_url: "",
              status: 1,
              is_root: true,
            },
            members: [
              {
                tenant_uuid: "6b5d0240-9920-46da-b707-88200e0f51ea",
                tenant_name: "System",
                member_id: 1,
                is_admin: true,
              },
            ],
          },
        }),
      });
    });

    await page.route("**/api/v1/admin/tenants**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          code: 200,
          data: {
            items: [
              {
                id: 1,
                uuid: "6b5d0240-9920-46da-b707-88200e0f51ea",
                name: "System",
                domain: "agent.xpersonatoy.com",
                status: "active",
                user_count: 1,
                createdAt: "2026-01-01T00:00:00Z",
                plan: "free",
              },
            ],
            pagination: { total: 1, page: 1, page_size: 10, pages: 1 },
          },
        }),
      });
    });

    await page.route("**/api/v1/admin/user/auth/me/switch-tenant", async (route) => {
      switchTenantCalls += 1;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            is_root: true,
            current_tenant_uuid: "6b5d0240-9920-46da-b707-88200e0f51ea",
            members: [],
          },
        }),
      });
    });

    await page.route("**/api/v1/admin/system/users**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          code: 200,
          data: { items: [], pagination: { total: 0, page: 1, page_size: 10, pages: 0 } },
        }),
      });
    });

    await page.goto("/settings/users");
    const main = page.locator("main");
    await main.getByRole("button", { name: "用户", exact: true }).click();
    await expect(main.locator(".divide-y > div").first()).toBeVisible();

    await main.locator(".divide-y > div").first().click();
    expect(switchTenantCalls).toBe(0);
    await expect(page).toHaveURL(/\/settings\/users/);

    const firstRow = main.locator(".divide-y > div").first();
    await expect(firstRow.locator("button").nth(1)).toBeVisible();
    await firstRow.locator("button").nth(1).click();
    expect(switchTenantCalls).toBe(1);
    await expect(page).toHaveURL(/\/settings\/users/);
  });
});
