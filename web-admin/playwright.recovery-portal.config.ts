import { defineConfig } from '@playwright/test';
import { resolve } from 'node:path';
const port = Number(process.env.POWERX_RECOVERY_PORTAL_BROWSER_PORT || 3487);
export default defineConfig({
  testDir: './tests/e2e/recovery-portal',
  timeout: 40_000,
  workers: 1,
  retries: 0,
  reporter: 'list',
  outputDir: '../test-results/recovery-portal-browser',
  // Login uses a disposable fixture credential. Avoid recording it in traces.
  use: { baseURL: `http://127.0.0.1:${port}`, headless: true, trace: 'off', screenshot: 'only-on-failure' },
  webServer: {
    command: '../test-results/recovery-portal-bin/portal-fixture.test -test.run=^TestRecoveryPortalBrowserFixture$ -test.v',
    env: { ...process.env, POWERX_RECOVERY_PORTAL_BROWSER: '1', POWERX_RECOVERY_PORTAL_BROWSER_PORT: String(port), POWERX_RECOVERY_PORTAL_BROWSER_RECORD: resolve('../test-results/recovery-portal-browser-process.json') },
    url: `http://127.0.0.1:${port}/__powerx_recovery/health`,
    reuseExistingServer: false,
    timeout: 30_000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 10_000 },
  },
});
