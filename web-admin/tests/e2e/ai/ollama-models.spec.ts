import { test, expect } from "@playwright/test";

for (const narrow of [false, true]) {
  test(`Ollama instance discovery, refresh and recovery (${narrow ? "narrow" : "desktop"})`, async ({ page }, info) => {
    await page.setViewportSize(narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
    const tenant = "00000000-0000-4000-8000-000000000001";
    await page.addInitScript(tenant => {
      const expiresAt = Date.now() + 3600_000;
      const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
      localStorage.setItem("powerx.session", JSON.stringify({ version: 1, status: "active", sessionId: "ollama-browser", revision: 1, accessToken: token, refreshToken: "fixture", tokenType: "Bearer", expiresAt, scope: "access", tenantUuid: tenant }));
      localStorage.setItem("welcome_guide_completed", "true");
      localStorage.setItem("px_locale", "zh");
    }, tenant);
    const requests: Record<string, string>[] = [];
    const connectionRequests: any[] = [];
    const runRequests: any[] = [];
    const errors: string[] = [];
    page.on("pageerror", error => errors.push(error.message));
    let offline = false, empty = false;
    let textModels = ["qwen3:14b", "qwen3:32b"];
    let delayNextModelRequest = true;
    let releaseModelRequest: (() => void) | undefined;
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith("/api/"), async route => {
      const url = new URL(route.request().url()), path = url.pathname;
      if (path.includes("/_nuxt_icon/")) return route.fallback();
      let data: unknown = {};
      const profile = (modality: string) => ({ env: "dev", modality, provider: "ollama", model: modality === "embedding" ? "bge-m3:latest" : "qwen3:8b", defaults: {} });
      if (path.endsWith("/setup/status")) data = { configured: true, requires_login: true, install_status: "installed" };
      else if (path.endsWith("/me/context")) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: "Ollama 验收", status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: "验收", member_id: 1, is_admin: true }] };
      else if (path.endsWith("/menus")) data = { menus: [{ id: "ai", title: "模型配置", path: "/settings/ai", visible: true, origin: "system" }] };
      else if (path.endsWith("/agents/providers")) data = { providers: [{ id: "ollama", name: "Ollama (Local)", auth: { scheme: "none", fields: ["base_url"], defaults: { base_url: "http://127.0.0.1:11434" } } }] };
      else if (path.endsWith("/agents/settings/profiles")) data = { env: "dev", profiles: [profile("llm"), profile("embedding")] };
      else if (path.endsWith("/agents/settings/credentials")) data = { env: "dev", credentials: [{ provider: "ollama", data: { base_url: "http://127.0.0.1:11434" } }] };
      else if (path.endsWith("/agents/settings/active")) data = { profile: profile(url.searchParams.get("modality") || "llm") };
      else if (path.endsWith("/agents/test/connection")) {
        connectionRequests.push(route.request().postDataJSON());
        data = { ok: true, verification: { provider: "ollama", endpoint_reachable: true, api_key_verified: false, authentication: "not_required", model_access_verified: false, generation_submitted: false } };
      }
      else if (path.endsWith("/agents/test/call")) {
        runRequests.push(route.request().postDataJSON());
        data = { ok: false };
      }
      else if (path.endsWith("/agents/models")) {
        requests.push(Object.fromEntries(url.searchParams));
        if (delayNextModelRequest) {
          delayNextModelRequest = false;
          await new Promise<void>(resolve => { releaseModelRequest = resolve; });
        }
        if (offline) return route.fulfill({ status: 502, contentType: "application/json", body: JSON.stringify({ code: 502, error_code: "AI_MODEL_DISCOVERY_UNREACHABLE", message: "AI_MODEL_DISCOVERY_UNREACHABLE" }) });
        data = { models: empty ? [] : url.searchParams.get("modality") === "embedding" ? ["bge-m3:latest", "mxbai-embed-large:latest"] : textModels };
      }
      else if (path.endsWith("/skills/source-policy")) data = { allowlist: ["builtin", "plugin"], effective_source: "default" };
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ code: 200, data }) });
    });
    await page.goto("/settings/ai");
    if (narrow) await page.getByRole("complementary").getByRole("button", { name: "收起", exact: true }).first().click();
    const model = page.getByTestId("ai-model-select");
    const provider = page.getByTestId("ai-provider-select");
    const base = page.getByTestId("ai-base-url");
    const refresh = page.getByRole("button", { name: "刷新模型", exact: true });
    try {
      await expect.poll(() => Boolean(releaseModelRequest)).toBe(true);
      await expect(provider).toHaveAttribute("aria-busy", "true");
      await expect(model).toHaveAttribute("aria-busy", "true");
      for (const [select, icon] of [[provider, "building-library"], [model, "cpu-chip"]] as const) {
        const leadingIcon = select.locator(`[class*="${icon}"]`).first();
        await expect(leadingIcon).toBeVisible();
        expect(await leadingIcon.evaluate(element => getComputedStyle(element).animationName)).toBe("none");
      }
      await expect(page.getByText("正在加载供应商…", { exact: true })).toBeVisible();
      await expect(page.getByText("正在加载模型…", { exact: true })).toBeVisible();
      await expect(refresh.locator(".animate-spin")).toHaveCount(1);
      await page.screenshot({ path: info.outputPath(`ollama-loading-${narrow ? "narrow" : "desktop"}.png`), fullPage: true });
    } finally {
      releaseModelRequest?.();
    }
    await expect(refresh).toBeEnabled();
    await expect(model).not.toContainText("qwen3:8b");
    await expect(page.getByText(/实例中未发现当前型号或其能力不匹配/)).toHaveCount(0);
    const connection = page.getByRole("button", { name: "测试连接", exact: true });
    const quick = page.getByRole("button", { name: "试跑文本（调用模型）", exact: true });
    await expect(connection).toBeEnabled();
    await expect(quick).toBeDisabled();
    await expect(page.getByTestId("ai-save-settings")).toBeDisabled();
    await base.click();
    await base.fill("");
    await base.press("Tab");
    await expect(refresh).toBeEnabled();
    await connection.click();
    await expect(page.getByText("入口可访问；此服务不要求 API Key，未验证 Key。未提交生成请求。", { exact: true })).toBeVisible();
    expect(connectionRequests).toHaveLength(1);
    expect(connectionRequests[0].llm).toMatchObject({ provider: "ollama", model: "", baseURL: "", apiKey: "" });
    expect(runRequests).toHaveLength(0);
    await page.screenshot({ path: info.outputPath(`ollama-provider-connection-${narrow ? "narrow" : "desktop"}.png`), fullPage: true });
    await model.click();
    await expect(page.getByRole("option", { name: "qwen3:8b", exact: true })).toHaveCount(0);
    await page.getByRole("option", { name: "qwen3:14b", exact: true }).click();
    await expect(page.getByRole("option")).toHaveCount(0);
    await expect(quick).toBeEnabled();
    await expect(page.getByTestId("ai-save-settings")).toBeEnabled();
    await base.click();
    await base.fill("http://second.invalid:11434");
    await base.press("Tab");
    await expect.poll(() => requests.at(-1)?.base_url).toBe("http://second.invalid:11434");
    await expect(refresh).toBeEnabled();
    await model.click();
    await expect(page.getByRole("option", { name: "qwen3:32b", exact: true })).toBeVisible();
    await page.getByRole("option", { name: "qwen3:32b", exact: true }).click();
    await page.getByRole("button", { name: "向量嵌入", exact: true }).click();
    await expect(model).toContainText("bge-m3:latest");
    await page.getByRole("button", { name: "LLM 文本", exact: true }).click();
    await expect(model).toContainText("qwen3:32b");
    await expect(base).toHaveValue("http://second.invalid:11434");
    await expect(refresh).toBeEnabled();
    const count = requests.length;
    await page.waitForTimeout(1200);
    expect(requests.length).toBe(count);
    await refresh.scrollIntoViewIfNeeded();
    const bounds = await refresh.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(narrow ? 390 : 1440);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(narrow ? 390 : 1440);
    await page.screenshot({ path: info.outputPath(`ollama-${narrow ? "narrow" : "desktop"}.png`), fullPage: true });
    offline = true;
    await refresh.click();
    await expect(page.getByText(/无法读取 Ollama 实例模型及能力/)).toBeVisible();
    await expect(model).toContainText("qwen3:32b");
    await expect(base).toHaveValue("http://second.invalid:11434");
    await page.screenshot({ path: info.outputPath(`ollama-error-${narrow ? "narrow" : "desktop"}.png`), fullPage: true });
    offline = false; empty = true;
    await page.getByRole("button", { name: "重试", exact: true }).click();
    await expect(page.getByText("当前 Ollama 实例没有支持此模态的模型。", { exact: true })).toBeVisible();
    await expect(page.getByText(/实例中未发现当前型号或其能力不匹配/)).toHaveCount(0);
    await expect(model).not.toContainText("qwen3:32b");
    await expect(model).toBeDisabled();
    await page.screenshot({ path: info.outputPath(`ollama-empty-${narrow ? "narrow" : "desktop"}.png`), fullPage: true });
    empty = false; textModels = ["qwen3:32b"];
    await refresh.click();
    await expect(model).toBeEnabled();
    await expect(model).not.toContainText("qwen3:32b");
    await model.click();
    await page.getByRole("option", { name: "qwen3:32b", exact: true }).click();
    // 实例刷新后移除当前型号，选择框必须立即回到待选择状态。
    await expect(page.getByRole("option")).toHaveCount(0);
    textModels = ["qwen3:14b"];
    await refresh.click();
    await expect(model).not.toContainText("qwen3:32b");
    await model.click();
    await expect(page.getByRole("option", { name: "qwen3:32b", exact: true })).toHaveCount(0);
    await page.getByRole("option", { name: "qwen3:14b", exact: true }).click();
    expect(errors).toEqual([]);
  });
}
