# 可访问性与键盘交互指南

> 目标：确保 PowerX Web Admin 在多语言、暗色模式下仍可被屏幕阅读器与键盘用户有效使用。本指南补充通用规范、组件封装建议与测试步骤。

---

## 1. 基础原则

- **语义优先**：尽量使用语义化标签（`<button>`、`<nav>`、`<header>`）。若必须使用 `div` 模拟按钮，务必加上 `role="button"`、`tabindex="0"` 以及 `keydown` 处理。  
- **颜色对比**：遵循 WCAG 2.1 AA 对比度，浅色与深色主题均需达到 4.5:1（正文）或 3:1（大型文本）。  
- **国际化**：所有文案通过 `t()` 获取，避免硬编码中文；ARIA 描述也应支持多语言。

---

## 2. 键盘导航

| 场景 | 要求 | 参考实现 |
| --- | --- | --- |
| 全局导航 | `Tab` 顺序按照视觉布局排列；隐藏项不应接收焦点 | 使用 `UVerticalNavigation` 或 `nav > ul`，设置 `aria-current`。 |
| 对话框 | 打开时聚焦首个可操作元素，`Esc` 关闭，关闭后回到触发按钮 | 使用 `UDialog` 或结合 `useFocusTrap()`；保持 `aria-modal="true"`。 |
| 表单 | 提供 `label` 与 `aria-describedby`，错误提示关联输入框 | `UFormGroup` 自动输出关联属性；自定义组件需手动连线。 |
| 菜单/下拉 | 支持方向键移动、`Enter/Space` 选择、`Esc` 退出 | 使用 `URadioGroup`/`USelectMenu` 或在封装组件中监听键盘事件。 |
| 通知 | 自动消息应加 `role="status"` 或 `role="alert"`，确保屏幕阅读器朗读 | 修改 `UAlert` Props 或外层包装。 |

---

## 3. 可访问属性与状态

- **按钮/链接**：`<UButton>` 默认是 `<button>`，若用作链接需指定 `to` 或包裹 `<NuxtLink>。`  
- **图标按钮**：提供 `aria-label`，或加入隐藏文本 `<span class="sr-only">关闭</span>`。  
- **表格**：设置 `<th scope="col">`、`<th scope="row">`，在分页控件上标注 `aria-label="下一页"`。  
- **空态/错误态**：使用 `<section role="status">` 输出友好提示，避免只有颜色变化。  
- **Loading**：在长时间操作中提供 `aria-live="polite"` 提示，例如全局 Loading 可以通过 `useGlobalLoading()` 派发 `aria` 信息。

---

## 4. 组件封装建议

- 在自研组件（`app/components/ui/*`）中，默认暴露 `ariaLabel`、`ariaDescribedby` 等 props，透传到内层元素。  
- 若组件内部管理焦点（例如 `SelectTree`），需在 `mounted` 时设置初始焦点，并在 `beforeUnmount` 清理事件监听。  
- 动态内容（如列表添加项）应通过动画外的容器设置 `aria-live="polite"`，避免阅读器漏读。

---

## 5. 测试流程

1. **键盘巡检**：关闭鼠标，仅用 `Tab` / `Shift+Tab` / `Enter` / `Space` / `Esc` 完成核心任务（登录、切换 Agent、安装插件）。  
2. **屏幕阅读器**：使用 macOS VoiceOver 或 NVDA，验证菜单、对话框、表单反馈是否可朗读。  
3. **对比度工具**：Chrome DevTools “Accessibility” 面板或 Axe 扩展检查颜色对比。  
4. **自动化检查**：集成 Axe DevTools 或 `@nuxtjs/eslint` 的 a11y 规则，对新增模板进行 lint。  
5. **无障碍模式**：未来可提供“高对比度”或“减少动效”开关，测试 CSS 媒体查询 `prefers-reduced-motion`、`prefers-contrast` 实现情况。

---

## 6. Review Checklist

- [ ] 所有可交互元素可通过键盘访问。  
- [ ] 对话框/抽屉具有焦点陷阱与正确的 ARIA 属性。  
- [ ] 图标按钮、状态消息提供屏幕阅读器描述。  
- [ ] 表单校验提示与输入框关联。  
- [ ] 颜色对比在亮/暗主题下均满足 WCAG AA。  
- [ ] 新增组件在 `docs/ui-and-design/Component_Library_Standards.md` 中登记封装方式。

---

## 7. 后续改进

- 制定全局 `skip to main` 键盘入口，方便快速跳过导航。  
- 引入 Playwright + Axe 组合，在 CI 中做自动化无障碍扫描。  
- 将主题切换事件与 `prefers-color-scheme` 同步，为阅读器提供主题状态。  
- 建立面向设计团队的可访问性 checklist，共享在 Figma / Notion。
