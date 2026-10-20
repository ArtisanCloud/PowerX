import { defineConfig } from "@playwright/test";

// 使用已启动的开发前端；所有 API 都在独立浏览器上下文中拦截，不写真实数据。
export default defineConfig({
  testDir: "./tests/e2e/agent-sessions",
  timeout: 45_000,
  workers: 1,
  retries: 0,
  outputDir: "../test-results/agent-sessions-browser",
  reporter: "list",
  use: {
    baseURL: process.env.POWERX_AGENT_BROWSER_URL || "http://127.0.0.1:3030",
    headless: true,
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
});
