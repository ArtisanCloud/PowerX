import { defineConfig } from '@playwright/test';

const port = Number(process.env.POWERX_SESSION_TEST_PORT || 3473);
const baseURL = `http://127.0.0.1:${port}`;

// Run these projects on the native OS. A Windows user agent on macOS is not
// evidence of Windows compatibility. Chrome and Edge must be installed there.
export default defineConfig({
  testDir: './tests/e2e/session',
  timeout: 30_000,
  expect: { timeout: 10_000 },
  workers: 1,
  retries: 0,
  outputDir: '../test-results/session-browsers',
  reporter: [['list'], ['json', { outputFile: '../test-results/session-browsers.json' }]],
  use: { baseURL, headless: true, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
    { name: 'chrome', use: { browserName: 'chromium', channel: 'chrome' } },
    { name: 'edge', use: { browserName: 'chromium', channel: 'msedge' } },
  ],
  webServer: {
    command: 'node scripts/session-browser-fixture.mjs',
    url: `${baseURL}/powerx-bridge-client.js?v=2`,
    reuseExistingServer: false,
    timeout: 30_000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 10_000 },
  },
});
