import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e/storage', testMatch: 'archive-native.spec.ts', workers: 1, retries: 0, timeout: 180000,
  outputDir: process.env.POWERX_ARCHIVE_UI_ARTIFACTS || '../tmp/archive-native-ui/browser', reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:3487', actionTimeout: 10000, headless: true, trace: 'off', screenshot: 'off', launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : {} },
  webServer: {
    command: 'node scripts/snapshot-browser-fixture.mjs',
    env: { POWERX_SNAPSHOT_BROWSER_PORT: '3487', POWERX_SNAPSHOT_BROWSER_BACKEND_PORT: '3488', POWERX_SNAPSHOT_WEB_ENTRY: process.env.POWERX_ARCHIVE_UI_WEB_ENTRY || '.output-storage-archive-validation/server/index.mjs', POWERX_BROWSER_RESULTS_DIR: process.env.POWERX_ARCHIVE_UI_ARTIFACTS || '../tmp/archive-native-ui/browser', POWERX_BROWSER_FIXTURE_OWNER: 'owned-native-archive-ui-browser' },
    url: 'http://127.0.0.1:3487/powerx-bridge-client.js?v=2', reuseExistingServer: false, timeout: 30000, gracefulShutdown: { signal: 'SIGTERM', timeout: 10000 },
  },
});
