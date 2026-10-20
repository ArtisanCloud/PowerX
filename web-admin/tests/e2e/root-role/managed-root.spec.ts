import { test, expect } from "@playwright/test";

for (const narrow of [false, true]) {
  test(`root is managed and tenant admin remains editable (${narrow ? "narrow" : "desktop"})`, async ({
    browser,
    baseURL,
  }, info) => {
    const expiresAt = Date.now() + 3600_000;
    const tenantUuid = "11111111-1111-4111-8111-111111111111";
    const session = {
      version: 1,
      sessionId: "root-role-browser",
      revision: 1,
      status: "active",
      accessToken: `header.${Buffer.from(JSON.stringify({ tid: tenantUuid, exp: Math.floor(expiresAt / 1000) })).toString("base64url")}.fixture`,
      refreshToken: "fixture",
      tokenType: "Bearer",
      expiresAt,
      scope: "access",
      tenantUuid,
    };
    const context = await browser.newContext({
      baseURL,
      locale: "zh-CN",
      viewport: narrow
        ? { width: 390, height: 844 }
        : { width: 1440, height: 1000 },
      storageState: {
        cookies: [],
        origins: [
          {
            origin: baseURL!,
            localStorage: [
              { name: "powerx.session", value: JSON.stringify(session) },
              { name: "welcome_guide_completed", value: "true" },
              {
                name: "powerx.permission.manager.view.v1",
                value: JSON.stringify({ roleId: 1 }),
              },
            ],
          },
        ],
      },
    });
    const page = await context.newPage();
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    const closeMobileSidebar = async () => {
      if (!narrow) return;
      const collapse = page
        .getByRole("complementary")
        .getByRole("button", { name: "收起", exact: true })
        .first();
      const box = await collapse.boundingBox();
      if (box && box.x >= 0) await collapse.click();
    };
    try {
      await page.goto("/settings/roles");
      const skip = page.getByRole("button", { name: "跳过引导", exact: true });
      if (await skip.isVisible()) await skip.click();
      await closeMobileSidebar();
      const root = page
        .getByRole("row")
        .filter({ has: page.getByText("System Root", { exact: true }) });
      await expect(root).toContainText("由 Core 维护");
      await expect(root.getByRole("button")).toHaveCount(0);
      const admin = page
        .getByRole("row")
        .filter({ has: page.getByText("Tenant Admin", { exact: true }) });
      await expect(
        admin.getByRole("button", { name: "编辑", exact: true }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath(
          `managed-root-${narrow ? "narrow" : "desktop"}.png`,
        ),
        fullPage: true,
      });
      await page.goto("/settings/users");
      await closeMobileSidebar();
      await page
        .getByRole("button", { name: "权限", exact: true })
        .first()
        .click();
      await page.getByRole("button", { name: /角色配置/ }).click();
      await expect(
        page.getByRole("heading", {
          name: "Tenant Admin",
          exact: true,
          level: 3,
        }),
      ).toBeVisible();
      await expect(page.getByText("System Root", { exact: true })).toHaveCount(
        0,
      );
      expect(
        await page.evaluate(
          () =>
            JSON.parse(
              localStorage.getItem("powerx.permission.manager.view.v1")!,
            ).roleId,
        ),
      ).toBe(2);
      await page.screenshot({
        path: info.outputPath(
          `business-role-editor-${narrow ? "narrow" : "desktop"}.png`,
        ),
        fullPage: true,
      });
      expect(errors).toEqual([]);
    } finally {
      await context.close();
    }
  });
}
