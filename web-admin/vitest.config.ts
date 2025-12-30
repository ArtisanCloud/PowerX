import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";
import path from "node:path";

export default defineConfig({
  cacheDir: path.resolve(__dirname, ".vitest-cache"),
  plugins: [vue()],
  resolve: {
    alias: {
      "~": path.resolve(__dirname, "app"),
      "@": path.resolve(__dirname, "app"),
      "#app": path.resolve(__dirname, "tests/support/nuxt-app-shim.ts"),
      "#imports": path.resolve(__dirname, "tests/support/nuxt-imports-shim.ts"),
    },
  },
  test: {
    environment: "jsdom",
    include: [
      "tests/unit/**/*.spec.{js,ts}",
      "app/**/__tests__/**/*.{spec,test}.{js,ts,jsx,tsx,mjs}",
    ],
    exclude: ["tests/e2e/**", "node_modules/**", ".nuxt/**", ".output/**"],
  },
});
