# 视觉回归测试策略

> 本文定义 PowerX Web Admin 的视觉回归测试方案。可基于 Playwright Screenshot、Chromatic 或 Storybook 视图进行对比，确保 UI 变更受控。

---

## 1. 工具选型

| 方案 | 优点 | 风险/成本 | 适用场景 |
| --- | --- | --- | --- |
| Playwright 截图 | 与现有 E2E 共用基础设施；支持自托管 | 对比粒度较大，易受动态数据影响 | 核心页面冒烟回归 |
| Chromatic (Storybook) | 托管对比、PR 注释、审批流 | 需维护 Storybook + 付费高级功能 | 组件级、可视化模块 |
| Percy / Applitools | 专业服务，容错和 AI 校正 | 付费，需额外集成 | 大规模团队/多品牌 |

建议阶段性方案：短期在 Playwright 上实现页面截图，长期引入 Storybook + Chromatic 管理组件级视觉测试。

---

## 2. Playwright 截图流程

1. 标记关键页面：登录后仪表盘、Agent 对话、插件市场、工作流编辑器。  
2. 在 E2E 测试中加入截图断言：
   ```ts
   test("dashboard visual baseline", async ({ page }) => {
     await page.goto("/dashboard");
     await expect(page.locator("main")).toHaveScreenshot("dashboard.png", {
       animations: "disabled",
       scale: "device",
     });
   });
   ```
3. 执行 `PLAYWRIGHT_UPDATE_SNAPSHOTS=1 npm run test:e2e` 更新基线。  
4. CI 失败时生成 `.png` diff，可在 PR 中展示。

### 稳定性建议

- Mock 动态数据：通过 API Mock 或固定种子，避免图表、时间导致差异。  
- 关闭动画：使用 `page.addStyleTag({ content: "* { animation-delay: -1ms !important; animation-duration: 1ms !important; transition: none !important; }" })`。  
- 设置固定视口、字体渲染。

---

## 3. Storybook + Chromatic（规划）

1. 搭建 Storybook，覆盖核心组件（卡片、表格、弹窗、Workflow 节点）。  
2. 在 CI 使用 `chromatic --project-token=...` 上传快照。  
3. 在 PR 中强制审批视觉 diff；若合法变更，可通过 Chromatic UI 批准。  
4. 将 `chromatic` 结果整合进 PR 检查状态。

---

## 4. 基线管理

- 基线文件存放在 `tests/e2e/__screenshots__/**` 或 `.storybook/__snapshots__/`。  
- 需审核变更：在 PR 中解释理由并附带对比图。  
- 避免在同一提交中混合大量视觉更新和功能代码，保持 diff 可控。

---

## 5. 常见问题

- **多主题**：对每种主题分别拍摄截图（`light`/`dark`）。  
- **响应式**：针对关键断点（720、1024、1440）生成多个基线。  
- **字体差异**：在 CI 使用固定字体包，或在截图前加载 Web 字体。  
- **动态图表**：为图表注入固定数据或使用 `echartsInstance.setOption(mockData)`。

---

## 6. Review Checklist

- [ ] 截图覆盖关键页面/组件。  
- [ ] 对比失败是否具有明确 diff，排除动态因素。  
- [ ] 新增或更新基线时是否附上说明。  
- [ ] CI 中保留 diff 附件，便于快速审查。  
- [ ] 视觉回归结果与设计团队同步确认。

---

## 7. 后续计划

- 与设计稿（Figma）对齐的自动对比。  
- 引入 `@nuxt/test-utils` + Storybook 的视觉测试模式。  
- 结合 Lighthouse 生成性能与可访问性指标。  
- 建立“视觉快照变更日志”，记录历史变更与审批人。
