import tailwindcss from "@tailwindcss/vite";

/**
 * 端口策略（固定约定）：
 * - dev:  web-admin/backend/grpc = 3030/8077/9001
 * - prod: web-admin/backend/grpc = 3000/8080/9010
 *
 * POWERX_BACKEND 支持两种写法：
 * 1) 仅 origin：  http://127.0.0.1:8077
 * 2) 含 API 前缀： http://127.0.0.1:8077/api/v1
 *
 * 若 POWERX_BACKEND 含 path，则自动把该 path 作为 apiBase（可被 NUXT_PUBLIC_API_BASE 覆盖）。
 */
const POWERX_ENV = (process.env.POWERX_ENV || "").toLowerCase();
const POWERX_BUILD_TARGET = (process.env.POWERX_BUILD_TARGET || "").toLowerCase();
const isProdEnv =
  POWERX_BUILD_TARGET === "prod" ||
  POWERX_BUILD_TARGET === "production" ||
  POWERX_ENV === "prod" ||
  POWERX_ENV === "production";
const DEFAULT_HTTP_UPSTREAM = isProdEnv
  ? "http://127.0.0.1:8080"
  : "http://127.0.0.1:8077";
const DEFAULT_WS_ORIGIN = isProdEnv
  ? "ws://127.0.0.1:8080"
  : "ws://127.0.0.1:8077";

const UPSTREAM_RAW =
  process.env.POWERX_BACKEND ||
  process.env.POWERX_BACKEND ||
  DEFAULT_HTTP_UPSTREAM;
let upstreamOrigin = UPSTREAM_RAW;
let inferredApiBase = "";
try {
  const u = new URL(UPSTREAM_RAW);
  upstreamOrigin = u.origin;
  const p = (u.pathname || "/").replace(/\/+$/, "");
  if (p && p !== "/") inferredApiBase = p;
} catch {
  // ignore: keep raw string
}

const API_BASE =
  process.env.NUXT_PUBLIC_API_BASE || inferredApiBase || "/api/v1";
const API_BASE_PREFIX = API_BASE.replace(/\/+$/, "");
const WS_PATH = process.env.NUXT_PUBLIC_WS_PATH || "/api/ws";
const WS_ORIGIN = process.env.NUXT_PUBLIC_WS_ORIGIN || DEFAULT_WS_ORIGIN;
const POWERX_CORE_BASE =
  process.env.NUXT_PUBLIC_POWERX_CORE_BASE || upstreamOrigin;


// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  ssr: false,
  router: {
    options: {
      strict: false, // 将 strict 设为 false
    },
  },

  colorMode: {
    preference: "dark",
    fallback: "light",
    storageKey: "powerx-color-mode",
  },

  runtimeConfig: {
    // 仅服务端可见
    upstream: upstreamOrigin,
    public: {
      upstreamOrigin, // 公开：用于拼接 presign 返回的相对 URL（如 /media/:uuid/resource）
      wsOrigin: WS_ORIGIN,
      wsPath: WS_PATH,
      powerxCoreBase: POWERX_CORE_BASE,
      wsAgentPrefix: process.env.NUXT_PUBLIC_WS_AGENT_PREFIX || "/ws",
      apiBase: API_BASE_PREFIX, // 前端请求前缀（可由 POWERX_BACKEND path 推断）

      // 语言配置
      defaultLanguage: process.env.NUXT_DEFAULT_LANGUAGE || "zh",
      availableLanguages: process.env.NUXT_AVAILABLE_LANGUAGES || "zh,en,ja,ko",
      forceLanguage: process.env.NUXT_FORCE_LANGUAGE || undefined,
      enableLanguageSwitch: process.env.NUXT_ENABLE_LANGUAGE_SWITCH !== "false",

      // 主题配置
      defaultTheme: process.env.NUXT_DEFAULT_THEME || "auto",
      availableThemes: process.env.NUXT_AVAILABLE_THEMES || "light,dark,auto",
      forceTheme: process.env.NUXT_FORCE_THEME || undefined,
      enableThemeSwitch: process.env.NUXT_ENABLE_THEME_SWITCH !== "false",

      // 应用配置
      appName: process.env.NUXT_APP_NAME || "PowerX Admin",
      appVersion: process.env.NUXT_APP_VERSION || "1.0.0",
      debugMode: process.env.NUXT_DEBUG_MODE === "true",

      // 功能开关
      enableUserPreferences:
        process.env.NUXT_ENABLE_USER_PREFERENCES !== "false",
      saasSignupVerificationEnabled:
        process.env.NUXT_PUBLIC_SAAS_SIGNUP_VERIFICATION_ENABLED === "true",

      // 测试专用：允许 Playwright 跳过 Auth 中间件
      e2eSkipAuth: process.env.NUXT_PUBLIC_E2E_SKIP_AUTH === "true",
    },
  },

  // 添加开发服务器代理配置
  nitro: {
    experimental: {
      websocket: true, // ✅ 开启 Nitro 原生 WS
    },
    prerender: { ignore: ['/_p/**'] },   // 不要静态化代理路径


    devProxy: {
      // 仅代理 API 前缀，避免误伤其它路径；并确保不会出现 /api/api/v1 的双拼
      [`${API_BASE_PREFIX}/`]: {
        target: upstreamOrigin,
        changeOrigin: true,
        prependPath: true,
        ws: true, // 必须：让 dev 代理支持 WebSocket
      },
      "/api/ws": {
        target: upstreamOrigin,
        changeOrigin: true,
        prependPath: true,
        ws: true,
      },
    },
  },
  srcDir: "app",
  devtools: { enabled: true },
  modules: [
    "@nuxtjs/color-mode",
    "@nuxt/ui",
    "@nuxt/icon",
    "@nuxtjs/i18n",
    "@nuxtjs/mdc",
    "@pinia/nuxt",
  ],
  mdc: {
    // 开启 GFM：自动识别裸 URL 为链接（如 https://golang.org/doc/）
    remarkPlugins: {
      "remark-emoji": {},
      "remark-gfm": {},
    },
    // 聊天场景不需要“标题锚点链接”，否则会让标题看起来像超链接
    headings: {
      anchorLinks: false,
    },
  },
  css: ["~/assets/css/main.css", "@/assets/scss/main.scss"],
  compatibilityDate: "2024-11-01",
  ui: { fonts: false },
  icon: {
    // 即使是 SPA 也强制走本地 server 端点（/_icon）
    provider: "server",
    // 如需自定义缓存或前缀，再加 server 相关配置
  },

  // i18n 配置
  i18n: {
    defaultLocale: process.env.NUXT_DEFAULT_LANGUAGE || "zh",
    strategy: "no_prefix",
    // 语言包通过 langDir/file 显式加载。保持 lazy=true，确保 @nuxtjs/i18n v10
    // 按 locale 文件注册完整基础消息，再叠加后端菜单动态 i18n。
    lazy: true,
    locales: [
      { code: "zh", name: "简体中文", file: "zh.json" },
      { code: "en", name: "English", file: "en.json" },
      { code: "ja", name: "日本語", file: "ja.json" },
      { code: "ko", name: "한국어", file: "ko.json" },
    ],
    // 注意：langDir 是相对于 i18nDir（默认是 `i18n/`）的
    // 语言包实际目录：`web-admin/i18n/locales/*.json`
    langDir: "locales",
    detectBrowserLanguage: {
      enabled: false,
      useCookie: false,
      alwaysRedirect: false,
    },
  },

  vite: {
    plugins: [tailwindcss()], // ✅ 官方 v4 推荐做法
    // dev 下浏览器请求会先打到 Vite dev server；加一层代理，避免 nitro.devProxy 在某些模式/版本下未生效导致 404。
    server: {
      proxy: {
        [API_BASE_PREFIX]: {
          target: upstreamOrigin,
          changeOrigin: true,
          ws: true,
        },
        "/api/ws": {
          target: upstreamOrigin,
          changeOrigin: true,
          ws: true,
        },
      },
    },
  },
});
