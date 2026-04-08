import { test, expect } from "@playwright/test";

test.describe("prod port proxy consistency", () => {
  test("api proxy target should pin to 8080 and not fallback to 8077", async ({ request }) => {
    const resp = await request.get("/api/v1/admin/setup/status");
    const target = String(resp.headers()["x-px-proxy-target"] || "");
    const proxyHit = String(resp.headers()["x-px-proxy-hit"] || "");

    expect(proxyHit).toBe("1");
    expect(target).toContain("127.0.0.1:8080");
    expect(target).not.toContain("8077");
  });
});

