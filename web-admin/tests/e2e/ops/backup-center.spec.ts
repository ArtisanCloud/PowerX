import { test, expect } from "../fixtures/authenticatedTest"

test.describe("ops backup center", () => {
  test("策略、备份任务与恢复演练主流程", async ({ page }) => {
    const policies = [
      {
        id: 1,
        name: "daily-main",
        backup_type: "logical",
        schedule: "0 2 * * *",
        retention_days: 30,
        enabled: true,
        storage_target: "s3://powerx-backup/main",
      },
    ]

    await page.route("**/api/v1/admin/backup/policies**", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: { items: policies, pagination: { total: 1, page: 1, page_size: 200 } } }),
        })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { policy: policies[0] } }),
      })
    })

    await page.route("**/api/v1/admin/backup/jobs**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { items: [{ id: 10, policy_id: 1, status: "success", trigger_type: "manual", operator: "root" }], pagination: { total: 1, page: 1, page_size: 50 } } }),
      })
    })

    await page.route("**/api/v1/admin/backup/jobs/run", async (route) => {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { job: { id: 11, policy_id: 1, status: "success", trigger_type: "manual", operator: "root" } } }) })
    })

    await page.route("**/api/v1/admin/backup/cleanup", async (route) => {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { status: "success" } }) })
    })

    await page.route("**/api/v1/admin/backup/restore-drills/run", async (route) => {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { drill: { id: 21, source_job_id: 11, status: "success", rto_seconds: 120 } } }) })
    })

    await page.goto("/ops/backup")

    await expect(page.getByRole("heading", { name: "备份恢复中心" })).toBeVisible()
    await page.selectOption("select", "1")
    await page.getByRole("button", { name: "手动触发备份" }).click()
    await page.getByRole("button", { name: "触发恢复演练" }).click()
    await expect(page.getByText("最近一次演练状态：success")).toBeVisible()
  })
})
