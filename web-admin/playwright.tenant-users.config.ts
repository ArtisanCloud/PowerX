import { defineConfig, devices } from '@playwright/test';

// 使用现有本地前端与浏览器 API fixture，不向开发库写入账号。
export default defineConfig({
  testDir: './tests/e2e/iam',
  testMatch: /(?:tenant-user-creation|users-pagination|user-departments)\.spec\.ts/,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  workers: 1,
  use: { baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:3030', trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
