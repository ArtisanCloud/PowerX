# 国际化结构与词条管理

> 说明 PowerX Web Admin 的 i18n 目录结构、命名规范、加载策略及协作流程。项目使用 `@nuxtjs/i18n`，语言文件位于 `i18n/locales/*.json`。

---

## 1. 配置概览

- `nuxt.config.ts: "74`："
  ```ts
  i18n: {
    defaultLocale: process.env.NUXT_DEFAULT_LANGUAGE || "zh",
    strategy: "no_prefix",
    locales: [
      { code: "zh", name: "简体中文", file: "zh.json" },
      { code: "en", name: "English", file: "en.json" },
      { code: "ja", name: "日本語", file: "ja.json" },
      { code: "ko", name: "한국어", file: "ko.json" },
    ],
    langDir: "locales",
    detectBrowserLanguage: {
      useCookie: true,
      cookieKey: "px_lang",
      redirectOn: "no prefix",
    },
  }
  ```
- 默认禁用路由前缀，语言切换由 `LanguageSwitcher` 组件处理。  
- 环境变量 `NUXT_FORCE_LANGUAGE`、`NUXT_ENABLE_LANGUAGE_SWITCH` 控制语言策略（参见 `Env_Variables_Schema.md`）。

---

## 2. 目录与命名

- 词条存放在 `i18n/locales/{lang}.json`，使用嵌套对象组织模块：`common`, `menu`, `dashboard`, `errors`。  
- Key 命名规则：`模块.功能.描述`，例如 `agent.chat.send`.  
- 对应组件引用 `t("agent.chat.send")`；不要硬编码字符串。

---

## 3. 新增/更新词条流程

1. 在默认语言（中文）文件添加 key，并补充英文翻译，其他语言可由翻译团队跟进。  
2. 执行 `npm run lint` 确认 JSON 格式合法。  
3. 如果词条涉及 UI 变更，更新截图/说明。  
4. 在 PR 中列出新增 key，便于翻译审核。

---

## 4. 动态加载与拆分

- 当前所有词条一次性加载，未来若词条过大可按模块拆分为多文件，并在 `locales` 配置中使用目录。  
- 对于插件生态，建议每个插件提供独立的语言包 JSON，在加载插件时调用 `addMessages(code, messages)`.

---

## 5. 协作与翻译

- 建议通过翻译平台（Crowdin/POEditor）管理词条，导出 JSON。  
- 词条变更需含上下文说明（注释或 README），避免误译。  
- QA 在多语言模式下验证布局与字符串长度。

---

## 6. 技术提示

- 使用 `useLocalePath()` 生成链接，适配多语言切换。  
- 在 store/composable 中使用 `useI18n()`，或传入 `t` 函数，避免在纯逻辑层直接引用 `globalThis`.  
- 本地调试时可设置 `NUXT_FORCE_LANGUAGE`，快速验证各语言。

---

## 7. Review Checklist

- [ ] 新增词条是否同步更新所有语言或标记 TODO。  
- [ ] Key 命名是否遵循模块化规范。  
- [ ] 切换语言后页面是否布局错乱（长文案需断词）。  
- [ ] 是否避免使用拼接字符串，改用占位符 `{count}`。  
- [ ] 词条是否避免包含 HTML，若需要请使用组件组合实现。

---

## 8. 后续计划

- 导出词条统计报表，提醒缺失翻译。  
- 在 CI 检测 `i18n/locales` 之间的 key 差异。  
- 支持按租户动态加载补充词条（定制化运营）。  
- 与 UX 团队建立语言校对流程，定期审查专业术语。
