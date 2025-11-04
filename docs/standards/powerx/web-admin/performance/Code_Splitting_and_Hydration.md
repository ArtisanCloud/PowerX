# 代码分割与 Hydration 优化

> 介绍 PowerX Web Admin 在 Nuxt 4 中的代码分割策略、异步组件加载方案和 Hydration 优化建议，重点关注 ECharts、Vue Flow 等重量级模块。

---

## 1. 路由级分割

- Nuxt 默认以页面为单位打包，`app/pages/**` 自动生成独立 chunk。  
- 大型模块（`/workflow/*`、`/dashboard`）已天然分割；确保不要在 `app/plugins` 中全局导入重量级库，否则会进入所有页面的 vendor 包。  
- 如需进一步拆分，可使用 [Nuxt route rules](https://nuxt.com/docs/guide/going-further/route-rules) 针对特定路由关闭 SSR 或自定义缓存。

---

## 2. 组件级懒加载

### 2.1 defineAsyncComponent

- 示例：全局 Loading 模态通过 `defineAsyncComponent` 懒加载（`app/plugins/gl-overlay.client.ts:3`），避免首屏加载体积飙升。  
- 推荐在以下场景使用：
  - 仅在用户操作时出现的模态、向导。  
  - 体积大的富文本编辑器、图表组件。  
  - 不同主题/租户专属组件。

### 2.2 client-only / lazy-hydrate

- 对依赖浏览器 API 的组件使用 `<ClientOnly>` 包裹，或 `defineNuxtPlugin({ ssr: false })` 限制在客户端加载。  
- 对于非关键组件可使用 `lazy-hydrate` 方案（Nuxt 4 TODO），在进入视口后再执行 Hydration，降低首屏开销。

---

## 3. 重量级依赖隔离

| 依赖 | 使用页面 | 优化措施 |
| --- | --- | --- |
| ECharts (`vue-echarts`) | `/dashboard` | 仅在页面内注册组件 (`use([...])`)；可考虑动态 import ECharts 模块。 |
| Vue Flow | `/workflow/workspace` | 懒加载组件/样式；在 `workflow` 布局中使用 `ClientOnly`。 |
| SSE/WS 工具 | `/agent` | 主要逻辑在 composable，不影响代码分割。 |

---

## 4. Hydration 优化提示

- 避免在 SSR 阶段输出与客户端不一致的随机内容（如 `Math.random`），否则出现 hydration mismatch。  
- 对 purely visual 的元素（装饰动画、背景粒子）可在客户端渲染，服务端返回占位。  
- 使用 `useHead` 设置首屏 `<title>`，减少客户端修正。

---

## 5. 资源预取与延迟加载

- 利用 Nuxt `definePageMeta({ middleware: [...] })` 在路由切换前预加载必要数据。  
- 对非关键脚本（例如第三方监控）采用 `lazy`/`defer` 或在用户首个交互后加载。  
- 可借助 [Nitro asset manifest](https://nuxt.com/docs/api/nuxt-config#nitro) 生成预加载提示，或在 `<link rel="prefetch">` 中声明下一个页面的 chunk。

---

## 6. 检查清单

- [ ] 大型库是否只在需要的页面/组件中导入。  
- [ ] 弹窗、向导等少用组件是否改成异步加载。  
- [ ] 使用 `<ClientOnly>` 包裹仅在浏览器运行的内容。  
- [ ] 路由切换时是否存在短暂白屏（需懒加载骨架/Placeholder）。  
- [ ] 分析 `npm run build --analyze` 结果，关注 vendor 包体积。  
- [ ] Hydration 是否无警告，暗色主题/SSR 下输出一致。

---

## 7. 后续计划

- 引入 `vite-plugin-inspect`、`nuxt analyze` 常规诊断包体。  
- 使用 `vue-lazy-hydration`（若 Nuxt 兼容）对非关键组件延迟 Hydration。  
- 为 ECharts/Vue Flow 提供 “轻量模式” 的按需构建。  
- 将 code splitting 规则纳入 CI（bundle size monitor）并设定阈值。
