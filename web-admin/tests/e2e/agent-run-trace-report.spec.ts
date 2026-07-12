import type { Locator } from "@playwright/test";
import { test, expect } from "./fixtures/authenticatedTest";

const setControlValue = async (locator: Locator, value: string) => {
  await locator.evaluate((element: HTMLInputElement | HTMLTextAreaElement, nextValue: string) => {
    const proto = element instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
    setter?.call(element, nextValue);
    element.dispatchEvent(new Event("input", { bubbles: true }));
    element.dispatchEvent(new Event("change", { bubbles: true }));
  }, value);
};

test.describe("agent run trace report", () => {
  test("root 页面查看 Message Trace、节点详情并下载报告", async ({ page }) => {
    const report = {
      report_scope: "message",
      format: "json",
      tenant_uuid: "tenant-e2e",
      session_id: "session-e2e",
      message_id: "message-e2e",
      run_id: "run-e2e",
      trace_id: "trace-e2e",
      generated_at: new Date().toISOString(),
      summary: {
        status: "completed",
        node_count: 1,
        event_count: 2,
        error_count: 0,
        duration_ms: 12,
      },
      timeline: [
        {
          trace_id: "trace-e2e",
          run_id: "run-e2e",
          tenant_uuid: "tenant-e2e",
          agent_id: "agent-e2e",
          session_id: "session-e2e",
          message_id: "message-e2e",
          node_id: "001_receive_message",
          node_seq: 1,
          node_kind: "receive_message",
          node_ref: "agent.stream",
          phase: "end",
          status: "success",
          duration_ms: 12,
          created_at: new Date().toISOString(),
        },
      ],
      nodes: [
        {
          node_id: "001_receive_message",
          node_seq: 1,
          node_kind: "receive_message",
          node_ref: "agent.stream",
          phase_status: "success",
          input_summary: { accepted: true },
          output_summary: { ok: true },
        },
      ],
      errors: [],
    };

    await page.route("**/*agent-traces/messages/message-e2e/report**", async (route) => {
      const url = new URL(route.request().url());
      if (url.searchParams.get("format") === "markdown") {
        await route.fulfill({
          status: 200,
          contentType: "text/markdown",
          body: "# Agent Run Report\n\n- Run: `run-e2e`\n",
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: report }),
      });
    });

    await page.route("**/admin/tenants**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          code: 200,
          message: "ok",
          data: {
            items: [
              {
                id: 1,
                uuid: "px-root-tenant",
                name: "PowerX Root Tenant",
                domain: "root.powerx.local",
                status: "active",
                createdAt: "",
                updatedAt: "",
              },
              {
                id: 2,
                uuid: "tenant-e2e",
                name: "E2E Tenant",
                domain: "e2e.powerx.local",
                status: "active",
                createdAt: "",
                updatedAt: "",
              },
            ],
            pagination: { total: 2, page: 1, page_size: 100, pages: 1 },
          },
          timestamp: Date.now(),
        }),
      });
    });

    await page.goto("/agent/traces");
    await expect(page.locator("h1", { hasText: "Agent 运行追踪" })).toBeVisible();
    await expect(page.getByText(/PowerX Root Tenant/)).toBeVisible();
    await setControlValue(page.locator('input[placeholder="session id"]'), "session-e2e");
    await setControlValue(page.locator('input[placeholder="message id"]'), "message-e2e");
    await expect(page.locator('input[placeholder="message id"]')).toHaveValue("message-e2e");
    await page.getByRole("button", { name: "查询" }).click();

    await expect(page.getByText("completed", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: /receive_message\s+agent\.stream/ }).click();
    await expect(page.locator("pre").first()).toContainText("accepted");
    await expect(page.getByRole("link", { name: "JSON" })).toHaveAttribute("href", /download=json/);
    await expect(page.getByRole("link", { name: "报告" })).toHaveAttribute("href", /format=markdown/);
  });
});
