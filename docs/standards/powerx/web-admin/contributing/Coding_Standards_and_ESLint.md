# 代码规范与 ESLint 指南

> 定义 PowerX Web Admin 的编码约定、Lint 规则以及常见修复流程，确保团队协作一致性。

---

## 1. 语言与文件约定

- **脚本**：全部使用 TypeScript。Vue 文件采用 `<script setup lang="ts">`。  
- **样式**：优先使用 Tailwind 实用类；复杂样式写在 `app/assets/css/*.css` 或 `*.scss`。  
- **缩进**：两个空格（VS Code 可通过 `.editorconfig` 自动设定）。  
- **命名**：组件文件 PascalCase（`AgentSidebar.vue`），composable 以 `useX.ts`，Pinia store `app/stores/x.ts` + `useXStore`。

---

## 2. ESLint 配置

- 项目使用 Nuxt ESLint Flat Config (`@nuxt/eslint`)。  
- `eslint.config.mjs` 动态加载 `.nuxt/eslint.config.mjs`，首次安装时也可运行（`eslint.config.mjs:3`）。  
- 命令：
  ```bash
  npx eslint . --ext .ts,.vue
  npx eslint . --ext .ts,.vue --fix
  ```
- 推荐在 VS Code 安装 ESLint 扩展并开启 `editor.codeActionsOnSave`.

---

## 3. 代码风格要点

- **组件模板**：保持语义清晰，适当拆分子组件，避免 template 过长。  
- **Composable**：返回 `readonly(...)` 的状态，减少外部误操作。  
- **异步函数**：统一 `try/catch` 错误，调用 `normalizeApiError`。  
- **Tailwind**：按结构分段排序（布局 → 尺寸 → 边距 → 背景 → 文本）。  
- **注释**：仅在复杂逻辑处添加简短说明，避免无意义注释。

---

## 4. 自动化工具

- 可选：引入 `prettier` 并通过 ESLint `plugin:prettier/recommended` 保持格式一致。  
- 提交前运行：
  ```bash
  npm run lint
  npm run test:unit   # 若已配置 Vitest
  ```
- 在 CI（参见 `CI_CD_Pipeline_for_Nuxt.md`）中强制执行 ESLint。

---

## 5. 常见问题与解决

| 问题 | 解决办法 |
| --- | --- |
| `Module not found: .nuxt/eslint.config.mjs` | 运行 `npm run dev` 或 `npm run lint` 先生成 `.nuxt`，或执行 `npx nuxt prepare`。 |
| 全局类型未识别 | 在 TS 文件顶部添加 `/// <reference types="nuxt" />`，或更新 `tsconfig.json`. |
| Tailwind 类过长 | 拆分成多个容器，或使用 `class` 变量/`clsx` 整理。 |

---

## 6. Review Checklist

- [ ] 是否通过 `npm run lint`，无警告或仅可接受的 TODO。  
- [ ] 新文件遵循命名惯例、目录结构。  
- [ ] TypeScript 类型是否完善，避免 `any`。  
- [ ] 组件/Composable 是否具备清晰边界与文档注释。  
- [ ] 国际化文本是否使用 `t()` 获取。  
- [ ] Tailwind 类是否合理，避免重复或冲突。

---

## 7. 后续计划

- 补充自定义 ESLint 规则（例如限制 `console.log`，允许 `console.error`）。  
- 引入 `eslint-plugin-security`、`eslint-plugin-sonarjs` 强化质量。  
- 在 Husky pre-commit 中运行部分 lint（谨慎避免影响开发体验）。
