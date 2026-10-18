import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './tests/e2e/current-recovery',
  timeout: 120_000,
  workers: 1,
  retries: 0,
  reporter: 'list',
  outputDir: '../test-results/current-recovery-browser',
  // The owned native Go fixture already runs ordinary Core and Nuxt. Never reuse user services.
  use: { baseURL: 'http://127.0.0.1:3489', headless: true, trace: 'off', screenshot: 'only-on-failure' },
});
