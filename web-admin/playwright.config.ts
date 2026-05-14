import { defineConfig, devices } from '@playwright/test'

const webHost = process.env.PLAYWRIGHT_WEB_HOST || '127.0.0.1'
const webPort = Number(process.env.PLAYWRIGHT_WEB_PORT || 3300)
const defaultBaseURL = `http://${webHost}:${webPort}`
const skipWebServer = process.env.PLAYWRIGHT_SKIP_WEBSERVER === '1'

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
    baseURL: process.env.PLAYWRIGHT_BASE_URL || defaultBaseURL,
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

  webServer: skipWebServer
    ? undefined
    : {
        command: `POWERX_ENV=prod POWERX_BACKEND=http://127.0.0.1:8080 NUXT_PUBLIC_WS_ORIGIN=ws://127.0.0.1:8080 NUXT_PUBLIC_WS_PATH=/api/ws NUXT_PUBLIC_POWERX_CORE_BASE=http://127.0.0.1:8080 NUXT_PUBLIC_E2E_SKIP_AUTH=true npm run build && POWERX_ENV=prod POWERX_BACKEND=http://127.0.0.1:8080 NUXT_PUBLIC_WS_ORIGIN=ws://127.0.0.1:8080 NUXT_PUBLIC_WS_PATH=/api/ws NUXT_PUBLIC_POWERX_CORE_BASE=http://127.0.0.1:8080 NUXT_PUBLIC_E2E_SKIP_AUTH=true npx nuxt preview --host ${webHost} --port ${webPort}`,
        url: defaultBaseURL,
        reuseExistingServer: !process.env.CI,
        timeout: 120 * 1000,
      },
})
