import { test, expect } from "../fixtures/authenticatedTest"

test.describe("ops deploy center", () => {
  test("查询发布记录并触发回滚", async ({ page }) => {
    const releases = [
      {
        id: 1,
        environment: "prod",
        backend_version: "v1.2.3",
        web_admin_version: "v1.2.3",
        action: "release",
        status: "success",
        operator: "root",
      },
    ]

    await page.route("**/api/v1/admin/deploy/releases**", async (route) => {
      const request = route.request()
      if (request.method() === "GET") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              items: releases,
              pagination: { total: releases.length, page: 1, page_size: 20 },
            },
          }),
        })
        return
      }

      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { release: { ...releases[0], id: 2, action: "release" } } }),
      })
    })

    await page.route("**/api/v1/admin/deploy/health", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { status: "healthy", summary: "deploy pipeline healthy" } }),
      })
    })

    await page.route("**/api/v1/admin/deploy/rollback**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { release: { ...releases[0], id: 3, action: "rollback" } } }),
      })
    })

    await page.goto("/ops/deploy")

    await expect(page.getByRole("heading", { name: "部署发布中心" })).toBeVisible()
    await expect(page.getByText("v1.2.3")).toBeVisible()

    await page.getByRole("button", { name: "回滚到此版本" }).first().click()
    await expect(page.getByText("健康状态")).toBeVisible()
  })
})
