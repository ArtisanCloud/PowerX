import { defineConfig } from '@playwright/test';
const port = Number(process.env.POWERX_CLIENT_APPS_TEST_PORT || 3543);
export default defineConfig({
  testDir: './tests/e2e/client-apps', workers: 1, retries: 0, timeout: 45000,
  outputDir: process.env.POWERX_CLIENT_APPS_ARTIFACTS || '../test-results/client-apps',
  reporter: [['list']],
  use: { baseURL: `http://127.0.0.1:${port}`, headless: true, actionTimeout: 10000, screenshot: 'only-on-failure', trace: 'retain-on-failure' },
  webServer: { command: 'node scripts/client-apps-browser-fixture.mjs', url: `http://127.0.0.1:${port}/powerx-bridge-client.js`, reuseExistingServer: false, timeout: 30000, gracefulShutdown: { signal: 'SIGTERM', timeout: 10000 } },
});
