import { defineConfig, devices } from "@playwright/test";

// 只读页面验收：独立会话与 API fixture，不登录或保存到开发数据库。
export default defineConfig({
  testDir: "./tests/e2e/ai",
  testMatch: ["image-defaults.spec.ts", "shared-provider-credentials.spec.ts", "model-parameters.spec.ts", "ollama-models.spec.ts", "online-models.spec.ts", "online-model-classification.spec.ts", "vision-settings.spec.ts", "online-first.spec.ts", "vision-trial.spec.ts", "vision-model-choice.spec.ts"],
  timeout: 60_000,
  expect: { timeout: 15_000 },
  workers: 1,
  use: {
    actionTimeout: 10_000,
    baseURL: process.env.PLAYWRIGHT_BASE_URL || "http://127.0.0.1:3030",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
