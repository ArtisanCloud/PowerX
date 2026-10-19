import { defineConfig } from '@playwright/test';
const port = Number(process.env.POWERX_NOTIFICATION_BROWSER_PORT || 3491);
export default defineConfig({
  testDir: './tests/e2e/notifications', timeout: 30_000, retries: 0, workers: 1,
  outputDir: '../test-results/notifications-browser', reporter: 'list',
  use: {baseURL: `http://127.0.0.1:${port}`, headless: true, trace: 'retain-on-failure', screenshot: 'only-on-failure'},
  projects: [{name: 'chromium', use: {browserName: 'chromium'}}],
  webServer: {command: 'node scripts/notification-browser-fixture.mjs', url: `http://127.0.0.1:${port}/powerx-bridge-client.js?v=2`, reuseExistingServer: false, timeout: 30_000, gracefulShutdown: {signal: 'SIGTERM', timeout: 10_000}},
});
