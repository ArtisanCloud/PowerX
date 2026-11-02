import { defineConfig } from '@playwright/test';

const port = Number(process.env.POWERX_STORAGE_BROWSER_PORT || 3485);
const backendPort = Number(process.env.POWERX_STORAGE_BROWSER_BACKEND_PORT || 3486);
export default defineConfig({
  testDir: './tests/e2e/storage', workers: 1, retries: 0, timeout: 45000,
  outputDir: '../test-results/storage-browser', reporter: 'list',
  use: {
    baseURL: `http://127.0.0.1:${port}`, headless: true, trace: 'retain-on-failure', screenshot: 'only-on-failure',
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : {},
  },
  webServer: {
    command: 'node scripts/snapshot-browser-fixture.mjs',
    env: {
      POWERX_SNAPSHOT_BROWSER_PORT: String(port), POWERX_SNAPSHOT_BROWSER_BACKEND_PORT: String(backendPort),
      POWERX_SNAPSHOT_WEB_ENTRY: '.output-storage-validation/server/index.mjs',
      POWERX_BROWSER_RESULTS_DIR: '../test-results/storage-browser', POWERX_BROWSER_FIXTURE_OWNER: 'storage-browser-tests',
    },
    url: `http://127.0.0.1:${port}/powerx-bridge-client.js?v=2`, reuseExistingServer: false, timeout: 30000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 10000 },
  },
});
