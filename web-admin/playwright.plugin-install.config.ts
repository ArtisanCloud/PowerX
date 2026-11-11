import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e/plugins', testMatch: 'install-recovery.spec.ts', workers: 1, retries: 0, timeout: 60000,
  outputDir: '../tmp/pr427-review-20261111/browser', reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:39455', headless: true, trace: 'retain-on-failure',
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : {} },
  webServer: {
    command: 'node scripts/snapshot-browser-fixture.mjs',
    env: { POWERX_SNAPSHOT_BROWSER_PORT: '39455', POWERX_SNAPSHOT_BROWSER_BACKEND_PORT: '39456',
      POWERX_SNAPSHOT_WEB_ENTRY: process.env.POWERX_PLUGIN_INSTALL_UI_ENTRY || '../tmp/pr427-review-20261111/nuxt-output/server/index.mjs',
      POWERX_BROWSER_RESULTS_DIR: '../tmp/pr427-review-20261111/browser', POWERX_BROWSER_FIXTURE_OWNER: 'owned-plugin-install-review-browser' },
    url: 'http://127.0.0.1:39455/powerx-bridge-client.js?v=2', reuseExistingServer: false, timeout: 30000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 10000 },
  },
});
