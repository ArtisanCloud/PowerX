import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests/e2e/root-role",
  timeout: 60_000,
  retries: 0,
  workers: 1,
  outputDir: "../test-results/root-role-browser",
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:3495",
    headless: true,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
  webServer: {
    command: "node scripts/root-role-browser-fixture.mjs",
    url: "http://127.0.0.1:3495/powerx-bridge-client.js?v=2",
    reuseExistingServer: false,
    timeout: 30_000,
    gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
  },
});
