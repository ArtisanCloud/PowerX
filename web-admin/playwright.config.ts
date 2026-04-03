import { defineConfig, devices } from '@playwright/test'

/**
 * @see https://playwright.dev/docs/test-configuration
 */
export default defineConfig({
  testDir: './tests/e2e',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  retries: 1,
  workers: 1, // 并行工作线程数，可以根据需要调整

  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:3300',
    storageState: './tests/e2e/.auth/admin.json',
    trace: 'on-first-retry',
    video: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'firefox',
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
    },
  ],

  // 全局设置
  globalSetup: './tests/e2e/auth.setup.ts',

  webServer: {
    command: 'POWERX_ENV=prod UPSTREAM=http://127.0.0.1:8080 WS_UPSTREAM=ws://127.0.0.1:8080/api/ws NUXT_PUBLIC_E2E_SKIP_AUTH=true npm run build && POWERX_ENV=prod UPSTREAM=http://127.0.0.1:8080 WS_UPSTREAM=ws://127.0.0.1:8080/api/ws NUXT_PUBLIC_E2E_SKIP_AUTH=true npx nuxt preview --host 127.0.0.1 --port 3300',
    url: 'http://127.0.0.1:3300',
    reuseExistingServer: !process.env.CI,
    timeout: 120 * 1000,
  },
})
