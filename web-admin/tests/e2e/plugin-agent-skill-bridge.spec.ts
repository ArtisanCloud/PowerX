import { test, expect } from "./fixtures/authenticatedTest";

test.describe("plugin agent skill bridge", () => {
  test("平台 Agent 任务消费统一 Agent Runtime SSE 链路", async ({ page }) => {
    const agentRuntimeRequests: string[] = [];
    const directBusinessRequests: string[] = [];

    await page.route("**/creation/video-automation/ingest**", async (route) => {
      directBusinessRequests.push(route.request().url());
      await route.fulfill({ status: 500, body: "direct MediaX business API is forbidden in chat" });
    });

    await page.route("**/admin/agents/settings/active**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { profile: { provider: "mock-provider", model: "mock-model" } } }),
      });
    });

    await page.route("**/admin/agents/*/ai-setting**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ code: 200, message: "ok", data: { provider: "mock-provider", model: "mock-model", params: {} }, timestamp: Date.now() }),
      });
    });

    await page.route("**/admin/agents/teams/*/members**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ code: 200, message: "ok", data: { items: [], total: 0 }, timestamp: Date.now() }),
      });
    });

    await page.route("**/admin/agents/teams**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          code: 200,
          message: "ok",
          data: {
            items: [
              {
                id: 11,
                tenant_uuid: "px-root-tenant",
                parent_agent_id: 1001,
                team_name: "plugin-skill-bridge-demo",
                dispatch_mode: "parallel",
                default_failure_policy: "continue",
                status: "active",
              },
            ],
            total: 1,
          },
          timestamp: Date.now(),
        }),
      });
    });

    await page.route("**/admin/agents**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          code: 200,
          message: "ok",
          data: {
            items: [
              {
                id: 1001,
                uuid: "agent-plugin-bridge",
                key: "orchestrator.plugin_bridge",
                name: "Plugin Bridge Agent",
                description: "",
                source: "custom",
                scope: "tenant",
                visibility: "private",
                status: "active",
                meta: {},
              },
            ],
          },
          timestamp: Date.now(),
        }),
      });
    });

    await page.route("**/agents/sessions**", async (route) => {
      if (route.request().method() !== "POST") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ code: 200, message: "ok", data: { items: [] }, timestamp: Date.now() }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: 9101,
            agentId: 1001,
            agent_id: 1001,
            title: "Plugin Bridge Session",
            status: "active",
            meta: {},
          },
          code: 200,
          message: "ok",
          timestamp: Date.now(),
        }),
      });
    });

    await page.route("**/agents/stream/sse**", async (route) => {
      const url = new URL(route.request().url());
      if (url.searchParams.get("probe") === "1") {
        await route.fulfill({
          status: 200,
          contentType: "text/event-stream",
          headers: {
            "cache-control": "no-cache",
            connection: "close",
          },
          body: "event: ready\ndata: {\"type\":\"ready\"}\n\n",
        });
        return;
      }

      agentRuntimeRequests.push(route.request().url());
      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        headers: {
          "cache-control": "no-cache",
          connection: "keep-alive",
        },
        body: [
          `event: intent`,
          `data: {"type":"intent","tasks":[{"task_id":"skill-1","node_kind":"skill","node_ref":"mediax.video_rebuilder.cn"}]}`,
          ``,
          `event: plan`,
          `data: {"type":"plan","plan":{"tasks":[{"task_id":"skill-1","node_kind":"skill","node_ref":"mediax.video_rebuilder.cn"}]}}`,
          ``,
          `event: node_start`,
          `data: {"type":"node_start","node_kind":"skill","node_ref":"mediax.video_rebuilder.cn","status":"running"}`,
          ``,
          `event: node_end`,
          `data: {"type":"node_end","node_kind":"skill","node_ref":"mediax.video_rebuilder.cn","status":"completed"}`,
          ``,
          `event: final`,
          `data: {"type":"final","text":"已创建视频重构任务，任务号 video-automation-task-001。"}`,
          ``,
        ].join("\n"),
      });
    });

    await page.goto("/agent/team-tasks?team_id=11");
    await expect(page.getByText("团队任务", { exact: true }).first()).toBeVisible();
    await page.evaluate(async () => {
      await fetch("/api/v1/agents/stream/sse?q=bridge-check&agent_uuid=agent-plugin-bridge&session_id=9101", {
        method: "GET",
        headers: {
          Accept: "text/event-stream",
          Authorization: "Bearer playwright-token",
        },
      });
    });

    expect(agentRuntimeRequests.length).toBeGreaterThanOrEqual(1);
    expect(directBusinessRequests).toHaveLength(0);
  });
});
