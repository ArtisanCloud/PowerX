import { test, expect } from "../fixtures/authenticatedTest"

test.describe("ops traceability", () => {
  test("部署记录展示 trace_id 以便链路追踪", async ({ page }) => {
    const traceId = "trace-e2e-frontend-001"
    const releases = [
      {
        id: 11,
        environment: "prod",
        backend_version: "v2.0.1",
        web_admin_version: "v2.0.1",
        action: "release",
        status: "success",
        operator: "root",
        trace_id: traceId,
      },
    ]

    await page.route("**/api/v1/admin/deploy/releases**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { items: releases, pagination: { total: 1, page: 1, page_size: 20 } } }),
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
        body: JSON.stringify({ data: { release: releases[0] } }),
      })
    })

    await page.goto("/ops/deploy")

    await expect(page.getByRole("heading", { name: "部署发布中心" })).toBeVisible()
    await expect(page.getByText(traceId)).toBeVisible()
  })
})

