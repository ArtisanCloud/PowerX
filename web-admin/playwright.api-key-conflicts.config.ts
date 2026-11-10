import { defineConfig, devices } from '@playwright/test';
export default defineConfig({
  testDir: './tests/e2e/iam',
  testMatch: /api-key-conflicts\.spec\.ts/,
  timeout: 90_000,
  expect: { timeout: 15_000 },
  workers: 1,
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:3030',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
