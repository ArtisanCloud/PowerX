import { test, expect } from "../fixtures/authenticatedTest"

test.describe("ops instance migration", () => {
  test("触发迁移、提交验收并执行切换与回切", async ({ page }) => {
    const baseRecord = {
      id: 101,
      source_env: "prod-a",
      target_env: "prod-b",
      status: "running",
      db_migration_status: "success",
      instance_acceptance_status: "pending",
      traffic_switch_status: "pending",
      traffic_rollback_status: "pending",
      dry_run: false,
      summary: "database migration completed; waiting instance acceptance",
    }

    await page.route("**/api/v1/admin/migration/runbooks/run", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { record: baseRecord } }),
      })
    })

    await page.route("**/api/v1/admin/migration/runbooks/101", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { record: baseRecord } }),
      })
    })

    await page.route("**/api/v1/admin/migration/runbooks/101/acceptance", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { record: { ...baseRecord, status: "success", instance_acceptance_status: "success", summary: "core checks passed" } } }),
      })
    })

    await page.route("**/api/v1/admin/migration/traffic/switch", async (route) => {
      const payload = JSON.parse(route.request().postData() || "{}")
      const rollback = !!payload.rollback
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            operation_id: rollback ? "op-rollback-1" : "op-switch-1",
            record: {
              ...baseRecord,
              status: "success",
              instance_acceptance_status: "success",
              traffic_switch_status: rollback ? "success" : "success",
              traffic_rollback_status: rollback ? "success" : "pending",
              summary: rollback ? "traffic rollback completed" : "traffic switch completed",
            },
          },
        }),
      })
    })

    await page.goto("/ops/migration")

    await expect(page.getByRole("heading", { name: "实例迁移中心" })).toBeVisible()
    await page.getByRole("button", { name: "触发迁移" }).click()
    await page.getByRole("button", { name: "提交验收" }).click()
    await page.getByRole("button", { name: "流量切换" }).click()
    await page.getByRole("button", { name: "回切" }).click()
    await expect(page.getByText("最近操作 ID：op-rollback-1")).toBeVisible()
  })
})

