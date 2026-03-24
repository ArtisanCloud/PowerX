import { test, expect } from "../fixtures/authenticatedTest"

test.describe("ops plugin lifecycle", () => {
  test("执行插件切换并查看审计时间线", async ({ page }) => {
    const audits = [
      {
        id: 1,
        plugin_id: "plugin.mediax",
        from_version: "1.0.0",
        to_version: "1.1.0",
        action: "switch",
        result: "success",
        operator: "root",
        gate_reason: "initial rollout",
      },
    ]

    await page.route("**/api/v1/admin/plugins/*/audit**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            items: audits,
            pagination: { total: audits.length, page: 1, page_size: 20 },
          },
        }),
      })
    })

    await page.route("**/api/v1/admin/plugins/*/actions", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { audit: { ...audits[0], id: 2, action: "rollback", gate_reason: "quick rollback" } } }),
      })
    })

    await page.goto("/ops/plugins")

    await expect(page.getByRole("heading", { name: "插件生命周期中心" })).toBeVisible()
    await expect(page.getByText("initial rollout")).toBeVisible()

    await page.getByRole("button", { name: "触发动作" }).click()
    await expect(page.getByText("执行中...").first()).toBeHidden({ timeout: 3000 })
  })
})
