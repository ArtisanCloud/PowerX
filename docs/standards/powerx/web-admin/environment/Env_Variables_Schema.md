<!--------------------------------------------------------------
PowerX Web Admin Environment Variable Schema
Maintainer: Frontend Platform Team
Last update: 2025-02-14
-------------------------------------------------------------->
# PowerX Web Admin 环境变量对照表

> 目标：明确 `.env` / `.env.*` 中可配置的键、预计取值及在 Nuxt Runtime 中的落点，方便不同环境（本地 / 测试 / 生产）保持一致。

---

## 目录
- [命名规范与加载顺序](#命名规范与加载顺序)
- [核心后端路由变量](#核心后端路由变量)
- [前端公开配置（`runtimeConfig.public`）](#前端公开配置runtimeconfigpublic)
- [主题与多语言配置](#主题与多语言配置)
- [功能开关与其他可选变量](#功能开关与其他可选变量)
- [环境示例推荐值](#环境示例推荐值)
- [常见排错提示](#常见排错提示)

---

## 命名规范与加载顺序

- Nuxt 4 会按顺序读取 `.env` → `.env.local` → `.env.[NODE_ENV]` → 系统变量，后者同名优先生效。
- 以 `NUXT_` 开头的变量会自动注入 `runtimeConfig.public`（见 [Nuxt 变量约定](https://nuxt.com/docs/guide/directory-structure/env)）。
- 其他自定义变量（如 `POWERX_BACKEND`）在服务器端可见，可通过 `useRuntimeConfig()` 安全获取。
- 请勿在 `.env` 中保存生产密钥，推荐使用部署平台的 Secret 注入方式。

---

## 核心后端路由变量

| 变量 | 描述 | 默认值 | 类型/格式 | 生效范围 | 备注 |
| --- | --- | --- | --- | --- | --- |
| `POWERX_BACKEND` | 前端向后端 API 转发的基础地址 | `http://127.0.0.1:8077` | URL | `runtimeConfig.upstream`、Nitro `devProxy` | 仅服务器可见；`api` 请求最终会拼接 `/api/v1`。 |
| `NUXT_PUBLIC_WS_ORIGIN` | WebSocket 主机地址 | `ws://127.0.0.1:8077` | URL | `runtimeConfig.public.wsOrigin` | 客户端统一以此作为 WS origin。 |
| `NUXT_PUBLIC_WS_PATH` | WebSocket 路径 | `/api/ws` | Path | `runtimeConfig.public.wsPath` | 建议固定 `/api/ws`。 |
| `NUXT_PUBLIC_POWERX_CORE_BASE` | 宿主 Core 地址 | `http://127.0.0.1:8077` | URL | `runtimeConfig.public.powerxCoreBase` | 当 `wsOrigin` 缺失时用于构造回退地址。 |
| `POWERX_BACKEND` | 反向代理 `/_p/**` 路径时指向的后端 | `http://127.0.0.1:8077` | URL | `app/server/middleware/00-proxy-plugins.ts` | 仅 Nuxt 服务端使用，用于插件 iframe 等直通代理。 |

> 若 `POWERX_BACKEND`、`POWERX_BACKEND` 填写 HTTPS，务必确保目标证书受信；否则需在本地加 `NODE_TLS_REJECT_UNAUTHORIZED=0`（不推荐，仅调试）。

---

## 前端公开配置（`runtimeConfig.public`）

Nuxt 将以下值编译至客户端，请勿放置敏感信息。

| 变量 | 默认值 | 解析逻辑 | 影响区域 |
| --- | --- | --- | --- |
| `NUXT_DEFAULT_LANGUAGE` | `zh` | 若未设定调用 fallback 值 | 控制 i18n 默认语言与浏览器回退语言 |
| `NUXT_AVAILABLE_LANGUAGES` | `zh,en,ja,ko` | 逗号分隔字符串，在运行时拆为数组 | 决定语言切换器枚举 |
| `NUXT_FORCE_LANGUAGE` | _未定义_ | 若设值，语言切换被禁用 | `colorMode` 与页面守卫读取 |
| `NUXT_ENABLE_LANGUAGE_SWITCH` | `"true"` | 字符串转换为布尔值（`"false"` 才为 false） | UI 是否展示语言切换 |
| `NUXT_DEFAULT_THEME` | `auto` | 允许 `light/dark/auto` | 影响 `@nuxtjs/color-mode` 初始值 |
| `NUXT_AVAILABLE_THEMES` | `light,dark,auto` | 逗号分隔 | 主题切换菜单 |
| `NUXT_FORCE_THEME` | _未定义_ | 存在时覆盖用户偏好 | `colorMode` 插件强制设定 |
| `NUXT_ENABLE_THEME_SWITCH` | `"true"` | `"false"` → 禁止用户切换 | Header 主题切换按钮 |
| `NUXT_APP_NAME` | `PowerX Web Admin` | 字符串 | 应用标题、Meta、欢迎引导 |
| `NUXT_APP_VERSION` | `1.0.0` | 字符串 | 设置、底部信息中的版本号 |
| `NUXT_DEBUG_MODE` | `"false"` | `"true"` → `debugMode = true` | 可用于显示调试 banner、额外日志 |
| `NUXT_ENABLE_USER_PREFERENCES` | `"true"` | `"false"` → 禁用用户偏好写入 | `useUserPreference` 相关功能 |

固定常量（非环境变量）：

- `runtimeConfig.public.apiBase = "/api/v1"`：前端请求 `/api/**` 会指向后端 `/api/v1/**`。
- `runtimeConfig.public.wsPath = "/api/ws"`：同域部署时客户端统一连接该路径。

---

## 主题与多语言配置

### 1. 语言参数组合

- 若设置 `NUXT_FORCE_LANGUAGE=en`，即使 `NUXT_ENABLE_LANGUAGE_SWITCH=true` 也不会显示 UI 切换。
- `NUXT_AVAILABLE_LANGUAGES` 必须包含 `NUXT_DEFAULT_LANGUAGE`，否则初次渲染加载不到词条。
- `NUXT_ENABLE_LANGUAGE_SWITCH` 仅接受字符串 `"true"`/`"false"`；推荐统一小写。

### 2. 主题参数组合

- `NUXT_FORCE_THEME` 优先级最高，适合品牌要求单一主题的场景。
- `NUXT_AVAILABLE_THEMES` 可减少到 `light,dark`，若去掉 `auto`，浏览器跟随系统的能力会被禁用。
- 需与 Tailwind `darkMode` 配置保持一致（当前在 `tailwind.config.ts` 中使用默认 class 模式）。

---

## 功能开关与其他可选变量

| 变量 | 默认值 | 类型 | 典型用途 |
| --- | --- | --- | --- |
| `NUXT_DEBUG_MODE` | `"false"` | Boolean 字符串 | 打开后可渲染额外的日志面板或 QA 水印（需在组件中判断）。 |
| `NUXT_ENABLE_USER_PREFERENCES` | `"true"` | Boolean 字符串 | 控制用户偏好存储功能，例如主题/语言记忆。 |
| `NUXT_DEVTOOLS` | _未设置_ | Boolean 字符串 | 当设为 `true` 时启用 Nuxt DevTools（仅开发环境生效）。 |
| `NODE_ENV` | `development` | Node 约定 | 控制 Nuxt 构建模式，CI/生产环境应设为 `production`。 |

> 文档中提及的布尔变量解析逻辑：只有当值严格等于字符串 `"false"` 时才会被视为 `false`；其他任意值（包括空字符串）都被当作 `true`。

---

## 环境示例推荐值

> 以下为示例，实际地址需根据后端部署拓扑调整。

```ini
# .env.development
POWERX_BACKEND=http://127.0.0.1:8077
NUXT_PUBLIC_WS_ORIGIN=ws://127.0.0.1:8077
NUXT_PUBLIC_WS_PATH=/api/ws
NUXT_PUBLIC_POWERX_CORE_BASE=http://127.0.0.1:8077

NUXT_DEFAULT_LANGUAGE=zh
NUXT_AVAILABLE_LANGUAGES=zh,en,ja,ko
NUXT_ENABLE_LANGUAGE_SWITCH=true

NUXT_DEFAULT_THEME=auto
NUXT_FORCE_THEME=
NUXT_ENABLE_THEME_SWITCH=true

NUXT_APP_NAME=PowerX Web Admin (Dev)
NUXT_APP_VERSION=1.0.0-dev
NUXT_DEBUG_MODE=true
```

```ini
# .env.staging
POWERX_BACKEND=https://staging-api.powerx.internal
NUXT_PUBLIC_WS_ORIGIN=wss://staging-ws.powerx.internal
NUXT_PUBLIC_WS_PATH=/api/ws
NUXT_PUBLIC_POWERX_CORE_BASE=https://staging-api.powerx.internal

NUXT_DEFAULT_LANGUAGE=en
NUXT_AVAILABLE_LANGUAGES=en,zh
NUXT_ENABLE_LANGUAGE_SWITCH=true

NUXT_DEFAULT_THEME=dark
NUXT_FORCE_THEME=dark
NUXT_ENABLE_THEME_SWITCH=false

NUXT_APP_NAME=PowerX Web Admin Staging
NUXT_APP_VERSION=1.1.0-rc1
NUXT_DEBUG_MODE=false
```

```ini
# .env.production
POWERX_BACKEND=https://api.powerx.example.com
NUXT_PUBLIC_WS_ORIGIN=wss://ws.powerx.example.com
NUXT_PUBLIC_WS_PATH=/api/ws
NUXT_PUBLIC_POWERX_CORE_BASE=https://api.powerx.example.com

NUXT_DEFAULT_LANGUAGE=en
NUXT_AVAILABLE_LANGUAGES=en,zh
NUXT_ENABLE_LANGUAGE_SWITCH=false

NUXT_DEFAULT_THEME=dark
NUXT_FORCE_THEME=dark
NUXT_ENABLE_THEME_SWITCH=false

NUXT_APP_NAME=PowerX Web Admin
NUXT_APP_VERSION=1.1.0
NUXT_DEBUG_MODE=false
```

- 若生产环境启用 CDN，需要同步配置 `NUXT_APP_VERSION` 以便排查缓存。
- WebSocket 地址建议使用独立域名或子域，避免与 REST API 共享负载均衡策略。

---

## 常见排错提示

- **前端请求仍指向 localhost**：检查部署环境是否覆盖 `POWERX_BACKEND`。Nuxt 会在构建时读取，构建后修改 `.env` 需重新构建。
- **语言切换缺失选项**：确认 `NUXT_AVAILABLE_LANGUAGES` 中的代码与 `i18n/locales/*.json` 文件一致。
- **WebSocket 连接 404/403**：若通过反向代理部署，需要同步配置 Nginx/Ingress 将 `NUXT_PUBLIC_WS_PATH`（默认 `/api/ws`）转发到后端。
- **DevTools 未生效**：`NUXT_DEVTOOLS=true` 仅在 `npm run dev` 时生效，生产构建会被忽略。

---

## 维护建议

- 更新或新增变量时，请同步：
  1. `.env.example`
  2. 本文档对应表格
  3. `docs/environment/Dev_Environment_Setup.md` 快速清单
  4. 如影响代理路径，务必执行 `bash scripts/check-refactor.sh`
- PR 需在描述中注明“Env Var Update”，提醒部署工程师同步发布配置。
