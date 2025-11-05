# Nuxt 渲染模式策略（SSR / SSG / ISR）

> 介绍 PowerX Web Admin 在不同部署模式下的渲染策略。当前配置为纯 SPA（`nuxt.config.ts:7`），但需要为未来的 SSR、静态导出和增量预渲染做规划。

---

## 1. 模式对比

| 模式 | 说明 | 优点 | 注意事项 |
| --- | --- | --- | --- |
| SPA (`ssr: false`) | 所有页面在客户端渲染 | 构建简单、部署静态资源即可 | 首屏依赖 JS，SEO 能力弱 |
| SSR | 每次请求在服务端渲染 HTML | 更快首屏、SEO 友好 | 服务端负载高，需要缓存 |
| SSG (`nuxt generate`) | 构建时预渲染静态 HTML | 可部署 CDN，首屏快速 | 动态数据需客户端获取 |
| ISR | 静态 + 按需重新生成 | 兼顾实时性和速度 | 需部署在支持 ISR 的平台（Vercel/Nitro） |

---

## 2. 现状

- `nuxt.config.ts` 中 `ssr: false`，所有页面运行在浏览器。  
- 开发阶段通过 `useAsyncData`、Pinia 存储数据，默认依赖客户端 API。  
- `npm run generate` 可导出静态站点用于 Demo/文档，但动态功能（Agent、插件市场）需真实后端。

---

## 3. 启用 SSR 的步骤

1. 将 `nuxt.config.ts` 的 `ssr` 设为 `true`，并确保 `nitro` 配置适配。  
2. 检查所有组件/Composable：  
   - 禁止在 setup 阶段直接访问 `window`、`localStorage`。  
   - 使用 `process.client` / `process.server` 守卫。  
3. 配置 Pinia 状态序列化：`plugins/pinia.client.ts` + `plugins/pinia.server.ts`，或启用 `@pinia/nuxt` 自动化。  
4. 结合缓存策略：对于菜单、配置等数据使用 `cachedEventHandler` 或反向代理缓存。

---

## 4. SSG 建议

- 对于 marketing/文档页面（`/intro`、`/home`）可通过 `nuxt generate` 预渲染。  
- 动态页面（`/agent`、`/dashboard`）在 SSG 模式下以 SPA 方式运行；需在 `generate.routes` 中排除或提供静态数据。  
- 导出前运行 `NODE_ENV=production npm run build && npm run generate`，产物位于 `.output/public`。

---

## 5. ISR (Incremental Static Regeneration)

- 适用场景：插件市场、公共文档等更新频率较低但需即时可见。  
- 实现路径：  
  1. 使用 Nitro 的 `cachedEventHandler` 或 `defineCachedFunction` 包裹接口。  
  2. 设置 `swr: true` 与 `maxAge`，让请求在后台刷新。  
  3. 部署在支持 ISR 的平台（Vercel/Netlify Edge/Nitro 自建）。  
- 前端保持 `useAsyncData`，后台刷新时自动同步。

---

## 6. 模块建议

| 页面 | 推荐模式 | 说明 |
| --- | --- | --- |
| `/home`, `/intro` | SSG | 静态内容，适合预渲染。 |
| `/dashboard` | SSR (可选) + 客户端实时数据 | 首屏指标可 SSR，实时数据通过 WS。 |
| `/agent` | SPA | 依赖 WebSocket/SSE，SSR 收益有限。 |
| `/plugins/market` | SSR 或 ISR | 可提升 SEO 与首屏加载，数据较稳定。 |
| `/workflow/**` | SPA | 交互型应用，主要在客户端运行。 |

---

## 7. 测试与验收

- [ ] SSR 下首屏是否正常渲染，Vue Hydration 无警告。  
- [ ] SSR/SSG 生成的 HTML 是否包含关键元信息（标题、描述、OpenGraph）。  
- [ ] 静态导出后动态路由是否回退到 SPA 模式。  
- [ ] 在 CDN/反向代理下缓存策略是否合理（避免缓存跨租户数据）。  
- [ ] WebSocket/SSE 在 SSR 环境下是否继续使用客户端初始化。

---

## 8. 后续计划

- 为 `/intro`、`/home` 等页面启用 SSG，并在 CI 验证导出结果。  
- 研究 `/plugins/market` 的 ISR 实现，减少管理员刷新频率。  
- 引入运行时缓存（Redis）存储 SSR 结果，提高高并发性能。  
- 编写自动回归脚本，在不同模式下运行端到端测试。
