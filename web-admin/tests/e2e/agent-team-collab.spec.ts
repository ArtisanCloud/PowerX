import { test, expect } from "./fixtures/authenticatedTest";

test.describe("agent team collaboration", () => {
  test("团队任务页展示 Intent/Plan/Node 且返回部分成功汇总", async ({ page }) => {
    await page.route("**/api/v1/admin/agents/settings/active**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            profile: {
              provider: "mock-provider",
              model: "mock-model",
            },
          },
        }),
      });
    });

    await page.route("**/api/v1/admin/agents/teams/*/members**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            items: [
              { id: 1, team_id: 11, child_agent_id: 2001, role: "retriever", priority: 1, enabled: true },
              { id: 2, team_id: 11, child_agent_id: 2002, role: "reviewer", priority: 1, enabled: true },
            ],
            total: 2,
          },
        }),
      });
    });

    await page.route("**/api/v1/admin/agents/teams**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            items: [
              {
                id: 11,
                tenant_uuid: "px-root-tenant",
                parent_agent_id: 1001,
                team_name: "incident-a2a-demo",
                dispatch_mode: "parallel",
                default_failure_policy: "continue",
                status: "active",
              },
            ],
            total: 1,
          },
        }),
      });
    });

    await page.route("**/api/v1/admin/agents**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            items: [
              { id: 1001, uuid: "agent-parent-uuid", key: "orchestrator.main", name: "Orchestrator", description: "", source: "custom", scope: "tenant", visibility: "private", status: "active", meta: {} },
              { id: 2001, uuid: "agent-child-a", key: "retriever.incident", name: "Retriever", description: "", source: "custom", scope: "tenant", visibility: "private", status: "active", meta: {} },
              { id: 2002, uuid: "agent-child-b", key: "reviewer.incident", name: "Reviewer", description: "", source: "custom", scope: "tenant", visibility: "private", status: "active", meta: {} },
            ],
          },
        }),
      });
    });

    await page.route("**/api/v1/agents/sessions**", async (route) => {
      if (route.request().method() === "POST") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              id: 9001,
              createdAt: "2026-04-22T10:00:00Z",
              updatedAt: "2026-04-22T10:00:00Z",
              DeletedAt: null,
              agentId: 1001,
              userId: 1,
              title: "A2A E2E Session",
              singleton: false,
              ttlDays: 30,
              maxKB: 1024,
              maxTokens: 120000,
              summary: "",
              status: "active",
              latestAt: "2026-04-22T10:00:00Z",
              expiredAt: "2026-05-22T10:00:00Z",
              meta: {},
            },
          }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          code: 200,
          message: "ok",
          data: { items: [] },
          timestamp: Date.now(),
        }),
      });
    });

    await page.route("**/api/v1/agents/stream/sse**", async (route) => {
      const url = new URL(route.request().url());
      const isProbe = url.searchParams.get("probe") === "1";
      const body = isProbe
        ? `event: ack\ndata: {"type":"ack"}\n\n`
        : [
            `event: start`,
            `data: {"type":"start"}`,
            ``,
            `event: intent`,
            `data: {"type":"intent","tasks":[{"task_id":"intent-1","flow_id":"agent.retriever","params":{"_candidate_name":"Retriever","_candidate_desc":"检索最近24小时变更"}},{"task_id":"intent-2","flow_id":"agent.reviewer","params":{"_candidate_name":"Reviewer","_candidate_desc":"生成修复建议并复核"}}]}`,
            ``,
            `event: plan`,
            `data: {"type":"plan","plan":{"tasks":[{"task_id":"handoff-a"},{"task_id":"handoff-b"}]}}`,
            ``,
            `event: node_start`,
            `data: {"type":"node_start","node_id":"n1","task_id":"handoff-a","node_kind":"agent_handoff","node_ref":"agent.retriever","status":"running"}`,
            ``,
            `event: node_end`,
            `data: {"type":"node_end","node_id":"n1","task_id":"handoff-a","node_kind":"agent_handoff","node_ref":"agent.retriever","status":"completed"}`,
            ``,
            `event: node_start`,
            `data: {"type":"node_start","node_id":"n2","task_id":"handoff-b","node_kind":"agent_handoff","node_ref":"agent.reviewer","status":"running"}`,
            ``,
            `event: node_end`,
            `data: {"type":"node_end","node_id":"n2","task_id":"handoff-b","node_kind":"agent_handoff","node_ref":"agent.reviewer","status":"failed","error":"timeout"}`,
            ``,
            `event: final`,
            `data: {"type":"final","text":"部分成功：已完成变更检索；复核节点失败（timeout）。综合建议：先回滚再观察。","step_id":"done"}`,
            ``,
          ].join("\n");

      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body,
        headers: {
          "cache-control": "no-cache",
          connection: "keep-alive",
        },
      });
    });

    await page.goto("/agent/team-tasks?team_id=11");

    await expect(page.getByText("团队任务工作台")).toBeVisible();
    const input = page.locator("textarea").first();
    await input.fill("请先检索变更，再给出建议并复核。");
    await page.locator('button[aria-label="发送"]').first().click();

    await expect(page.getByText("执行过程")).toBeVisible();
    await expect(page.getByText("Intent：")).toBeVisible();
    await expect(page.getByText("Plan：")).toBeVisible();
    await expect(page.getByText("子智能体分发")).toBeVisible();
    await expect(page.getByText("失败")).toBeVisible();
    await expect(page.getByText("部分成功：已完成变更检索")).toBeVisible();
  });
});
