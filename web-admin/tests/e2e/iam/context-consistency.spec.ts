import { test, expect } from "../fixtures/authenticatedTest";

test.describe("iam context consistency", () => {
  test("storage 事件触发后应重新请求 me/context", async ({ page }) => {
    let meContextCalls = 0;

    await page.route("**/api/v1/admin/user/auth/me/context**", async (route) => {
      meContextCalls += 1;
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

    await page.route("**/api/v1/admin/setup/status**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            configured: true,
            requires_login: false,
            restart_required: false,
            install_status: "installed",
          },
        }),
      });
    });

    await page.goto("/settings/users");
    await page.waitForTimeout(200);
    const initialCalls = meContextCalls;
    expect(initialCalls).toBeGreaterThan(0);

    await page.evaluate(() => {
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: "px_current_tenant_uuid",
          oldValue: "6b5d0240-9920-46da-b707-88200e0f51ea",
          newValue: "77c95f9f-4218-4f6c-bfd4-a9b3705d67ce",
          storageArea: window.localStorage,
          url: window.location.href,
        })
      );
    });

    await page.waitForTimeout(300);
    expect(meContextCalls).toBeGreaterThan(initialCalls);
  });
});

