# 组件库与封装规范

> 约束 PowerX Web Admin 前端在使用 `@nuxt/ui`、Tailwind 与自研组件时的封装方式、命名习惯与复用策略，确保 UI 风格一致且易于维护。

---

## 1. 组件来源与职责划分

| 来源 | 位置 | 说明 |
| --- | --- | --- |
| `@nuxt/ui` | 全局注册，直接使用 `UButton`、`UCard`、`UForm` 等 | 提供一致的基础交互与主题支持，默认样式与 `design tokens` 保持一致。 |
| Tailwind 工具类 | 任意模板内使用 | 负责布局、间距与响应式，禁止在同一节点混入行内样式。 |
| 自研组件 | `app/components/**` | 根据业务分类：`layout/`、`agent/`、`ui/` 等；命名使用 PascalCase。 |
| 通用 UI 封装 | `app/components/ui/*` | 对 `@nuxt/ui` 进行二次封装或跨业务复用的复合组件，如 `SelectTree.vue`。 |

> 若第三方库缺省 Nuxt 支持（如图表、流程编辑器），统一新建目录（如 `workflow/`）封装入口，避免直接在页面中操作底层 API。

---

## 2. 封装策略

### 2.1 基础控件

- **直接使用 `@nuxt/ui` 组件**：  
  ```vue
  <UButton color="primary" variant="solid" @click="save">
    保存
  </UButton>
  ```
- 如需调整默认样式，优先通过 `props`、`color`、`variant`。若无法满足，可在 `app/app.config.ts`（待创建）统一覆盖，而非在组件内自定义类名。

### 2.2 二次封装

- 当多个页面重复使用某组合时，在 `app/components/ui/` 创建封装组件：  
  - `GuideButton.vue`：包装 `UButton` 并追加指标上报。  
  - `SelectTree.vue`：封装树形选择逻辑与接口交互。  
- 封装组件应暴露清晰的 props/emit，内部仍复用基础库，避免在业务层复制粘贴模板。

### 2.3 业务组件

- 业务较重的组件（如 `agent/ChatMessageList.vue`）保持“容器 + 子组件”的结构，将布局与数据加载拆分：  
  - 容器组件：负责拉取数据/与 store 交互。  
  - 表现组件：仅接收 props 渲染 UI，可在 Storybook 中独立展示（后续计划）。

---

## 3. 样式与命名规范

- 组件文件名与导出的 `name`/`defineComponent` 均使用 PascalCase。  
- Template 中类名优先使用 Tailwind，若需要局部覆盖，可在 `<style scoped>` 中使用语义化类，如 `.chat-header`。  
- 避免滥用 `!important`，除非在全局令牌样式（`theme.css`）中统一控制。  
- 复合类：当 Tailwind 难以表达复杂逻辑时，可改用 CSS 变量或抽出样式片段。

---

## 4. 图标与插画

- 统一使用 `@nuxt/icon`（`nuxt.config.ts:67`）和 `@iconify-json` 资源。  
- 在代码中写 `icon="i-heroicons-cog-6-tooth"`，保持大小写与前缀一致。  
- 若需要自定义 SVG，放入 `app/components/icons/`，通过 `<Icon name="custom:xxx" />` 或注册组件使用。

---

## 5. 可访问性

- 组件在设计时需遵循 `docs/ui-and-design/Accessibility_and_Keyboard.md`（待补充），至少确保：  
  - 可聚焦控件带有 `aria-label` 或文本。  
  - 键盘操作可覆盖主要流程（Tab 顺序、Enter/Space 触发）。  
  - 对话框使用 `UDialog` 或自定义时调用 `useFocusTrap()`。

---

## 6. 组件库升级流程

1. 评估 `@nuxt/ui` 或外部库版本变更影响，记录在 PR 描述。  
2. 如需新增第三方库（例如图表、富文本），必须在本文件或 README 记录使用范围与替代方案。  
3. 升级前在 `npm run dev` 下回归关键页面：登录、仪表盘、Agent、插件市场。  
4. 若组件 API 发生 breaking change，同时更新封装组件与使用文档。

---

## 7. Review Checklist

- [ ] 组件是否按目录归类，避免跨模块引用。  
- [ ] 是否重复封装已有的 UI 组件（若是，考虑抽至 `ui/`）。  
- [ ] props/emit 是否具备类型定义，默认值是否合理。  
- [ ] 模板中是否存在硬编码颜色、字体等，应改为 CSS 变量或 Tailwind 语义类。  
- [ ] 是否提供 Storybook 或示例页面（如 `app/pages/test/**`）便于演示。  
- [ ] 是否考虑国际化文本 `t('...')`。

---

## 8. 后续计划

- 引入 Storybook/Playroom，集中展示组件库示例。  
- 建立 `packages/ui` 子包以承载复用组件，降低页面间依赖。  
- 编写自动化快照测试（结合 `@nuxt/ui` + Playwright）检测 UI 回归。  
- 与设计团队共享令牌 JSON，实现设计工具与代码的双向同步。
