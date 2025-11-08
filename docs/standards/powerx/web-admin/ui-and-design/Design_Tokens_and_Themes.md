# 设计令牌与主题策略

> 指南说明 PowerX Web Admin 中设计令牌（Design Tokens）的存放方式、主题切换流程及如何在 Tailwind 与 Nuxt UI 间保持一致，为设计与前端协作提供依据。

---

## 1. 令牌资产概览

| 层级 | 文件/模块 | 说明 |
| --- | --- | --- |
| CSS 令牌 | `app/assets/css/theme.css:1` | 定义基础 CSS Variables（背景、文本、边框、卡片等），`body` 与核心组件使用变量驱动。 |
| Tailwind Tokens | `tailwind.config.ts:1` | 采用 `darkMode: 'class'`，后续可在 `theme.extend` 中映射 CSS 变量或自定义语义色。 |
| Nuxt UI Tokens | 未创建 `app/app.config.ts` | 可通过 `defineAppConfig({ ui: { primary: '...' } })` 调整 `@nuxt/ui` 主题色。 |
| Runtime Config | `nuxt.config.ts:20` | `colorMode` 默认偏好为 `dark`，同时允许通过环境变量强制主题。 |
| Workflow 特例 | `app/assets/css/workflow.css:1` | 针对 Vue Flow 画布的隔离样式，避免主题覆盖节点样式。 |

令牌结构遵循“**CSS 变量 → Tailwind 映射 → 组件库覆盖**”的顺序，确保不同技术栈共享一致的色板与间距。

---

## 2. 主题切换机制

### 2.1 color-mode 配置

- `nuxt.config.ts:14` 设置 `colorMode.preference = "dark"`、`fallback = "light"`，并使用 `powerx-color-mode` 作为 localStorage Key。  
- `runtimeConfig.public` 提供 `defaultTheme`、`forceTheme`、`enableThemeSwitch` 等开关（`nuxt.config.ts:38`），可通过 `.env` 控制终端行为。

### 2.2 同步 DOM 属性

- `app/plugins/theme.ts:1` 监听 `useColorMode()`，将主题偏好同步至 `document.documentElement` 上的 `data-theme`、`data-color-mode` 属性，并切换 `.dark` / `.light` class。  
- 插件还派发 `theme-changed` 事件，供第三方组件监听动态换肤。

### 2.3 CSS 变量生效方式

- `app/app.vue` 全局引入 `~/assets/css/theme.css`，对 body、header、sidebar 等核心区域应用变量。  
- `.dark` 作用域内覆盖对应变量，实现深色主题。  
- Tailwind 公用类如 `.bg-white`、`.text-gray-500` 等通过 `!important` 方式统一改造，避免组件内手动切换。

---

## 3. 与 Tailwind 的协作

- Tailwind 4 使用 `@import "tailwindcss";`，所有类名在运行时解析。若需扩展语义色，建议在 `tailwind.config.ts` 中：
  ```ts
  export default <Config>{
    theme: {
      extend: {
        colors: {
          surface: "var(--card-bg)",
          text: "var(--text-primary)",
        },
        borderColor: {
          DEFAULT: "var(--border-color)",
        },
      },
    },
  };
  ```
- 组件中优先使用语义类（如 `bg-surface`、`text-text`），减少对具体色值的绑定。  
- 注意 `workflow.css` 已通过 `isolation: isolate` 防止 Tailwind reset 影响 Vue Flow；若新增第三方组件，也应采用类似策略。

---

## 4. @nuxt/ui 主题定制

- 目前未覆盖 `@nuxt/ui` 默认令牌。若需要统一外观，可新增 `app/app.config.ts`：
  ```ts
  export default defineAppConfig({
    ui: {
      primary: "blue",
      gray: "slate",
      button: {
        default: {
          base: "bg-surface text-text",
        },
      },
    },
  });
  ```
- 与 CSS 变量结合时，可通过 CSS 覆盖 `.u-button` 等类名，将背景/边框对齐。  
- 对于自定义组件库，建议使用 CSS 变量（如 `var(--card-bg)`) 作为颜色来源，避免与 `@nuxt/ui` tokens 冲突。

---

## 5. 品牌与多主题支持

1. **强制主题**：通过 `.env` 设置 `NUXT_FORCE_THEME=dark`，前端将禁用主题切换，仅展示指定模式。  
2. **品牌切换**：可在 CSS 变量层定义品牌命名前缀，如 `--brand-primary`，并在 `:root[data-brand="acme"]` 中覆盖。  
3. **按租户定制**：结合 `useEnvStore()` 或租户配置，在登录后为根节点设置 `data-tenant-theme="tenant-key"`，加载对应的 CSS 变量文件。  
4. **令牌输出**：建议使用设计工具（如 Figma Tokens）生成 JSON，后续脚本导出至 `theme.css`，避免手写错误。

---

## 6. QA / 验收要点

- [ ] 测试在浅色、深色模式下，常用页面（仪表盘、Agent 工作区、插件市场）是否保持对比度。  
- [ ] 切换主题时，Loading、弹窗、Teleport 组件是否跟随更新。  
- [ ] Vue Flow、ECharts 等第三方库是否使用 CSS 变量自适应背景与文本。  
- [ ] 若启用 `NUXT_FORCE_THEME`，UI 是否隐藏主题切换入口。  
- [ ] 替换品牌色后，`@nuxt/ui` 组件与 Tailwind 元素是否一致。

---

## 7. 后续工作

- 在 `theme.css` 中补充字体、间距、阴影等非颜色令牌，并同步更新 Tailwind tokens。  
- 引入 `@nuxtjs/color-mode` 的“对比度模式”或“高对比度”方案，以满足无障碍要求。  
- 将令牌发布到 Storybook/Tokens Studio，便于设计与前端共享。  
- 若需要支持动态主题（例如节日皮肤），可通过 `prefetch` 预加载 CSS，并在 `theme-changed` 事件中切换数据源。
