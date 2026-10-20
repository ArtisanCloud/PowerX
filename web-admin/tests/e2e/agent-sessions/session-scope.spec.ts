import { writeFile } from "node:fs/promises";
import { test, expect } from "@playwright/test";

for (const viewport of [
  { width: 1440, height: 1000 },
  { width: 390, height: 844 },
]) {
  test(`separates individual, team and unclassified sessions at ${viewport.width}px`, async ({
    browser,
    baseURL,
  }, info) => {
    const tenant = "f23491d2-b8cd-4107-a17b-301a44168aa6";
    const agentUUID = "ff08c8c4-dc14-4a83-aaf2-9b966e20b8b1";
    const expiresAt = Date.now() + 3_600_000;
    const auth = {
      version: 1,
      sessionId: "agent-browser-fixture",
      revision: 1,
      status: "active",
      accessToken: `header.${Buffer.from(JSON.stringify({ tid: tenant, exp: Math.floor(expiresAt / 1000) })).toString("base64url")}.fixture`,
      refreshToken: "fixture",
      tokenType: "Bearer",
      expiresAt,
      scope: "access",
      tenantUuid: tenant,
    };
    const context = await browser.newContext({
      baseURL,
      viewport,
      locale: "zh-CN",
      storageState: {
        cookies: [],
        origins: [
          {
            origin: baseURL!,
            localStorage: [
              { name: "welcome_guide_completed", value: "true" },
              { name: "powerx.session", value: JSON.stringify(auth) },
            ],
          },
        ],
      },
    });
    const page = await context.newPage();
    const errors: string[] = [];
    const consoleErrors: string[] = [];
    page.on("pageerror", (error) => {
      errors.push(error.message);
      console.error("PAGE_ERROR", error.message);
    });
    page.on("console", (message) => {
      if (message.type() === "error") {
        consoleErrors.push(message.text());
        console.error("BROWSER_ERROR", message.text());
      }
    });
    const sessions = [
      {
        id: 101,
        sessionType: "agent",
        teamId: 0,
        title: "独立会话记录",
        agentId: 8,
      },
      {
        id: 103,
        sessionType: "team",
        teamId: 3,
        title: "团队甲会话记录",
        agentId: 8,
      },
      {
        id: 104,
        sessionType: "team",
        teamId: 4,
        title: "团队乙会话记录",
        agentId: 8,
      },
      {
        id: 199,
        sessionType: "legacy",
        teamId: 0,
        title: "待确认旧会话记录",
        agentId: 8,
      },
    ];
    const requests: {
      path: string;
      method: string;
      params: Record<string, string>;
      body?: any;
    }[] = [];
    await context.routeWebSocket(/\/api\/ws(?:\?|$)/, (socket) =>
      socket.close(),
    );
    await context.route("**/api/**", async (route) => {
      const request = route.request(),
        url = new URL(request.url()),
        path = url.pathname;
      if (!path.startsWith("/api/") || path.startsWith("/api/_nuxt_icon/"))
        return route.continue();
      const headers = {
        "access-control-allow-origin": new URL(baseURL!).origin,
        "access-control-allow-credentials": "true",
        "access-control-allow-methods": "GET,POST,PATCH,DELETE,OPTIONS",
        "access-control-allow-headers": "*",
      };
      if (request.method() === "OPTIONS")
        return route.fulfill({ status: 204, headers });
      const body = request.postDataJSON?.();
      requests.push({
        path,
        method: request.method(),
        params: Object.fromEntries(url.searchParams),
        body,
      });
      const json = (data: unknown) =>
        route.fulfill({ headers, json: { code: 200, message: "ok", data } });
      if (path.endsWith("/setup/status"))
        return json({
          configured: true,
          requires_login: true,
          restart_required: false,
          install_status: "installed",
        });
      if (path.endsWith("/user/auth/me/context"))
        return json({
          is_root: true,
          current_tenant_uuid: tenant,
          current_member_id: 1,
          user: {
            id: 1,
            email: "fixture@example.invalid",
            display_name: "Fixture",
            status: 1,
            is_root: true,
          },
          members: [
            {
              tenant_uuid: tenant,
              tenant_name: "Fixture",
              member_id: 1,
              is_admin: true,
            },
          ],
        });
      if (path.endsWith("/admin/menus"))
        return json({
          categories: [
            {
              id: "agent",
              title: "智能体",
              children: [
                {
                  id: "sessions",
                  title: "智能会话",
                  path: "/agent/sessions",
                  origin: "core",
                  visible: true,
                },
                {
                  id: "teams",
                  title: "团队任务",
                  path: "/agent/team-tasks",
                  origin: "core",
                  visible: true,
                },
              ],
            },
          ],
        });
      if (path.endsWith("/admin/agents"))
        return json({
          items: [
            {
              id: 8,
              uuid: agentUUID,
              name: "共用负责人",
              key: "browser.leader",
              status: "active",
              env: "dev",
              source: "core",
              meta: {},
            },
          ],
        });
      if (path.endsWith("/admin/agents/teams"))
        return json({
          items: [3, 4].map((id) => ({
            id,
            tenant_uuid: tenant,
            parent_agent_id: 8,
            team_key: `fixture.${id}`,
            display_name_i18n: { "zh-CN": id === 3 ? "团队甲" : "团队乙" },
            status: "active",
          })),
        });
      if (path.endsWith("/settings/active") || path.endsWith("/ai-setting"))
        return json({ provider: "fixture", model: "fixture", params: {} });
      if (path.endsWith("/agents/sessions")) {
        if (request.method() === "POST") {
          const session = {
            id: 200 + sessions.length,
            title: "未命名会话",
            agentId: 8,
            sessionType: body.sessionType,
            teamId: body.teamId,
          };
          sessions.push(session);
          return json(session);
        }
        return json({
          items: sessions.filter(
            (s) =>
              s.sessionType === url.searchParams.get("session_type") &&
              (s.sessionType !== "team" ||
                String(s.teamId) === url.searchParams.get("team_id")),
          ),
        });
      }
      const messages = path.match(/\/agents\/sessions\/(\d+)\/messages$/);
      if (messages) {
        const session = sessions.find((s) => s.id === Number(messages[1]))!;
        return json({
          items: [
            {
              id: session.id * 2,
              sessionId: session.id,
              agentId: 8,
              role: "user",
              content: `${session.title}的提问`,
              createdAt: new Date().toISOString(),
              meta: {},
            },
            {
              id: session.id * 2 + 1,
              sessionId: session.id,
              agentId: 8,
              role: "assistant",
              content: `${session.title}的回答`,
              createdAt: new Date().toISOString(),
              meta: {},
            },
          ],
        });
      }
      if (path.endsWith("/agents/stream/sse"))
        return route.fulfill({
          headers,
          contentType: "text/event-stream",
          body: ': connected\n\nevent: end\ndata: {"success":true}\n\n',
        });
      return json({ items: [], total: 0 });
    });
    try {
      await page.goto("/agent/sessions");
      await expect(page.getByText("启动中…", { exact: true })).toHaveCount(0);
      const skip = page.getByRole("button", { name: "跳过引导", exact: true });
      if (await skip.isVisible()) await skip.click();
      const collapse = page
        .getByRole("complementary")
        .getByRole("button", { name: "收起", exact: true })
        .first();
      const workspace = page.getByTestId("agent-workspace");
      await expect(
        workspace.getByText("独立会话记录", { exact: true }),
      ).toBeVisible({ timeout: 20000 });
      if (viewport.width < 768) {
        await expect(page.getByText("启动中…", { exact: true })).toHaveCount(0);
        const bounds = await collapse.boundingBox();
        if (bounds && bounds.x >= 0) await collapse.click();
        await expect
          .poll(async () => {
            const b = await page.getByRole("complementary").boundingBox();
            return b ? b.x + b.width : 0;
          })
          .toBeLessThanOrEqual(2);
      }
      await expect(
        workspace.getByText("团队甲会话记录", { exact: true }),
      ).toHaveCount(0);
      await workspace.getByText("独立会话记录", { exact: true }).click();
      await expect(
        workspace.getByText("独立会话记录的回答", { exact: true }),
      ).toBeVisible();
      await workspace
        .getByRole("tab", { name: "待确认历史", exact: true })
        .click();
      await workspace.getByText("待确认旧会话记录", { exact: true }).click();
      await expect(
        workspace.getByText("待确认旧会话记录的提问", { exact: true }),
      ).toBeVisible();
      await expect(
        workspace.getByText("这段历史的会话归属待确认", { exact: true }),
      ).toBeVisible();
      await expect(
        workspace.getByRole("button", { name: "新建会话", exact: true }),
      ).toBeDisabled();
      await expect(workspace.locator("textarea")).toBeDisabled();
      await workspace
        .getByRole("tab", { name: "当前会话", exact: true })
        .click();
      await expect(
        workspace.getByText("独立会话记录的回答", { exact: true }),
      ).toBeVisible();
      await workspace
        .getByRole("link", { name: "团队任务", exact: true })
        .click();
      await expect(
        workspace.getByText("团队甲会话记录", { exact: true }),
      ).toBeVisible();
      await expect(
        workspace.getByText("独立会话记录", { exact: true }),
      ).toHaveCount(0);
      await workspace.getByText("团队甲会话记录", { exact: true }).click();
      await expect(
        workspace.getByText("团队甲会话记录的回答", { exact: true }),
      ).toBeVisible();
      await workspace
        .getByRole("button", { name: "新建任务", exact: true })
        .click();
      await expect(
        workspace.getByText("未命名会话", { exact: true }),
      ).toBeVisible();
      expect(
        requests
          .filter(
            (r) => r.path.endsWith("/agents/sessions") && r.method === "POST",
          )
          .at(-1)?.body,
      ).toMatchObject({ sessionType: "team", teamId: 3 });
      await page.goto("/agent/team-tasks?team_id=4");
      await expect(
        workspace.getByText("团队乙会话记录", { exact: true }),
      ).toBeVisible();
      if (viewport.width < 768) {
        await expect(page.getByText("启动中…", { exact: true })).toHaveCount(0);
        const bounds = await collapse.boundingBox();
        if (bounds && bounds.x >= 0) await collapse.click();
        await expect
          .poll(async () => {
            const b = await page.getByRole("complementary").boundingBox();
            return b ? b.x + b.width : 0;
          })
          .toBeLessThanOrEqual(2);
      }
      await expect(
        workspace.getByText("团队甲会话记录", { exact: true }),
      ).toHaveCount(0);
      await workspace
        .getByRole("button", { name: "新建任务", exact: true })
        .click();
      await expect(
        workspace.getByText("未命名会话", { exact: true }),
      ).toBeVisible();
      await workspace.getByText("团队乙会话记录", { exact: true }).click();
      await expect(workspace.getByText("团队乙会话记录的回答", { exact: true })).toBeVisible();
      if (viewport.width < 768) {
        await workspace.getByRole("button", { name: "收缩面板", exact: true }).click();
        await expect(workspace.getByText("团队乙会话记录的回答", { exact: true })).toBeVisible();
        await workspace.getByRole("button", { name: "展开面板", exact: true }).click();
        await expect(workspace.getByText("团队乙会话记录的回答", { exact: true })).toBeVisible();
        await workspace.getByRole("button", { name: "收缩面板", exact: true }).click();
      }
      const box = await workspace.boundingBox();
      expect(box!.width).toBeGreaterThan(200);
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width + 2);
      await expect(page.getByText("启动中…", { exact: true })).toHaveCount(0);
      const composer = await workspace.locator("textarea").boundingBox();
      expect(composer!.width).toBeGreaterThan(180);
      expect(composer!.x + composer!.width).toBeLessThanOrEqual(
        viewport.width + 2,
      );
      await page.screenshot({
        path: info.outputPath(`workspace-${viewport.width}.png`),
        fullPage: true,
      });
      expect(errors).toEqual([]);
      expect(consoleErrors).toEqual([]);
    } finally {
      await writeFile(
        info.outputPath("browser-diagnostics.json"),
        JSON.stringify(
          {
            url: page.url(),
            errors,
            requests,
            text: await page
              .locator("body")
              .innerText()
              .catch(() => ""),
            html: (await page.content().catch(() => "closed")).slice(-3000),
          },
          null,
          2,
        ),
      );
      await info.attach("browser-diagnostics", {
        body: JSON.stringify(
          {
            url: page.url(),
            errors,
            requests,
            text: await page
              .locator("body")
              .innerText()
              .catch(() => ""),
          },
          null,
          2,
        ),
        contentType: "application/json",
      });
      await context.close();
    }
  });
}
